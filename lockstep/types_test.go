package lockstep

import (
	"testing"
	kcp2k "github.com/O-Keh-Hunter/kcp2k-go"
)

// TestDefaultRoomConfig 测试默认房间配置
func TestDefaultRoomConfig(t *testing.T) {
	config := DefaultRoomConfig()

	if config == nil {
		t.Fatal("DefaultRoomConfig should not return nil")
	}

	if config.MaxPlayers != 8 {
		t.Errorf("Expected MaxPlayers to be 8, got %d", config.MaxPlayers)
	}

	if config.MinPlayers != 2 {
		t.Errorf("Expected MinPlayers to be 2, got %d", config.MinPlayers)
	}

	if config.FrameRate != 15 {
		t.Errorf("Expected FrameRate to be 15, got %d", config.FrameRate)
	}

	if config.RetryWindow != 50 {
		t.Errorf("Expected RetryWindow to be 50, got %d", config.RetryWindow)
	}
}

// TestDefaultLockStepConfig 测试默认帧同步配置
func TestDefaultLockStepConfig(t *testing.T) {
	config := DefaultLockStepConfig()

	if config.RoomConfig == nil {
		t.Fatal("RoomConfig should not be nil")
	}

	if config.ServerPort != 8888 {
		t.Errorf("Expected ServerPort to be 8888, got %d", config.ServerPort)
	}

	if config.LogLevel != "info" {
		t.Errorf("Expected LogLevel to be 'info', got %s", config.LogLevel)
	}

	if config.MetricsPort != 9090 {
		t.Errorf("Expected MetricsPort to be 9090, got %d", config.MetricsPort)
	}

	// 验证 KCP 配置
	if config.KcpConfig.NoDelay != true {
		t.Errorf("Expected KcpConfig.NoDelay to be true, got %v", config.KcpConfig.NoDelay)
	}

	if config.KcpConfig.Interval != 10 {
		t.Errorf("Expected KcpConfig.Interval to be 10, got %d", config.KcpConfig.Interval)
	}

	if config.KcpConfig.FastResend != 2 {
		t.Errorf("Expected KcpConfig.FastResend to be 2, got %d", config.KcpConfig.FastResend)
	}

	if config.KcpConfig.CongestionWindow != false {
		t.Errorf("Expected KcpConfig.CongestionWindow to be false, got %v", config.KcpConfig.CongestionWindow)
	}

	if config.KcpConfig.DualMode != false {
		t.Errorf("Expected KcpConfig.DualMode to be false, got %v", config.KcpConfig.DualMode)
	}
}

// TestValidateConfig_ValidConfig 测试有效配置的验证
func TestValidateConfig_ValidConfig(t *testing.T) {
	config := DefaultLockStepConfig()

	err := config.ValidateConfig()
	if err != nil {
		t.Errorf("Expected no error for valid config, got %v", err)
	}
}

