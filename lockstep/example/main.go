package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/O-Keh-Hunter/kcp2k-go/lockstep"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go [server|client] [args...]")
		fmt.Println("Server: go run main.go server [port]")
		fmt.Println("Client: go run main.go client [server_host] [server_port] [player_id]")
		os.Exit(1)
	}

	mode := os.Args[1]
	switch mode {
	case "server":
		runServer()
	case "client":
		runClient()
	default:
		fmt.Printf("Unknown mode: %s\n", mode)
		os.Exit(1)
	}
}

func runServer() {
	port := uint16(8888)
	if len(os.Args) > 2 {
		if p, err := strconv.ParseUint(os.Args[2], 10, 16); err == nil {
			port = uint16(p)
		}
	}

	// 创建服务器配置
	config := lockstep.DefaultLockStepConfig()
	config.ServerPort = port

	// 创建服务器
	server := lockstep.NewLockStepServer(&config)

	// 启动服务器
	err := server.Start()
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	log.Printf("LockStep server started on port %d", port)

	// 创建房间配置
	roomConfig := lockstep.DefaultRoomConfig()
	// 使用默认的15帧/秒 (66ms间隔)
	roomConfig.MinPlayers = 2 // 设置为2个玩家即可开始游戏
	// 显示配置信息
	log.Printf("Room config: MinPlayers=%d, MaxPlayers=%d, FrameRate=%d",
		roomConfig.MinPlayers, roomConfig.MaxPlayers, roomConfig.FrameRate)

	// 创建房间
	roomID := lockstep.RoomID("room1")
	room, err := server.CreateRoom(roomID, roomConfig)
	if err != nil {
		log.Fatalf("Failed to create room: %v", err)
	}

	log.Printf("Created room %s on port %d (MinPlayers: %d, MaxPlayers: %d)",
		roomID, room.Port, room.Config.MinPlayers, room.Config.MaxPlayers)

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动状态监控
	go func() {
		ticker := time.NewTicker(time.Second * 5)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				stats := server.GetServerStats()
				log.Printf("Server Stats: %+v", stats)

				// 显示所有房间状态
				rooms := server.GetRooms()
				for roomID, room := range rooms {
					room.Mutex.RLock()
					log.Printf("Room %s (Port %d): Players=%d, CurrentFrame=%d, Status=%d",
						roomID, room.Port, len(room.Players), room.CurrentFrameID, room.State.Status)
					room.Mutex.RUnlock()
				}
			case <-sigChan:
				return
			}
		}
	}()

	<-sigChan
	log.Println("Shutting down server...")
	server.Stop()
}

