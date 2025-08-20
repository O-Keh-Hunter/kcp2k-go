package lockstep

import (
	"testing"
	"time"

	kcp2k "github.com/O-Keh-Hunter/kcp2k-go"
)

// TestGetServerStats_EmptyServer 测试空服务器的统计信息
func TestGetServerStats_EmptyServer(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8888,
		LogLevel:    "info",
		MetricsPort: 9999,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 等待一小段时间确保服务器完全启动
	time.Sleep(10 * time.Millisecond)

	// 获取统计信息
	stats := server.GetServerStats()

	// 验证基本字段
	if stats["total_rooms"] != 0 {
		t.Errorf("Expected total_rooms to be 0, got %v", stats["total_rooms"])
	}

	if stats["total_players"] != 0 {
		t.Errorf("Expected total_players to be 0, got %v", stats["total_players"])
	}

	if stats["running"] != true {
		t.Errorf("Expected running to be true, got %v", stats["running"])
	}

	// 验证uptime是非负数（可能为0如果时间太短）
	uptime, ok := stats["uptime"].(int64)
	if !ok {
		t.Errorf("Expected uptime to be int64, got %T", stats["uptime"])
	}
	if uptime < 0 {
		t.Errorf("Expected uptime to be non-negative, got %d", uptime)
	}

	// 验证帧统计信息
	frameStats, ok := stats["frame_stats"].(map[string]interface{})
	if !ok {
		t.Errorf("Expected frame_stats to be map[string]interface{}, got %T", stats["frame_stats"])
	}

	expectedFrameFields := []string{"total_frames", "missed_frames", "late_frames", "avg_frame_time"}
	for _, field := range expectedFrameFields {
		if _, exists := frameStats[field]; !exists {
			t.Errorf("Expected frame_stats to contain field %s", field)
		}
		if frameStats[field] != uint64(0) && frameStats[field] != float64(0) {
			t.Errorf("Expected frame_stats.%s to be 0, got %v", field, frameStats[field])
		}
	}

	// 验证网络统计信息
	networkStats, ok := stats["network_stats"].(map[string]interface{})
	if !ok {
		t.Errorf("Expected network_stats to be map[string]interface{}, got %T", stats["network_stats"])
	}

	expectedNetworkFields := []string{"total_packets", "lost_packets", "bytes_received", "bytes_sent", "avg_rtt", "max_rtt", "min_rtt"}
	for _, field := range expectedNetworkFields {
		if _, exists := networkStats[field]; !exists {
			t.Errorf("Expected network_stats to contain field %s", field)
		}
		if networkStats[field] != uint64(0) && networkStats[field] != float64(0) && networkStats[field] != int64(0) {
			t.Errorf("Expected network_stats.%s to be 0, got %v", field, networkStats[field])
		}
	}
}

// TestGetServerStats_WithRooms 测试有房间的服务器统计信息
func TestGetServerStats_WithRooms(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8890,
		LogLevel:    "info",
		MetricsPort: 9990,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建测试房间配置
	roomConfig := &RoomConfig{
		MaxPlayers:  4,
		MinPlayers:  2,
		FrameRate:   30,
		RetryWindow: 50,
	}

	// 创建多个房间
	room1, err := server.CreateRoom("room1", roomConfig)
	if err != nil {
		t.Fatalf("Failed to create room1: %v", err)
	}

	room2, err := server.CreateRoom("room2", roomConfig)
	if err != nil {
		t.Fatalf("Failed to create room2: %v", err)
	}

	// 模拟添加玩家到房间
	room1.Players[1] = &Player{
		ID:           1,
		ConnectionID: 1,
		State: &PlayerState{
			Online: true,
		},
		InputBuffer: make(map[FrameID][]*InputMessage),
	}

	room1.Players[2] = &Player{
		ID:           2,
		ConnectionID: 2,
		State: &PlayerState{
			Online: true,
		},
		InputBuffer: make(map[FrameID][]*InputMessage),
	}

	room2.Players[3] = &Player{
		ID:           3,
		ConnectionID: 3,
		State: &PlayerState{
			Online: true,
		},
		InputBuffer: make(map[FrameID][]*InputMessage),
	}

	// 获取统计信息
	stats := server.GetServerStats()

	// 验证房间和玩家数量
	if stats["total_rooms"] != 2 {
		t.Errorf("Expected total_rooms to be 2, got %v", stats["total_rooms"])
	}

	if stats["total_players"] != 3 {
		t.Errorf("Expected total_players to be 3, got %v", stats["total_players"])
	}

	if stats["running"] != true {
		t.Errorf("Expected running to be true, got %v", stats["running"])
	}
}

