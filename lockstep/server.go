package lockstep

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	kcp2k "github.com/O-Keh-Hunter/kcp2k-go"
	"google.golang.org/protobuf/proto"
)

// LockStepServer 帧同步服务器
type LockStepServer struct {
	config      LockStepConfig
	rooms       *RoomManager // 房间管理器
	portManager *PortManager // 端口管理器
	mutex       sync.RWMutex
	running     bool
	stopChan    chan struct{}
	logger      *log.Logger

	// 性能监控字段
	startTime     time.Time
	metricsServer *http.Server
}

// PortManager 端口管理器
type PortManager struct {
	startPort   uint16
	currentPort uint16
	usedPorts   map[uint16]bool
	mutex       sync.Mutex
}

// NewLockStepServer 创建新的帧同步服务器
func NewLockStepServer(config *LockStepConfig) *LockStepServer {
	// 创建端口管理器
	portManager := &PortManager{
		startPort:   config.ServerPort,
		currentPort: config.ServerPort,
		usedPorts:   make(map[uint16]bool),
	}

	// 创建日志记录器
	logger := log.New(os.Stdout, "[LockStep] ", log.LstdFlags)

	server := &LockStepServer{
		config:      *config,
		mutex:       sync.RWMutex{},
		running:     false,
		stopChan:    make(chan struct{}),
		logger:      logger,
		portManager: portManager,
		startTime:   time.Now(),
	}

	// 创建房间管理器
	server.rooms = NewRoomManager(portManager, logger, &config.KcpConfig)

	return server
}

// AllocatePort 分配一个可用端口
func (pm *PortManager) AllocatePort() uint16 {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	for {
		if !pm.usedPorts[pm.currentPort] {
			pm.usedPorts[pm.currentPort] = true
			port := pm.currentPort
			pm.currentPort++
			return port
		}
		pm.currentPort++
		if pm.currentPort == math.MaxUint16 {
			pm.currentPort = pm.startPort
		}
	}
}

// ReleasePort 释放端口
func (pm *PortManager) ReleasePort(port uint16) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	delete(pm.usedPorts, port)
}

// Start 启动服务器
func (s *LockStepServer) Start() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.running {
		return fmt.Errorf("server is already running")
	}

	s.running = true
	s.logger.Printf("LockStep server started")

	// 启动房间管理器
	s.rooms.Start()

	// 启动指标服务器（如果配置了MetricsPort）
	if s.config.MetricsPort > 0 {
		s.StartMetricsServer()
	}

	return nil
}

// Stop 停止服务器
func (s *LockStepServer) Stop() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.running {
		return
	}

	s.running = false
	close(s.stopChan)

	// 停止指标服务器
	s.StopMetricsServer()

	// 停止房间管理器
	s.rooms.Stop()

	s.logger.Printf("LockStep server stopped")
}

// GracefulShutdown 优雅关闭服务器
func (s *LockStepServer) GracefulShutdown(ctx context.Context) error {
	s.logger.Printf("Starting graceful shutdown...")

	// 创建一个通道来接收关闭完成信号
	done := make(chan struct{})

	go func() {
		defer close(done)

		// 停止接受新连接
		s.mutex.Lock()
		s.running = false
		s.mutex.Unlock()

		// 通知所有房间准备关闭
		for _, room := range s.rooms.GetAllRooms() {
			s.broadcastError(room, ErrorCodeServerOverload, "Server shutting down", "Please reconnect later")
		}

		// 等待一段时间让客户端处理关闭消息
		time.Sleep(2 * time.Second)

		// 停止房间管理器
		s.rooms.Stop()

		s.logger.Printf("All rooms stopped")
	}()

	// 等待关闭完成或超时
	select {
	case <-done:
		s.logger.Printf("Graceful shutdown completed")
		return nil
	case <-ctx.Done():
		s.logger.Printf("Graceful shutdown timeout, forcing stop")
		s.Stop()
		return ctx.Err()
	}
}

