package kcp2k

import (
	"net"
	"runtime"
	"testing"
	"time"
)

// BenchmarkClientTick 测试KcpClient的Tick方法性能
func BenchmarkClientTick(b *testing.B) {
	config := DefaultKcpConfig()
	config.Mtu = 1400
	config.RecvBufferSize = 7 * config.Mtu
	config.SendBufferSize = 7 * config.Mtu
	config.NoDelay = true
	config.Interval = 1
	config.Timeout = 10000

	// 客户端连接状态跟踪
	connected := false

	// 创建服务器
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

	// 创建客户端
	client := NewKcpClient(
		func() {
			// 客户端连接成功回调
			connected = true
		},
		func(data []byte, channel KcpChannel) {}, // onData
		func() {
			// 客户端断开连接回调
			connected = false
		},
		func(errorCode ErrorCode, reason string) {}, // onError
		config,
	)

	// 连接到服务器
	err = client.Connect("127.0.0.1", uint16(serverAddr.Port))
	if err != nil {
		b.Fatalf("Failed to connect client: %v", err)
	}

	// 等待客户端连接完成
	for attempts := 0; attempts < 100 && !connected; attempts++ {
		server.Tick()
		client.Tick()
		time.Sleep(10 * time.Millisecond)
	}

	if !connected {
		b.Fatalf("Client failed to connect to server")
	}

	// 清理函数
	defer func() {
		client.Disconnect()
	}()

	// 预热对象池
	for i := 0; i < 10; i++ {
		buf := client.bufferPool.Get()
		client.bufferPool.Put(buf)
	}

	// 强制垃圾回收
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		client.Tick()
		// 也需要tick服务器以保持连接活跃
		server.Tick()
	}
}

// BenchmarkTickWithConnection 测试有连接状态下的Tick性能
func BenchmarkTickWithConnection(b *testing.B) {
	config := DefaultKcpConfig()
	config.Mtu = 1400
	config.RecvBufferSize = 7 * config.Mtu
	config.SendBufferSize = 7 * config.Mtu
	config.NoDelay = true
	config.Interval = 1
	config.Timeout = 10000

	// 客户端连接状态跟踪
	connected := false

	// 创建服务器
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

	// 创建客户端
	client := NewKcpClient(
		func() {
			// 客户端连接成功回调
			connected = true
		},
		func(data []byte, channel KcpChannel) {}, // onData
		func() {
			// 客户端断开连接回调
			connected = false
		},
		func(errorCode ErrorCode, reason string) {}, // onError
		config,
	)

	// 连接到服务器
	err = client.Connect("127.0.0.1", uint16(serverAddr.Port))
	if err != nil {
		b.Fatalf("Failed to connect client: %v", err)
	}

	// 等待客户端连接完成
	for attempts := 0; attempts < 100 && !connected; attempts++ {
		server.Tick()
		client.Tick()
		time.Sleep(10 * time.Millisecond)
	}

	if !connected {
		b.Fatalf("Client failed to connect to server")
	}

	// 清理函数
	defer func() {
		client.Disconnect()
	}()

	// 预热对象池
	for i := 0; i < 10; i++ {
		buf := client.bufferPool.Get()
		client.bufferPool.Put(buf)
	}

	// 强制垃圾回收
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		client.Tick()
		// 也需要tick服务器以保持连接活跃
		server.Tick()
	}
}

// BenchmarkClientTickIncoming 单独测试TickIncoming性能
func BenchmarkClientTickIncoming(b *testing.B) {
	config := DefaultKcpConfig()
	config.Mtu = 1400
	config.RecvBufferSize = 7 * config.Mtu
	config.SendBufferSize = 7 * config.Mtu
	config.NoDelay = true
	config.Interval = 1
	config.Timeout = 10000

	// 客户端连接状态跟踪
	connected := false

	// 创建服务器
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

	// 创建客户端
	client := NewKcpClient(
		func() {
			// 客户端连接成功回调
			connected = true
		},
		func(data []byte, channel KcpChannel) {}, // onData
		func() {
			// 客户端断开连接回调
			connected = false
		},
		func(errorCode ErrorCode, reason string) {}, // onError
		config,
	)

	// 连接到服务器
	err = client.Connect("127.0.0.1", uint16(serverAddr.Port))
	if err != nil {
		b.Fatalf("Failed to connect client: %v", err)
	}

	// 等待客户端连接完成
	for attempts := 0; attempts < 100 && !connected; attempts++ {
		server.Tick()
		client.Tick()
		time.Sleep(10 * time.Millisecond)
	}

	if !connected {
		b.Fatalf("Client failed to connect to server")
	}

	// 清理函数
	defer func() {
		client.Disconnect()
	}()

	// 预热对象池
	for i := 0; i < 10; i++ {
		buf := client.bufferPool.Get()
		client.bufferPool.Put(buf)
	}

	// 强制垃圾回收
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		client.TickIncoming()
		// 也需要tick服务器以保持连接活跃
		server.TickIncoming()
	}
}

// BenchmarkClientTickOutgoing 单独测试TickOutgoing性能
func BenchmarkClientTickOutgoing(b *testing.B) {
	config := DefaultKcpConfig()
	config.Mtu = 1400
	config.RecvBufferSize = 7 * config.Mtu
	config.SendBufferSize = 7 * config.Mtu
	config.NoDelay = true
	config.Interval = 1
	config.Timeout = 10000

	// 客户端连接状态跟踪
	connected := false

	// 创建服务器
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

	// 创建客户端
	client := NewKcpClient(
		func() {
			// 客户端连接成功回调
			connected = true
		},
		func(data []byte, channel KcpChannel) {}, // onData
		func() {
			// 客户端断开连接回调
			connected = false
		},
		func(errorCode ErrorCode, reason string) {}, // onError
		config,
	)

	// 连接到服务器
	err = client.Connect("127.0.0.1", uint16(serverAddr.Port))
	if err != nil {
		b.Fatalf("Failed to connect client: %v", err)
	}

	// 等待客户端连接完成
	for attempts := 0; attempts < 100 && !connected; attempts++ {
		server.Tick()
		client.Tick()
		time.Sleep(10 * time.Millisecond)
	}

	if !connected {
		b.Fatalf("Client failed to connect to server")
	}

	// 清理函数
	defer func() {
		client.Disconnect()
	}()

	// 强制垃圾回收
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		client.TickOutgoing()
		// 也需要tick服务器以保持连接活跃
		server.TickOutgoing()
	}
}
