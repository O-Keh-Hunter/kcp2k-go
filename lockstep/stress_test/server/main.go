package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/O-Keh-Hunter/kcp2k-go/lockstep"
)

// 全局 YAML 配置变量
var globalYamlConfig *Config

// StressTestConfig 压测配置
type StressTestConfig struct {
	ServerAddr     string
	ServerPort     uint16
	MetricsPort    uint16
	RoomCount      int
	PlayersPerRoom int
	FrameRate      int
	DownstreamRate int
}

// getDefaultConfig 获取默认配置
func getDefaultConfig() StressTestConfig {
	return StressTestConfig{
		ServerAddr:     "127.0.0.1",
		ServerPort:     8888,
		MetricsPort:    8887,
		RoomCount:      10,
		PlayersPerRoom: 10,
		FrameRate:      30, // 客户端上行30帧/秒
		DownstreamRate: 15, // 服务端下行15帧/秒
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
		serverAddr     = flag.String("addr", "", "Server address")
		port           = flag.Uint64("port", 0, "Server port")
		metricsPort    = flag.Uint64("metrics-port", 0, "Metrics port")
		roomCount      = flag.Int("rooms", 0, "Number of rooms")
		playersPerRoom = flag.Int("players", 0, "Players per room")
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
		config.MetricsPort = yamlConfig.Server.MetricsPort
		config.RoomCount = yamlConfig.Server.Rooms
		config.PlayersPerRoom = yamlConfig.Server.PlayersPerRoom
		config.DownstreamRate = yamlConfig.Server.FrameRate
	}

	// 第三步：使用命令行参数覆盖配置文件中的值
	if *serverAddr != "" {
		config.ServerAddr = *serverAddr
	}
	if *port != 0 {
		config.ServerPort = uint16(*port)
	}
	if *metricsPort != 0 {
		config.MetricsPort = uint16(*metricsPort)
	}
	if *roomCount != 0 {
		config.RoomCount = *roomCount
	}
	if *playersPerRoom != 0 {
		config.PlayersPerRoom = *playersPerRoom
	}

	// 创建日志记录器
	logger := log.New(os.Stdout, "[STRESS-SERVER] ", log.LstdFlags|log.Lshortfile)

	logger.Printf("Starting stress test server with config: %+v", config)
	logger.Printf("Expected total clients: %d", config.RoomCount*config.PlayersPerRoom)

	// 创建LockStep服务器配置
	lockstepConfig := lockstep.DefaultLockStepConfig()
	lockstepConfig.ServerPort = config.ServerPort
	lockstepConfig.MetricsPort = config.MetricsPort // 设置指标服务器端口
	lockstepConfig.RoomConfig.MaxPlayers = uint32(config.PlayersPerRoom)
	lockstepConfig.RoomConfig.MinPlayers = uint32(config.PlayersPerRoom) // 设置为满员才开始
	lockstepConfig.RoomConfig.FrameRate = uint32(config.DownstreamRate)  // 服务端下行15帧/秒

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

	// 创建LockStep服务器
	server := lockstep.NewLockStepServer(&lockstepConfig)

	// 启动服务器
	err := server.Start()
	if err != nil {
		logger.Fatalf("Failed to start server: %v", err)
	}

	logger.Printf("LockStep server started on %s:%d", config.ServerAddr, config.ServerPort)

	// 创建房间配置
	roomConfig := lockstep.DefaultRoomConfig()
	roomConfig.MaxPlayers = uint32(config.PlayersPerRoom)
	roomConfig.MinPlayers = uint32(config.PlayersPerRoom)
	roomConfig.FrameRate = uint32(config.DownstreamRate)

	// 批量创建房间
	logger.Printf("Creating %d rooms...", config.RoomCount)
	for i := 0; i < config.RoomCount; i++ {
		roomID := lockstep.RoomID(fmt.Sprintf("stress_room_%d", i))
		room, err := server.CreateRoom(roomID, roomConfig)
		if err != nil {
			logger.Printf("Failed to create room %s: %v", roomID, err)
			continue
		}
		logger.Printf("Created room %s on port %d", roomID, room.Port)
	}

	logger.Printf("All %d rooms created successfully", config.RoomCount)

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动状态监控
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				stats := server.GetServerStats()
				logger.Println(strings.Repeat("=", 80))
				
				// Server Status
				logger.Printf("[STATUS] Running: %v | Uptime: %dms | Rooms: %v | Players: %v", 
					stats["running"], stats["uptime"], stats["total_rooms"], stats["total_players"])
				
				// Frame Statistics
				if frameStats, ok := stats["frame_stats"].(map[string]interface{}); ok {
					logger.Printf("[FRAMES] Total: %v | AvgTime: %.2fms | Empty: %v | Late: %v",
						frameStats["total_frames"], frameStats["avg_frame_time"], 
						frameStats["empty_frames"], frameStats["late_frames"])
				}
				
				// Network Statistics
				if networkStats, ok := stats["network_stats"].(map[string]interface{}); ok {
					bytesSent := networkStats["bytes_sent"]
					bytesReceived := networkStats["bytes_received"]
					totalPackets := networkStats["total_packets"]
					lostPackets := networkStats["lost_packets"]
					
					// Calculate packet loss rate
					lossRate := 0.0
					if total, ok := totalPackets.(uint64); ok && total > 0 {
						if lost, ok := lostPackets.(uint64); ok {
							lossRate = float64(lost) / float64(total) * 100
						}
					}
					
					logger.Printf("[NETWORK] Sent: %.2fMB | Recv: %.2fMB | Packets: %v | Lost: %v (%.3f%%)",
						float64(bytesSent.(uint64))/1024/1024, float64(bytesReceived.(uint64))/1024/1024,
						totalPackets, lostPackets, lossRate)
				}
				
				// Input Latency Statistics
				if inputStats, ok := stats["input_latency_stats"].(map[string]interface{}); ok {
					logger.Printf("[LATENCY] Avg: %.2fms | Range: %vms~%vms | Samples: %v",
						inputStats["avg_input_latency"], inputStats["min_input_latency"], 
						inputStats["max_input_latency"], inputStats["input_latency_count"])
				}
				
				// Jitter Statistics
				if jitterStats, ok := stats["jitter_stats"].(map[string]interface{}); ok {
					logger.Printf("[JITTER] Avg: %.2fms | Range: %vms~%vms | Samples: %v",
						jitterStats["avg_jitter"], jitterStats["min_jitter"], 
						jitterStats["max_jitter"], jitterStats["jitter_count"])
				}

				// 显示房间状态统计
				rooms := server.GetRooms()
				activeRooms := 0
				totalPlayers := 0
				for _, room := range rooms {
					room.Mutex.RLock()
					if len(room.Players) > 0 {
						activeRooms++
					}
					totalPlayers += len(room.Players)
					room.Mutex.RUnlock()
				}
				logger.Printf("[ROOMS] Total: %d | Active: %d | Players: %d", len(rooms), activeRooms, totalPlayers)
			case <-sigChan:
				return
			}
		}
	}()

	<-sigChan
	logger.Println("Shutting down stress test server...")
	server.Stop()
	logger.Println("Server stopped")
}