// WaitForShutdown 等待关闭信号
func (s *LockStepServer) WaitForShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	s.logger.Printf("Received signal: %v", sig)

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.GracefulShutdown(ctx); err != nil {
		s.logger.Printf("Graceful shutdown failed: %v", err)
	}
}

// CreateRoom 创建房间
func (s *LockStepServer) CreateRoom(roomID RoomID, config *RoomConfig) (*Room, error) {
	return s.rooms.CreateRoom(roomID, config, s)
}

// GetRoom 获取房间
func (s *LockStepServer) GetRoom(roomID RoomID) (*Room, bool) {
	return s.rooms.GetRoom(roomID)
}

// GetRooms 获取所有房间
func (s *LockStepServer) GetRooms() map[RoomID]*Room {
	return s.rooms.GetRooms()
}

// JoinRoom 玩家加入房间
func (s *LockStepServer) JoinRoom(roomID RoomID, playerID PlayerID, connectionID int) error {
	return s.rooms.JoinRoom(roomID, playerID, connectionID, s)
}

// processFrame 处理一帧
func (s *LockStepServer) processFrame(room *Room) {
	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	if room.State.Status != uint32(RoomStatusRunning) {
		return
	}

	frameStartTime := time.Now()

	// 记录帧统计
	if room.FrameStats != nil {
		room.FrameStats.mutex.Lock()
		room.FrameStats.totalFrames++

		if !room.FrameStats.lastFrameTime.IsZero() {
			frameDuration := frameStartTime.Sub(room.FrameStats.lastFrameTime)
			room.FrameStats.frameTimeSum += frameDuration

			// 检查是否为迟到的帧
			expectedInterval := time.Duration(1000/room.Config.FrameRate) * time.Millisecond
			if frameDuration > expectedInterval*110/100 { // 允许10%的误差
				room.FrameStats.lateFrames++
			}
		}
		room.FrameStats.lastFrameTime = frameStartTime
		room.FrameStats.mutex.Unlock()
	}

	// 创建新帧
	frameID := room.CurrentFrameID + 1
	frame := &Frame{
		Id:     uint32(frameID),
		Inputs: make([]*PlayerInput, 0),
		Metadata: &FrameMetadata{
			Timestamp:    time.Now().UnixMilli(),
			PlayerStates: s.getPlayerStates(room),
			RoomState:    room.State,
		},
	}

	// 收集玩家输入
	missedInputCount := 0
	for playerID, player := range room.Players {
		player.Mutex.RLock()
		if inputData, exists := player.InputBuffer[frameID]; exists {
			frame.Inputs = append(frame.Inputs, &PlayerInput{
				PlayerId: uint32(playerID),
				Data:     inputData,
				Flag:     uint32(InputFlagNormal),
			})
			delete(player.InputBuffer, frameID)
		} else {
			// 没有输入数据，添加空输入
			frame.Inputs = append(frame.Inputs, &PlayerInput{
				PlayerId: uint32(playerID),
				Data:     []byte{},
				Flag:     uint32(InputFlagNormal),
			})
			missedInputCount++
		}
		player.Mutex.RUnlock()
	}

	// 更新帧统计 - 如果有玩家缺少输入，记录为丢帧
	if missedInputCount > 0 && room.FrameStats != nil {
		room.FrameStats.mutex.Lock()
		room.FrameStats.missedFrames++
		room.FrameStats.mutex.Unlock()
	}

	// 存储帧数据
	room.Frames[frameID] = frame
	room.CurrentFrameID = frameID
	if frameID > room.MaxFrameID {
		room.MaxFrameID = frameID
	}

	// 广播帧数据
	if currentFrame, exists := room.Frames[frameID]; exists {
		frames := []*Frame{currentFrame}
		resp := &FrameResponse{
			Frames:  frames,
			Success: true,
		}

		respData, err := proto.Marshal(resp)
		if err != nil {
			s.logger.Printf("Room %s: Failed to marshal frame response: %v", room.ID, err)
			return
		}

		msg := &LockStepMessage{
			Type:    LockStepMessage_FRAME_RESP,
			Payload: respData,
		}

		s.broadcastToRoom(room, msg)
		s.logger.Printf("Room %s: Broadcasted frame %d", room.ID, frameID)
	}
}