// TestGetServerStats_WithFrameStats 测试带有帧统计信息的服务器
func TestGetServerStats_WithFrameStats(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8891,
		LogLevel:    "info",
		MetricsPort: 9991,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := &RoomConfig{
		MaxPlayers:  2,
		MinPlayers:  1,
		FrameRate:   30,
		RetryWindow: 50,
	}

	room, err := server.CreateRoom("test_room", roomConfig)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// 模拟帧统计数据
	room.FrameStats.mutex.Lock()
	room.FrameStats.totalFrames = 100
	room.FrameStats.missedFrames = 5
	room.FrameStats.lateFrames = 3
	room.FrameStats.frameTimeSum = 100 * time.Millisecond
	room.FrameStats.mutex.Unlock()

	// 获取统计信息
	stats := server.GetServerStats()

	// 验证帧统计信息
	frameStats, ok := stats["frame_stats"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected frame_stats to be map[string]interface{}, got %T", stats["frame_stats"])
	}

	if frameStats["total_frames"] != uint64(100) {
		t.Errorf("Expected total_frames to be 100, got %v", frameStats["total_frames"])
	}

	if frameStats["missed_frames"] != uint64(5) {
		t.Errorf("Expected missed_frames to be 5, got %v", frameStats["missed_frames"])
	}

	if frameStats["late_frames"] != uint64(3) {
		t.Errorf("Expected late_frames to be 3, got %v", frameStats["late_frames"])
	}

	// 验证平均帧时间计算
	avgFrameTime, ok := frameStats["avg_frame_time"].(float64)
	if !ok {
		t.Errorf("Expected avg_frame_time to be float64, got %T", frameStats["avg_frame_time"])
	}
	expectedAvgFrameTime := float64(100*time.Millisecond.Nanoseconds()) / float64(100) / 1e6 // 转换为毫秒
	if avgFrameTime != expectedAvgFrameTime {
		t.Errorf("Expected avg_frame_time to be %f, got %f", expectedAvgFrameTime, avgFrameTime)
	}
}

// TestGetServerStats_WithNetworkStats 测试带有网络统计信息的服务器
func TestGetServerStats_WithNetworkStats(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8892,
		LogLevel:    "info",
		MetricsPort: 9992,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间
	roomConfig := &RoomConfig{
		MaxPlayers:  2,
		MinPlayers:  1,
		FrameRate:   30,
		RetryWindow: 50,
	}

	room, err := server.CreateRoom("test_room", roomConfig)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// 模拟网络统计数据
	room.NetworkStats.mutex.Lock()
	room.NetworkStats.totalPackets = 1000
	room.NetworkStats.lostPackets = 10
	room.NetworkStats.bytesReceived = 50000
	room.NetworkStats.bytesSent = 60000
	room.NetworkStats.rttSum = 500 * time.Millisecond
	room.NetworkStats.rttCount = 10
	room.NetworkStats.maxRTT = 100 * time.Millisecond
	room.NetworkStats.minRTT = 20 * time.Millisecond
	room.NetworkStats.mutex.Unlock()

	// 获取统计信息
	stats := server.GetServerStats()

	// 验证网络统计信息
	networkStats, ok := stats["network_stats"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected network_stats to be map[string]interface{}, got %T", stats["network_stats"])
	}

	if networkStats["total_packets"] != uint64(1000) {
		t.Errorf("Expected total_packets to be 1000, got %v", networkStats["total_packets"])
	}

	if networkStats["lost_packets"] != uint64(10) {
		t.Errorf("Expected lost_packets to be 10, got %v", networkStats["lost_packets"])
	}

	if networkStats["bytes_received"] != uint64(50000) {
		t.Errorf("Expected bytes_received to be 50000, got %v", networkStats["bytes_received"])
	}

	if networkStats["bytes_sent"] != uint64(60000) {
		t.Errorf("Expected bytes_sent to be 60000, got %v", networkStats["bytes_sent"])
	}

	// 验证平均延迟计算
	avgLatency, ok := networkStats["avg_rtt"].(float64)
	if !ok {
		t.Errorf("Expected avg_rtt to be float64, got %T", networkStats["avg_rtt"])
	}
	expectedAvgLatency := float64(500*time.Millisecond.Nanoseconds()) / float64(10) / 1e6 // 转换为毫秒
	if avgLatency != expectedAvgLatency {
		t.Errorf("Expected avg_rtt to be %f, got %f", expectedAvgLatency, avgLatency)
	}

	if networkStats["max_rtt"] != int64(100) {
		t.Errorf("Expected max_rtt to be 100, got %v", networkStats["max_rtt"])
	}

	if networkStats["min_rtt"] != int64(20) {
		t.Errorf("Expected min_rtt to be 20, got %v", networkStats["min_rtt"])
	}
}

