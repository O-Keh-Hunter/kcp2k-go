package lockstep

import (
	"context"
	"fmt"
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

	server := &LockStepServer{
		config:      *config,
		mutex:       sync.RWMutex{},
		running:     false,
		stopChan:    make(chan struct{}),
		portManager: portManager,
		startTime:   time.Now(),
	}

	// 创建房间管理器
	server.rooms = NewRoomManager(portManager, &config.KcpConfig)

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
	Log.Info("LockStep server started")

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

	Log.Info("LockStep server stopped")
}

// GracefulShutdown 优雅关闭服务器
func (s *LockStepServer) GracefulShutdown(ctx context.Context) error {
	Log.Info("Starting graceful shutdown...")

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

		Log.Info("All rooms stopped")
	}()

	// 等待关闭完成或超时
	select {
	case <-done:
		Log.Info("Graceful shutdown completed")
		return nil
	case <-ctx.Done():
		Log.Warning("Graceful shutdown timeout, forcing stop")
		s.Stop()
		return ctx.Err()
	}
}

// WaitForShutdown 等待关闭信号
func (s *LockStepServer) WaitForShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	Log.Info("Received signal: %v", sig)

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.GracefulShutdown(ctx); err != nil {
		Log.Error("Graceful shutdown failed: %v", err)
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

	if room.State.Status != RoomStatus_ROOM_STATUS_RUNNING {
		return
	}

	frameStartTime := time.Now()
	frameID := room.CurrentFrameID + 1

	// 计算帧间隔和统计信息
	var frameDuration time.Duration
	var isLateFrame bool
	if room.FrameStats != nil && !room.FrameStats.lastFrameTime.IsZero() {
		frameDuration = frameStartTime.Sub(room.FrameStats.lastFrameTime)
		expectedInterval := time.Duration(1000/room.Config.FrameRate) * time.Millisecond
		isLateFrame = frameDuration > expectedInterval*110/100 // 允许10%的误差
	}

	// 更新网络抖动统计
	if room.NetworkStats != nil {
		expectedInterval := time.Duration(1000/room.Config.FrameRate) * time.Millisecond
		room.NetworkStats.UpdateJitterStats(frameStartTime, expectedInterval)
	}

	// 创建新帧并收集输入
	frame := s.createFrameWithInputs(room, frameID)

	// 判断帧是否为空
	isEmpty := len(frame.DataCollection) == 0

	// 存储帧数据
	room.Frames[frameID] = frame
	room.CurrentFrameID = frameID
	if frameID > room.MaxFrameID {
		room.MaxFrameID = frameID
	}

	// 广播帧数据
	s.broadcastFrame(room, frame)

	// 更新统计信息
	s.updateFrameStats(room, frameStartTime, frameDuration, isLateFrame, isEmpty)
}

// createFrameWithInputs 创建帧并收集玩家输入
func (s *LockStepServer) createFrameWithInputs(room *Room, frameID FrameID) *Frame {
	frame := &Frame{
		FrameId:        uint32(frameID),
		RecvTickMs:     uint32(time.Now().UnixMilli()),
		ValidDataCount: 0,
		DataCollection: make([]*RelayData, 0),
	}

	// 收集玩家输入
	for playerID, player := range room.Players {
		player.Mutex.RLock()

		if inputList, exists := player.InputBuffer[frameID]; exists && len(inputList) > 0 {
			// 处理该帧的所有输入消息
			for _, inputData := range inputList {
				// 计算输入延迟
				delayMs := uint32(time.Now().UnixMilli() - int64(inputData.Timestamp))

				// 记录输入延迟统计
				if room.NetworkStats != nil {
					inputLatency := time.Duration(delayMs) * time.Millisecond
					room.NetworkStats.IncrementInputLatencyStats(inputLatency)
				}

				// 设置玩家在线状态标志
				var flag uint32 = 0
				if player.State.Online {
					flag |= 1 // 第0位表示在线状态
				}

				frame.DataCollection = append(frame.DataCollection, &RelayData{
					SequenceId: inputData.SequenceId,
					PlayerId:   uint32(playerID),
					Data:       inputData.Data,
					DelayMs:    delayMs,
					Flag:       flag,
				})
				frame.ValidDataCount++
			}
			// 清理已处理的输入
			delete(player.InputBuffer, frameID)
		}
		player.Mutex.RUnlock()
	}

	return frame
}

