package lockstep

import (
	"context"
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
		ServerPort:  8701,
		LogLevel:    "info",
		MetricsPort: 9701,
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

	expectedFrameFields := []string{"total_frames", "empty_frames", "late_frames", "avg_frame_time"}
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
		ServerPort:  8702,
		LogLevel:    "info",
		MetricsPort: 9702,
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
	room1, err := server.CreateRoom(roomConfig, []PlayerID{})
	if err != nil {
		t.Fatalf("Failed to create room1: %v", err)
	}

	room2, err := server.CreateRoom(roomConfig, []PlayerID{})
	if err != nil {
		t.Fatalf("Failed to create room2: %v", err)
	}

	// 模拟添加玩家到房间
	room1.Players[1] = &LockStepPlayer{
		Player: &Player{
			PlayerId: 1,
			Status:   PlayerStatus_PLAYER_STATUS_ONLINE,
		},
		ConnectionID: 1,
		InputBuffer:  make(map[FrameID][]*InputMessage),
	}

	room1.Players[2] = &LockStepPlayer{
		Player: &Player{
			PlayerId: 2,
			Status:   PlayerStatus_PLAYER_STATUS_ONLINE,
		},
		ConnectionID: 2,
		InputBuffer:  make(map[FrameID][]*InputMessage),
	}

	room2.Players[3] = &LockStepPlayer{
		Player: &Player{
			PlayerId: 3,
			Status:   PlayerStatus_PLAYER_STATUS_ONLINE,
		},
		ConnectionID: 3,
		InputBuffer:  make(map[FrameID][]*InputMessage),
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
		ServerPort:  8704,
		LogLevel:    "info",
		MetricsPort: 9704,
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

	room, err := server.CreateRoom(roomConfig, []PlayerID{})
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// 模拟帧统计数据
	room.FrameStats.mutex.Lock()
	room.FrameStats.totalFrames = 100
	room.FrameStats.emptyFrames = 5
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

	if frameStats["empty_frames"] != uint64(5) {
		t.Errorf("Expected empty_frames to be 5, got %v", frameStats["empty_frames"])
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
		ServerPort:  8705,
		LogLevel:    "info",
		MetricsPort: 9705,
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

	room, err := server.CreateRoom(roomConfig, []PlayerID{})
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
		ServerPort:  8706,
		LogLevel:    "info",
		MetricsPort: 9706,
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
	room1, err := server.CreateRoom(roomConfig, []PlayerID{})
	if err != nil {
		t.Fatalf("Failed to create room1: %v", err)
	}

	room2, err := server.CreateRoom(roomConfig, []PlayerID{})
	if err != nil {
		t.Fatalf("Failed to create room2: %v", err)
	}

	// 为房间1设置统计数据
	room1.FrameStats.mutex.Lock()
	room1.FrameStats.totalFrames = 100
	room1.FrameStats.emptyFrames = 5
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
	room2.FrameStats.emptyFrames = 10
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

	if frameStats["empty_frames"] != uint64(15) { // 5 + 10
		t.Errorf("Expected empty_frames to be 15, got %v", frameStats["empty_frames"])
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
		ServerPort:  8708,
		LogLevel:    "info",
		MetricsPort: 9708,
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
		ServerPort:  8709,
		LogLevel:    "info",
		MetricsPort: 9709,
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
	room, err := server.CreateRoom(roomConfig, []PlayerID{})
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

// TestLockStepServer_Start 测试服务器启动
func TestLockStepServer_Start(t *testing.T) {
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8710,
		LogLevel:    "info",
		MetricsPort: 9710,
	}

	server := NewLockStepServer(config)

	// 测试启动
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// 验证服务器状态
	server.mutex.RLock()
	running := server.running
	server.mutex.RUnlock()

	if !running {
		t.Error("Expected server to be running after Start()")
	}

	// 测试重复启动
	err = server.Start()
	if err == nil {
		t.Error("Expected error when starting already running server")
	}

	expected := "server is already running"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}

	// 清理
	server.Stop()
}

// TestLockStepServer_Stop 测试服务器停止
func TestLockStepServer_Stop(t *testing.T) {
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8711,
		LogLevel:    "info",
		MetricsPort: 9711,
	}

	server := NewLockStepServer(config)

	// 启动服务器
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// 验证服务器正在运行
	server.mutex.RLock()
	running := server.running
	server.mutex.RUnlock()

	if !running {
		t.Error("Expected server to be running before Stop()")
	}

	// 停止服务器
	server.Stop()

	// 验证服务器已停止
	server.mutex.RLock()
	running = server.running
	server.mutex.RUnlock()

	if running {
		t.Error("Expected server to be stopped after Stop()")
	}

	// 测试重复停止（应该不会出错）
	server.Stop()
}

// TestLockStepServer_GracefulShutdown 测试服务器优雅关闭
func TestLockStepServer_GracefulShutdown(t *testing.T) {
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8712,
		LogLevel:    "info",
		MetricsPort: 9712,
	}

	server := NewLockStepServer(config)

	// 启动服务器
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// 创建上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 优雅关闭
	err = server.GracefulShutdown(ctx)
	if err != nil {
		t.Errorf("GracefulShutdown failed: %v", err)
	}

	// 验证服务器已停止
	server.mutex.RLock()
	running := server.running
	server.mutex.RUnlock()

	if running {
		t.Error("Expected server to be stopped after GracefulShutdown()")
	}
}