// getPlayerStates 获取玩家状态
func (s *LockStepServer) getPlayerStates(room *Room) map[uint32]*PlayerState {
	states := make(map[uint32]*PlayerState)
	for playerID, player := range room.Players {
		player.Mutex.RLock()
		states[uint32(playerID)] = player.State
		player.Mutex.RUnlock()
	}
	return states
}

// broadcastToRoom 向房间广播消息
func (s *LockStepServer) broadcastToRoom(room *Room, msg *LockStepMessage) {
	data, err := proto.Marshal(msg)
	if err != nil {
		s.logger.Printf("Failed to marshal message: %v", err)
		return
	}

	sentCount := uint64(0)
	for _, player := range room.Players {
		// 加锁确保读取玩家状态的一致性
		player.Mutex.RLock()
		online := player.State.Online
		connectionID := player.ConnectionID
		player.Mutex.RUnlock()

		if online {
			// 发送消息
			room.KcpServer.Send(connectionID, data, kcp2k.KcpReliable)
			sentCount++
		}
	}

	// 更新网络统计 - 记录发送的包数量和字节数
	if sentCount > 0 && room.NetworkStats != nil {
		room.NetworkStats.mutex.Lock()
		room.NetworkStats.totalPackets += sentCount
		room.NetworkStats.bytesSent += uint64(len(data)) * sentCount
		room.NetworkStats.mutex.Unlock()
	}
}

// broadcastPlayerState 广播玩家状态变更
func (s *LockStepServer) broadcastPlayerState(room *Room, playerID PlayerID, state *PlayerState, reason string) {
	playerStateMsg := PlayerStateMessage{
		PlayerId: uint32(playerID),
		State:    state,
		Reason:   reason,
	}

	stateData, err := proto.Marshal(&playerStateMsg)
	if err != nil {
		s.logger.Printf("Failed to marshal player state message: %v", err)
		return
	}

	msg := &LockStepMessage{
		Type:    LockStepMessage_PLAYER_STATE,
		Payload: stateData,
	}

	s.broadcastToRoom(room, msg)
	s.logger.Printf("Room %s: Broadcasted player %d state change: %s", room.ID, playerID, reason)
}

// broadcastRoomState 广播房间状态变更
func (s *LockStepServer) broadcastRoomState(room *Room, reason string) {
	roomStateMsg := RoomStateMessage{
		RoomId: string(room.ID),
		State:  room.State,
		Reason: reason,
	}

	s.logger.Printf("BroadcastRoomState: %v\n", roomStateMsg.State)

	stateData, err := proto.Marshal(&roomStateMsg)
	if err != nil {
		s.logger.Printf("Failed to marshal room state message: %v", err)
		return
	}

	msg := &LockStepMessage{
		Type:    LockStepMessage_ROOM_STATE,
		Payload: stateData,
	}

	s.broadcastToRoom(room, msg)
	s.logger.Printf("Room %s: Broadcasted room state change: %s", room.ID, reason)
}

// sendError 发送错误消息给指定连接
func (s *LockStepServer) sendError(room *Room, connectionID int, errorCode uint32, message string, details string) {
	errorMsg := ErrorMessage{
		Code:    errorCode,
		Message: message,
		Details: details,
	}

	errorData, err := proto.Marshal(&errorMsg)
	if err != nil {
		s.logger.Printf("Failed to marshal error message: %v", err)
		return
	}

	msg := &LockStepMessage{
		Type:    LockStepMessage_ERROR,
		Payload: errorData,
	}

	msgData, err := proto.Marshal(msg)
	if err != nil {
		s.logger.Printf("Failed to marshal lock step message: %v", err)
		return
	}

	room.KcpServer.Send(connectionID, msgData, kcp2k.KcpReliable)
	s.logger.Printf("Sent error %d to connection %d: %s", errorCode, connectionID, message)
}