// broadcastFrame 广播帧数据到房间内所有玩家
func (s *LockStepServer) broadcastFrame(room *Room, frame *Frame) {
	respData, err := proto.Marshal(frame)
	if err != nil {
		Log.Error("Room %s: Failed to marshal frame response: %v", room.ID, err)
		return
	}

	msg := &LockStepMessage{
		Type:    LockStepMessage_FRAME,
		Payload: respData,
	}

	s.broadcastToRoom(room, msg)
}

// updateFrameStats 更新帧统计信息
func (s *LockStepServer) updateFrameStats(room *Room, frameStartTime time.Time, frameDuration time.Duration, isLateFrame, isEmpty bool) {
	if room.FrameStats == nil {
		return
	}

	// 更新帧统计
	room.IncrementFrameStats(isLateFrame, isEmpty, frameDuration)

	// 更新最后帧时间
	room.FrameStats.mutex.Lock()
	room.FrameStats.lastFrameTime = frameStartTime
	room.FrameStats.mutex.Unlock()
}

// broadcastToRoom 向房间广播消息
func (s *LockStepServer) broadcastToRoom(room *Room, msg *LockStepMessage) {
	data, err := proto.Marshal(msg)
	if err != nil {
		Log.Error("Failed to marshal message: %v", err)
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
		Log.Error("Failed to marshal player state message: %v", err)
		return
	}

	msg := &LockStepMessage{
		Type:    LockStepMessage_PLAYER_STATE,
		Payload: stateData,
	}

	s.broadcastToRoom(room, msg)
	Log.Debug("Room %s: Broadcasted player %d state change: %s", room.ID, playerID, reason)
}

// broadcastRoomState 广播房间状态变更
func (s *LockStepServer) broadcastRoomState(room *Room, reason string) {
	roomStateMsg := RoomStateMessage{
		RoomId: string(room.ID),
		State:  room.State,
		Reason: reason,
	}

	Log.Debug("BroadcastRoomState: %v", roomStateMsg.State)

	stateData, err := proto.Marshal(&roomStateMsg)
	if err != nil {
		Log.Error("Failed to marshal room state message: %v", err)
		return
	}

	msg := &LockStepMessage{
		Type:    LockStepMessage_ROOM_STATE,
		Payload: stateData,
	}

	s.broadcastToRoom(room, msg)
	Log.Debug("Room %s: Broadcasted room state change: %s", room.ID, reason)
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
		Log.Error("Failed to marshal error message: %v", err)
		return
	}

	msg := &LockStepMessage{
		Type:    LockStepMessage_ERROR,
		Payload: errorData,
	}

	msgData, err := proto.Marshal(msg)
	if err != nil {
		Log.Error("Failed to marshal lock step message: %v", err)
		return
	}

	room.KcpServer.Send(connectionID, msgData, kcp2k.KcpReliable)
	Log.Debug("Sent error %d to connection %d: %s", errorCode, connectionID, message)
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
		Log.Error("Failed to marshal error message: %v", err)
		return
	}

	msg := &LockStepMessage{
		Type:    LockStepMessage_ERROR,
		Payload: errorData,
	}

	s.broadcastToRoom(room, msg)
	Log.Debug("Room %s: Broadcasted error %d: %s", room.ID, errorCode, message)
}

// KCP回调函数
func (s *LockStepServer) onRoomConnected(room *Room, connectionID int) {
	Log.Debug("Room %s: Connection %d established", room.ID, connectionID)
}

func (s *LockStepServer) onRoomData(room *Room, connectionID int, data []byte, channel kcp2k.KcpChannel) {
	// 更新网络统计 - 记录接收到的包和字节数
	if room.NetworkStats != nil {
		room.NetworkStats.mutex.Lock()
		room.NetworkStats.totalPackets++
		room.NetworkStats.bytesReceived += uint64(len(data))
		room.NetworkStats.mutex.Unlock()
	}

	if len(data) == 0 {
		Log.Error("Room %s: Received empty message from connection %d, channel: %d", room.ID, connectionID, channel)
		return
	}

	var msg LockStepMessage
	err := proto.Unmarshal(data, &msg)
	if err != nil {
		Log.Error("Room %s: Failed to unmarshal message from connection %d, channel: %d: %v", room.ID, connectionID, channel, err)
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

		Log.Error("Room %s: Player %d disconnected", room.ID, disconnectedPlayer.ID)

		// 广播玩家离线状态给房间内的其他玩家
		s.broadcastPlayerState(room, disconnectedPlayer.ID, disconnectedPlayer.State, "Player disconnected")

		// 更新房间状态，检查是否所有玩家都离线
		s.rooms.UpdateRoomStatus(room, s)
	}
}

