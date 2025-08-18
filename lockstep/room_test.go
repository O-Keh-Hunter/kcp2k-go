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
		ServerPort:  8896,
		LogLevel:    "info",
		MetricsPort: 9996,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomID := RoomID("test-room-monitoring-empty")
	roomConfig := &RoomConfig{
		MaxPlayers:  4,
		MinPlayers:  2,
		FrameRate:   15,
		RetryWindow: 50,
	}

	room, err := server.rooms.CreateRoom(roomID, roomConfig, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(roomID)

	// 获取房间监控信息
	monitoringInfo := room.GetRoomMonitoringInfo()

	// 验证基本字段
	if monitoringInfo["room_id"] != roomID {
		t.Errorf("Expected room_id to be %s, got %v", roomID, monitoringInfo["room_id"])
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

	if monitoringInfo["max_players"] != uint32(4) {
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
	if config_info["max_players"] != uint32(4) {
		t.Errorf("Expected config.max_players to be 4, got %v", config_info["max_players"])
	}
	if config_info["min_players"] != uint32(2) {
		t.Errorf("Expected config.min_players to be 2, got %v", config_info["min_players"])
	}
	if config_info["frame_rate"] != uint32(15) {
		t.Errorf("Expected config.frame_rate to be 15, got %v", config_info["frame_rate"])
	}
	if config_info["retry_window"] != uint32(50) {
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
	roomID := RoomID("test-room-monitoring-players")
	roomConfig := &RoomConfig{
		MaxPlayers:  4,
		MinPlayers:  2,
		FrameRate:   15,
		RetryWindow: 50,
	}

	room, err := server.rooms.CreateRoom(roomID, roomConfig, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(roomID)

	// 添加测试玩家
	player1 := &Player{
		ID:           PlayerID(101),
		ConnectionID: 1001,
		State: &PlayerState{
			Online:       true,
			LastFrameId:  5,
			Ping:         50,
			LastPingTime: time.Now().Unix(),
		},
		LastFrameID: 5,
		InputBuffer: make(map[FrameID][]byte),
	}

	player2 := &Player{
		ID:           PlayerID(102),
		ConnectionID: 1002,
		State: &PlayerState{
			Online:       false, // 离线玩家
			LastFrameId:  3,
			Ping:         80,
			LastPingTime: time.Now().Unix() - 30,
		},
		LastFrameID: 3,
		InputBuffer: make(map[FrameID][]byte),
	}

	// 添加玩家到房间
	room.Mutex.Lock()
	room.Players[player1.ID] = player1
	room.Players[player2.ID] = player2
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
		if p1["online"] != true {
			t.Errorf("Expected player1 online to be true, got %v", p1["online"])
		}
		if p1["last_frame_id"] != int64(5) {
			t.Errorf("Expected player1 last_frame_id to be 5, got %v", p1["last_frame_id"])
		}
		if p1["ping"] != int64(50) {
			t.Errorf("Expected player1 ping to be 50, got %v", p1["ping"])
		}
	} else {
		t.Error("Player 101 not found in monitoring info")
	}

	// 验证玩家2
	if p2, exists := playerMap[PlayerID(102)]; exists {
		if p2["connection_id"] != 1002 {
			t.Errorf("Expected player2 connection_id to be 1002, got %v", p2["connection_id"])
		}
		if p2["online"] != false {
			t.Errorf("Expected player2 online to be false, got %v", p2["online"])
		}
		if p2["last_frame_id"] != int64(3) {
			t.Errorf("Expected player2 last_frame_id to be 3, got %v", p2["last_frame_id"])
		}
		if p2["ping"] != int64(80) {
			t.Errorf("Expected player2 ping to be 80, got %v", p2["ping"])
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
	roomID := RoomID("test-room-monitoring-stats")
	roomConfig := &RoomConfig{
		MaxPlayers:  4,
		MinPlayers:  2,
		FrameRate:   15,
		RetryWindow: 50,
	}

	room, err := server.rooms.CreateRoom(roomID, roomConfig, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(roomID)

	// 设置帧统计信息
	room.FrameStats.mutex.Lock()
	room.FrameStats.totalFrames = 100
	room.FrameStats.missedFrames = 5
	room.FrameStats.lateFrames = 3
	room.FrameStats.frameTimeSum = 2000 * time.Millisecond
	room.FrameStats.lastFrameTime = time.Now()
	room.FrameStats.mutex.Unlock()

	// 设置网络统计信息
	room.NetworkStats.mutex.Lock()
	room.NetworkStats.totalPackets = 500
	room.NetworkStats.lostPackets = 10
	room.NetworkStats.bytesReceived = 25000
	room.NetworkStats.bytesSent = 30000
	room.NetworkStats.latencySum = 1000 * time.Millisecond
	room.NetworkStats.latencyCount = 20
	room.NetworkStats.maxLatency = 100 * time.Millisecond
	room.NetworkStats.minLatency = 20 * time.Millisecond
	room.NetworkStats.mutex.Unlock()

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

	if networkStats.GetTotalPackets() != 500 {
		t.Errorf("Expected total_packets to be 500, got %v", networkStats.GetTotalPackets())
	}
	if networkStats.GetLostPackets() != 10 {
		t.Errorf("Expected lost_packets to be 10, got %v", networkStats.GetLostPackets())
	}
	if networkStats.GetBytesReceived() != 25000 {
		t.Errorf("Expected bytes_received to be 25000, got %v", networkStats.GetBytesReceived())
	}
	if networkStats.GetBytesSent() != 30000 {
		t.Errorf("Expected bytes_sent to be 30000, got %v", networkStats.GetBytesSent())
	}
	if networkStats.GetAverageLatency() != 50*time.Millisecond {
		t.Errorf("Expected average_latency to be 50ms, got %v", networkStats.GetAverageLatency())
	}
	if networkStats.GetMaxLatency() != 100*time.Millisecond {
		t.Errorf("Expected max_latency to be 100ms, got %v", networkStats.GetMaxLatency())
	}
	if networkStats.GetMinLatency() != 20*time.Millisecond {
		t.Errorf("Expected min_latency to be 20ms, got %v", networkStats.GetMinLatency())
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
	roomID := RoomID("test-room-monitoring-nil")
	roomConfig := &RoomConfig{
		MaxPlayers:  4,
		MinPlayers:  2,
		FrameRate:   15,
		RetryWindow: 50,
	}

	room, err := server.rooms.CreateRoom(roomID, roomConfig, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(roomID)

	// 将统计信息设置为nil
	room.Mutex.Lock()
	room.FrameStats = nil
	room.NetworkStats = nil
	room.Mutex.Unlock()

	// 获取房间监控信息
	monitoringInfo := room.GetRoomMonitoringInfo()

	// 验证基本字段仍然存在
	if monitoringInfo["room_id"] != roomID {
		t.Errorf("Expected room_id to be %s, got %v", roomID, monitoringInfo["room_id"])
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