// broadcastError 广播错误消息给房间内所有玩家
func (s *LockStepServer) broadcastError(room *Room, errorCode uint32, message string, details string) {
	errorMsg := ErrorMessage{
		Code:    errorCode,
		Message: message,
		Details: details,
	}

	errorData, err := proto.Marshal(&errorMsg)
	if err != nil {
		s.logger.Printf("Failed to marshal error message: %v", err)
		return
	}

	msg := &LockStepMessage{
		Type:    LockStepMessage_ERROR,
		Payload: errorData,
	}

	s.broadcastToRoom(room, msg)
	s.logger.Printf("Room %s: Broadcasted error %d: %s", room.ID, errorCode, message)
}

// KCP回调函数
func (s *LockStepServer) onRoomConnected(room *Room, connectionID int) {
	s.logger.Printf("Room %s: Connection %d established", room.ID, connectionID)
}

func (s *LockStepServer) onRoomData(room *Room, connectionID int, data []byte, _ kcp2k.KcpChannel) {
	// 更新网络统计 - 记录接收到的包和字节数
	if room.NetworkStats != nil {
		room.NetworkStats.mutex.Lock()
		room.NetworkStats.totalPackets++
		room.NetworkStats.bytesReceived += uint64(len(data))
		room.NetworkStats.mutex.Unlock()
	}

	var msg LockStepMessage
	err := proto.Unmarshal(data, &msg)
	if err != nil {
		s.logger.Printf("Room %s: Failed to unmarshal message from connection %d: %v", room.ID, connectionID, err)
		// 记录为丢包（解析失败）
		if room.NetworkStats != nil {
			room.NetworkStats.mutex.Lock()
			room.NetworkStats.lostPackets++
			room.NetworkStats.mutex.Unlock()
		}
		return
	}

	s.handleRoomMessage(room, connectionID, &msg)
}

func (s *LockStepServer) onRoomDisconnected(room *Room, connectionID int) {
	// 记录网络断开为丢包
	if room.NetworkStats != nil {
		room.NetworkStats.mutex.Lock()
		room.NetworkStats.lostPackets++
		room.NetworkStats.mutex.Unlock()
	}

	// 通过房间查找玩家
	room.Mutex.Lock()
	var disconnectedPlayer *Player
	for _, player := range room.Players {
		if player.ConnectionID == connectionID {
			disconnectedPlayer = player
			break
		}
	}
	room.Mutex.Unlock()

	if disconnectedPlayer != nil {
		// 使用锁确保状态更新的原子性
		disconnectedPlayer.Mutex.Lock()
		disconnectedPlayer.State.Online = false
		disconnectedPlayer.Mutex.Unlock()

		s.logger.Printf("Room %s: Player %d disconnected", room.ID, disconnectedPlayer.ID)

		// 广播玩家离线状态给房间内的其他玩家
		s.broadcastPlayerState(room, disconnectedPlayer.ID, disconnectedPlayer.State, "Player disconnected")
	}
}

func (s *LockStepServer) onRoomError(room *Room, connectionID int, error kcp2k.ErrorCode, reason string) {
	// 记录网络错误为丢包
	if room.NetworkStats != nil {
		room.NetworkStats.mutex.Lock()
		room.NetworkStats.lostPackets++
		room.NetworkStats.mutex.Unlock()
	}

	s.logger.Printf("Room %s: Connection %d error: %v - %s", room.ID, connectionID, error, reason)
}

