package lockstep

import (
	"testing"
	"time"

	kcp2k "github.com/O-Keh-Hunter/kcp2k-go"
)

// TestHandleLoginRequest 测试登录请求处理
func TestHandleLoginRequest(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8601,
		LogLevel:    "info",
		MetricsPort: 9601,
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

	// 验证玩家初始状态（ConnectionID应该为-1）
	player, exists := room.Players[1]
	if !exists || player == nil {
		t.Fatal("Expected player to exist in room")
	}
	if player.ConnectionID != -1 {
		t.Errorf("Expected initial connection ID to be -1, got %d", player.ConnectionID)
	}

	// 测试有效的登录请求
	loginReq := &LoginRequest{
		Token:    "valid_token",
		PlayerId: 1,
	}

	// 模拟连接ID
	connectionID := 1

	// 调用处理函数
	server.handleLoginRequest(room, connectionID, loginReq)

	// 验证玩家连接ID被正确设置
	player, exists = room.Players[1]
	if !exists || player == nil {
		t.Error("Expected player to exist in room")
	}

	if player.ConnectionID != connectionID {
		t.Errorf("Expected connection ID %d, got %d", connectionID, player.ConnectionID)
	}

	// 验证玩家状态变为在线
	if player.Status != PlayerStatus_PLAYER_STATUS_ONLINE {
		t.Errorf("Expected player status to be ONLINE, got %v", player.Status)
	}

	// 测试重连（同一玩家用不同连接ID登录）
	newConnectionID := 2
	server.handleLoginRequest(room, newConnectionID, loginReq)

	// 验证连接ID被更新为新的连接ID
	player, exists = room.Players[1]
	if exists && player.ConnectionID != newConnectionID {
		t.Errorf("Expected connection ID to be updated to %d, got %d", newConnectionID, player.ConnectionID)
	}

	// 测试无效token的登录请求
	invalidLoginReq := &LoginRequest{
		Token:    "",
		PlayerId: 1,
	}
	server.handleLoginRequest(room, 3, invalidLoginReq)

	// 验证连接ID没有改变（无效token应该被拒绝）
	player, exists = room.Players[1]
	if exists && player.ConnectionID != newConnectionID {
		t.Errorf("Expected connection ID to remain %d after invalid token, got %d", newConnectionID, player.ConnectionID)
	}

	// 测试不存在的玩家登录
	nonExistentPlayerReq := &LoginRequest{
		Token:    "valid_token",
		PlayerId: 999,
	}
	server.handleLoginRequest(room, 4, nonExistentPlayerReq)

	// 验证不存在的玩家不会被添加到房间
	_, exists = room.Players[999]
	if exists {
		t.Error("Expected non-existent player not to be added to room")
	}
}

// TestHandleLogoutRequest 测试登出请求处理
func TestHandleLogoutRequest(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8602,
		LogLevel:    "info",
		MetricsPort: 9602,
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

	// 先登录一个玩家
	connectionID := 1
	loginReq := &LoginRequest{
		Token:    "valid_token",
		PlayerId: 1,
	}
	server.handleLoginRequest(room, connectionID, loginReq)

	// 验证玩家已登录
	player, exists := room.Players[1]
	if !exists || player == nil {
		t.Fatal("Expected player to be logged in")
	}

	// 测试登出请求
	logoutReq := &LogoutRequest{}
	server.handleLogoutRequest(room, connectionID, logoutReq)

	// 验证玩家状态变为离线
	player, exists = room.Players[1]
	if exists && player != nil && player.Status != PlayerStatus_PLAYER_STATUS_OFFLINE {
		t.Errorf("Expected player status to be offline, got %v", player.Status)
	}
}

// TestHandlePlayerReadyRequest 测试玩家准备请求处理
func TestHandlePlayerReadyRequest(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8603,
		LogLevel:    "info",
		MetricsPort: 9603,
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

	// 先登录一个玩家
	connectionID := 1
	loginReq := &LoginRequest{
		Token:    "valid_token",
		PlayerId: 1,
	}
	server.handleLoginRequest(room, connectionID, loginReq)

	// 测试准备请求
	readyReq := &ReadyRequest{
		Ready: true,
	}
	server.handlePlayerReadyRequest(room, connectionID, readyReq)

	// 验证玩家状态变为准备
	player, exists := room.Players[1]
	if !exists || player == nil {
		t.Fatal("Expected player to exist")
	}

	if player.Status != PlayerStatus_PLAYER_STATUS_READY {
		t.Errorf("Expected player status to be ready, got %v", player.Status)
	}
}

// TestHandlePlayerInputRequest 测试玩家输入请求处理
func TestHandlePlayerInputRequest(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8604,
		LogLevel:    "info",
		MetricsPort: 9604,
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

	// 先登录一个玩家
	connectionID := 1
	loginReq := &LoginRequest{
		Token:    "valid_token",
		PlayerId: 1,
	}
	server.handleLoginRequest(room, connectionID, loginReq)

	// 设置玩家为准备状态
	player, exists := room.Players[1]
	if exists && player != nil {
		player.Status = PlayerStatus_PLAYER_STATUS_READY
	}

	// 启动房间
	room.State.Status = RoomStatus_ROOM_STATUS_RUNNING

	// 测试输入请求
	inputMsg := &InputMessage{
		SequenceId: 1,
		Timestamp:  time.Now().UnixMilli(),
		Data:       []byte("test input"),
		Flag:       InputMessage_None,
	}

	server.handlePlayerInputRequest(room, connectionID, inputMsg)

	// 验证输入被记录
	// 这里我们主要验证函数不会崩溃
	// 实际的输入验证需要更复杂的设置
}

