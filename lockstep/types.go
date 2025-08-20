package lockstep

import (
	"fmt"
	"sync"
	"time"

	kcp2k "github.com/O-Keh-Hunter/kcp2k-go"
	"google.golang.org/protobuf/proto"
)

// 类型别名，保持向后兼容
type FrameID = uint32
type PlayerID = uint32
type RoomID = string

// RoomStatus 房间状态枚举
type RoomStatus uint32

const (
	RoomStatusWaiting RoomStatus = 0 // 等待玩家
	RoomStatusReady   RoomStatus = 1 // 准备开始
	RoomStatusRunning RoomStatus = 2 // 游戏进行中
	RoomStatusEnded   RoomStatus = 3 // 游戏结束
)

// 错误码定义
const (
	ErrorCodeUnknown             = 1000 // 未知错误
	ErrorCodeInvalidMessage      = 1001 // 无效消息
	ErrorCodeRoomNotFound        = 1002 // 房间不存在
	ErrorCodeRoomFull            = 1003 // 房间已满
	ErrorCodePlayerNotFound      = 1004 // 玩家不存在
	ErrorCodePlayerAlreadyInRoom = 1005 // 玩家已在房间中
	ErrorCodeGameNotRunning      = 1006 // 游戏未运行
	ErrorCodeInvalidInput        = 1007 // 无效输入
	ErrorCodeConnectionLost      = 1008 // 连接丢失
	ErrorCodeTimeout             = 1009 // 超时
	ErrorCodeServerOverload      = 1010 // 服务器过载
	ErrorCodePermissionDenied    = 1011 // 权限不足
	ErrorCodeInvalidFrame        = 1012 // 无效帧
	ErrorCodeSyncFailed          = 1013 // 同步失败
)

// 序列化辅助函数
func SerializeMessage(msg proto.Message) ([]byte, error) {
	return proto.Marshal(msg)
}

func DeserializeMessage(data []byte, msg proto.Message) error {
	return proto.Unmarshal(data, msg)
}

// 将 protobuf PlayerState 转换为兼容格式
func (ps *PlayerState) ToLegacyPlayerState() LegacyPlayerState {
	return LegacyPlayerState{
		Online:     ps.Online,
		LastSeen:   time.Now(), // 使用当前时间，ping 信息由 kcp2k-go 基础库管理
		Latency:    0,          // ping 延迟由 kcp2k-go 基础库管理
		PacketLoss: 0.0,        // protobuf 版本暂不支持
	}
}

// 兼容性结构体（用于需要 time.Time 的场景）
type LegacyPlayerState struct {
	Online     bool      `json:"online"`      // 是否在线
	LastSeen   time.Time `json:"last_seen"`   // 最后活跃时间
	Latency    int       `json:"latency"`     // 延迟（毫秒）
	PacketLoss float32   `json:"packet_loss"` // 丢包率
}

type LegacyRoomState struct {
	Status      RoomStatus `json:"status"`       // 房间状态
	PlayerCount int        `json:"player_count"` // 玩家数量
	MaxPlayers  int        `json:"max_players"`  // 最大玩家数
	FrameRate   int        `json:"frame_rate"`   // 帧率
	StartTime   time.Time  `json:"start_time"`   // 开始时间
}

// Player 玩家信息
type Player struct {
	ID           PlayerID                    `json:"id"`            // 玩家ID
	ConnectionID int                         `json:"connection_id"` // KCP连接ID
	State        *PlayerState                `json:"state"`         // 玩家状态（使用 protobuf 结构体）
	LastFrameID  FrameID                     `json:"last_frame_id"` // 最后接收的帧ID
	InputBuffer  map[FrameID][]*InputMessage `json:"-"`             // 输入缓冲区
	Mutex        sync.RWMutex                `json:"-"`             // 读写锁
}

// Room 房间信息
type Room struct {
	ID             RoomID               `json:"id"`               // 房间ID
	Port           uint16               `json:"port"`             // 房间专用端口
	KcpServer      *kcp2k.KcpServer     `json:"-"`                // 房间专用KCP服务器
	Players        map[PlayerID]*Player `json:"players"`          // 玩家列表
	Frames         map[FrameID]*Frame   `json:"-"`                // 帧数据缓存
	CurrentFrameID FrameID              `json:"current_frame_id"` // 当前帧ID
	MaxFrameID     FrameID              `json:"max_frame_id"`     // 最大帧ID
	State          *RoomState           `json:"state"`            // 房间状态（使用 protobuf 结构体）
	Config         *RoomConfig          `json:"config"`           // 房间配置（使用 protobuf 结构体）
	Mutex          sync.RWMutex         `json:"-"`                // 读写锁
	Ticker         *time.Ticker         `json:"-"`                // 帧定时器
	StopChan       chan struct{}        `json:"-"`                // 停止信号通道
	CreatedAt      time.Time            `json:"created_at"`       // 创建时间
	running        bool                 `json:"-"`                // 运行状态

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
		stats["current_players"] = r.State.CurrentPlayers
		stats["max_players"] = r.State.MaxPlayers
		stats["start_time"] = r.State.StartTime
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

	if c.RoomConfig == nil {
		return fmt.Errorf("room config cannot be nil")
	}

	if c.RoomConfig.MaxPlayers == 0 {
		return fmt.Errorf("max players cannot be 0")
	}

	if c.RoomConfig.MinPlayers > c.RoomConfig.MaxPlayers {
		return fmt.Errorf("min players cannot be greater than max players")
	}

	if c.RoomConfig.FrameRate == 0 {
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