// handleRoomMessage 处理房间消息
func (s *LockStepServer) handleRoomMessage(room *Room, connectionID int, msg *LockStepMessage) {
	switch msg.Type {
	case LockStepMessage_JOIN_ROOM:
		s.handleJoinRoom(room, connectionID, msg.Payload)
	case LockStepMessage_INPUT:
		s.handlePlayerInput(room, connectionID, msg.Payload)
	case LockStepMessage_FRAME_REQ:
		s.handleFrameRequest(room, connectionID, msg.Payload)
	case LockStepMessage_PING:
		s.handlePing(room, connectionID, msg.Payload)
	default:
		s.logger.Printf("Room %s: Unknown message type %d from connection %d", room.ID, msg.Type, connectionID)
	}
}

// handlePlayerInput 处理玩家输入
func (s *LockStepServer) handlePlayerInput(room *Room, connectionID int, payload []byte) {
	// 通过房间查找玩家
	room.Mutex.RLock()
	var player *Player
	for _, p := range room.Players {
		if p.ConnectionID == connectionID {
			player = p
			break
		}
	}
	room.Mutex.RUnlock()

	if player == nil {
		s.logger.Printf("Room %s: Player not found for connection %d", room.ID, connectionID)
		return
	}

	// 解析输入数据
	var input PlayerInput
	err := proto.Unmarshal(payload, &input)
	if err != nil {
		s.logger.Printf("Room %s: Failed to unmarshal player input: %v", room.ID, err)
		return
	}

	frameID := room.CurrentFrameID + 1
	s.handleInput(room, player, frameID, input.Data)
}

// handleInput 处理输入
func (s *LockStepServer) handleInput(room *Room, player *Player, frameID FrameID, data []byte) {
	player.Mutex.Lock()
	player.InputBuffer[frameID] = data
	player.Mutex.Unlock()
}

// handleFrameRequest 处理补帧请求
func (s *LockStepServer) handleFrameRequest(room *Room, connectionID int, payload []byte) {
	var req FrameRequest
	err := proto.Unmarshal(payload, &req)
	if err != nil {
		s.logger.Printf("Room %s: Failed to unmarshal frame request: %v", room.ID, err)
		return
	}

	room.Mutex.RLock()
	frames := make([]*Frame, 0)
	missingFrames := make([]FrameID, 0)

	// 获取请求范围内的帧
	for frameID := FrameID(req.StartId); frameID <= FrameID(req.EndId); frameID++ {
		if frame, exists := room.Frames[frameID]; exists {
			frames = append(frames, frame)
		} else {
			missingFrames = append(missingFrames, frameID)
		}
	}
	room.Mutex.RUnlock()

	// 记录缺失的帧
	if len(missingFrames) > 0 {
		s.logger.Printf("Room %s: Missing frames in request range %d-%d: %v (total missing: %d)",
			room.ID, req.StartId, req.EndId, missingFrames, len(missingFrames))
	}

	// 记录帧请求信息
	s.logger.Printf("Room %s: Sending frame response, requested range: %d-%d, found frames: %d",
		room.ID, req.StartId, req.EndId, len(frames))

	// 发送补帧响应
	resp := FrameResponse{
		Frames:  frames,
		Success: true,
	}

	respData, err := proto.Marshal(&resp)
	if err != nil {
		s.logger.Printf("Room %s: Failed to marshal frame response: %v", room.ID, err)
		return
	}

	msg := &LockStepMessage{
		Type:    LockStepMessage_FRAME_RESP,
		Payload: respData,
	}

	msgData, err := proto.Marshal(msg)
	if err != nil {
		s.logger.Printf("Room %s: Failed to marshal message: %v", room.ID, err)
		return
	}

	room.KcpServer.Send(connectionID, msgData, kcp2k.KcpReliable)
}