// TestValidateConfig_InvalidServerPort 测试无效服务器端口
func TestValidateConfig_InvalidServerPort(t *testing.T) {
	config := DefaultLockStepConfig()
	config.ServerPort = 0

	err := config.ValidateConfig()
	if err == nil {
		t.Error("Expected error for server port 0")
	}

	expected := "server port cannot be 0"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

// TestValidateConfig_SameServerAndMetricsPort 测试相同的服务器端口和监控端口
func TestValidateConfig_SameServerAndMetricsPort(t *testing.T) {
	config := DefaultLockStepConfig()
	config.ServerPort = 8080
	config.MetricsPort = 8080

	err := config.ValidateConfig()
	if err == nil {
		t.Error("Expected error for same server and metrics port")
	}

	expected := "server port and metrics port cannot be the same"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

// TestValidateConfig_NilRoomConfig 测试空房间配置
func TestValidateConfig_NilRoomConfig(t *testing.T) {
	config := DefaultLockStepConfig()
	config.RoomConfig = nil

	err := config.ValidateConfig()
	if err == nil {
		t.Error("Expected error for nil room config")
	}

	expected := "room config cannot be nil"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

// TestValidateConfig_ZeroMaxPlayers 测试最大玩家数为0
func TestValidateConfig_ZeroMaxPlayers(t *testing.T) {
	config := DefaultLockStepConfig()
	config.RoomConfig.MaxPlayers = 0

	err := config.ValidateConfig()
	if err == nil {
		t.Error("Expected error for max players 0")
	}

	expected := "max players cannot be 0"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

// TestValidateConfig_MinPlayersGreaterThanMax 测试最小玩家数大于最大玩家数
func TestValidateConfig_MinPlayersGreaterThanMax(t *testing.T) {
	config := DefaultLockStepConfig()
	config.RoomConfig.MinPlayers = 10
	config.RoomConfig.MaxPlayers = 5

	err := config.ValidateConfig()
	if err == nil {
		t.Error("Expected error for min players greater than max players")
	}

	expected := "min players cannot be greater than max players"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

// TestValidateConfig_ZeroFrameRate 测试帧率为0
func TestValidateConfig_ZeroFrameRate(t *testing.T) {
	config := DefaultLockStepConfig()
	config.RoomConfig.FrameRate = 0

	err := config.ValidateConfig()
	if err == nil {
		t.Error("Expected error for frame rate 0")
	}

	expected := "frame rate cannot be 0"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

// TestValidateConfig_HighFrameRate 测试过高的帧率
func TestValidateConfig_HighFrameRate(t *testing.T) {
	config := DefaultLockStepConfig()
	config.RoomConfig.FrameRate = 150

	err := config.ValidateConfig()
	if err == nil {
		t.Error("Expected error for frame rate > 120")
	}

	expected := "frame rate cannot exceed 120 FPS"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

// TestValidateConfig_InvalidLogLevel 测试无效的日志级别
func TestValidateConfig_InvalidLogLevel(t *testing.T) {
	config := DefaultLockStepConfig()
	config.LogLevel = "invalid"

	err := config.ValidateConfig()
	if err == nil {
		t.Error("Expected error for invalid log level")
	}

	expected := "invalid log level: invalid, must be one of: debug, info, warn, error, fatal"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

// TestValidateConfig_ValidLogLevels 测试所有有效的日志级别
func TestValidateConfig_ValidLogLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error", "fatal"}

	for _, level := range validLevels {
		t.Run(level, func(t *testing.T) {
			config := DefaultLockStepConfig()
			config.LogLevel = level

			err := config.ValidateConfig()
			if err != nil {
				t.Errorf("Expected no error for log level '%s', got %v", level, err)
			}
		})
	}
}

// TestValidateConfig_SameServerAndGrpcPort 测试相同的服务器端口和gRPC端口
func TestValidateConfig_SameServerAndGrpcPort(t *testing.T) {
	config := DefaultLockStepConfig()
	config.ServerPort = 8080
	config.GrpcPort = 8080

	err := config.ValidateConfig()
	if err == nil {
		t.Error("Expected error for same server and gRPC port")
	}

	expected := "server port and gRPC port cannot be the same"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

// TestValidateConfig_SameMetricsAndGrpcPort 测试相同的监控端口和gRPC端口
func TestValidateConfig_SameMetricsAndGrpcPort(t *testing.T) {
	config := DefaultLockStepConfig()
	config.MetricsPort = 9090
	config.GrpcPort = 9090

	err := config.ValidateConfig()
	if err == nil {
		t.Error("Expected error for same metrics and gRPC port")
	}

	expected := "metrics port and gRPC port cannot be the same"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

// TestValidateConfig_ZeroMinPlayers 测试最小玩家数为0
func TestValidateConfig_ZeroMinPlayers(t *testing.T) {
	config := DefaultLockStepConfig()
	config.RoomConfig.MinPlayers = 0

	err := config.ValidateConfig()
	if err == nil {
		t.Error("Expected error for min players 0")
	}

	expected := "min players cannot be 0"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

// TestValidateConfig_NegativeFrameRate 测试负数帧率
func TestValidateConfig_NegativeFrameRate(t *testing.T) {
	config := DefaultLockStepConfig()
	config.RoomConfig.FrameRate = -1

	err := config.ValidateConfig()
	if err == nil {
		t.Error("Expected error for negative frame rate")
	}

	expected := "frame rate cannot be 0"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

// TestRoomGetRoomStats 测试房间统计信息获取
func TestRoomGetRoomStats(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8900,
		LogLevel:    "info",
		MetricsPort: 10000,
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

	room, err := server.rooms.CreateRoom(roomConfig, []PlayerID{}, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(room.ID)

	// 获取房间统计信息
	stats := room.GetRoomStats()

	// 验证基本字段
	if stats["room_id"] != room.ID {
		t.Errorf("Expected room_id to be %s, got %v", room.ID, stats["room_id"])
	}

	if stats["port"] != room.Port {
		t.Errorf("Expected port to be %d, got %v", room.Port, stats["port"])
	}

	if stats["current_frame_id"] != room.CurrentFrameID {
		t.Errorf("Expected current_frame_id to be %d, got %v", room.CurrentFrameID, stats["current_frame_id"])
	}

	if stats["max_frame_id"] != room.MaxFrameID {
		t.Errorf("Expected max_frame_id to be %d, got %v", room.MaxFrameID, stats["max_frame_id"])
	}

	if stats["player_count"] != len(room.Players) {
		t.Errorf("Expected player_count to be %d, got %v", len(room.Players), stats["player_count"])
	}

	if stats["max_players"] != room.Config.MaxPlayers {
		t.Errorf("Expected max_players to be %d, got %v", room.Config.MaxPlayers, stats["max_players"])
	}

	// 验证统计信息字段存在
	if _, exists := stats["total_frames"]; !exists {
		t.Error("Expected total_frames to exist in stats")
	}

	if _, exists := stats["missed_frames"]; !exists {
		t.Error("Expected missed_frames to exist in stats")
	}

	if _, exists := stats["late_frames"]; !exists {
		t.Error("Expected late_frames to exist in stats")
	}

	if _, exists := stats["total_packets"]; !exists {
		t.Error("Expected total_packets to exist in stats")
	}

	if _, exists := stats["lost_packets"]; !exists {
		t.Error("Expected lost_packets to exist in stats")
	}
}

// TestRoomGetRoomStats_NilStats 测试房间统计信息为nil的情况
func TestRoomGetRoomStats_NilStats(t *testing.T) {
	// 创建测试配置
	config := &LockStepConfig{
		KcpConfig:   kcp2k.DefaultKcpConfig(),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8901,
		LogLevel:    "info",
		MetricsPort: 10001,
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

	room, err := server.rooms.CreateRoom(roomConfig, []PlayerID{}, server)
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	defer server.rooms.RemoveRoom(room.ID)

	// 将统计信息设置为nil
	room.Mutex.Lock()
	room.FrameStats = nil
	room.NetworkStats = nil
	room.State = nil
	room.Config = nil
	room.Mutex.Unlock()

	// 获取房间统计信息（应该不会崩溃）
	stats := room.GetRoomStats()

	// 验证基本字段仍然存在
	if stats["room_id"] != room.ID {
		t.Errorf("Expected room_id to be %s, got %v", room.ID, stats["room_id"])
	}

	if stats["port"] != room.Port {
		t.Errorf("Expected port to be %d, got %v", room.Port, stats["port"])
	}

	// 验证统计信息字段不存在（因为统计对象为nil）
	if _, exists := stats["total_frames"]; exists {
		t.Error("Expected total_frames to not exist when FrameStats is nil")
	}

	if _, exists := stats["total_packets"]; exists {
		t.Error("Expected total_packets to not exist when NetworkStats is nil")
	}

	if _, exists := stats["status"]; exists {
		t.Error("Expected status to not exist when State is nil")
	}

	if _, exists := stats["max_players"]; exists {
		t.Error("Expected max_players to not exist when Config is nil")
	}
}