func (s *LockStepServer) onRoomError(room *Room, connectionID int, error kcp2k.ErrorCode, reason string) {
	// 记录网络错误为丢包
	if room.NetworkStats != nil {
		room.NetworkStats.mutex.Lock()
		room.NetworkStats.lostPackets++
		room.NetworkStats.mutex.Unlock()
	}

	Log.Error("Room %s: Connection %d error: %v - %s", room.ID, connectionID, error, reason)
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
	case LockStepMessage_READY:
		s.handlePlayerReady(room, connectionID, msg.Payload)
	default:
		Log.Error("Room %s: Unknown message type %d from connection %d", room.ID, msg.Type, connectionID)
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
		Log.Error("Room %s: Player not found for connection %d", room.ID, connectionID)
		return
	}

	// 解析输入数据
	var input InputMessage
	err := proto.Unmarshal(payload, &input)
	if err != nil {
		Log.Error("Room %s: Failed to unmarshal player input: %v", room.ID, err)
		return
	}

	frameID := room.CurrentFrameID + 1
	s.handleInput(room, player, frameID, &input)
}

// handleInput 处理输入
func (s *LockStepServer) handleInput(room *Room, player *Player, frameID FrameID, input *InputMessage) {
	player.Mutex.Lock()
	// 检查是否已经有输入在这个帧中
	if existingInputs, exists := player.InputBuffer[frameID]; exists {
		// Log.Error("Room %s: Player %d input seq %d APPENDING to existing %d inputs for frame %d",
		// 	room.ID, player.ID, input.SequenceId, len(existingInputs), frameID)
		player.InputBuffer[frameID] = append(existingInputs, input)
	} else {
		// Log.Error("Room %s: Player %d input seq %d assigned to frame %d", room.ID, player.ID, input.SequenceId, frameID)
		player.InputBuffer[frameID] = []*InputMessage{input}
	}
	player.Mutex.Unlock()
}

// handlePlayerReady 处理玩家准备状态
func (s *LockStepServer) handlePlayerReady(room *Room, connectionID int, payload []byte) {
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
		Log.Error("Room %s: Player not found for connection %d", room.ID, connectionID)
		return
	}

	// 解析Ready消息
	var readyMsg ReadyMessage
	err := proto.Unmarshal(payload, &readyMsg)
	if err != nil {
		Log.Error("Room %s: Failed to unmarshal ready message: %v", room.ID, err)
		return
	}

	// 验证玩家ID是否匹配
	if readyMsg.PlayerId != uint32(player.ID) {
		Log.Error("Room %s: Player ID mismatch in ready message: expected %d, got %d", room.ID, player.ID, readyMsg.PlayerId)
		return
	}

	// 更新玩家准备状态
	player.Mutex.Lock()
	player.Ready = readyMsg.Ready
	player.Mutex.Unlock()

	Log.Debug("Room %s: Player %d ready status changed to %v", room.ID, player.ID, readyMsg.Ready)

	// 通过更新房间状态来触发游戏开始检查
	s.rooms.UpdateRoomStatus(room, s)
}

// handleFrameRequest 处理补帧请求
func (s *LockStepServer) handleFrameRequest(room *Room, connectionID int, payload []byte) {
	// 解析帧请求
	req, err := s.parseFrameRequest(payload)
	if err != nil {
		Log.Error("Room %s: Failed to unmarshal frame request: %v", room.ID, err)
		return
	}

	// 收集请求的帧
	frames, missingFrames := s.collectRequestedFrames(room, req)

	// 记录缺失的帧
	s.logMissingFrames(room, req, missingFrames)

	// 记录帧请求信息
	s.logFrameRequestInfo(room, req, len(frames))

	// 发送补帧响应
	s.sendFrameResponse(room, connectionID, frames)
}