// handleJoinRoom 处理加入房间消息
func (s *LockStepServer) handleJoinRoom(room *Room, connectionID int, payload []byte) {
	// 解析JoinRoom消息
	var joinMsg JoinRoomRequest

	if err := proto.Unmarshal(payload, &joinMsg); err != nil {
		s.logger.Printf("Room %s: Failed to parse join room message from connection %d: %v", room.ID, connectionID, err)
		s.sendError(room, connectionID, ErrorCodeInvalidMessage, "Invalid join room message", err.Error())
		return
	}

	// 调用JoinRoom方法
	if err := s.JoinRoom(RoomID(joinMsg.RoomId), PlayerID(joinMsg.PlayerId), connectionID); err != nil {
		s.logger.Printf("Room %s: Failed to join room for player %d: %v", room.ID, joinMsg.PlayerId, err)
		// 根据错误类型发送相应的错误码
		if err.Error() == "room is full" || err.Error() == fmt.Sprintf("room %s is full", joinMsg.RoomId) {
			s.sendError(room, connectionID, ErrorCodeRoomFull, "Room is full", "Cannot join room: maximum players reached")
		} else if err.Error() == "player already in room" || err.Error() == fmt.Sprintf("player %d already in room %s", joinMsg.PlayerId, joinMsg.RoomId) {
			s.sendError(room, connectionID, ErrorCodePlayerAlreadyInRoom, "Player already in room", "Player is already a member of this room")
		} else {
			s.sendError(room, connectionID, ErrorCodeUnknown, "Failed to join room", err.Error())
		}
		return
	}

	// 发送成功响应 - 使用broadcastRoomState的相同格式
	room.Mutex.RLock()
	roomStateMsg := RoomStateMessage{
		RoomId: string(room.ID),
		State:  room.State,
		Reason: "Player joined room",
	}
	room.Mutex.RUnlock()

	stateData, err := proto.Marshal(&roomStateMsg)
	if err != nil {
		s.logger.Printf("Failed to marshal room state message: %v", err)
		return
	}

	responseMsg := &LockStepMessage{
		Type:    LockStepMessage_ROOM_STATE,
		Payload: stateData,
	}

	if data, err := proto.Marshal(responseMsg); err == nil {
		room.KcpServer.Send(connectionID, data, kcp2k.KcpReliable)
	}
}

// handlePing 处理Ping消息
func (s *LockStepServer) handlePing(room *Room, connectionID int, payload []byte) {
	// 解析Ping消息
	pingMsg := &PingMessage{}
	if err := proto.Unmarshal(payload, pingMsg); err != nil {
		s.logger.Printf("Failed to unmarshal ping message: %v", err)
		// 如果解析失败，直接回复原始payload
		msg := &LockStepMessage{
			Type:    LockStepMessage_PONG,
			Payload: payload,
		}
		data, _ := proto.Marshal(msg)
		room.KcpServer.Send(connectionID, data, kcp2k.KcpReliable)
		return
	}

	// 计算延迟并更新统计
	clientTimestamp := pingMsg.Timestamp
	currentTime := time.Now().UnixMilli()

	if clientTimestamp > 0 {
		latency := time.Duration(currentTime-clientTimestamp) * time.Millisecond

		// 更新网络统计信息
		s.updateNetworkStats(room, latency)

		// 通过房间查找玩家并更新延迟信息
		room.Mutex.RLock()
		var player *Player
		for _, p := range room.Players {
			if p.ConnectionID == connectionID {
				player = p
				break
			}
		}
		room.Mutex.RUnlock()

		if player != nil && player.State != nil {
			player.Mutex.Lock()
			player.State.Ping = int64(latency.Milliseconds())
			player.State.LastPingTime = currentTime
			player.Mutex.Unlock()
		}
	}

	// 回复Pong
	pongMsg := &PongMessage{
		Timestamp: clientTimestamp,
		PlayerId:  pingMsg.PlayerId,
	}
	pongPayload, err := proto.Marshal(pongMsg)
	if err != nil {
		s.logger.Printf("Failed to marshal pong message: %v", err)
		return
	}

	msg := &LockStepMessage{
		Type:    LockStepMessage_PONG,
		Payload: pongPayload,
	}
	data, err := proto.Marshal(msg)
	if err != nil {
		s.logger.Printf("Room %s: Failed to marshal pong message: %v", room.ID, err)
		return
	}

	room.KcpServer.Send(connectionID, data, kcp2k.KcpReliable)
}

