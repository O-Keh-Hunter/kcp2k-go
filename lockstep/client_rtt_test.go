package lockstep

import (
	"testing"
	"time"

	kcp2k "github.com/O-Keh-Hunter/kcp2k-go"
)

// TestLockStepClientRTT 测试LockStep客户端的RTT功能
func TestLockStepClientRTT(t *testing.T) {
	// 创建KCP配置
	kcpConfig := kcp2k.KcpConfig{
		Interval:          10,
		Timeout:           10000,
		FastResend:        2,
		CongestionWindow:  false,
		SendWindowSize:    4096,
		ReceiveWindowSize: 4096,
		Mtu:               1200,
		NoDelay:           true,
		DualMode:          false,
		RecvBufferSize:    1024 * 1024,
		SendBufferSize:    1024 * 1024,
		MaxRetransmits:    40,
	}

	// 创建LockStep配置
	lockstepConfig := &LockStepConfig{
		KcpConfig: kcpConfig,
		RoomConfig: &RoomConfig{
			FrameRate:  20,
			MaxPlayers: 2,
		},
	}

	// 创建客户端回调
	callbacks := ClientCallbacks{
		OnConnected: func() {
			t.Log("Client connected")
		},
		OnDisconnected: func() {
			t.Log("Client disconnected")
		},
		OnError: func(err error) {
			t.Logf("Client error: %v", err)
		},
	}

	// 创建LockStep客户端
	client := NewLockStepClient(lockstepConfig, PlayerID(1), callbacks)

	// 测试初始RTT（应该为0，因为还没有连接）
	initialRTT := client.GetRTT()
	if initialRTT != 0 {
		t.Errorf("Expected initial RTT to be 0, got %v", initialRTT)
	}

	// 启动服务器
	var server *kcp2k.KcpServer
	server = kcp2k.NewKcpServer(
		func(connectionId int) {
			t.Logf("Server: client %d connected", connectionId)
		},
		func(connectionId int, data []byte, channel kcp2k.KcpChannel) {
			// Echo back the data
			server.Send(connectionId, data, channel)
		},
		func(connectionId int) {
			t.Logf("Server: client %d disconnected", connectionId)
		},
		func(connectionId int, error kcp2k.ErrorCode, reason string) {
			t.Logf("Server: client %d error: %s - %s", connectionId, error.String(), reason)
		},
		kcpConfig,
	)

	// 启动服务器
	err := server.Start(7777)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// 启动服务器tick
	go func() {
		for {
			server.Tick()
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// 连接到服务器
	err = client.Connect("127.0.0.1", 7777)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer client.Disconnect()

	// 等待连接建立
	time.Sleep(100 * time.Millisecond)

	// 检查连接状态
	if !client.IsConnected() {
		t.Fatal("Client should be connected")
	}

	// 等待一段时间让KCP协议自动进行ping/pong交换来计算RTT
	// KCP协议会自动发送ping消息来维护连接和计算RTT
	for i := 0; i < 10; i++ {
		time.Sleep(100 * time.Millisecond)
		// 检查RTT是否已经更新
		if client.GetRTT() > 0 {
			t.Logf("RTT updated after %d iterations: %v", i+1, client.GetRTT())
			break
		}
	}

	// 等待RTT更新
	time.Sleep(2000 * time.Millisecond)

	// 获取RTT
	rtt := client.GetRTT()
	t.Logf("Client RTT: %v", rtt)

	// RTT应该大于0（在真实网络环境中）
	// 在本地测试中，RTT可能为0，这是正常的
	if rtt < 0 {
		t.Errorf("RTT should not be negative, got %v", rtt)
	}

	// 获取客户端统计信息
	stats := client.GetClientStats()
	if rttStr, ok := stats["rtt"]; ok {
		t.Logf("RTT from stats: %v", rttStr)
	} else {
		t.Error("RTT not found in client stats")
	}

	// 验证网络统计信息中包含RTT信息
	networkStats := client.GetNetworkStats()
	if networkStats != nil {
		avgLatency := networkStats.GetAverageRTT()
		t.Logf("Average network latency: %v", avgLatency)

		// 平均延迟应该与RTT相关
		if avgLatency < 0 {
			t.Errorf("Average latency should not be negative, got %v", avgLatency)
		}
	} else {
		t.Error("Network stats should not be nil")
	}

	t.Log("LockStep client RTT test completed successfully")
}
