package lockstep

import (
	"testing"
	"time"

	kcp2k "github.com/O-Keh-Hunter/kcp2k-go"
)

// TestLockStepClient_Connect_Disconnect 测试客户端连接和断开
func TestLockStepClient_Connect_Disconnect(t *testing.T) {
	// 创建KCP配置
	kcpConfig := kcp2k.DefaultKcpConfig()

	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcpConfig,
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8920,
		LogLevel:    "info",
		MetricsPort: 10020,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := DefaultRoomConfig()
	playerIDs := []PlayerID{1, 2}
	room, err := server.CreateRoom(roomConfig, playerIDs)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// 等待房间KCP服务器启动
	time.Sleep(100 * time.Millisecond)

	// 创建信号通道
	connectedChan := make(chan bool, 1)
	disconnectedChan := make(chan bool, 1)

	// 创建客户端回调
	connected := false
	disconnected := false
	callbacks := ClientCallbacks{
		OnConnected: func() {
			connected = true
			select {
			case connectedChan <- true:
			default:
			}
		},
		OnDisconnected: func() {
			disconnected = true
			select {
			case disconnectedChan <- true:
			default:
			}
		},
		OnError: func(err error) {
			t.Logf("Client error: %v", err)
		},
	}

	// 创建客户端
	client := NewLockStepClient(config, PlayerID(1), callbacks)

	// 测试连接
	err = client.Connect("127.0.0.1", uint16(room.Port))
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// 等待连接建立
	select {
	case <-connectedChan:
		// 连接建立
	case <-time.After(5 * time.Second):
		t.Fatal("Connection timeout")
	}

	// 验证连接状态
	if !client.IsConnected() {
		t.Error("Client should be connected")
	}
	if !connected {
		t.Error("OnConnected callback should be called")
	}

	// 测试断开连接
	client.Disconnect()

	// 等待断开完成
	select {
	case <-disconnectedChan:
		// 断开连接完成
	case <-time.After(5 * time.Second):
		t.Fatal("Disconnect timeout")
	}

	// 验证断开状态
	if client.IsConnected() {
		t.Error("Client should be disconnected")
	}
	if !disconnected {
		t.Error("OnDisconnected callback should be called")
	}
}

// TestLockStepClient_SendLogin 测试发送登录请求
func TestLockStepClient_SendLogin(t *testing.T) {
	// 创建KCP配置
	kcpConfig := kcp2k.DefaultKcpConfig()

	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcpConfig,
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8921,
		LogLevel:    "info",
		MetricsPort: 10021,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := DefaultRoomConfig()
	playerIDs := []PlayerID{1, 2}
	room, err := server.CreateRoom(roomConfig, playerIDs)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// 等待房间KCP服务器启动
	time.Sleep(100 * time.Millisecond)

	// 创建信号通道
	connectedChan := make(chan bool, 1)

	// 创建客户端回调
	loginResponseReceived := false
	callbacks := ClientCallbacks{
		OnConnected: func() {
			select {
			case connectedChan <- true:
			default:
			}
		},
		OnDisconnected: func() {},
		OnLoginResponse: func(errorCode ErrorCode, errorMessage string) {
			loginResponseReceived = true
			if errorCode != ErrorCode_ERROR_CODE_SUCC {
				t.Errorf("Login failed: %s", errorMessage)
			}
		},
		OnError: func(err error) {
			t.Logf("Client error: %v", err)
		},
	}

	// 创建客户端
	client := NewLockStepClient(config, PlayerID(1), callbacks)

	// 连接到服务器
	err = client.Connect("127.0.0.1", uint16(room.Port))
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	// 等待连接建立
	select {
	case <-connectedChan:
		// 连接建立
	case <-time.After(5 * time.Second):
		t.Fatal("Connection timeout")
	}

	// 验证连接状态
	if !client.IsConnected() {
		t.Fatal("Client should be connected")
	}

	// 测试发送登录请求
	err = client.SendLogin("test_token", 1)
	if err != nil {
		t.Errorf("SendLogin failed: %v", err)
	}

	// 等待响应
	time.Sleep(200 * time.Millisecond)

	// 验证登录响应
	if !loginResponseReceived {
		t.Error("Login response should be received")
	}
}