// updateNetworkStats 更新网络统计信息
func (s *LockStepServer) updateNetworkStats(room *Room, latency time.Duration) {
	if room.NetworkStats == nil {
		return
	}

	room.NetworkStats.mutex.Lock()
	defer room.NetworkStats.mutex.Unlock()

	// 更新延迟统计
	room.NetworkStats.latencySum += latency
	room.NetworkStats.latencyCount++

	// 更新最大/最小延迟
	if latency > room.NetworkStats.maxLatency {
		room.NetworkStats.maxLatency = latency
	}
	if latency < room.NetworkStats.minLatency {
		room.NetworkStats.minLatency = latency
	}

	// 更新总包数
	room.NetworkStats.totalPackets++
}

// GetRoomInfo 获取房间信息
func (s *LockStepServer) GetRoomInfo(roomID RoomID) (*Room, bool) {
	return s.GetRoom(roomID)
}

// GetRoomMonitoringInfo 获取单个房间的监控信息
func (s *LockStepServer) GetRoomMonitoringInfo(roomID RoomID) (map[string]interface{}, error) {
	room, exists := s.GetRoom(roomID)
	if !exists {
		return nil, fmt.Errorf("room %s not found", roomID)
	}

	return room.GetRoomMonitoringInfo(), nil
}

// GetServerStats 获取服务器统计信息（汇总所有房间）
func (s *LockStepServer) GetServerStats() map[string]interface{} {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	allRooms := s.rooms.GetAllRooms()

	// 汇总所有房间的帧统计信息
	var totalFrames, missedFrames, lateFrames uint64
	var frameTimeSum time.Duration
	frameCount := 0

	// 汇总所有房间的网络统计信息
	var totalPackets, lostPackets, bytesReceived, bytesSent uint64
	var latencySum time.Duration
	var maxLatency, minLatency time.Duration
	latencyCount := 0
	minLatency = time.Hour // 初始化为很大的值

	// 计算总玩家数
	totalPlayers := 0

	for _, room := range allRooms {
		totalPlayers += len(room.Players)

		// 汇总帧统计
		if room.FrameStats != nil {
			frameStats := room.GetFrameStats()
			totalFrames += frameStats.GetTotalFrames()
			missedFrames += frameStats.GetMissedFrames()
			lateFrames += frameStats.GetLateFrames()
			frameTimeSum += frameStats.frameTimeSum
			frameCount++
		}

		// 汇总网络统计
		if room.NetworkStats != nil {
			netStats := room.GetNetworkStats()
			totalPackets += netStats.GetTotalPackets()
			lostPackets += netStats.GetLostPackets()
			bytesReceived += netStats.GetBytesReceived()
			bytesSent += netStats.GetBytesSent()

			// 延迟统计
			netStats.mutex.RLock()
			latencySum += netStats.latencySum
			latencyCount += int(netStats.latencyCount)
			if netStats.maxLatency > maxLatency {
				maxLatency = netStats.maxLatency
			}
			if netStats.minLatency < minLatency && netStats.minLatency > 0 {
				minLatency = netStats.minLatency
			}
			netStats.mutex.RUnlock()
		}
	}

	// 计算平均帧时间
	avgFrameTime := float64(0)
	if totalFrames > 0 {
		avgFrameTime = float64(frameTimeSum.Nanoseconds()) / float64(totalFrames) / 1e6 // 转换为毫秒
	}

	// 计算平均延迟
	avgLatency := float64(0)
	if latencyCount > 0 {
		avgLatency = float64(latencySum.Nanoseconds()) / float64(latencyCount) / 1e6 // 转换为毫秒
	}

	// 处理最小延迟
	minLatencyMs := int64(0)
	if minLatency != time.Hour && minLatency > 0 {
		minLatencyMs = minLatency.Milliseconds()
	}

	// 计算运行时间
	uptime := time.Since(s.startTime).Milliseconds()

	return map[string]interface{}{
		"total_rooms":   len(allRooms),
		"total_players": totalPlayers,
		"running":       s.running,
		"uptime":        uptime,
		"frame_stats": map[string]interface{}{
			"total_frames":   totalFrames,
			"missed_frames":  missedFrames,
			"late_frames":    lateFrames,
			"avg_frame_time": avgFrameTime,
		},
		"network_stats": map[string]interface{}{
			"total_packets":  totalPackets,
			"lost_packets":   lostPackets,
			"bytes_received": bytesReceived,
			"bytes_sent":     bytesSent,
			"avg_latency":    avgLatency,
			"max_latency":    maxLatency.Milliseconds(),
			"min_latency":    minLatencyMs,
		},
	}
}

