package kcp2k

import (
	"net"
	"runtime"
	"testing"
	"time"
)

// BenchmarkTick benchmarks the KcpServer Tick method performance
func BenchmarkTick(b *testing.B) {
	config := DefaultKcpConfig()
	config.Mtu = 1400
	config.RecvBufferSize = 7 * config.Mtu
	config.SendBufferSize = 7 * config.Mtu

	server := NewKcpServer(nil, nil, nil, nil, config)
	err := server.Start(0)
	if err != nil {
		b.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.Tick()
	}
}

// BenchmarkTickIncoming benchmarks only the incoming tick performance
func BenchmarkTickIncoming(b *testing.B) {
	config := DefaultKcpConfig()
	config.Mtu = 1200

	server := NewKcpServer(
		func(connectionId int) {},
		func(connectionId int, data []byte, channel KcpChannel) {},
		func(connectionId int) {},
		func(connectionId int, errorCode ErrorCode, reason string) {},
		config,
	)

	err := server.Start(0)
	if err != nil {
		b.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		server.TickIncoming()
	}
}

// BenchmarkTickOutgoing benchmarks only the outgoing tick performance
func BenchmarkTickOutgoing(b *testing.B) {
	config := DefaultKcpConfig()
	config.Mtu = 1200

	server := NewKcpServer(
		func(connectionId int) {},
		func(connectionId int, data []byte, channel KcpChannel) {},
		func(connectionId int) {},
		func(connectionId int, errorCode ErrorCode, reason string) {},
		config,
	)

	err := server.Start(0)
	if err != nil {
		b.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		server.TickOutgoing()
	}
}

// BenchmarkTickWithConnections benchmarks tick performance with real connections
// BenchmarkTickWithConnectionsManual 使用手动内存统计测量Tick操作的内存分配
func BenchmarkTickWithConnectionsManual(b *testing.B) {
	config := DefaultKcpConfig()
	config.Mtu = 1400
	config.RecvBufferSize = 7 * config.Mtu
	config.SendBufferSize = 7 * config.Mtu
	config.NoDelay = true
	config.Interval = 1
	config.Timeout = 10000

	// 客户端连接状态跟踪
	const numClients = 10
	connectedClients := make([]bool, numClients)
	connectedCount := 0

	server := NewKcpServer(
		func(connectionId int) {},
		func(connectionId int, data []byte, channel KcpChannel) {},
		func(connectionId int) {},
		func(connectionId int, errorCode ErrorCode, reason string) {},
		config,
	)

	err := server.Start(0)
	if err != nil {
		b.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// 获取服务器端口
	serverAddr := server.LocalEndPoint().(*net.UDPAddr)

	// 创建多个客户端连接来模拟真实场景
	clients := make([]*KcpClient, numClients)

	for i := 0; i < numClients; i++ {
		clientIndex := i // 捕获循环变量
		client := NewKcpClient(
			func() {
				// 客户端连接成功回调
				connectedClients[clientIndex] = true
				connectedCount++
			},
			func(data []byte, channel KcpChannel) {},
			func() {
				// 客户端断开连接回调
				if connectedClients[clientIndex] {
					connectedClients[clientIndex] = false
					connectedCount--
				}
			},
			func(errorCode ErrorCode, reason string) {},
			config,
		)
		clients[i] = client

		// 连接到服务器
		err := client.Connect("127.0.0.1", uint16(serverAddr.Port))
		if err != nil {
			b.Fatalf("Failed to connect client %d: %v", i, err)
		}
	}

	// 等待所有客户端连接完成
	for attempts := 0; attempts < 100 && connectedCount < numClients; attempts++ {
		server.Tick()
		for _, client := range clients {
			client.Tick()
		}
		time.Sleep(10 * time.Millisecond)
	}

	if connectedCount != numClients {
		b.Fatalf("Expected %d connected clients, got %d", numClients, connectedCount)
	}

	// 清理函数
	defer func() {
		for _, client := range clients {
			if client != nil {
				client.Disconnect()
			}
		}
	}()

	// 强制GC并获取基准内存统计
	runtime.GC()
	runtime.GC() // 调用两次确保完全清理
	var memStatsBefore runtime.MemStats
	runtime.ReadMemStats(&memStatsBefore)

	// 重置计时器，只测量Tick操作的性能
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		server.Tick()
		// 也需要tick客户端以保持连接活跃
		for _, client := range clients {
			client.Tick()
		}
	}

	// 停止计时器并获取结束时的内存统计
	b.StopTimer()
	var memStatsAfter runtime.MemStats
	runtime.ReadMemStats(&memStatsAfter)

	// 计算实际的内存分配
	allocBytes := memStatsAfter.TotalAlloc - memStatsBefore.TotalAlloc
	allocCount := memStatsAfter.Mallocs - memStatsBefore.Mallocs

	// 手动报告内存统计
	b.ReportMetric(float64(allocBytes)/float64(b.N), "B/op")
	b.ReportMetric(float64(allocCount)/float64(b.N), "allocs/op")
}

// BenchmarkTickWithConnections 使用标准benchmark内存统计（包含连接建立）
func BenchmarkTickWithConnections(b *testing.B) {
	config := DefaultKcpConfig()
	config.Mtu = 1400
	config.RecvBufferSize = 7 * config.Mtu
	config.SendBufferSize = 7 * config.Mtu
	config.NoDelay = true
	config.Interval = 1
	config.Timeout = 10000

	// 客户端连接状态跟踪
	const numClients = 5
	connectedClients := make([]bool, numClients)
	connectedCount := 0

	server := NewKcpServer(
		func(connectionId int) {},
		func(connectionId int, data []byte, channel KcpChannel) {},
		func(connectionId int) {},
		func(connectionId int, errorCode ErrorCode, reason string) {},
		config,
	)

	err := server.Start(0)
	if err != nil {
		b.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// 获取服务器端口
	serverAddr := server.LocalEndPoint().(*net.UDPAddr)

	// 创建多个客户端连接来模拟真实场景
	clients := make([]*KcpClient, numClients)

	for i := 0; i < numClients; i++ {
		clientIndex := i // 捕获循环变量
		client := NewKcpClient(
			func() {
				// 客户端连接成功回调
				connectedClients[clientIndex] = true
				connectedCount++
			},
			func(data []byte, channel KcpChannel) {},
			func() {
				// 客户端断开连接回调
				if connectedClients[clientIndex] {
					connectedClients[clientIndex] = false
					connectedCount--
				}
			},
			func(errorCode ErrorCode, reason string) {},
			config,
		)
		clients[i] = client

		// 连接到服务器
		err := client.Connect("127.0.0.1", uint16(serverAddr.Port))
		if err != nil {
			b.Fatalf("Failed to connect client %d: %v", i, err)
		}
	}

	// 等待所有客户端连接完成
	for attempts := 0; attempts < 100 && connectedCount < numClients; attempts++ {
		server.Tick()
		for _, client := range clients {
			client.Tick()
		}
		time.Sleep(10 * time.Millisecond)
	}

	if connectedCount != numClients {
		b.Fatalf("Expected %d connected clients, got %d", numClients, connectedCount)
	}

	// 清理函数
	defer func() {
		for _, client := range clients {
			if client != nil {
				client.Disconnect()
			}
		}
	}()

	// 所有客户端连接建立完成后重置计时器，只测量Tick操作的性能
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		server.Tick()
		// 也需要tick客户端以保持连接活跃
		for _, client := range clients {
			client.Tick()
		}
	}
}