// TestLockStepClient_SendLogout 测试发送登出请求
func TestLockStepClient_SendLogout(t *testing.T) {
	// 创建KCP配置
	kcpConfig := kcp2k.DefaultKcpConfig()

	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcpConfig,
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8922,
		LogLevel:    "info",
		MetricsPort: 10022,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := DefaultRoomConfig()
	playerIDs := []PlayerID{1, 2}
	room, err := server.CreateRoom(roomConfig, playerIDs)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// 等待房间KCP服务器启动
	time.Sleep(100 * time.Millisecond)

	// 创建信号通道
	connectedChan := make(chan bool, 1)

	// 创建客户端回调
	logoutResponseReceived := false
	callbacks := ClientCallbacks{
		OnConnected: func() {
			select {
			case connectedChan <- true:
			default:
			}
		},
		OnDisconnected: func() {},
		OnLogoutResponse: func(errorCode ErrorCode, errorMessage string) {
			logoutResponseReceived = true
			if errorCode != ErrorCode_ERROR_CODE_SUCC {
				t.Errorf("Logout failed: %s", errorMessage)
			}
		},
		OnError: func(err error) {
			t.Logf("Client error: %v", err)
		},
	}

	// 创建客户端
	client := NewLockStepClient(config, PlayerID(1), callbacks)

	// 连接到服务器
	err = client.Connect("127.0.0.1", uint16(room.Port))
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	// 等待连接建立
	select {
	case <-connectedChan:
		// 连接建立
	case <-time.After(5 * time.Second):
		t.Fatal("Connection timeout")
	}

	// 验证连接状态
	if !client.IsConnected() {
		t.Fatal("Client should be connected")
	}

	// 先登录
	err = client.SendLogin("test_token", 1)
	if err != nil {
		t.Fatalf("SendLogin failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// 测试未连接时发送登出请求
	client.Disconnect()
	time.Sleep(100 * time.Millisecond)
	err = client.SendLogout()
	if err == nil {
		t.Error("SendLogout should fail when not connected")
	}

	// 重新连接和登录
	err = client.Connect("127.0.0.1", uint16(room.Port))
	if err != nil {
		t.Fatalf("Failed to reconnect: %v", err)
	}
	// 等待重新连接建立
	select {
	case <-connectedChan:
		// 重新连接建立
	case <-time.After(5 * time.Second):
		t.Fatal("Reconnection timeout")
	}
	if !client.IsConnected() {
		t.Fatal("Client should be reconnected")
	}
	err = client.SendLogin("test_token", 1)
	if err != nil {
		t.Fatalf("SendLogin failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// 测试发送登出请求
	err = client.SendLogout()
	if err != nil {
		t.Errorf("SendLogout failed: %v", err)
	}

	// 等待响应
	time.Sleep(200 * time.Millisecond)

	// 验证登出响应
	if !logoutResponseReceived {
		t.Error("Logout response should be received")
	}
}

// TestLockStepClient_SendReady 测试发送准备状态
func TestLockStepClient_SendReady(t *testing.T) {
	// 创建KCP配置
	kcpConfig := kcp2k.DefaultKcpConfig()

	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcpConfig,
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8923,
		LogLevel:    "info",
		MetricsPort: 10023,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := DefaultRoomConfig()
	playerIDs := []PlayerID{1, 2}
	room, err := server.CreateRoom(roomConfig, playerIDs)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// 等待房间KCP服务器启动
	time.Sleep(100 * time.Millisecond)

	// 创建信号通道
	connectedChan := make(chan bool, 1)

	// 创建客户端回调
	readyResponseReceived := false
	callbacks := ClientCallbacks{
		OnConnected: func() {
			select {
			case connectedChan <- true:
			default:
			}
		},
		OnDisconnected: func() {},
		OnReadyResponse: func(errorCode ErrorCode, errorMessage string) {
			readyResponseReceived = true
			if errorCode != ErrorCode_ERROR_CODE_SUCC {
				t.Errorf("Ready failed: %s", errorMessage)
			}
		},
		OnError: func(err error) {
			t.Logf("Client error: %v", err)
		},
	}

	// 创建客户端
	client := NewLockStepClient(config, PlayerID(1), callbacks)

	// 连接到服务器
	err = client.Connect("127.0.0.1", uint16(room.Port))
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	// 等待连接建立
	select {
	case <-connectedChan:
		// 连接建立
	case <-time.After(5 * time.Second):
		t.Fatal("Connection timeout")
	}

	// 验证连接状态
	if !client.IsConnected() {
		t.Fatal("Client should be connected")
	}

	// 先登录
	err = client.SendLogin("test_token", 1)
	if err != nil {
		t.Fatalf("SendLogin failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// 测试未连接时发送准备状态
	client.Disconnect()
	time.Sleep(100 * time.Millisecond)
	err = client.SendReady(true)
	if err == nil {
		t.Error("SendReady should fail when not connected")
	}

	// 重新连接和登录
	err = client.Connect("127.0.0.1", uint16(room.Port))
	if err != nil {
		t.Fatalf("Failed to reconnect: %v", err)
	}
	// 等待重新连接建立
	select {
	case <-connectedChan:
		// 重新连接建立
	case <-time.After(5 * time.Second):
		t.Fatal("Reconnection timeout")
	}
	if !client.IsConnected() {
		t.Fatal("Client should be reconnected")
	}
	err = client.SendLogin("test_token", 1)
	if err != nil {
		t.Fatalf("SendLogin failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// 测试发送准备状态
	err = client.SendReady(true)
	if err != nil {
		t.Errorf("SendReady failed: %v", err)
	}

	// 等待响应
	time.Sleep(200 * time.Millisecond)

	// 验证准备响应
	if !readyResponseReceived {
		t.Error("Ready response should be received")
	}

	// 测试取消准备状态
	readyResponseReceived = false
	err = client.SendReady(false)
	if err != nil {
		t.Errorf("SendReady(false) failed: %v", err)
	}

	// 等待响应
	time.Sleep(200 * time.Millisecond)

	// 验证取消准备响应
	if !readyResponseReceived {
		t.Error("Ready(false) response should be received")
	}
}

// TestLockStepClient_GetStats 测试获取客户端统计信息
func TestLockStepClient_GetStats(t *testing.T) {
	// 创建KCP配置
	kcpConfig := kcp2k.DefaultKcpConfig()

	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcpConfig,
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8924,
		LogLevel:    "info",
		MetricsPort: 10024,
	}

	// 创建客户端回调
	callbacks := ClientCallbacks{
		OnConnected:    func() {},
		OnDisconnected: func() {},
		OnError: func(err error) {
			t.Logf("Client error: %v", err)
		},
	}

	// 创建客户端
	client := NewLockStepClient(config, PlayerID(1), callbacks)

	// 测试获取客户端统计信息
	stats := client.GetClientStats()
	if stats == nil {
		t.Error("Client stats should not be nil")
	}

	// 验证统计信息包含基本字段
	if _, ok := stats["player_id"]; !ok {
		t.Error("Stats should contain player_id")
	}
	if _, ok := stats["connected"]; !ok {
		t.Error("Stats should contain connected")
	}
	if _, ok := stats["current_frame"]; !ok {
		t.Error("Stats should contain current_frame")
	}

	// 测试获取网络统计信息
	networkStats := client.GetNetworkStats()
	if networkStats == nil {
		t.Error("Network stats should not be nil")
	}

	// 测试获取帧统计信息
	frameStats := client.GetFrameStats()
	if frameStats == nil {
		t.Error("Frame stats should not be nil")
	}

	// 测试RTT功能
	rtt := client.GetRTT()
	if rtt < 0 {
		t.Errorf("RTT should not be negative, got %v", rtt)
	}
}

// TestLockStepClient_MessageHandling 测试客户端消息处理
func TestLockStepClient_MessageHandling(t *testing.T) {
	// 创建KCP配置
	kcpConfig := kcp2k.DefaultKcpConfig()

	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcpConfig,
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8925,
		LogLevel:    "info",
		MetricsPort: 10025,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := DefaultRoomConfig()
	playerIDs := []PlayerID{1, 2}
	room, err := server.CreateRoom(roomConfig, playerIDs)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// 等待房间KCP服务器启动
	time.Sleep(100 * time.Millisecond)

	// 创建信号通道
	connectedChan := make(chan bool, 1)

	// 创建客户端回调
	playerStateChanged := false
	roomStateChanged := false

	callbacks := ClientCallbacks{
		OnConnected: func() {
			select {
			case connectedChan <- true:
			default:
			}
		},
		OnDisconnected: func() {},
		OnPlayerJoined: func(playerID PlayerID) {
			// Player joined callback
		},
		OnPlayerLeft: func(playerID PlayerID) {
			// Player left callback
		},
		OnGameStarted: func() {
			// Game started callback
		},
		OnBroadcastReceived: func(playerID PlayerID, data []byte) {
			// Broadcast received callback
		},
		OnPlayerStateChanged: func(playerID PlayerID, status PlayerStatus, reason string) {
			playerStateChanged = true
		},
		OnRoomStateChanged: func(status RoomStatus) {
			roomStateChanged = true
		},
		OnError: func(err error) {
			t.Logf("Client error: %v", err)
		},
	}

	// 创建客户端
	client := NewLockStepClient(config, PlayerID(1), callbacks)

	// 连接到服务器
	err = client.Connect("127.0.0.1", uint16(room.Port))
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	// 等待连接建立
	select {
	case <-connectedChan:
		// 连接建立
	case <-time.After(5 * time.Second):
		t.Fatal("Connection timeout")
	}

	// 验证连接状态
	if !client.IsConnected() {
		t.Fatal("Client should be connected")
	}

	// 登录
	err = client.SendLogin("test_token", 1)
	if err != nil {
		t.Fatalf("SendLogin failed: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	// 验证回调是否被调用
	if !playerStateChanged {
		t.Error("OnPlayerStateChanged should be called after login")
	}
	if !roomStateChanged {
		t.Error("OnRoomStateChanged should be called after login")
	}

	// 注意：其他回调（如 OnPlayerJoined, OnGameStarted 等）需要特定的游戏状态才会触发
	// 这里主要测试回调机制是否正常工作
	t.Log("Message handling test completed successfully")
}
