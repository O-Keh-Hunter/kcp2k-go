package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/O-Keh-Hunter/kcp2k-go/lockstep"
	"google.golang.org/protobuf/proto"
)

// 全局 YAML 配置变量
var globalYamlConfig *YamlConfig

// ClientConfig 客户端配置
type ClientConfig struct {
	ServerAddr       string
	ServerPort       uint16
	ClientCount      int
	TestDuration     time.Duration
	UpstreamInterval time.Duration // 33ms上行间隔
	InputPacketSize  int           // 40字节input包
	ReconnectRate    float64       // 重连率
}

// ClientMetrics 客户端性能指标
type ClientMetrics struct {
	ConnectedClients    int64
	TotalInputsSent     int64
	TotalFramesReceived int64
	TotalBytesReceived  int64
	TotalBytesSent      int64
	ConnectionErrors    int64
	InputErrors         int64

	// 从 LockStepClient 获取的详细统计信息
	TotalFrames          int64
	MissedFrames         int64
	LateFrames           int64
	TotalPackets         int64
	LostPackets          int64
	PacketLossRate       float64
	AverageLatency       time.Duration
	MaxLatency           time.Duration
	MinLatency           time.Duration
	AverageFrameTime     time.Duration
	NetworkBytesReceived int64
	NetworkBytesSent     int64

	// 输入延时统计
	InputLatencyCount   int64         // 输入延时样本数量
	TotalInputLatency   time.Duration // 总输入延时
	MaxInputLatency     time.Duration // 最大输入延时
	MinInputLatency     time.Duration // 最小输入延时
	AverageInputLatency time.Duration // 平均输入延时

	// Jitter统计
	JitterCount   int64         // Jitter样本数量
	TotalJitter   time.Duration // 总Jitter时间
	MaxJitter     time.Duration // 最大Jitter
	MinJitter     time.Duration // 最小Jitter
	AverageJitter time.Duration // 平均Jitter

	StartTime time.Time
	mutex     sync.RWMutex
}

// StressClient 压测客户端
type StressClient struct {
	id             int
	client         *lockstep.LockStepClient
	config         *ClientConfig
	metrics        *ClientMetrics
	logger         *log.Logger
	stopChan       chan struct{}
	wg             *sync.WaitGroup
	connectedCh    chan bool
	disconnectedCh chan bool
}

// NewStressClient 创建压测客户端
func NewStressClient(id int, config *ClientConfig, metrics *ClientMetrics, logger *log.Logger, wg *sync.WaitGroup) *StressClient {
	return &StressClient{
		id:             id,
		config:         config,
		metrics:        metrics,
		logger:         logger,
		stopChan:       make(chan struct{}),
		wg:             wg,
		connectedCh:    make(chan bool, 1),
		disconnectedCh: make(chan bool, 1),
	}
}

// Start 启动客户端
func (sc *StressClient) Start() {
	sc.wg.Add(1)
	go sc.run()
}

// Stop 停止客户端
func (sc *StressClient) Stop() {
	close(sc.stopChan)
}