// parseFrameRequest 解析帧请求
func (s *LockStepServer) parseFrameRequest(payload []byte) (*FrameRequest, error) {
	var req FrameRequest
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// collectRequestedFrames 收集请求范围内的帧
func (s *LockStepServer) collectRequestedFrames(room *Room, req *FrameRequest) ([]*Frame, []FrameID) {
	room.Mutex.RLock()
	defer room.Mutex.RUnlock()

	frames := make([]*Frame, 0)
	missingFrames := make([]FrameID, 0)

	startFrameID := FrameID(req.FrameId)
	endFrameID := FrameID(req.FrameId + req.Count - 1)
	for frameID := startFrameID; frameID <= endFrameID; frameID++ {
		if frame, exists := room.Frames[frameID]; exists {
			frames = append(frames, frame)
		} else {
			missingFrames = append(missingFrames, frameID)
		}
	}

	return frames, missingFrames
}

// logMissingFrames 记录缺失的帧
func (s *LockStepServer) logMissingFrames(room *Room, req *FrameRequest, missingFrames []FrameID) {
	if len(missingFrames) > 0 {
		startFrameID := FrameID(req.FrameId)
		endFrameID := FrameID(req.FrameId + req.Count - 1)
		Log.Debug("Room %s: Missing frames in request range %d-%d: %v (total missing: %d)",
			room.ID, startFrameID, endFrameID, missingFrames, len(missingFrames))
	}
}

// logFrameRequestInfo 记录帧请求信息
func (s *LockStepServer) logFrameRequestInfo(room *Room, req *FrameRequest, foundFrames int) {
	startFrameID := FrameID(req.FrameId)
	endFrameID := FrameID(req.FrameId + req.Count - 1)
	Log.Debug("Room %s: Sending frame response, requested range: %d-%d, found frames: %d",
		room.ID, startFrameID, endFrameID, foundFrames)
}

// sendFrameResponse 发送补帧响应
func (s *LockStepServer) sendFrameResponse(room *Room, connectionID int, frames []*Frame) {
	resp := FrameResponse{
		Frames:  frames,
		Success: true,
	}

	respData, err := proto.Marshal(&resp)
	if err != nil {
		Log.Error("Room %s: Failed to marshal frame response: %v", room.ID, err)
		return
	}

	msg := &LockStepMessage{
		Type:    LockStepMessage_FRAME_RESP,
		Payload: respData,
	}

	msgData, err := proto.Marshal(msg)
	if err != nil {
		Log.Error("Room %s: Failed to marshal message: %v", room.ID, err)
		return
	}

	room.KcpServer.Send(connectionID, msgData, kcp2k.KcpReliable)
}

// handleJoinRoom 处理加入房间消息
func (s *LockStepServer) handleJoinRoom(room *Room, connectionID int, payload []byte) {
	// 解析JoinRoom消息
	joinMsg, err := s.parseJoinRoomMessage(payload)
	if err != nil {
		Log.Error("Room %s: Failed to parse join room message from connection %d: %v", room.ID, connectionID, err)
		s.sendError(room, connectionID, ErrorCodeInvalidMessage, "Invalid join room message", err.Error())
		return
	}

	// 尝试加入房间
	if err := s.JoinRoom(RoomID(joinMsg.RoomId), PlayerID(joinMsg.PlayerId), connectionID); err != nil {
		Log.Error("Room %s: Failed to join room for player %d: %v", room.ID, joinMsg.PlayerId, err)
		s.handleJoinRoomError(room, connectionID, err, joinMsg)
		return
	}

	// 发送成功响应
	s.sendJoinRoomSuccess(room, connectionID)
}

// parseJoinRoomMessage 解析加入房间消息
func (s *LockStepServer) parseJoinRoomMessage(payload []byte) (*JoinRoomRequest, error) {
	var joinMsg JoinRoomRequest
	if err := proto.Unmarshal(payload, &joinMsg); err != nil {
		return nil, err
	}
	return &joinMsg, nil
}

// handleJoinRoomError 处理加入房间错误
func (s *LockStepServer) handleJoinRoomError(room *Room, connectionID int, err error, joinMsg *JoinRoomRequest) {
	// 根据错误类型发送相应的错误码
	if err.Error() == "room is full" || err.Error() == fmt.Sprintf("room %s is full", joinMsg.RoomId) {
		s.sendError(room, connectionID, ErrorCodeRoomFull, "Room is full", "Cannot join room: maximum players reached")
	} else if err.Error() == "player already in room" || err.Error() == fmt.Sprintf("player %d already in room %s", joinMsg.PlayerId, joinMsg.RoomId) {
		s.sendError(room, connectionID, ErrorCodePlayerAlreadyInRoom, "Player already in room", "Player is already a member of this room")
	} else {
		s.sendError(room, connectionID, ErrorCodeUnknown, "Failed to join room", err.Error())
	}
}

// sendJoinRoomSuccess 发送加入房间成功响应
func (s *LockStepServer) sendJoinRoomSuccess(room *Room, connectionID int) {
	roomStateMsg := RoomStateMessage{
		RoomId: string(room.ID),
		State:  room.State,
		Reason: "Player joined room",
	}

	stateData, err := proto.Marshal(&roomStateMsg)
	if err != nil {
		Log.Error("Failed to marshal room state message: %v", err)
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
// aggregatedStats 聚合统计数据结构
type aggregatedStats struct {
	// 基础信息
	totalRooms   int
	totalPlayers int

	// 帧统计
	totalFrames  uint64
	lateFrames   uint64
	emptyFrames  uint64
	frameTimeSum time.Duration

	// 网络统计
	totalPackets  uint64
	lostPackets   uint64
	bytesReceived uint64
	bytesSent     uint64
	rttSum        time.Duration
	rttCount      int
	maxRtt        time.Duration
	minRtt        time.Duration

	// 输入延迟统计
	inputLatencySum   time.Duration
	inputLatencyCount int
	maxInputLatency   time.Duration
	minInputLatency   time.Duration

	// 抖动统计
	jitterSum   time.Duration
	jitterCount int
	maxJitter   time.Duration
	minJitter   time.Duration
}

func (s *LockStepServer) GetServerStats() map[string]interface{} {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	allRooms := s.rooms.GetAllRooms()
	stats := s.aggregateRoomStats(allRooms)

	return map[string]interface{}{
		"total_rooms":         stats.totalRooms,
		"total_players":       stats.totalPlayers,
		"running":             s.running,
		"uptime":              time.Since(s.startTime).Milliseconds(),
		"frame_stats":         s.buildFrameStats(stats),
		"network_stats":       s.buildNetworkStats(stats),
		"input_latency_stats": s.buildInputLatencyStats(stats),
		"jitter_stats":        s.buildJitterStats(stats),
	}
}

// aggregateRoomStats 聚合所有房间的统计数据
func (s *LockStepServer) aggregateRoomStats(allRooms map[RoomID]*Room) *aggregatedStats {
	stats := &aggregatedStats{
		totalRooms:      len(allRooms),
		minRtt:          time.Hour, // 初始化为很大的值
		minInputLatency: time.Hour,
		minJitter:       time.Hour,
	}

	for _, room := range allRooms {
		stats.totalPlayers += len(room.Players)
		s.aggregateFrameStats(room, stats)
		s.aggregateNetworkStats(room, stats)
	}

	return stats
}

// aggregateFrameStats 聚合帧统计数据
func (s *LockStepServer) aggregateFrameStats(room *Room, stats *aggregatedStats) {
	if room.FrameStats == nil {
		return
	}

	frameStats := room.GetFrameStats()
	stats.totalFrames += frameStats.GetTotalFrames()
	stats.lateFrames += frameStats.GetLateFrames()
	stats.emptyFrames += frameStats.GetEmptyFrames()
	stats.frameTimeSum += frameStats.GetFrameTimeSum()
}

// aggregateNetworkStats 聚合网络统计数据
func (s *LockStepServer) aggregateNetworkStats(room *Room, stats *aggregatedStats) {
	netStats := room.GetNetworkStats()
	if netStats == nil {
		return
	}

	// 基础网络统计
	stats.totalPackets += netStats.GetTotalPackets()
	stats.lostPackets += netStats.GetLostPackets()
	stats.bytesReceived += netStats.GetBytesReceived()
	stats.bytesSent += netStats.GetBytesSent()

	netStats.mutex.RLock()
	defer netStats.mutex.RUnlock()

	// RTT统计
	stats.rttSum += netStats.rttSum
	stats.rttCount += int(netStats.rttCount)
	if netStats.maxRTT > stats.maxRtt {
		stats.maxRtt = netStats.maxRTT
	}
	if netStats.minRTT < stats.minRtt && netStats.minRTT > 0 {
		stats.minRtt = netStats.minRTT
	}

	// 输入延迟统计
	stats.inputLatencySum += netStats.inputLatencySum
	stats.inputLatencyCount += int(netStats.inputLatencyCount)
	if netStats.maxInputLatency > stats.maxInputLatency {
		stats.maxInputLatency = netStats.maxInputLatency
	}
	if netStats.minInputLatency < stats.minInputLatency && netStats.minInputLatency > 0 {
		stats.minInputLatency = netStats.minInputLatency
	}

	// 抖动统计
	stats.jitterSum += netStats.jitterSum
	stats.jitterCount += int(netStats.jitterCount)
	if netStats.maxJitter > stats.maxJitter {
		stats.maxJitter = netStats.maxJitter
	}
	if netStats.minJitter < stats.minJitter && netStats.minJitter > 0 {
		stats.minJitter = netStats.minJitter
	}
}

// buildFrameStats 构建帧统计信息
func (s *LockStepServer) buildFrameStats(stats *aggregatedStats) map[string]interface{} {
	avgFrameTime := float64(0)
	if stats.totalFrames > 0 {
		avgFrameTime = float64(stats.frameTimeSum.Nanoseconds()) / float64(stats.totalFrames) / 1e6
	}

	return map[string]interface{}{
		"total_frames":   stats.totalFrames,
		"late_frames":    stats.lateFrames,
		"empty_frames":   stats.emptyFrames,
		"avg_frame_time": avgFrameTime,
	}
}

// buildNetworkStats 构建网络统计信息
func (s *LockStepServer) buildNetworkStats(stats *aggregatedStats) map[string]interface{} {
	avgRtt := float64(0)
	if stats.rttCount > 0 {
		avgRtt = float64(stats.rttSum.Nanoseconds()) / float64(stats.rttCount) / 1e6
	}

	minRttMs := int64(0)
	if stats.minRtt != time.Hour && stats.minRtt > 0 {
		minRttMs = stats.minRtt.Milliseconds()
	}

	return map[string]interface{}{
		"total_packets":  stats.totalPackets,
		"lost_packets":   stats.lostPackets,
		"bytes_received": stats.bytesReceived,
		"bytes_sent":     stats.bytesSent,
		"avg_rtt":        avgRtt,
		"max_rtt":        stats.maxRtt.Milliseconds(),
		"min_rtt":        minRttMs,
	}
}

// buildInputLatencyStats 构建输入延迟统计信息
func (s *LockStepServer) buildInputLatencyStats(stats *aggregatedStats) map[string]interface{} {
	avgInputLatency := float64(0)
	if stats.inputLatencyCount > 0 {
		avgInputLatency = float64(stats.inputLatencySum.Nanoseconds()) / float64(stats.inputLatencyCount) / 1e6
	}

	minInputLatencyMs := int64(0)
	if stats.minInputLatency != time.Hour && stats.minInputLatency > 0 {
		minInputLatencyMs = stats.minInputLatency.Milliseconds()
	}

	return map[string]interface{}{
		"input_latency_count": stats.inputLatencyCount,
		"avg_input_latency":   avgInputLatency,
		"max_input_latency":   stats.maxInputLatency.Milliseconds(),
		"min_input_latency":   minInputLatencyMs,
	}
}

// buildJitterStats 构建抖动统计信息
func (s *LockStepServer) buildJitterStats(stats *aggregatedStats) map[string]interface{} {
	avgJitter := float64(0)
	if stats.jitterCount > 0 {
		avgJitter = float64(stats.jitterSum.Nanoseconds()) / float64(stats.jitterCount) / 1e6
	}

	minJitterMs := int64(0)
	if stats.minJitter != time.Hour && stats.minJitter > 0 {
		minJitterMs = stats.minJitter.Milliseconds()
	}

	return map[string]interface{}{
		"jitter_count": stats.jitterCount,
		"avg_jitter":   avgJitter,
		"max_jitter":   stats.maxJitter.Milliseconds(),
		"min_jitter":   minJitterMs,
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
