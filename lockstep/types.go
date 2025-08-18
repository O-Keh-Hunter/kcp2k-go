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
	
	// 房间级别的统计信息
	FrameStats   *FrameStats   `json:"-"` // 帧统计信息
	NetworkStats *NetworkStats `json:"-"` // 网络统计信息
}

// FrameStats 帧统计信息
type FrameStats struct {
	totalFrames   uint64
	missedFrames  uint64
	lateFrames    uint64
	frameTimeSum  time.Duration
	lastFrameTime time.Time
	mutex         sync.RWMutex
}

// GetTotalFrames 获取总帧数
func (fs *FrameStats) GetTotalFrames() uint64 {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()
	return fs.totalFrames
}

// GetMissedFrames 获取丢失帧数
func (fs *FrameStats) GetMissedFrames() uint64 {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()
	return fs.missedFrames
}

// GetLateFrames 获取延迟帧数
func (fs *FrameStats) GetLateFrames() uint64 {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()
	return fs.lateFrames
}

// GetAverageFrameTime 获取平均帧时间
func (fs *FrameStats) GetAverageFrameTime() time.Duration {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()
	if fs.totalFrames == 0 {
		return 0
	}
	return fs.frameTimeSum / time.Duration(fs.totalFrames)
}

// NetworkStats 网络统计信息
type NetworkStats struct {
	totalPackets uint64
	lostPackets  uint64
	bytesReceived uint64
	bytesSent    uint64
	latencySum   time.Duration
	latencyCount uint64
	maxLatency   time.Duration
	minLatency   time.Duration
	mutex        sync.RWMutex
}

// GetTotalPackets 获取总包数
func (ns *NetworkStats) GetTotalPackets() uint64 {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.totalPackets
}

// GetLostPackets 获取丢包数
func (ns *NetworkStats) GetLostPackets() uint64 {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.lostPackets
}

// GetAverageLatency 获取平均延迟
func (ns *NetworkStats) GetAverageLatency() time.Duration {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	if ns.latencyCount == 0 {
		return 0
	}
	return ns.latencySum / time.Duration(ns.latencyCount)
}

// GetMaxLatency 获取最大延迟
func (ns *NetworkStats) GetMaxLatency() time.Duration {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.maxLatency
}

// GetMinLatency 获取最小延迟
func (ns *NetworkStats) GetMinLatency() time.Duration {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.minLatency
}

// GetBytesReceived 获取接收字节数
func (ns *NetworkStats) GetBytesReceived() uint64 {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.bytesReceived
}

// GetBytesSent 获取发送字节数
func (ns *NetworkStats) GetBytesSent() uint64 {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.bytesSent
}

// Room统计信息方法

// GetFrameStats 获取房间帧统计信息
func (r *Room) GetFrameStats() *FrameStats {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()
	return r.FrameStats
}

// GetNetworkStats 获取房间网络统计信息
func (r *Room) GetNetworkStats() *NetworkStats {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()
	return r.NetworkStats
}

// UpdateFrameStats 更新帧统计信息
func (r *Room) UpdateFrameStats(totalFrames, missedFrames, lateFrames uint64, frameTimeSum time.Duration, lastFrameTime time.Time) {
	if r.FrameStats == nil {
		return
	}
	r.FrameStats.mutex.Lock()
	defer r.FrameStats.mutex.Unlock()
	r.FrameStats.totalFrames = totalFrames
	r.FrameStats.missedFrames = missedFrames
	r.FrameStats.lateFrames = lateFrames
	r.FrameStats.frameTimeSum = frameTimeSum
	r.FrameStats.lastFrameTime = lastFrameTime
}

// IncrementFrameStats 增量更新帧统计信息
func (r *Room) IncrementFrameStats(missed, late bool, frameDuration time.Duration) {
	if r.FrameStats == nil {
		return
	}
	r.FrameStats.mutex.Lock()
	defer r.FrameStats.mutex.Unlock()
	r.FrameStats.totalFrames++
	if missed {
		r.FrameStats.missedFrames++
	}
	if late {
		r.FrameStats.lateFrames++
	}
	r.FrameStats.frameTimeSum += frameDuration
	r.FrameStats.lastFrameTime = time.Now()
}

// UpdateNetworkStats 更新网络统计信息
func (r *Room) UpdateNetworkStats(totalPackets, lostPackets, bytesReceived, bytesSent uint64, latency time.Duration) {
	if r.NetworkStats == nil {
		return
	}
	r.NetworkStats.mutex.Lock()
	defer r.NetworkStats.mutex.Unlock()
	r.NetworkStats.totalPackets = totalPackets
	r.NetworkStats.lostPackets = lostPackets
	r.NetworkStats.bytesReceived = bytesReceived
	r.NetworkStats.bytesSent = bytesSent
	if latency > 0 {
		r.NetworkStats.latencySum += latency
		r.NetworkStats.latencyCount++
		if latency > r.NetworkStats.maxLatency {
			r.NetworkStats.maxLatency = latency
		}
		if latency < r.NetworkStats.minLatency || r.NetworkStats.minLatency == 0 {
			r.NetworkStats.minLatency = latency
		}
	}
}

// IncrementNetworkStats 增量更新网络统计信息
func (r *Room) IncrementNetworkStats(packetsReceived, packetsSent, bytesReceived, bytesSent uint64, lost bool, latency time.Duration) {
	if r.NetworkStats == nil {
		return
	}
	r.NetworkStats.mutex.Lock()
	defer r.NetworkStats.mutex.Unlock()
	r.NetworkStats.totalPackets += packetsReceived + packetsSent
	if lost {
		r.NetworkStats.lostPackets++
	}
	r.NetworkStats.bytesReceived += bytesReceived
	r.NetworkStats.bytesSent += bytesSent
	if latency > 0 {
		r.NetworkStats.latencySum += latency
		r.NetworkStats.latencyCount++
		if latency > r.NetworkStats.maxLatency {
			r.NetworkStats.maxLatency = latency
		}
		if latency < r.NetworkStats.minLatency || r.NetworkStats.minLatency == 0 {
			r.NetworkStats.minLatency = latency
		}
	}
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
		stats["average_latency"] = r.NetworkStats.GetAverageLatency().Milliseconds()
		stats["max_latency"] = r.NetworkStats.GetMaxLatency().Milliseconds()
		stats["min_latency"] = r.NetworkStats.GetMinLatency().Milliseconds()
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