// run 运行客户端
func (sc *StressClient) run() {
	defer sc.wg.Done()

	// 创建客户端配置
	lockstepConfig := lockstep.DefaultLockStepConfig()

	// 如果有全局的 YAML 配置，应用 KCP 设置
	if globalYamlConfig != nil {
		// 应用 KCP 配置
		lockstepConfig.KcpConfig.Mtu = globalYamlConfig.Network.KCP.MTU
		lockstepConfig.KcpConfig.NoDelay = globalYamlConfig.Network.KCP.Nodelay == 1
		lockstepConfig.KcpConfig.Interval = uint(globalYamlConfig.Network.KCP.Interval)
		lockstepConfig.KcpConfig.FastResend = globalYamlConfig.Network.KCP.Resend
		lockstepConfig.KcpConfig.CongestionWindow = globalYamlConfig.Network.KCP.NC == 0
		lockstepConfig.KcpConfig.SendWindowSize = uint(globalYamlConfig.Network.KCP.Sndwnd)
		lockstepConfig.KcpConfig.ReceiveWindowSize = uint(globalYamlConfig.Network.KCP.Rcvwnd)
	}

	playerID := lockstep.PlayerID(sc.id)

	// 设置回调函数
	callbacks := lockstep.ClientCallbacks{
		OnPlayerJoined: func(pid lockstep.PlayerID) {
			// 静默处理
		},
		OnPlayerLeft: func(pid lockstep.PlayerID) {
			// 静默处理
		},
		OnGameStarted: func() {
			sc.logger.Printf("[Client %d] Game started", sc.id)
		},
		OnGameEnded: func() {
			sc.logger.Printf("[Client %d] Game ended", sc.id)
		},
		OnConnected: func() {
			select {
			case sc.connectedCh <- true:
			default:
			}
		},
		OnDisconnected: func() {
			select {
			case sc.disconnectedCh <- true:
			default:
			}
		},
		OnError: func(err error) {
			atomic.AddInt64(&sc.metrics.ConnectionErrors, 1)
			sc.logger.Printf("[Client %d] Error: %v", sc.id, err)
		},
		Logger: sc.logger, // 传递logger给LockStepClient
	}

	// 创建客户端
	sc.client = lockstep.NewLockStepClient(&lockstepConfig, playerID, callbacks)

	// 计算房间端口（每10个客户端一个房间）
	roomIndex := sc.id / 10
	roomPort := sc.config.ServerPort + uint16(roomIndex)
	err := sc.client.Connect(sc.config.ServerAddr, roomPort)
	if err != nil {
		atomic.AddInt64(&sc.metrics.ConnectionErrors, 1)
		sc.logger.Printf("[Client %d] Failed to connect: %v", sc.id, err)
		return
	}

	// 等待连接建立（使用回调通道）
	maxWait := 10 * time.Second
	select {
	case <-sc.connectedCh:
		atomic.AddInt64(&sc.metrics.ConnectedClients, 1)
		sc.logger.Printf("[Client %d] Connected successfully", sc.id)
	case <-sc.stopChan:
		return
	case <-time.After(maxWait):
		atomic.AddInt64(&sc.metrics.ConnectionErrors, 1)
		sc.logger.Printf("[Client %d] Connection timeout", sc.id)
		return
	}

	// 加入房间
	roomID := lockstep.RoomID(fmt.Sprintf("stress_room_%d", sc.id/10)) // 每10个客户端一个房间
	err = sc.client.JoinRoom(roomID)
	if err != nil {
		atomic.AddInt64(&sc.metrics.ConnectionErrors, 1)
		sc.logger.Printf("[Client %d] Failed to join room %s: %v", sc.id, roomID, err)
		return
	}

	sc.logger.Printf("[Client %d] Joined room %s", sc.id, roomID)

	// 启动输入发送循环
	go sc.inputLoop()

	// 启动帧接收循环
	go sc.frameLoop()

	// 等待停止信号或断开连接事件
	select {
	case <-sc.stopChan:
		// 主动断开连接
		sc.client.Disconnect()
		atomic.AddInt64(&sc.metrics.ConnectedClients, -1)
		sc.logger.Printf("[Client %d] Disconnected by stop signal", sc.id)
	case <-sc.disconnectedCh:
		// 被动断开连接（网络问题等）
		atomic.AddInt64(&sc.metrics.ConnectedClients, -1)
		sc.logger.Printf("[Client %d] Disconnected by server or network issue", sc.id)
	}
}