// TestLockStepServer_GracefulShutdown_Timeout 测试优雅关闭超时
func TestLockStepServer_GracefulShutdown_Timeout(t *testing.T) {
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8713,
		LogLevel:    "info",
		MetricsPort: 9713,
	}

	server := NewLockStepServer(config)

	// 启动服务器
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// 创建一个很短的超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// 等待上下文超时
	time.Sleep(2 * time.Millisecond)

	// 优雅关闭（应该立即返回因为上下文已超时）
	err = server.GracefulShutdown(ctx)
	if err == nil {
		t.Error("Expected timeout error for GracefulShutdown with expired context")
	}

	// 清理
	server.Stop()
}

// TestLockStepServer_WaitForShutdown 测试等待关闭信号
func TestLockStepServer_WaitForShutdown(t *testing.T) {
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8714,
		LogLevel:    "info",
		MetricsPort: 9714,
	}

	server := NewLockStepServer(config)

	// 启动服务器
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// 在另一个 goroutine 中等待关闭信号
	done := make(chan bool)
	go func() {
		server.WaitForShutdown()
		done <- true
	}()

	// 等待一小段时间确保 WaitForShutdown 开始等待
	time.Sleep(100 * time.Millisecond)

	// 停止服务器
	server.Stop()

	// 验证 WaitForShutdown 返回
	select {
	case <-done:
		// 成功
	case <-time.After(2 * time.Second):
		t.Error("WaitForShutdown did not return after Stop()")
	}
}

// TestLockStepServer_HealthCheck 测试健康检查
func TestLockStepServer_HealthCheck(t *testing.T) {
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8715,
		LogLevel:    "info",
		MetricsPort: 9715,
	}

	server := NewLockStepServer(config)

	// 测试未启动的服务器
	health := server.HealthCheck()
	if health["status"] != "unhealthy" {
		t.Errorf("Expected status to be 'unhealthy' for stopped server, got %v", health["status"])
	}

	// 启动服务器
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// 测试运行中的服务器
	health = server.HealthCheck()
	if health["status"] != "healthy" {
		t.Errorf("Expected status to be 'healthy' for running server, got %v", health["status"])
	}

	// 验证其他健康检查字段
	if _, exists := health["uptime"]; !exists {
		t.Error("Expected uptime field in health check")
	}

	if _, exists := health["memory_usage"]; !exists {
		t.Error("Expected memory_usage field in health check")
	}

	if _, exists := health["goroutines"]; !exists {
		t.Error("Expected goroutines field in health check")
	}

	if _, exists := health["total_rooms"]; !exists {
		t.Error("Expected total_rooms field in health check")
	}

	if _, exists := health["total_players"]; !exists {
		t.Error("Expected total_players field in health check")
	}
}

// TestLockStepServer_StartGrpcServer 测试启动gRPC服务器
func TestLockStepServer_StartGrpcServer(t *testing.T) {
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8716,
		LogLevel:    "info",
		MetricsPort: 9716,
		GrpcPort:    17016,
	}

	server := NewLockStepServer(config)

	// 启动服务器
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// 验证gRPC服务器已启动
	if server.grpcServer == nil {
		t.Error("Expected gRPC server to be started")
	}

	// 测试重复启动gRPC服务器
	err = server.StartGrpcServer()
	if err == nil {
		t.Error("Expected error when starting gRPC server that is already running")
	}

	expected := "gRPC server already started"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

// TestLockStepServer_StartGrpcServer_NoPort 测试未配置gRPC端口时不启动gRPC服务器
func TestLockStepServer_StartGrpcServer_NoPort(t *testing.T) {
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8717,
		LogLevel:    "info",
		MetricsPort: 9717,
		GrpcPort:    0, // 未配置gRPC端口
	}

	server := NewLockStepServer(config)

	// 启动服务器
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// 验证gRPC服务器未启动
	if server.grpcServer != nil {
		t.Error("Expected gRPC server to not be started when GrpcPort is 0")
	}
}
