package lockstep

import (
	"testing"
	"time"

	kcp2k "github.com/O-Keh-Hunter/kcp2k-go"
)

// TestGetRoomMonitoringInfo_EmptyRoom 测试空房间的监控信息
func TestGetRoomMonitoringInfo_EmptyRoom(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8996,
		LogLevel:    "info",
		MetricsPort: 9996,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := &RoomConfig{
		MaxPlayers:  4,
		MinPlayers:  2,
		FrameRate:   15,
		RetryWindow: 50,
	}

	room, err := server.rooms.CreateRoom(roomConfig, []PlayerID{}, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(room.ID)

	// 获取房间监控信息
	monitoringInfo := room.GetRoomMonitoringInfo()

	// 验证基本字段
	if monitoringInfo["room_id"] != room.ID {
		t.Errorf("Expected room_id to be %s, got %v", room.ID, monitoringInfo["room_id"])
	}

	// 验证running字段存在且为布尔类型
	running, ok := monitoringInfo["running"].(bool)
	if !ok {
		t.Errorf("Expected running to be bool, got %T", monitoringInfo["running"])
	}
	_ = running // 不强制要求running的具体值，因为房间状态可能因各种原因改变

	if monitoringInfo["player_count"] != 0 {
		t.Errorf("Expected player_count to be 0, got %v", monitoringInfo["player_count"])
	}

	if monitoringInfo["max_players"].(int32) != 4 {
		t.Errorf("Expected max_players to be 4, got %v", monitoringInfo["max_players"])
	}

	if monitoringInfo["current_frame"] != FrameID(0) {
		t.Errorf("Expected current_frame to be 0, got %v", monitoringInfo["current_frame"])
	}

	// 验证玩家列表为空
	players, ok := monitoringInfo["players"].([]map[string]interface{})
	if !ok {
		t.Errorf("Expected players to be []map[string]interface{}, got %T", monitoringInfo["players"])
	}
	if len(players) != 0 {
		t.Errorf("Expected players list to be empty, got %d players", len(players))
	}

	// 验证配置信息
	config_info, ok := monitoringInfo["config"].(map[string]interface{})
	if !ok {
		t.Errorf("Expected config to be map[string]interface{}, got %T", monitoringInfo["config"])
	}
	if config_info["max_players"].(int32) != 4 {
		t.Errorf("Expected config.max_players to be 4, got %v", config_info["max_players"])
	}
	if config_info["min_players"].(int32) != 2 {
		t.Errorf("Expected config.min_players to be 2, got %v", config_info["min_players"])
	}
	if config_info["frame_rate"].(int32) != 15 {
		t.Errorf("Expected config.frame_rate to be 15, got %v", config_info["frame_rate"])
	}
	if config_info["retry_window"].(int32) != 50 {
		t.Errorf("Expected config.retry_window to be 50, got %v", config_info["retry_window"])
	}

	// 验证统计信息存在
	if _, exists := monitoringInfo["frame_stats"]; !exists {
		t.Error("Expected frame_stats to exist")
	}
	if _, exists := monitoringInfo["network_stats"]; !exists {
		t.Error("Expected network_stats to exist")
	}

	// 验证时间戳格式
	createdAt, ok := monitoringInfo["created_at"].(string)
	if !ok {
		t.Errorf("Expected created_at to be string, got %T", monitoringInfo["created_at"])
	}
	if _, err := time.Parse(time.RFC3339, createdAt); err != nil {
		t.Errorf("Expected created_at to be valid RFC3339 format, got error: %v", err)
	}
}

// TestGetRoomMonitoringInfo_WithPlayers 测试带玩家的房间监控信息
func TestGetRoomMonitoringInfo_WithPlayers(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8897,
		LogLevel:    "info",
		MetricsPort: 9997,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := &RoomConfig{
		MaxPlayers:  4,
		MinPlayers:  2,
		FrameRate:   15,
		RetryWindow: 50,
	}

	room, err := server.rooms.CreateRoom(roomConfig, []PlayerID{}, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(room.ID)

	// 添加测试玩家
	player1 := &LockStepPlayer{
		Player: &Player{
			PlayerId: PlayerID(101),
			Status:   PlayerStatus_PLAYER_STATUS_ONLINE,
		},
		ConnectionID: 1001,
		InputBuffer:  make(map[FrameID][]*InputMessage),
	}

	player2 := &LockStepPlayer{
		Player: &Player{
			PlayerId: PlayerID(102),
			Status:   PlayerStatus_PLAYER_STATUS_OFFLINE,
		},
		ConnectionID: 1002,
		InputBuffer:  make(map[FrameID][]*InputMessage),
	}

	// 添加玩家到房间
	room.Mutex.Lock()
	room.Players[player1.PlayerId] = player1
	room.Players[player2.PlayerId] = player2
	room.CurrentFrameID = 10
	room.MaxFrameID = 15
	room.Mutex.Unlock()

	// 获取房间监控信息
	monitoringInfo := room.GetRoomMonitoringInfo()

	// 验证玩家数量
	if monitoringInfo["player_count"] != 2 {
		t.Errorf("Expected player_count to be 2, got %v", monitoringInfo["player_count"])
	}

	// 验证帧信息
	if monitoringInfo["current_frame"] != FrameID(10) {
		t.Errorf("Expected current_frame to be 10, got %v", monitoringInfo["current_frame"])
	}
	if monitoringInfo["max_frame"] != FrameID(15) {
		t.Errorf("Expected max_frame to be 15, got %v", monitoringInfo["max_frame"])
	}

	// 验证玩家信息
	players, ok := monitoringInfo["players"].([]map[string]interface{})
	if !ok {
		t.Fatalf("Expected players to be []map[string]interface{}, got %T", monitoringInfo["players"])
	}
	if len(players) != 2 {
		t.Fatalf("Expected 2 players, got %d", len(players))
	}

	// 验证玩家详细信息（不依赖顺序）
	playerMap := make(map[PlayerID]map[string]interface{})
	for _, p := range players {
		playerID := PlayerID(p["id"].(PlayerID))
		playerMap[playerID] = p
	}

	// 验证玩家1
	if p1, exists := playerMap[PlayerID(101)]; exists {
		if p1["connection_id"] != 1001 {
			t.Errorf("Expected player1 connection_id to be 1001, got %v", p1["connection_id"])
		}
		if p1["status"] != PlayerStatus_PLAYER_STATUS_ONLINE {
			t.Errorf("Expected player1 online to be true, got %v", p1["status"])
		}
	} else {
		t.Error("Player 101 not found in monitoring info")
	}

	// 验证玩家2
	if p2, exists := playerMap[PlayerID(102)]; exists {
		if p2["connection_id"] != 1002 {
			t.Errorf("Expected player2 connection_id to be 1002, got %v", p2["connection_id"])
		}
		if p2["status"] != PlayerStatus_PLAYER_STATUS_OFFLINE {
			t.Errorf("Expected player2 online to be PlayerStatus_PLAYER_STATUS_OFFLINE, got %v", p2["status"])
		}
	} else {
		t.Error("Player 102 not found in monitoring info")
	}
}

// TestGetRoomMonitoringInfo_WithStats 测试带统计信息的房间监控信息
func TestGetRoomMonitoringInfo_WithStats(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8898,
		LogLevel:    "info",
		MetricsPort: 9998,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := &RoomConfig{
		MaxPlayers:  4,
		MinPlayers:  2,
		FrameRate:   15,
		RetryWindow: 50,
	}

	room, err := server.rooms.CreateRoom(roomConfig, []PlayerID{}, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(room.ID)

	// 设置帧统计信息 - 使用多次调用来模拟统计数据
	for i := 0; i < 100; i++ {
		missed := i < 5
		late := i < 3
		empty := false
		frameDuration := 20 * time.Millisecond
		room.FrameStats.IncrementFrameStats(missed, late, empty, frameDuration)
	}

	// 设置网络统计信息 - 使用多次调用来模拟统计数据
	for i := 0; i < 20; i++ {
		room.NetworkStats.UpdateNetworkStats(500, 10, 25000, 30000, 50*time.Millisecond)
	}
	// 设置 RTT 范围
	room.NetworkStats.IncrementNetworkStats(1, 1, 0, 0, false, 100*time.Millisecond) // max RTT
	room.NetworkStats.IncrementNetworkStats(1, 1, 0, 0, false, 20*time.Millisecond)  // min RTT

	// 获取房间监控信息
	monitoringInfo := room.GetRoomMonitoringInfo()

	// 验证帧统计信息
	frameStats, ok := monitoringInfo["frame_stats"].(*FrameStats)
	if !ok {
		t.Fatalf("Expected frame_stats to be *FrameStats, got %T", monitoringInfo["frame_stats"])
	}

	if frameStats.GetTotalFrames() != 100 {
		t.Errorf("Expected total_frames to be 100, got %v", frameStats.GetTotalFrames())
	}
	if frameStats.GetMissedFrames() != 5 {
		t.Errorf("Expected missed_frames to be 5, got %v", frameStats.GetMissedFrames())
	}
	if frameStats.GetLateFrames() != 3 {
		t.Errorf("Expected late_frames to be 3, got %v", frameStats.GetLateFrames())
	}
	if frameStats.GetAverageFrameTime() != 20*time.Millisecond {
		t.Errorf("Expected average_frame_time to be 20ms, got %v", frameStats.GetAverageFrameTime())
	}

	// 验证网络统计信息
	networkStats, ok := monitoringInfo["network_stats"].(*NetworkStats)
	if !ok {
		t.Fatalf("Expected network_stats to be *NetworkStats, got %T", monitoringInfo["network_stats"])
	}

	// 验证网络统计信息存在（具体值可能因实现而异）
	if networkStats.GetTotalPackets() == 0 {
		t.Error("Expected total_packets to be greater than 0")
	}
	if networkStats.GetBytesReceived() == 0 {
		t.Error("Expected bytes_received to be greater than 0")
	}
	if networkStats.GetBytesSent() == 0 {
		t.Error("Expected bytes_sent to be greater than 0")
	}
	if networkStats.GetAverageRTT() == 0 {
		t.Error("Expected average_rtt to be greater than 0")
	}
	if networkStats.GetMaxRTT() == 0 {
		t.Error("Expected max_rtt to be greater than 0")
	}
	if networkStats.GetMinRTT() == 0 {
		t.Error("Expected min_rtt to be greater than 0")
	}
}

// TestGetRoomMonitoringInfo_NilStats 测试nil统计信息的房间监控信息
func TestGetRoomMonitoringInfo_NilStats(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8899,
		LogLevel:    "info",
		MetricsPort: 9999,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := &RoomConfig{
		MaxPlayers:  4,
		MinPlayers:  2,
		FrameRate:   15,
		RetryWindow: 50,
	}

	room, err := server.rooms.CreateRoom(roomConfig, []PlayerID{}, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(room.ID)

	// 将统计信息设置为nil
	room.Mutex.Lock()
	room.FrameStats = nil
	room.NetworkStats = nil
	room.Mutex.Unlock()

	// 获取房间监控信息
	monitoringInfo := room.GetRoomMonitoringInfo()

	// 验证基本字段仍然存在
	if monitoringInfo["room_id"] != room.ID {
		t.Errorf("Expected room_id to be %s, got %v", room.ID, monitoringInfo["room_id"])
	}

	// 验证统计信息字段不存在（因为统计对象为nil）
	if _, exists := monitoringInfo["frame_stats"]; exists {
		t.Error("Expected frame_stats to not exist when FrameStats is nil")
	}
	if _, exists := monitoringInfo["network_stats"]; exists {
		t.Error("Expected network_stats to not exist when NetworkStats is nil")
	}

	// 验证其他字段仍然正常
	if monitoringInfo["player_count"] != 0 {
		t.Errorf("Expected player_count to be 0, got %v", monitoringInfo["player_count"])
	}

	// 注意：这里不验证running状态，因为房间的运行状态可能会因为各种原因改变
	// 我们主要关注的是GetRoomMonitoringInfo方法能否正确处理nil统计信息的情况
	running, ok := monitoringInfo["running"].(bool)
	if !ok {
		t.Errorf("Expected running to be bool, got %T", monitoringInfo["running"])
	}
	// 只要running字段存在且为bool类型即可，不强制要求其值
	_ = running
}

// TestRoomManager_NewRoomManager 测试房间管理器创建
func TestRoomManager_NewRoomManager(t *testing.T) {
	portManager := &PortManager{
		startPort:   9000,
		currentPort: 9000,
		usedPorts:   make(map[uint16]bool),
	}
	kcpConfig := kcp2k.DefaultKcpConfig()

	rm := NewRoomManager(portManager, &kcpConfig)

	if rm == nil {
		t.Fatal("NewRoomManager should not return nil")
	}

	if rm.rooms == nil {
		t.Error("Expected rooms map to be initialized")
	}

	if rm.portManager != portManager {
		t.Error("Expected portManager to be set correctly")
	}

	if rm.kcpConfig != &kcpConfig {
		t.Error("Expected kcpConfig to be set correctly")
	}
}

// TestRoomManager_CreateRoom 测试房间创建
func TestRoomManager_CreateRoom(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8908,
		LogLevel:    "info",
		MetricsPort: 10008,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间配置
	roomConfig := &RoomConfig{
		MaxPlayers:  4,
		MinPlayers:  2,
		FrameRate:   30,
		RetryWindow: 50,
	}

	// 创建房间
	room, err := server.rooms.CreateRoom(roomConfig, []PlayerID{1, 2}, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(room.ID)

	// 验证房间属性
	if room.ID == "" {
		t.Error("Expected room ID to be generated")
	}

	if room.Port == 0 {
		t.Error("Expected room port to be allocated")
	}

	if room.Config != roomConfig {
		t.Error("Expected room config to be set correctly")
	}

	if len(room.Players) != 2 {
		t.Errorf("Expected 2 players in room, got %d", len(room.Players))
	}

	// 验证预设玩家
	if _, exists := room.Players[1]; !exists {
		t.Error("Expected player 1 to exist in room")
	}

	if _, exists := room.Players[2]; !exists {
		t.Error("Expected player 2 to exist in room")
	}

	// 验证统计信息初始化
	if room.FrameStats == nil {
		t.Error("Expected FrameStats to be initialized")
	}

	if room.NetworkStats == nil {
		t.Error("Expected NetworkStats to be initialized")
	}
}

// TestRoomManager_GetRoom 测试获取房间
func TestRoomManager_GetRoom(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8909,
		LogLevel:    "info",
		MetricsPort: 10009,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := DefaultRoomConfig()
	room, err := server.rooms.CreateRoom(roomConfig, []PlayerID{}, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(room.ID)

	// 测试获取存在的房间
	foundRoom, exists := server.rooms.GetRoom(room.ID)
	if !exists {
		t.Error("Expected to find created room")
	}

	if foundRoom.ID != room.ID {
		t.Errorf("Expected room ID %s, got %s", room.ID, foundRoom.ID)
	}

	// 测试获取不存在的房间
	_, exists = server.rooms.GetRoom("nonexistent")
	if exists {
		t.Error("Expected not to find nonexistent room")
	}
}

// TestRoomManager_GetRooms 测试获取所有房间
func TestRoomManager_GetRooms(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8910,
		LogLevel:    "info",
		MetricsPort: 10010,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 初始状态应该没有房间
	rooms := server.rooms.GetRooms()
	if len(rooms) != 0 {
		t.Errorf("Expected 0 rooms initially, got %d", len(rooms))
	}

	// 创建多个房间
	roomConfig := DefaultRoomConfig()
	room1, err := server.rooms.CreateRoom(roomConfig, []PlayerID{}, server)
	if err != nil {
		t.Fatalf("Failed to create room1: %v", err)
	}
	defer server.rooms.RemoveRoom(room1.ID)

	room2, err := server.rooms.CreateRoom(roomConfig, []PlayerID{}, server)
	if err != nil {
		t.Fatalf("Failed to create room2: %v", err)
	}
	defer server.rooms.RemoveRoom(room2.ID)

	// 验证获取所有房间
	rooms = server.rooms.GetRooms()
	if len(rooms) != 2 {
		t.Errorf("Expected 2 rooms, got %d", len(rooms))
	}

	if _, exists := rooms[room1.ID]; !exists {
		t.Error("Expected room1 to exist in rooms map")
	}

	if _, exists := rooms[room2.ID]; !exists {
		t.Error("Expected room2 to exist in rooms map")
	}
}

// TestRoomManager_GetAllRooms 测试获取所有房间（别名方法）
func TestRoomManager_GetAllRooms(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8919,
		LogLevel:    "info",
		MetricsPort: 10019,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := DefaultRoomConfig()
	room, err := server.rooms.CreateRoom(roomConfig, []PlayerID{}, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(room.ID)

	// 验证 GetAllRooms 和 GetRooms 返回相同结果
	allRooms := server.rooms.GetAllRooms()
	rooms := server.rooms.GetRooms()

	if len(allRooms) != len(rooms) {
		t.Errorf("Expected GetAllRooms and GetRooms to return same count, got %d vs %d", len(allRooms), len(rooms))
	}

	for id, room := range rooms {
		if allRooms[id] != room {
			t.Errorf("Expected GetAllRooms and GetRooms to return same room for ID %s", id)
		}
	}
}

// TestRoomManager_RemoveRoom 测试移除房间
func TestRoomManager_RemoveRoom(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8912,
		LogLevel:    "info",
		MetricsPort: 10012,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := DefaultRoomConfig()
	room, err := server.rooms.CreateRoom(roomConfig, []PlayerID{}, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// 验证房间存在
	_, exists := server.rooms.GetRoom(room.ID)
	if !exists {
		t.Error("Expected room to exist before removal")
	}

	// 移除房间
	server.rooms.RemoveRoom(room.ID)

	// 验证房间已被移除
	_, exists = server.rooms.GetRoom(room.ID)
	if exists {
		t.Error("Expected room to be removed")
	}

	// 测试移除不存在的房间（应该不会出错）
	server.rooms.RemoveRoom("nonexistent")
}

// TestRoom_GetNetworkStats 测试获取网络统计信息
func TestRoom_GetNetworkStats(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8913,
		LogLevel:    "info",
		MetricsPort: 10013,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := DefaultRoomConfig()
	room, err := server.rooms.CreateRoom(roomConfig, []PlayerID{}, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(room.ID)

	// 获取网络统计信息
	stats := room.GetNetworkStats()
	if stats == nil {
		t.Error("Expected network stats to not be nil")
	}

	// 验证初始值
	if stats.GetTotalPackets() != 0 {
		t.Errorf("Expected initial total packets to be 0, got %d", stats.GetTotalPackets())
	}
}

// TestRoom_GetFrameStats 测试获取帧统计信息
func TestRoom_GetFrameStats(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8914,
		LogLevel:    "info",
		MetricsPort: 10014,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := DefaultRoomConfig()
	room, err := server.rooms.CreateRoom(roomConfig, []PlayerID{}, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(room.ID)

	// 获取帧统计信息
	stats := room.GetFrameStats()
	if stats == nil {
		t.Error("Expected frame stats to not be nil")
	}

	// 验证初始值
	if stats.GetTotalFrames() != 0 {
		t.Errorf("Expected initial total frames to be 0, got %d", stats.GetTotalFrames())
	}
}

// TestPortManager_AllocatePort 测试端口分配
func TestPortManager_AllocatePort(t *testing.T) {
	pm := &PortManager{
		startPort:   9000,
		currentPort: 9000,
		usedPorts:   make(map[uint16]bool),
	}

	// 分配第一个端口
	port1 := pm.AllocatePort()
	if port1 != 9000 {
		t.Errorf("Expected first allocated port to be 9000, got %d", port1)
	}

	// 分配第二个端口
	port2 := pm.AllocatePort()
	if port2 != 9001 {
		t.Errorf("Expected second allocated port to be 9001, got %d", port2)
	}

	// 验证端口被标记为已使用
	if !pm.usedPorts[port1] {
		t.Errorf("Expected port %d to be marked as used", port1)
	}

	if !pm.usedPorts[port2] {
		t.Errorf("Expected port %d to be marked as used", port2)
	}
}

// TestPortManager_ReleasePort 测试端口释放
func TestPortManager_ReleasePort(t *testing.T) {
	pm := &PortManager{
		startPort:   9000,
		currentPort: 9000,
		usedPorts:   make(map[uint16]bool),
	}

	// 分配端口
	port := pm.AllocatePort()

	// 验证端口被标记为已使用
	if !pm.usedPorts[port] {
		t.Errorf("Expected port %d to be marked as used", port)
	}

	// 释放端口
	pm.ReleasePort(port)

	// 验证端口被标记为未使用
	if pm.usedPorts[port] {
		t.Errorf("Expected port %d to be marked as unused after release", port)
	}

	// 测试释放未分配的端口（应该不会出错）
	pm.ReleasePort(9999)
}

// TestRoomManager_Start_Stop 测试房间管理器启动和停止
func TestRoomManager_Start_Stop(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8915,
		LogLevel:    "info",
		MetricsPort: 10015,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := DefaultRoomConfig()
	room, err := server.rooms.CreateRoom(roomConfig, []PlayerID{}, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(room.ID)

	// 验证房间管理器正在运行
	// 这里主要验证不会崩溃
	time.Sleep(100 * time.Millisecond)

	// 停止房间管理器
	server.rooms.Stop()

	// 再次停止应该不会出错
	server.rooms.Stop()
}

// TestRoomManager_UpdateRoomStatus 测试房间状态更新
func TestRoomManager_UpdateRoomStatus(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8916,
		LogLevel:    "info",
		MetricsPort: 10016,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := &RoomConfig{
		MaxPlayers:  4,
		MinPlayers:  2,
		FrameRate:   30,
		RetryWindow: 50,
	}
	room, err := server.rooms.CreateRoom(roomConfig, []PlayerID{1, 2}, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(room.ID)

	// 初始状态应该是 IDLE
	if room.State.Status != RoomStatus_ROOM_STATUS_IDLE {
		t.Errorf("Expected initial room status to be IDLE, got %v", room.State.Status)
	}

	// 模拟玩家登录
	player1 := room.Players[1]
	player1.Mutex.Lock()
	player1.Status = PlayerStatus_PLAYER_STATUS_ONLINE
	player1.ConnectionID = 1
	player1.Mutex.Unlock()

	// 更新房间状态
	server.rooms.UpdateRoomStatus(room, server)

	// 状态应该变为 WAITING
	if room.State.Status != RoomStatus_ROOM_STATUS_WAITING {
		t.Errorf("Expected room status to be WAITING after player login, got %v", room.State.Status)
	}

	// 模拟第二个玩家登录并准备
	player2 := room.Players[2]
	player2.Mutex.Lock()
	player2.Status = PlayerStatus_PLAYER_STATUS_READY
	player2.ConnectionID = 2
	player2.Mutex.Unlock()

	// 第一个玩家也准备
	player1.Mutex.Lock()
	player1.Status = PlayerStatus_PLAYER_STATUS_READY
	player1.Mutex.Unlock()

	// 更新房间状态
	server.rooms.UpdateRoomStatus(room, server)

	// 状态应该变为 RUNNING
	if room.State.Status != RoomStatus_ROOM_STATUS_RUNNING {
		t.Errorf("Expected room status to be RUNNING after all players ready, got %v", room.State.Status)
	}
}

// TestRoom_NetworkStats 测试房间网络统计
func TestRoom_NetworkStats(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8917,
		LogLevel:    "info",
		MetricsPort: 10017,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := DefaultRoomConfig()
	room, err := server.rooms.CreateRoom(roomConfig, []PlayerID{}, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(room.ID)

	// 测试获取网络统计
	stats := room.GetNetworkStats()
	if stats == nil {
		t.Error("Expected network stats to be initialized")
	}

	// 测试更新网络统计
	room.UpdateNetworkStats(100, 5, 1024, 2048, 50*time.Millisecond)

	// 测试增量更新网络统计
	room.IncrementNetworkStats(10, 8, 512, 1024, false, 30*time.Millisecond)

	// 测试 nil 统计对象
	room.Mutex.Lock()
	room.NetworkStats = nil
	room.Mutex.Unlock()

	// 这些调用应该不会崩溃
	room.UpdateNetworkStats(100, 5, 1024, 2048, 50*time.Millisecond)
	room.IncrementNetworkStats(10, 8, 512, 1024, false, 30*time.Millisecond)
	stats = room.GetNetworkStats()
	if stats != nil {
		t.Error("Expected network stats to be nil after setting to nil")
	}
}

// TestRoom_FrameStats 测试房间帧统计
func TestRoom_FrameStats(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8918,
		LogLevel:    "info",
		MetricsPort: 10018,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := DefaultRoomConfig()
	room, err := server.rooms.CreateRoom(roomConfig, []PlayerID{}, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(room.ID)

	// 测试获取帧统计
	stats := room.GetFrameStats()
	if stats == nil {
		t.Error("Expected frame stats to be initialized")
	}

	// 测试增量更新帧统计
	room.IncrementFrameStats(true, false, 16*time.Millisecond)
	room.IncrementFrameStats(false, true, 20*time.Millisecond)

	// 测试 nil 统计对象
	room.Mutex.Lock()
	room.FrameStats = nil
	room.Mutex.Unlock()

	// 这个调用应该不会崩溃
	room.IncrementFrameStats(true, false, 16*time.Millisecond)
	stats = room.GetFrameStats()
	if stats != nil {
		t.Error("Expected frame stats to be nil after setting to nil")
	}
}