// inputLoop 输入发送循环
func (sc *StressClient) inputLoop() {
	ticker := time.NewTicker(sc.config.UpstreamInterval) // 33ms间隔
	defer ticker.Stop()

	inputCounter := uint32(0)
	for {
		select {
		case <-ticker.C:
			if !sc.client.IsRunning() {
				continue
			}

			// 创建40字节的输入数据
			inputCounter++

			// 生成40字节的模拟输入数据
			inputData := make([]byte, sc.config.InputPacketSize)
			// 填充一些有意义的数据
			copy(inputData[:8], []byte(fmt.Sprintf("%08d", inputCounter)))
			copy(inputData[8:16], []byte(fmt.Sprintf("%08d", sc.id)))
			// 剩余字节填充随机数据
			for i := 16; i < len(inputData); i++ {
				inputData[i] = byte(rand.Intn(256))
			}

			err := sc.client.SendInput(inputData, lockstep.InputMessage_None)
			if err != nil {
				atomic.AddInt64(&sc.metrics.InputErrors, 1)
			} else {
				atomic.AddInt64(&sc.metrics.TotalInputsSent, 1)
				atomic.AddInt64(&sc.metrics.TotalBytesSent, int64(len(inputData)))
			}
		case <-sc.stopChan:
			return
		}
	}
}

// frameLoop 帧接收循环
func (sc *StressClient) frameLoop() {
	ticker := time.NewTicker(sc.config.UpstreamInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !sc.client.IsRunning() {
				continue
			}

			// 使用PopFrame接口获取帧数据
			frame := sc.client.PopFrame()
			if frame != nil {
				atomic.AddInt64(&sc.metrics.TotalFramesReceived, 1)
				// 估算接收字节数
				frameSize := 0
				for i := 0; i < len(frame.DataCollection); i++ {
					frameSize += proto.Size(frame.DataCollection[i])
				}
				atomic.AddInt64(&sc.metrics.TotalBytesReceived, int64(frameSize))

				// 输出帧接收日志
				// sc.logger.Printf("[Client %d] Received frame %d with %d inputs", sc.id, frame.FrameId, len(frame.DataCollection))
			}
		case <-sc.stopChan:
			return
		}
	}
}

// collectDetailedStats 收集客户端的详细统计信息
func (sc *StressClient) collectDetailedStats() {
	if sc.client == nil {
		return
	}

	// 获取帧统计信息
	frameStats := sc.client.GetFrameStats()
	if frameStats != nil {
		atomic.StoreInt64(&sc.metrics.TotalFrames, int64(frameStats.GetTotalFrames()))
		atomic.StoreInt64(&sc.metrics.MissedFrames, int64(frameStats.GetMissedFrames()))
		atomic.StoreInt64(&sc.metrics.LateFrames, int64(frameStats.GetLateFrames()))
		sc.metrics.mutex.Lock()
		sc.metrics.AverageFrameTime = frameStats.GetAverageFrameTime()
		sc.metrics.mutex.Unlock()
	}

	// 获取网络统计信息
	networkStats := sc.client.GetNetworkStats()
	if networkStats != nil {
		atomic.StoreInt64(&sc.metrics.TotalPackets, int64(networkStats.GetTotalPackets()))
		atomic.StoreInt64(&sc.metrics.LostPackets, int64(networkStats.GetLostPackets()))
		atomic.StoreInt64(&sc.metrics.NetworkBytesReceived, int64(networkStats.GetBytesReceived()))
		atomic.StoreInt64(&sc.metrics.NetworkBytesSent, int64(networkStats.GetBytesSent()))
		sc.metrics.mutex.Lock()

		sc.metrics.PacketLossRate = sc.client.GetPacketLossRate()
		sc.metrics.AverageLatency = networkStats.GetAverageLatency()
		sc.metrics.MaxLatency = networkStats.GetMaxLatency()
		sc.metrics.MinLatency = networkStats.GetMinLatency()

		// 收集输入延迟统计
		sc.metrics.InputLatencyCount += int64(networkStats.GetInputLatencyCount())
		sc.metrics.TotalInputLatency += networkStats.GetAverageInputLatency() * time.Duration(networkStats.GetInputLatencyCount())
		if networkStats.GetMaxInputLatency() > sc.metrics.MaxInputLatency {
			sc.metrics.MaxInputLatency = networkStats.GetMaxInputLatency()
		}
		if networkStats.GetMinInputLatency() < sc.metrics.MinInputLatency || sc.metrics.MinInputLatency == 0 {
			sc.metrics.MinInputLatency = networkStats.GetMinInputLatency()
		}

		// 收集Jitter统计
		if networkStats.GetJitterCount() > 0 {
			sc.metrics.JitterCount += int64(networkStats.GetJitterCount())
			sc.metrics.TotalJitter += networkStats.GetAverageJitter() * time.Duration(networkStats.GetJitterCount())
			if networkStats.GetMaxJitter() > sc.metrics.MaxJitter {
				sc.metrics.MaxJitter = networkStats.GetMaxJitter()
			}
			if networkStats.GetMinJitter() < sc.metrics.MinJitter || sc.metrics.MinJitter == 0 {
				sc.metrics.MinJitter = networkStats.GetMinJitter()
			}
			// 计算平均Jitter
			if sc.metrics.JitterCount > 0 {
				sc.metrics.AverageJitter = sc.metrics.TotalJitter / time.Duration(sc.metrics.JitterCount)
			}
		}

		sc.metrics.mutex.Unlock()
	}
}