func runClient() {
	serverHost := "127.0.0.1"
	serverPort := uint16(8888)
	playerID := lockstep.PlayerID(1)

	if len(os.Args) > 2 {
		serverHost = os.Args[2]
	}
	if len(os.Args) > 3 {
		if p, err := strconv.ParseUint(os.Args[3], 10, 16); err == nil {
			serverPort = uint16(p)
		}
	}
	if len(os.Args) > 4 {
		if id, err := strconv.ParseUint(os.Args[4], 10, 32); err == nil {
			playerID = lockstep.PlayerID(id)
		}
	}

	// 创建客户端配置 - 使用默认KCP配置进行对比测试
	config := lockstep.DefaultLockStepConfig()

	// 设置回调函数
	callbacks := lockstep.ClientCallbacks{
		OnPlayerJoined: func(pid lockstep.PlayerID) {
			log.Printf("[Player %d] Player %d joined", playerID, pid)
		},
		OnPlayerLeft: func(pid lockstep.PlayerID) {
			log.Printf("[Player %d] Player %d left", playerID, pid)
		},
		OnGameStarted: func() {
			log.Printf("[Player %d] Game started!", playerID)
		},
		OnGameEnded: func() {
			log.Printf("[Player %d] Game ended!", playerID)
		},
		OnError: func(err error) {
			log.Printf("[Player %d] Error: %v", playerID, err)
		},
	}

	// 创建客户端
	client := lockstep.NewLockStepClient(&config, playerID, callbacks)

	// 注意：在实际应用中，客户端需要先连接到主服务器获取房间信息
	// 这里为了简化示例，直接连接到指定的房间端口
	roomPort := serverPort

	// 连接到房间的KCP服务器
	err := client.Connect(serverHost, roomPort)
	if err != nil {
		log.Fatalf("Failed to connect to room server: %v", err)
	}

	log.Printf("[Player %d] Connecting to room server %s:%d", playerID, serverHost, roomPort)

	// 等待连接建立
	maxWait := 10 * time.Second
	startTime := time.Now()
	for !client.IsConnected() && time.Since(startTime) < maxWait {
		time.Sleep(100 * time.Millisecond)
	}

	if !client.IsConnected() {
		log.Fatalf("[Player %d] Failed to connect to server within %v", playerID, maxWait)
	}

	log.Printf("[Player %d] Successfully connected to server", playerID)

	// 加入房间
	roomID := lockstep.RoomID("room1")
	err = client.JoinRoom(roomID)
	if err != nil {
		log.Printf("Failed to join room: %v", err)
	} else {
		log.Printf("[Player %d] Joined room: %s", playerID, roomID)
	}

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 客户端帧率
	frameInterval := time.Duration(1000/30) * time.Millisecond // (30帧)

	// 启动输入模拟
	go func() {
		ticker := time.NewTicker(frameInterval)
		defer ticker.Stop()

		inputCounter := 0
		for {
			select {
			case <-ticker.C:
				if client.IsRunning() {
					// 模拟玩家输入
					inputCounter++

					// 创建简单的输入数据（可以根据需要定义更复杂的结构）
					inputData := []byte(fmt.Sprintf("x:%d,y:%d,counter:%d", inputCounter%100, (inputCounter*2)%100, inputCounter))

					err := client.SendInput(inputData, lockstep.InputMessage_None)
					if err != nil {
						log.Printf("[Player %d] Failed to send input: %v", playerID, err)
					}
				}
			case <-sigChan:
				return
			}
		}
	}()

	// 启动状态监控
	go func() {
		ticker := time.NewTicker(time.Second * 5)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				log.Printf("[Player %d] Status - Connected: %v, Running: %v, CurrentFrame: %d, LastFrame: %d",
					playerID, client.IsConnected(), client.IsRunning(),
					client.GetCurrentFrameID(), client.GetLastFrameID())

				// 输出 FrameStats
				if client.FrameStats != nil {
					totalFrames := client.FrameStats.GetTotalFrames()
					missedFrames := client.FrameStats.GetMissedFrames()
					lateFrames := client.FrameStats.GetLateFrames()
					avgFrameTime := client.FrameStats.GetAverageFrameTime()
					log.Printf("[Player %d] FrameStats - Total: %d, Missed: %d, Late: %d, AvgTime: %.2fms",
						playerID, totalFrames, missedFrames, lateFrames, float64(avgFrameTime.Nanoseconds())/1e6)
				}

				// 输出 NetworkStats
				if client.NetworkStats != nil {
					totalPackets := client.NetworkStats.GetTotalPackets()
					lostPackets := client.NetworkStats.GetLostPackets()
					avgRtt := client.NetworkStats.GetAverageRTT()
					maxRtt := client.NetworkStats.GetMaxRTT()
					minRtt := client.NetworkStats.GetMinRTT()
					bytesReceived := client.NetworkStats.GetBytesReceived()
					bytesSent := client.NetworkStats.GetBytesSent()

					avgInputLatency := client.NetworkStats.GetAverageInputLatency()
					maxInputLatency := client.NetworkStats.GetMaxInputLatency()
					minInputLatency := client.NetworkStats.GetMinInputLatency()
					inputLatencyCount := client.NetworkStats.GetInputLatencyCount()

					avgJitter := client.NetworkStats.GetAverageJitter()
					maxJitter := client.NetworkStats.GetMaxJitter()
					minJitter := client.NetworkStats.GetMinJitter()
					jitterCount := client.NetworkStats.GetJitterCount()

					log.Printf("[Player %d] NetworkStats - Packets: Total=%d packets, Lost=%d packets, Bytes: Recv=%d bytes, Sent=%d bytes",
						playerID, totalPackets, lostPackets, bytesReceived, bytesSent)
					log.Printf("[Player %d] NetworkStats - RTT: Avg=%dms, Max=%dms, Min=%dms",
						playerID, avgRtt.Milliseconds(), maxRtt.Milliseconds(), minRtt.Milliseconds())
					log.Printf("[Player %d] NetworkStats - InputLatency: Count=%d, Avg=%dms, Max=%dms, Min=%dms",
						playerID, inputLatencyCount, avgInputLatency.Milliseconds(), maxInputLatency.Milliseconds(), minInputLatency.Milliseconds())
					log.Printf("[Player %d] NetworkStats - Jitter: Count=%d, Avg=%dms, Max=%dms, Min=%dms",
						playerID, jitterCount, avgJitter.Milliseconds(), maxJitter.Milliseconds(), minJitter.Milliseconds())
				}
			case <-sigChan:
				return
			}
		}
	}()

	// 启动PopFrame示例 - 主动获取帧数据
	go func() {
		ticker := time.NewTicker(frameInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if client.IsRunning() {
					// 使用PopFrame接口主动获取下一帧
					frame := client.PopFrame()
					if frame != nil {
						log.Printf("[Player %d] PopFrame got frame %d with %d inputs", playerID, frame.FrameId, len(frame.DataCollection))
					}
				}
			case <-sigChan:
				return
			}
		}
	}()

	<-sigChan
	log.Printf("[Player %d] Shutting down client...", playerID)
	client.Disconnect()
}