// TestGetServerStats_MultipleRoomsAggregation 测试多个房间的统计信息聚合
func TestGetServerStats_MultipleRoomsAggregation(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8893,
		LogLevel:    "info",
		MetricsPort: 9993,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间配置
	roomConfig := &RoomConfig{
		MaxPlayers:  2,
		MinPlayers:  1,
		FrameRate:   30,
		RetryWindow: 50,
	}

	// 创建两个房间
	room1, err := server.CreateRoom("room1", roomConfig)
	if err != nil {
		t.Fatalf("Failed to create room1: %v", err)
	}

	room2, err := server.CreateRoom("room2", roomConfig)
	if err != nil {
		t.Fatalf("Failed to create room2: %v", err)
	}

	// 为房间1设置统计数据
	room1.FrameStats.mutex.Lock()
	room1.FrameStats.totalFrames = 100
	room1.FrameStats.missedFrames = 5
	room1.FrameStats.lateFrames = 2
	room1.FrameStats.mutex.Unlock()

	room1.NetworkStats.mutex.Lock()
	room1.NetworkStats.totalPackets = 500
	room1.NetworkStats.lostPackets = 5
	room1.NetworkStats.bytesReceived = 25000
	room1.NetworkStats.bytesSent = 30000
	room1.NetworkStats.rttSum = 250 * time.Millisecond
	room1.NetworkStats.rttCount = 5
	room1.NetworkStats.maxRTT = 80 * time.Millisecond
	room1.NetworkStats.minRTT = 30 * time.Millisecond
	room1.NetworkStats.mutex.Unlock()

	// 为房间2设置统计数据
	room2.FrameStats.mutex.Lock()
	room2.FrameStats.totalFrames = 200
	room2.FrameStats.missedFrames = 10
	room2.FrameStats.lateFrames = 3
	room2.FrameStats.mutex.Unlock()

	room2.NetworkStats.mutex.Lock()
	room2.NetworkStats.totalPackets = 800
	room2.NetworkStats.lostPackets = 8
	room2.NetworkStats.bytesReceived = 40000
	room2.NetworkStats.bytesSent = 45000
	room2.NetworkStats.rttSum = 400 * time.Millisecond
	room2.NetworkStats.rttCount = 8
	room2.NetworkStats.maxRTT = 120 * time.Millisecond // 更大的最大延迟
	room2.NetworkStats.minRTT = 15 * time.Millisecond  // 更小的最小延迟
	room2.NetworkStats.mutex.Unlock()

	// 获取统计信息
	stats := server.GetServerStats()

	// 验证聚合的帧统计信息
	frameStats, ok := stats["frame_stats"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected frame_stats to be map[string]interface{}, got %T", stats["frame_stats"])
	}

	if frameStats["total_frames"] != uint64(300) { // 100 + 200
		t.Errorf("Expected total_frames to be 300, got %v", frameStats["total_frames"])
	}

	if frameStats["missed_frames"] != uint64(15) { // 5 + 10
		t.Errorf("Expected missed_frames to be 15, got %v", frameStats["missed_frames"])
	}

	if frameStats["late_frames"] != uint64(5) { // 2 + 3
		t.Errorf("Expected late_frames to be 5, got %v", frameStats["late_frames"])
	}

	// 验证聚合的网络统计信息
	networkStats, ok := stats["network_stats"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected network_stats to be map[string]interface{}, got %T", stats["network_stats"])
	}

	if networkStats["total_packets"] != uint64(1300) { // 500 + 800
		t.Errorf("Expected total_packets to be 1300, got %v", networkStats["total_packets"])
	}

	if networkStats["lost_packets"] != uint64(13) { // 5 + 8
		t.Errorf("Expected lost_packets to be 13, got %v", networkStats["lost_packets"])
	}

	if networkStats["bytes_received"] != uint64(65000) { // 25000 + 40000
		t.Errorf("Expected bytes_received to be 65000, got %v", networkStats["bytes_received"])
	}

	if networkStats["bytes_sent"] != uint64(75000) { // 30000 + 45000
		t.Errorf("Expected bytes_sent to be 75000, got %v", networkStats["bytes_sent"])
	}

	// 验证最大延迟取最大值
	if networkStats["max_rtt"] != int64(120) { // max(80, 120)
		t.Errorf("Expected max_rtt to be 120, got %v", networkStats["max_rtt"])
	}

	// 验证最小延迟取最小值
	if networkStats["min_rtt"] != int64(15) { // min(30, 15)
		t.Errorf("Expected min_rtt to be 15, got %v", networkStats["min_rtt"])
	}

	// 验证平均延迟计算
	avgLatency, ok := networkStats["avg_rtt"].(float64)
	if !ok {
		t.Errorf("Expected avg_rtt to be float64, got %T", networkStats["avg_rtt"])
	}
	// 总延迟: 250ms + 400ms = 650ms, 总计数: 5 + 8 = 13
	expectedAvgLatency := float64(650*time.Millisecond.Nanoseconds()) / float64(13) / 1e6
	if avgLatency != expectedAvgLatency {
		t.Errorf("Expected avg_rtt to be %f, got %f", expectedAvgLatency, avgLatency)
	}
}

