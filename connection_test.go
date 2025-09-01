package kcp2k

import (
	"net"
	"runtime"
	"testing"
	"time"
)

// BenchmarkServerConnectionTickIncoming 测试KcpServerConnection的TickIncoming方法性能
func BenchmarkServerConnectionTickIncoming(b *testing.B) {
	config := DefaultKcpConfig()
	config.Mtu = 1400
	config.RecvBufferSize = 7 * config.Mtu
	config.SendBufferSize = 7 * config.Mtu
	config.NoDelay = true
	config.Interval = 1
	config.Timeout = 10000

	// 连接状态跟踪
	connected := false
	var serverConnection *KcpServerConnection

	// 创建服务器
	server := NewKcpServer(
		func(connectionId int) {
			connected = true
		},
		func(connectionId int, data []byte, channel KcpChannel) {},
		func(connectionId int) {
			connected = false
		},
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

	// 创建客户端
	client := NewKcpClient(
		func() {},
		func(data []byte, channel KcpChannel) {},
		func() {},
		func(errorCode ErrorCode, reason string) {},
		config,
	)

	// 连接到服务器
	err = client.Connect("127.0.0.1", uint16(serverAddr.Port))
	if err != nil {
		b.Fatalf("Failed to connect client: %v", err)
	}

	// 等待连接建立并获取服务器连接
	for attempts := 0; attempts < 100 && !connected; attempts++ {
		server.Tick()
		client.Tick()
		time.Sleep(10 * time.Millisecond)
	}

	if !connected {
		b.Fatalf("Client failed to connect to server")
	}

	// 获取服务器连接对象
	server.mu.RLock()
	for _, conn := range server.connections {
		serverConnection = conn
		break
	}
	server.mu.RUnlock()

	if serverConnection == nil {
		b.Fatalf("Failed to get server connection")
	}

	defer func() {
		client.Disconnect()
	}()

	// 强制垃圾回收
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		serverConnection.TickIncoming()
		// 也需要tick客户端以保持连接活跃
		client.TickIncoming()
	}
}

// BenchmarkServerConnectionTickOutgoing 测试KcpServerConnection的TickOutgoing方法性能
func BenchmarkServerConnectionTickOutgoing(b *testing.B) {
	config := DefaultKcpConfig()
	config.Mtu = 1400
	config.RecvBufferSize = 7 * config.Mtu
	config.SendBufferSize = 7 * config.Mtu
	config.NoDelay = true
	config.Interval = 1
	config.Timeout = 10000

	// 连接状态跟踪
	connected := false
	var serverConnection *KcpServerConnection

	// 创建服务器
	server := NewKcpServer(
		func(connectionId int) {
			connected = true
		},
		func(connectionId int, data []byte, channel KcpChannel) {},
		func(connectionId int) {
			connected = false
		},
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

	// 创建客户端
	client := NewKcpClient(
		func() {},
		func(data []byte, channel KcpChannel) {},
		func() {},
		func(errorCode ErrorCode, reason string) {},
		config,
	)

	// 连接到服务器
	err = client.Connect("127.0.0.1", uint16(serverAddr.Port))
	if err != nil {
		b.Fatalf("Failed to connect client: %v", err)
	}

	// 等待连接建立并获取服务器连接
	for attempts := 0; attempts < 100 && !connected; attempts++ {
		server.Tick()
		client.Tick()
		time.Sleep(10 * time.Millisecond)
	}

	if !connected {
		b.Fatalf("Client failed to connect to server")
	}

	// 获取服务器连接对象
	server.mu.RLock()
	for _, conn := range server.connections {
		serverConnection = conn
		break
	}
	server.mu.RUnlock()

	if serverConnection == nil {
		b.Fatalf("Failed to get server connection")
	}

	defer func() {
		client.Disconnect()
	}()

	// 强制垃圾回收
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		serverConnection.TickOutgoing()
		// 也需要tick客户端以保持连接活跃
		client.TickOutgoing()
	}
}

// BenchmarkServerConnectionSend 测试KcpServerConnection的Send方法性能
func BenchmarkServerConnectionSend(b *testing.B) {
	config := DefaultKcpConfig()
	config.Mtu = 1400
	config.RecvBufferSize = 7 * config.Mtu
	config.SendBufferSize = 7 * config.Mtu
	config.NoDelay = true
	config.Interval = 1
	config.Timeout = 10000

	// 连接状态跟踪
	connected := false
	var serverConnection *KcpServerConnection

	// 创建服务器
	server := NewKcpServer(
		func(connectionId int) {
			connected = true
		},
		func(connectionId int, data []byte, channel KcpChannel) {},
		func(connectionId int) {
			connected = false
		},
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

	// 创建客户端
	client := NewKcpClient(
		func() {},
		func(data []byte, channel KcpChannel) {},
		func() {},
		func(errorCode ErrorCode, reason string) {},
		config,
	)

	// 连接到服务器
	err = client.Connect("127.0.0.1", uint16(serverAddr.Port))
	if err != nil {
		b.Fatalf("Failed to connect client: %v", err)
	}

	// 等待连接建立并获取服务器连接
	for attempts := 0; attempts < 100 && !connected; attempts++ {
		server.Tick()
		client.Tick()
		time.Sleep(10 * time.Millisecond)
	}

	if !connected {
		b.Fatalf("Client failed to connect to server")
	}

	// 获取服务器连接对象
	server.mu.RLock()
	for _, conn := range server.connections {
		serverConnection = conn
		break
	}
	server.mu.RUnlock()

	if serverConnection == nil {
		b.Fatalf("Failed to get server connection")
	}

	defer func() {
		client.Disconnect()
	}()

	// 准备测试数据
	testData := make([]byte, 100)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// 强制垃圾回收
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		serverConnection.Send(testData, KcpReliable)
		// 定期tick以处理发送队列
		if i%100 == 0 {
			serverConnection.TickOutgoing()
			client.TickIncoming()
		}
	}
}
