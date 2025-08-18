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

// InputFlag 输入标志
type InputFlag uint32

const (
	InputFlagNormal    InputFlag = 0 // 普通输入
	InputFlagCritical  InputFlag = 1 // 关键输入（需要冗余发送）
	InputFlagRedundant InputFlag = 2 // 冗余输入
)

// RoomStatus 房间状态枚举
type RoomStatus uint32

const (
	RoomStatusWaiting RoomStatus = 0 // 等待玩家
	RoomStatusReady   RoomStatus = 1 // 准备开始
	RoomStatusRunning RoomStatus = 2 // 游戏进行中
	RoomStatusEnded   RoomStatus = 3 // 游戏结束
)

// MessageType 消息类型别名，映射到 protobuf 枚举
type MessageType = LockStepMessage_MessageType

// 消息类型常量
const (
	MessageTypeFrame       = LockStepMessage_FRAME
	MessageTypeInput       = LockStepMessage_INPUT
	MessageTypeFrameReq    = LockStepMessage_FRAME_REQ
	MessageTypeFrameResp   = LockStepMessage_FRAME_RESP
	MessageTypeReady       = LockStepMessage_START // 映射到 START
	MessageTypeStart       = LockStepMessage_START
	MessageTypePlayerState = LockStepMessage_PLAYER_STATE
	MessageTypeRoomState   = LockStepMessage_ROOM_STATE
	MessageTypePing        = LockStepMessage_PING
	MessageTypePong        = LockStepMessage_PONG
	MessageTypeJoinRoom    = LockStepMessage_JOIN_ROOM
	MessageTypeEnd         = LockStepMessage_END
	MessageTypeError       = LockStepMessage_ERROR
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

// 创建 LockStepMessage 的辅助函数
func NewLockStepMessage(msgType MessageType, payload []byte) *LockStepMessage {
	return &LockStepMessage{
		Type:    msgType,
		Payload: payload,
	}
}

// 将 protobuf PlayerState 转换为兼容格式
func (ps *PlayerState) ToLegacyPlayerState() LegacyPlayerState {
	return LegacyPlayerState{
		Online:     ps.Online,
		LastSeen:   time.Unix(ps.LastPingTime/1000, 0),
		Latency:    int(ps.Ping),
		PacketLoss: 0.0, // protobuf 版本暂不支持
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
	ID           PlayerID           `json:"id"`            // 玩家ID
	ConnectionID int                `json:"connection_id"` // KCP连接ID
	State        *PlayerState       `json:"state"`         // 玩家状态（使用 protobuf 结构体）
	LastFrameID  FrameID            `json:"last_frame_id"` // 最后接收的帧ID
	InputBuffer  map[FrameID][]byte `json:"-"`             // 输入缓冲区
	Mutex        sync.RWMutex       `json:"-"`             // 读写锁
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
		KcpConfig: kcp2k.NewKcpConfig(kcp2k.WithDualMode(false),
			kcp2k.WithInterval(1)),
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