// TestGetServerStats_StoppedServer 测试停止的服务器
func TestGetServerStats_StoppedServer(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8894,
		LogLevel:    "info",
		MetricsPort: 9994,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()

	// 停止服务器
	server.Stop()

	// 获取统计信息
	stats := server.GetServerStats()

	// 验证running状态
	if stats["running"] != false {
		t.Errorf("Expected running to be false, got %v", stats["running"])
	}

	// 其他字段应该仍然可以正常获取
	if stats["total_rooms"] != 0 {
		t.Errorf("Expected total_rooms to be 0, got %v", stats["total_rooms"])
	}
}

// TestGetServerStats_NilStats 测试房间统计信息为nil的情况
func TestGetServerStats_NilStats(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8895,
		LogLevel:    "info",
		MetricsPort: 9995,
	}

	// 创建服务器
	server := NewLockStepServer(config)
	server.Start()
	defer server.Stop()

	// 创建房间配置
	roomConfig := &RoomConfig{
		MaxPlayers:  2,
		MinPlayers:  1,
		FrameRate:   30,
		RetryWindow: 50,
	}

	// 创建房间
	room, err := server.CreateRoom("test_room", roomConfig)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// 将统计信息设置为nil
	room.FrameStats = nil
	room.NetworkStats = nil

	// 获取统计信息（应该不会崩溃）
	stats := server.GetServerStats()

	// 验证基本字段
	if stats["total_rooms"] != 1 {
		t.Errorf("Expected total_rooms to be 1, got %v", stats["total_rooms"])
	}

	// 验证统计信息都是零值
	frameStats, ok := stats["frame_stats"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected frame_stats to be map[string]interface{}, got %T", stats["frame_stats"])
	}

	for field, value := range frameStats {
		if value != uint64(0) && value != float64(0) {
			t.Errorf("Expected frame_stats.%s to be 0, got %v", field, value)
		}
	}

	networkStats, ok := stats["network_stats"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected network_stats to be map[string]interface{}, got %T", stats["network_stats"])
	}

	for field, value := range networkStats {
		if value != uint64(0) && value != float64(0) && value != int64(0) {
			t.Errorf("Expected network_stats.%s to be 0, got %v", field, value)
		}
	}
}