// HealthCheck 健康检查接口
func (s *LockStepServer) HealthCheck() map[string]interface{} {
	s.mutex.RLock()
	isRunning := s.running
	s.mutex.RUnlock()

	allRooms := s.rooms.GetAllRooms()
	roomCount := len(allRooms)
	// 计算总玩家数
	playerCount := 0
	for _, room := range allRooms {
		playerCount += len(room.Players)
	}

	// 计算运行时间
	uptime := time.Since(s.startTime)

	// 获取系统资源使用情况
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// 计算丢包率 - 汇总所有房间的统计
	var totalPackets, lostPackets uint64
	s.mutex.RLock()
	for _, room := range s.rooms.rooms {
		if room.NetworkStats != nil {
			room.NetworkStats.mutex.RLock()
			totalPackets += room.NetworkStats.totalPackets
			lostPackets += room.NetworkStats.lostPackets
			room.NetworkStats.mutex.RUnlock()
		}
	}
	s.mutex.RUnlock()

	var packetLossRate float64
	if totalPackets > 0 {
		packetLossRate = float64(lostPackets) / float64(totalPackets) * 100
	}

	// 判断健康状态
	status := "healthy"
	if !isRunning {
		status = "unhealthy"
	} else if packetLossRate > 10.0 { // 丢包率超过10%认为不健康
		status = "degraded"
	} else if roomCount == 0 && uptime > 5*time.Minute { // 运行超过5分钟但没有房间
		status = "idle"
	}

	return map[string]interface{}{
		"status":           status,
		"running":          isRunning,
		"uptime_seconds":   uptime.Seconds(),
		"rooms":            roomCount,
		"players":          playerCount,
		"packet_loss_rate": packetLossRate,
		"memory_usage_mb":  float64(memStats.Alloc) / 1024 / 1024,
		"timestamp":        time.Now().Unix(),
	}
}

// StartMetricsServer 启动指标HTTP服务器
func (s *LockStepServer) StartMetricsServer() {
	mux := http.NewServeMux()
	
	// Metrics endpoint
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		stats := s.GetServerStats()
		json.NewEncoder(w).Encode(stats)
	})
	
	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		health := s.HealthCheck()
		json.NewEncoder(w).Encode(health)
	})
	
	// Create HTTP server
	s.metricsServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.MetricsPort),
		Handler: mux,
	}
	
	// Start server in goroutine
	go func() {
		s.logger.Printf("Starting metrics server on port %d", s.config.MetricsPort)
		if err := s.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Printf("Metrics server error: %v", err)
		}
	}()
}

// StopMetricsServer 停止指标HTTP服务器
func (s *LockStepServer) StopMetricsServer() {
	if s.metricsServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		if err := s.metricsServer.Shutdown(ctx); err != nil {
			s.logger.Printf("Error stopping metrics server: %v", err)
		} else {
			s.logger.Printf("Metrics server stopped")
		}
		s.metricsServer = nil
	}
}
