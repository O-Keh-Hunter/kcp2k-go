package lockstep

import (
	"fmt"
	"sync"
	"time"

	kcp2k "github.com/O-Keh-Hunter/kcp2k-go"
)

// 类型别名，保持向后兼容
type FrameID = int32
type PlayerID = int32
type RoomID = string

// RoomConfig 房间配置
type RoomConfig struct {
	MaxPlayers  int32 `json:"max_players"`  // 最大玩家数
	MinPlayers  int32 `json:"min_players"`  // 最小玩家数
	FrameRate   int32 `json:"frame_rate"`   // 帧率
	RetryWindow int32 `json:"retry_window"` // 重试窗口
}

// LockStepPlayer 扩展的玩家信息（包含连接信息）
type LockStepPlayer struct {
	*Player                                         // 嵌入protobuf生成的Player
	ConnectionID        int                         `json:"connection_id"`          // KCP连接ID
	InputBuffer         map[FrameID][]*InputMessage `json:"-"`                      // 输入缓冲区
	LastInputSequenceId int32                       `json:"last_input_sequence_id"` // 最后一次输入的序列ID
	Mutex               sync.RWMutex                `json:"-"`                      // 读写锁
}

// Room 房间信息
type Room struct {
	ID             RoomID                       `json:"id"`                  // 房间ID
	Port           uint16                       `json:"port"`                // 房间专用端口
	KcpServer      *kcp2k.KcpServer             `json:"-"`                   // 房间专用KCP服务器
	Players        map[PlayerID]*LockStepPlayer `json:"players"`             // 玩家列表
	Frames         map[FrameID]*FrameMessage    `json:"-"`                   // 帧数据缓存
	CurrentFrameID FrameID                      `json:"current_frame_id"`    // 当前帧ID
	MaxFrameID     FrameID                      `json:"max_frame_id"`        // 最大帧ID
	State          *RoomStateMessage            `json:"room_status_message"` // 房间状态（使用 protobuf 结构体）
	Config         *RoomConfig                  `json:"config"`              // 房间配置
	Mutex          sync.RWMutex                 `json:"-"`                   // 读写锁
	Ticker         *time.Ticker                 `json:"-"`                   // 帧定时器
	StopChan       chan struct{}                `json:"-"`                   // 停止信号通道
	CreatedAt      time.Time                    `json:"created_at"`          // 创建时间
	running        bool                         `json:"-"`                   // 运行状态

	// 房间级别的统计信息
	FrameStats   *FrameStats   `json:"-"` // 帧统计信息
	NetworkStats *NetworkStats `json:"-"` // 网络统计信息
}

// GetRoomStats 获取房间统计信息摘要
func (r *Room) GetRoomStats() map[string]interface{} {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()

	stats := make(map[string]interface{})
	stats["room_id"] = r.ID
	stats["port"] = r.Port
	stats["current_frame_id"] = r.CurrentFrameID
	stats["max_frame_id"] = r.MaxFrameID
	stats["player_count"] = len(r.Players)
	stats["created_at"] = r.CreatedAt
	stats["running"] = r.running

	if r.State != nil {
		stats["status"] = r.State.Status
		stats["current_players"] = len(r.Players)
		stats["start_time"] = r.State.StartTime
	}

	if r.Config != nil {
		stats["max_players"] = r.Config.MaxPlayers
	}

	if r.FrameStats != nil {
		stats["total_frames"] = r.FrameStats.GetTotalFrames()
		stats["missed_frames"] = r.FrameStats.GetMissedFrames()
		stats["late_frames"] = r.FrameStats.GetLateFrames()
		stats["average_frame_time"] = r.FrameStats.GetAverageFrameTime().Milliseconds()
	}

	if r.NetworkStats != nil {
		stats["total_packets"] = r.NetworkStats.GetTotalPackets()
		stats["lost_packets"] = r.NetworkStats.GetLostPackets()
		stats["bytes_received"] = r.NetworkStats.GetBytesReceived()
		stats["bytes_sent"] = r.NetworkStats.GetBytesSent()
		stats["average_rtt"] = r.NetworkStats.GetAverageRTT().Milliseconds()
		stats["max_rtt"] = r.NetworkStats.GetMaxRTT().Milliseconds()
		stats["min_rtt"] = r.NetworkStats.GetMinRTT().Milliseconds()
	}

	return stats
}

// DefaultRoomConfig 默认房间配置
func DefaultRoomConfig() *RoomConfig {
	return &RoomConfig{
		MaxPlayers:  8,
		MinPlayers:  2,  // 最少需要2个玩家才能开始游戏
		FrameRate:   15, // 15帧/秒，下行66ms间隔
		RetryWindow: 50,
	}
}

// LockStepConfig 帧同步服务配置
type LockStepConfig struct {
	KcpConfig   kcp2k.KcpConfig `json:"kcp_config"`   // KCP配置
	RoomConfig  *RoomConfig     `json:"room_config"`  // 房间配置
	ServerPort  uint16          `json:"server_port"`  // 服务器端口
	LogLevel    string          `json:"log_level"`    // 日志级别
	MetricsPort uint16          `json:"metrics_port"` // 监控端口
	GrpcPort    uint16          `json:"grpc_port"`    // gRPC端口
}

// DefaultLockStepConfig 默认帧同步配置
func DefaultLockStepConfig() LockStepConfig {
	return LockStepConfig{
		KcpConfig: kcp2k.NewKcpConfig(
			kcp2k.WithNoDelay(true),
			kcp2k.WithInterval(10),
			kcp2k.WithFastResend(2),
			kcp2k.WithCongestionWindow(false),
			kcp2k.WithDualMode(false)),
		RoomConfig:  DefaultRoomConfig(),
		ServerPort:  8888,
		LogLevel:    "info",
		MetricsPort: 9090,
	}
}

// ValidateConfig 验证配置的有效性
func (c *LockStepConfig) ValidateConfig() error {
	if c.ServerPort == 0 {
		return fmt.Errorf("server port cannot be 0")
	}

	if c.ServerPort == c.MetricsPort {
		return fmt.Errorf("server port and metrics port cannot be the same")
	}

	if c.ServerPort == c.GrpcPort {
		return fmt.Errorf("server port and gRPC port cannot be the same")
	}

	if c.MetricsPort == c.GrpcPort {
		return fmt.Errorf("metrics port and gRPC port cannot be the same")
	}

	if c.RoomConfig == nil {
		return fmt.Errorf("room config cannot be nil")
	}

	if c.RoomConfig.MaxPlayers == 0 {
		return fmt.Errorf("max players cannot be 0")
	}

	if c.RoomConfig.MinPlayers == 0 {
		return fmt.Errorf("min players cannot be 0")
	}

	if c.RoomConfig.MinPlayers > c.RoomConfig.MaxPlayers {
		return fmt.Errorf("min players cannot be greater than max players")
	}

	if c.RoomConfig.FrameRate <= 0 {
		return fmt.Errorf("frame rate cannot be 0")
	}

	if c.RoomConfig.FrameRate > 120 {
		return fmt.Errorf("frame rate cannot exceed 120 FPS")
	}

	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
		"fatal": true,
	}

	if !validLogLevels[c.LogLevel] {
		return fmt.Errorf("invalid log level: %s, must be one of: debug, info, warn, error, fatal", c.LogLevel)
	}

	return nil
}