// TestHandleBroadcastRequest 测试广播请求处理
func TestHandleBroadcastRequest(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8605,
		LogLevel:    "info",
		MetricsPort: 9605,
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

	// 先登录一个玩家
	connectionID := 1
	loginReq := &LoginRequest{
		Token:    "valid_token",
		PlayerId: 1,
	}
	server.handleLoginRequest(room, connectionID, loginReq)

	// 测试广播请求
	broadcastReq := &BroadcastRequest{
		Data: []byte("test broadcast message"),
	}

	server.handleBroadcastRequest(room, connectionID, broadcastReq)

	// 验证函数执行不会崩溃
	// 实际的广播验证需要多个客户端连接
}

// TestHandleFrameRequest 测试帧请求处理
func TestHandleFrameRequest(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8606,
		LogLevel:    "info",
		MetricsPort: 9606,
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

	// 先登录一个玩家
	connectionID := 1
	loginReq := &LoginRequest{
		Token:    "valid_token",
		PlayerId: 1,
	}
	server.handleLoginRequest(room, connectionID, loginReq)

	// 添加一些帧数据到房间
	room.Mutex.Lock()
	for i := 1; i <= 5; i++ {
		frame := &FrameMessage{
			FrameId:        FrameID(i),
			RecvTickMs:     int32(time.Now().UnixMilli()),
			ValidDataCount: 0,
			DataCollection: []*RelayData{},
		}
		room.Frames[FrameID(i)] = frame
	}
	room.Mutex.Unlock()

	// 测试帧请求
	frameReq := &FrameRequest{
		FrameId: 1,
		Count:   3,
	}

	server.handleFrameRequest(room, connectionID, frameReq)

	// 验证函数执行不会崩溃
}

// TestGetRoomInfo 测试获取房间信息
func TestGetRoomInfo(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8607,
		LogLevel:    "info",
		MetricsPort: 9607,
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

	// 测试获取存在的房间信息
	foundRoom, exists := server.GetRoomInfo(room.ID)
	if !exists {
		t.Error("Expected room to exist")
	}

	if foundRoom.ID != room.ID {
		t.Errorf("Expected room ID %s, got %s", room.ID, foundRoom.ID)
	}

	// 测试获取不存在的房间信息
	_, exists = server.GetRoomInfo("nonexistent")
	if exists {
		t.Error("Expected room to not exist")
	}
}

// TestGetRoomMonitoringInfo 测试获取房间监控信息
func TestGetRoomMonitoringInfo(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8608,
		LogLevel:    "info",
		MetricsPort: 9608,
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

	// 测试获取存在的房间监控信息
	info, err := server.GetRoomMonitoringInfo(room.ID)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if info == nil {
		t.Error("Expected monitoring info to be returned")
	}

	// 验证基本字段
	if roomID, ok := info["room_id"]; !ok || roomID != string(room.ID) {
		t.Errorf("Expected room_id %s, got %v", room.ID, roomID)
	}

	// 测试获取不存在的房间监控信息
	_, err = server.GetRoomMonitoringInfo("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent room")
	}
}

// TestSendLoginResponse 测试发送登录响应
func TestSendLoginResponse(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8609,
		LogLevel:    "info",
		MetricsPort: 9609,
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

	// 测试发送登录响应
	connectionID := 1
	server.sendLoginResponse(room, connectionID, ErrorCode_ERROR_CODE_SUCC, 1)

	// 验证函数执行不会崩溃
	// 实际的响应验证需要客户端连接
}

// TestSendLogoutResponse 测试发送登出响应
func TestSendLogoutResponse(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8610,
		LogLevel:    "info",
		MetricsPort: 9610,
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

	// 测试发送登出响应
	connectionID := 1
	server.sendLogoutResponse(room, connectionID, ErrorCode_ERROR_CODE_SUCC)

	// 验证函数执行不会崩溃
}

// TestSendReadyResponse 测试发送准备响应
func TestSendReadyResponse(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8611,
		LogLevel:    "info",
		MetricsPort: 9611,
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

	// 测试发送准备响应
	connectionID := 1
	server.sendReadyResponse(room, connectionID, ErrorCode_ERROR_CODE_SUCC)

	// 验证函数执行不会崩溃
}

// TestSendBroadcastResponse 测试发送广播响应
func TestSendBroadcastResponse(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8612,
		LogLevel:    "info",
		MetricsPort: 9612,
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

	// 测试发送广播响应
	connectionID := 1
	server.sendBroadcastResponse(room, connectionID, ErrorCode_ERROR_CODE_SUCC)

	// 验证函数执行不会崩溃
}

// TestSendFrameResponse 测试发送帧响应
func TestSendFrameResponse(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8613,
		LogLevel:    "info",
		MetricsPort: 9613,
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

	// 创建测试帧
	frames := []*FrameMessage{
		{
			FrameId:        1,
			RecvTickMs:     int32(time.Now().UnixMilli()),
			ValidDataCount: 0,
			DataCollection: []*RelayData{},
		},
		{
			FrameId:        2,
			RecvTickMs:     int32(time.Now().UnixMilli()),
			ValidDataCount: 0,
			DataCollection: []*RelayData{},
		},
	}

	// 测试发送帧响应
	connectionID := 1
	server.sendFrameResponse(room, connectionID, frames)

	// 验证函数执行不会崩溃
}

// TestSendGameStartMessage 测试发送游戏开始消息
func TestSendGameStartMessage(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8614,
		LogLevel:    "info",
		MetricsPort: 9614,
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

	// 测试发送游戏开始消息
	connectionID := 1
	server.sendGameStartMessage(room, connectionID, 1, false)

	// 验证函数执行不会崩溃
}