// getDefaultConfig 获取默认配置
func getDefaultConfig() ClientConfig {
	return ClientConfig{
		ServerAddr:       "127.0.0.1",
		ServerPort:       8888,
		ClientCount:      100,
		TestDuration:     30 * time.Minute,
		UpstreamInterval: 33 * time.Millisecond, // 33ms上行间隔 (30帧/秒)
		InputPacketSize:  40,                    // 40字节input包
		ReconnectRate:    0.01,                  // 1%重连率
	}
}

func main() {
	// 获取默认配置
	config := getDefaultConfig()

	// 第一步：解析命令行参数以获取配置文件路径
	var configFile string
	flag.StringVar(&configFile, "config", "", "Path to YAML config file")

	// 定义命令行参数变量
	var (
		serverAddr    = flag.String("addr", "", "Server address")
		port          = flag.Uint64("port", 0, "Server port")
		clientCount   = flag.Int("clients", 0, "Number of clients")
		duration      = flag.String("duration", "", "Test duration")
		reconnectRate = flag.Float64("reconnect", -1, "Reconnect rate")
	)

	flag.Parse()

	// 第二步：如果指定了配置文件，从配置文件加载配置
	if configFile != "" {
		yamlConfig, err := LoadConfigFromFile(configFile)
		if err != nil {
			log.Fatalf("Failed to load config file: %v", err)
		}

		// 验证配置
		if err := yamlConfig.Validate(); err != nil {
			log.Fatalf("Invalid config: %v", err)
		}

		// 设置全局配置变量
		globalYamlConfig = yamlConfig

		// 使用YAML配置覆盖默认配置
		config.ServerAddr = yamlConfig.Server.Address
		config.ServerPort = yamlConfig.Server.Port
		config.ClientCount = yamlConfig.Client.TotalClients
		if testDuration, err := yamlConfig.GetTestDuration(); err == nil {
			config.TestDuration = testDuration
		}
		config.UpstreamInterval = yamlConfig.GetInputInterval()
		config.InputPacketSize = yamlConfig.Client.InputSize
	}

	// 第三步：使用命令行参数覆盖配置文件中的值
	if *serverAddr != "" {
		config.ServerAddr = *serverAddr
	}
	if *port != 0 {
		config.ServerPort = uint16(*port)
	}
	if *clientCount != 0 {
		config.ClientCount = *clientCount
	}
	if *duration != "" {
		if d, err := time.ParseDuration(*duration); err == nil {
			config.TestDuration = d
		}
	}
	if *reconnectRate >= 0 {
		config.ReconnectRate = *reconnectRate
	}

	// 创建日志记录器
	logger := log.New(os.Stdout, "[STRESS-CLIENT] ", log.LstdFlags)

	logger.Printf("Starting stress test client with config: %+v", config)

	// 创建性能指标
	metrics := &ClientMetrics{
		StartTime: time.Now(),
	}

	// 创建客户端
	var wg sync.WaitGroup
	clients := make([]*StressClient, config.ClientCount)

	logger.Printf("Creating %d clients...", config.ClientCount)
	for i := 0; i < config.ClientCount; i++ {
		clients[i] = NewStressClient(i, &config, metrics, logger, &wg)
	}

	// 启动性能监控
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			// 收集所有客户端的详细统计信息
			for _, client := range clients {
				if client != nil {
					client.collectDetailedStats()
				}
			}

			metrics.mutex.RLock()
			uptime := time.Since(metrics.StartTime)
			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			// 计算平均输入延时
			var avgInputLatency time.Duration
			if metrics.InputLatencyCount > 0 {
				avgInputLatency = metrics.TotalInputLatency / time.Duration(metrics.InputLatencyCount)
				metrics.AverageInputLatency = avgInputLatency
			}

			// 基础统计信息
			logger.Printf("[METRICS] Uptime: %v, Connected: %d, InputsSent: %d, FramesReceived: %d, Errors: %d/%d, Memory: %.2fMB, Goroutines: %d",
				uptime.Round(time.Second),
				metrics.ConnectedClients,
				metrics.TotalInputsSent,
				metrics.TotalFramesReceived,
				metrics.ConnectionErrors,
				metrics.InputErrors,
				float64(m.Alloc)/1024/1024,
				runtime.NumGoroutine(),
			)

			// 详细统计信息
			logger.Printf("[DETAILED] TotalFrames: %d, MissedFrames: %d, LateFrames: %d, PacketLoss: %.2f%%, AvgLatency: %v, MaxLatency: %v, MinLatency: %v",
				metrics.TotalFrames,
				metrics.MissedFrames,
				metrics.LateFrames,
				metrics.PacketLossRate*100,
				metrics.AverageLatency.Round(time.Millisecond),
				metrics.MaxLatency.Round(time.Millisecond),
				metrics.MinLatency.Round(time.Millisecond),
			)

			// 网络统计信息
			logger.Printf("[NETWORK] NetworkBytesReceived: %d, NetworkBytesSent: %d, AvgFrameTime: %v",
				metrics.NetworkBytesReceived,
				metrics.NetworkBytesSent,
				metrics.AverageFrameTime.Round(time.Microsecond),
			)

			// 输入延时统计信息
			logger.Printf("[INPUT_LATENCY] Count: %d, Avg: %v, Max: %v, Min: %v",
				metrics.InputLatencyCount,
				avgInputLatency.Round(time.Millisecond),
				metrics.MaxInputLatency.Round(time.Millisecond),
				metrics.MinInputLatency.Round(time.Millisecond),
			)

			// Jitter统计信息
			logger.Printf("[JITTER] Count: %d, Avg: %v, Max: %v, Min: %v",
				metrics.JitterCount,
				metrics.AverageJitter.Round(time.Millisecond),
				metrics.MaxJitter.Round(time.Millisecond),
				metrics.MinJitter.Round(time.Millisecond),
			)

			metrics.mutex.RUnlock()
		}
	}()

	// 分批启动客户端，避免同时连接过多
	batchSize := 50
	logger.Printf("Starting clients in batches of %d...", batchSize)
	for i := 0; i < config.ClientCount; i += batchSize {
		end := i + batchSize
		if end > config.ClientCount {
			end = config.ClientCount
		}

		logger.Printf("Starting clients %d-%d...", i, end-1)
		for j := i; j < end; j++ {
			clients[j].Start()
		}

		// 等待一段时间再启动下一批
		time.Sleep(100 * time.Millisecond)
	}

	logger.Printf("All %d clients started", config.ClientCount)

	// 等待中断信号或测试时间结束
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	timeoutChan := time.After(config.TestDuration)

	select {
	case <-sigChan:
		logger.Println("Received interrupt signal")
	case <-timeoutChan:
		logger.Printf("Test duration %v completed", config.TestDuration)
	}

	// 停止所有客户端
	logger.Println("Stopping all clients...")
	for _, client := range clients {
		client.Stop()
	}

	// 等待所有客户端停止
	wg.Wait()

	// 最后一次收集详细统计信息
	for _, client := range clients {
		if client != nil {
			client.collectDetailedStats()
		}
	}

	// 输出最终统计
	metrics.mutex.RLock()
	totalTime := time.Since(metrics.StartTime)
	logger.Printf("[FINAL STATS] Total time: %v", totalTime.Round(time.Second))
	logger.Printf("[FINAL STATS] Total inputs sent: %d", metrics.TotalInputsSent)
	logger.Printf("[FINAL STATS] Total frames received: %d", metrics.TotalFramesReceived)
	logger.Printf("[FINAL STATS] Total bytes sent: %d", metrics.TotalBytesSent)
	logger.Printf("[FINAL STATS] Total bytes received: %d", metrics.TotalBytesReceived)
	logger.Printf("[FINAL STATS] Connection errors: %d", metrics.ConnectionErrors)
	logger.Printf("[FINAL STATS] Input errors: %d", metrics.InputErrors)

	// 详细的帧和网络统计
	logger.Printf("[FINAL STATS] Total frames processed: %d", metrics.TotalFrames)
	logger.Printf("[FINAL STATS] Missed frames: %d", metrics.MissedFrames)
	logger.Printf("[FINAL STATS] Late frames: %d", metrics.LateFrames)
	logger.Printf("[FINAL STATS] Total packets: %d", metrics.TotalPackets)
	logger.Printf("[FINAL STATS] Lost packets: %d", metrics.LostPackets)
	logger.Printf("[FINAL STATS] Packet loss rate: %.2f%%", metrics.PacketLossRate*100)
	logger.Printf("[FINAL STATS] Average latency: %v", metrics.AverageLatency.Round(time.Millisecond))
	logger.Printf("[FINAL STATS] Max latency: %v", metrics.MaxLatency.Round(time.Millisecond))
	logger.Printf("[FINAL STATS] Min latency: %v", metrics.MinLatency.Round(time.Millisecond))
	logger.Printf("[FINAL STATS] Average frame time: %v", metrics.AverageFrameTime.Round(time.Microsecond))
	logger.Printf("[FINAL STATS] Network bytes received: %d", metrics.NetworkBytesReceived)
	logger.Printf("[FINAL STATS] Network bytes sent: %d", metrics.NetworkBytesSent)

	// 输入延时统计
	logger.Printf("[FINAL STATS] Input latency samples: %d", metrics.InputLatencyCount)
	// 计算平均输入延迟
	if metrics.InputLatencyCount > 0 {
		metrics.AverageInputLatency = metrics.TotalInputLatency / time.Duration(metrics.InputLatencyCount)
	}
	logger.Printf("[FINAL STATS] Average input latency: %v", metrics.AverageInputLatency.Round(time.Millisecond))
	logger.Printf("[FINAL STATS] Max input latency: %v", metrics.MaxInputLatency.Round(time.Millisecond))
	logger.Printf("[FINAL STATS] Min input latency: %v", metrics.MinInputLatency.Round(time.Millisecond))

	// Jitter统计
	logger.Printf("[FINAL STATS] Jitter samples: %d", metrics.JitterCount)
	logger.Printf("[FINAL STATS] Average jitter: %v", metrics.AverageJitter.Round(time.Millisecond))
	logger.Printf("[FINAL STATS] Max jitter: %v", metrics.MaxJitter.Round(time.Millisecond))
	logger.Printf("[FINAL STATS] Min jitter: %v", metrics.MinJitter.Round(time.Millisecond))

	if totalTime.Seconds() > 0 {
		logger.Printf("[FINAL STATS] Input rate: %.2f inputs/sec", float64(metrics.TotalInputsSent)/totalTime.Seconds())
		logger.Printf("[FINAL STATS] Frame rate: %.2f frames/sec", float64(metrics.TotalFramesReceived)/totalTime.Seconds())
		if metrics.TotalFrames > 0 {
			missedRate := float64(metrics.MissedFrames) / float64(metrics.TotalFrames) * 100
			lateRate := float64(metrics.LateFrames) / float64(metrics.TotalFrames) * 100
			logger.Printf("[FINAL STATS] Missed frame rate: %.2f%%", missedRate)
			logger.Printf("[FINAL STATS] Late frame rate: %.2f%%", lateRate)
		}
	}
	metrics.mutex.RUnlock()

	logger.Println("Stress test client completed")
}
