package lockstep

import (
	"fmt"
	"sync"
	"time"

	kcp2k "github.com/O-Keh-Hunter/kcp2k-go"
	"google.golang.org/protobuf/proto"
)

// RoomManager 房间管理器
type RoomManager struct {
	rooms       map[RoomID]*Room
	portManager *PortManager
	mutex       sync.RWMutex
	kcpConfig   *kcp2k.KcpConfig
	stopChan    chan struct{}
}

// NewRoomManager 创建房间管理器
func NewRoomManager(portManager *PortManager, kcpConfig *kcp2k.KcpConfig) *RoomManager {
	return &RoomManager{
		rooms:       make(map[RoomID]*Room),
		portManager: portManager,
		kcpConfig:   kcpConfig,
		stopChan:    make(chan struct{}),
	}
}

// Start 启动房间管理器
func (rm *RoomManager) Start() {
	go rm.roomCleanupLoop()
}

// Stop 停止房间管理器
func (rm *RoomManager) Stop() {
	close(rm.stopChan)

	// 停止所有房间
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	for _, room := range rm.rooms {
		rm.stopRoom(room)
	}
}

// CreateRoom 创建房间
func (rm *RoomManager) CreateRoom(roomID RoomID, config *RoomConfig, server *LockStepServer) (*Room, error) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	if _, exists := rm.rooms[roomID]; exists {
		return nil, fmt.Errorf("room %s already exists", roomID)
	}

	// 分配端口
	port := rm.portManager.AllocatePort()

	// 创建房间
	room := &Room{
		ID:             roomID,
		Port:           port,
		Players:        make(map[PlayerID]*LockStepPlayer),
		Frames:         make(map[FrameID]*FrameMessage),
		CurrentFrameID: 0,
		MaxFrameID:     0,
		State: &RoomStateMessage{
			Status:         RoomStatus_ROOM_STATUS_IDLE,
			CurrentPlayers: 0,
			MaxPlayers:     config.MaxPlayers,
			StartTime:      0,
			EndTime:        0,
			CurrentFrameId: 0,
		},
		Config:    config,
		Mutex:     sync.RWMutex{},
		StopChan:  make(chan struct{}),
		CreatedAt: time.Now(),
		running:   false,
		// 初始化统计信息
		FrameStats:   &FrameStats{},
		NetworkStats: &NetworkStats{},
	}

	// 创建房间专用的KCP服务器
	kcpServer := kcp2k.NewKcpServer(
		func(connectionID int) { server.onRoomConnected(room, connectionID) },
		func(connectionID int, data []byte, channel kcp2k.KcpChannel) {
			server.onRoomData(room, connectionID, data, channel)
		},
		func(connectionID int) { server.onRoomDisconnected(room, connectionID) },
		func(connectionID int, error kcp2k.ErrorCode, reason string) {
			server.onRoomError(room, connectionID, error, reason)
		},
		*rm.kcpConfig,
	)

	room.KcpServer = kcpServer

	// 启动房间的KCP服务器
	err := kcpServer.Start(port)
	if err != nil {
		rm.portManager.ReleasePort(port)
		return nil, fmt.Errorf("failed to start room KCP server: %v", err)
	}

	rm.rooms[roomID] = room
	Log.Debug("Room %s created on port %d", roomID, port)

	// 启动房间逻辑
	go rm.startRoom(room, server)

	return room, nil
}

// GetRoom 获取房间
func (rm *RoomManager) GetRoom(roomID RoomID) (*Room, bool) {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	room, exists := rm.rooms[roomID]
	return room, exists
}

// GetRooms 获取所有房间
func (rm *RoomManager) GetRooms() map[RoomID]*Room {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	rooms := make(map[RoomID]*Room)
	for id, room := range rm.rooms {
		rooms[id] = room
	}
	return rooms
}

// GetAllRooms 获取所有房间（别名方法）
func (rm *RoomManager) GetAllRooms() map[RoomID]*Room {
	return rm.GetRooms()
}

// JoinRoom 玩家加入房间
func (rm *RoomManager) JoinRoom(roomID RoomID, playerID PlayerID, connectionID int, server *LockStepServer) ErrorCode {
	room, exists := rm.GetRoom(roomID)
	if !exists {
		return ErrorCode_ERROR_CODE_ROOM_NOT_FOUND
	}

	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	// 检查玩家是否已存在（重连情况）
	if existingPlayer, exists := room.Players[playerID]; exists {
		return rm.handlePlayerReconnection(room, playerID, connectionID, existingPlayer, server)
	}

	// 检查房间是否已满
	if len(room.Players) >= int(room.Config.MaxPlayers) {
		Log.Error("")
		return ErrorCode_ERROR_CODE_ROOM_FULL
	}

	// 创建新玩家并加入房间
	return rm.addNewPlayerToRoom(room, playerID, connectionID, server)
}

// handlePlayerReconnection 处理玩家重连
func (rm *RoomManager) handlePlayerReconnection(room *Room, playerID PlayerID, connectionID int, existingPlayer *LockStepPlayer, server *LockStepServer) ErrorCode {
	// 更新连接信息
	existingPlayer.Mutex.Lock()
	existingPlayer.ConnectionID = connectionID
	existingPlayer.Status = PlayerStatus_PLAYER_STATUS_ONLINE
	existingPlayer.Mutex.Unlock()

	// 广播玩家重新上线状态
	server.broadcastPlayerState(room, playerID, existingPlayer.Status, "Player reconnected")

	// 如果游戏正在运行，发送游戏开始消息
	if room.State.Status == RoomStatus_ROOM_STATUS_RUNNING {
		rm.sendGameStartMessage(room, connectionID, "reconnected", playerID)
	}

	Log.Debug("Player %d reconnected to room %s", playerID, room.ID)
	return ErrorCode_ERROR_CODE_SUCC
}

// addNewPlayerToRoom 添加新玩家到房间
func (rm *RoomManager) addNewPlayerToRoom(room *Room, playerID PlayerID, connectionID int, server *LockStepServer) ErrorCode {
	// 创建新玩家
	player := rm.createNewPlayer(playerID, connectionID, room)

	// 添加到房间
	room.Players[playerID] = player

	// 广播玩家状态
	server.broadcastPlayerState(room, playerID, player.Status, "Player joined room")

	// 如果游戏正在运行，发送游戏开始消息
	if room.State.Status == RoomStatus_ROOM_STATUS_RUNNING {
		rm.sendGameStartMessage(room, connectionID, "new", playerID)
	}

	// 更新房间状态
	rm.updateRoomStatusLocked(room, server)

	Log.Debug("Player %d joined room %s (players: %d/%d)", playerID, room.ID, len(room.Players), room.Config.MaxPlayers)
	return ErrorCode_ERROR_CODE_SUCC
}

// createNewPlayer 创建新玩家
func (rm *RoomManager) createNewPlayer(playerID PlayerID, connectionID int, room *Room) *LockStepPlayer {
	player := &LockStepPlayer{
		Player: &Player{
			PlayerId: playerID,
			Status:   PlayerStatus_PLAYER_STATUS_ONLINE,
		},
		ConnectionID: connectionID,
		InputBuffer:  make(map[FrameID][]*InputMessage),
		Mutex:        sync.RWMutex{},
	}

	return player
}

// sendGameStartMessage 发送游戏开始消息
func (rm *RoomManager) sendGameStartMessage(room *Room, connectionID int, playerType string, playerID PlayerID) {
	gameStartMsg := &GameStartMessage{
		CurrentFrameId:     int32(room.CurrentFrameID),
		GameAlreadyRunning: true,
	}

	startMsg := &LockStepMessage{
		Type: LockStepMessage_GAME_START,
		Body: &LockStepMessage_GameStart{
			GameStart: gameStartMsg,
		},
	}
	msgData, err := proto.Marshal(startMsg)
	if err != nil {
		return
	}

	room.KcpServer.Send(connectionID, msgData, kcp2k.KcpReliable)
	Log.Debug("Sent game start message to %s player %d in room %s (current frame: %d)", playerType, playerID, room.ID, room.CurrentFrameID)
}

// startRoom 启动房间
func (rm *RoomManager) startRoom(room *Room, server *LockStepServer) {
	room.Mutex.Lock()
	room.running = true
	room.Mutex.Unlock()

	// 启动帧循环
	go rm.frameLoop(room, server)

	// 启动KCP tick循环
	go func() {
		tickInterval := time.Duration(rm.kcpConfig.Interval) * time.Millisecond
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				room.KcpServer.Tick()
			case <-room.StopChan:
				return
			}
		}
	}()
}

// stopRoom 停止房间
func (rm *RoomManager) stopRoom(room *Room) {
	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	if !room.running {
		return
	}

	room.running = false
	close(room.StopChan)

	if room.KcpServer != nil {
		room.KcpServer.Stop()
	}

	// 释放端口
	rm.portManager.ReleasePort(room.Port)

	Log.Debug("Room %s stopped", room.ID)
}

// frameLoop 帧循环
func (rm *RoomManager) frameLoop(room *Room, server *LockStepServer) {
	frameInterval := time.Duration(1000/room.Config.FrameRate) * time.Millisecond
	room.Ticker = time.NewTicker(frameInterval)
	defer room.Ticker.Stop()

	for {
		select {
		case <-room.Ticker.C:
			server.processFrame(room)
		case <-room.StopChan:
			return
		}
	}
}

// roomCleanupLoop 房间清理循环
func (rm *RoomManager) roomCleanupLoop() {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rm.cleanupRooms()
		case <-rm.stopChan:
			return
		}
	}
}

// cleanupRooms 清理空房间
func (rm *RoomManager) cleanupRooms() {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	// 收集需要清理的房间ID，避免在遍历时修改map
	roomsToCleanup := make([]RoomID, 0)

	for roomID, room := range rm.rooms {
		room.Mutex.RLock()
		onlineCount := 0
		for _, player := range room.Players {
			if player.Status == PlayerStatus_PLAYER_STATUS_ONLINE || player.Status == PlayerStatus_PLAYER_STATUS_READY {
				onlineCount++
			}
		}
		createdAt := room.CreatedAt
		room.Mutex.RUnlock()

		// 只清理创建时间超过5分钟且没有在线玩家的房间
		if onlineCount == 0 && time.Since(createdAt) > 5*time.Minute {
			roomsToCleanup = append(roomsToCleanup, roomID)
		}
	}

	// 清理收集到的房间
	for _, roomID := range roomsToCleanup {
		if room, exists := rm.rooms[roomID]; exists {
			rm.stopRoom(room)
			delete(rm.rooms, roomID)
			Log.Debug("Room %s cleaned up (empty for %v)", roomID, time.Since(room.CreatedAt))
		}
	}
}

// UpdateRoomStatus 根据玩家数量和当前状态更新房间状态
func (rm *RoomManager) UpdateRoomStatus(room *Room, server *LockStepServer) {
	room.Mutex.Lock()
	defer room.Mutex.Unlock()
	rm.updateRoomStatusLocked(room, server)
}

// updateRoomStatusLocked 内部方法，假设已经持有锁
func (rm *RoomManager) updateRoomStatusLocked(room *Room, server *LockStepServer) {
	playerCount := len(room.Players)
	currentStatus := room.State.Status

	// 更新当前玩家数量
	room.State.CurrentPlayers = int32(playerCount)

	// 单向状态转换逻辑: idle -> waiting -> running -> ended
	switch currentStatus {
	case RoomStatus_ROOM_STATUS_IDLE:
		// 有玩家加入时，从idle转换到waiting
		if playerCount > 0 {
			room.State.Status = RoomStatus_ROOM_STATUS_WAITING
			Log.Debug("Room %s: Status changed to WAITING (players joined: %d)", room.ID, playerCount)
			server.broadcastRoomState(room, "Players joined, waiting for more")
		}

	case RoomStatus_ROOM_STATUS_WAITING:
		// 检查是否满足游戏开始条件
		readyCount := 0
		for _, player := range room.Players {
			if player.Player.Status == PlayerStatus_PLAYER_STATUS_READY {
				readyCount++
			}
		}

		minPlayers := int(room.Config.MinPlayers)
		if playerCount >= minPlayers && readyCount == playerCount {
			rm.startGame(room, server, uint32(readyCount), uint32(minPlayers))
		}

	case RoomStatus_ROOM_STATUS_RUNNING:
		// 检查是否所有玩家都离线一段时间
		onlineCount := 0
		for _, player := range room.Players {
			if player.Status == PlayerStatus_PLAYER_STATUS_ONLINE || player.Status == PlayerStatus_PLAYER_STATUS_READY {
				onlineCount++
			}
		}
		// 如果没有在线玩家，游戏结束
		if onlineCount == 0 {
			room.State.Status = RoomStatus_ROOM_STATUS_ENDED
			room.State.EndTime = time.Now().Unix()
			Log.Debug("Room %s: Game ENDED (all players offline)", room.ID)
			server.broadcastRoomState(room, "Game ended - all players offline")
		}

	case RoomStatus_ROOM_STATUS_ENDED:
		// 游戏结束状态，不再转换
		// 房间将由清理机制处理
	}
}

// startGameInternal 开始游戏
func (rm *RoomManager) startGame(room *Room, server *LockStepServer, readyPlayers, minPlayers uint32) {
	// 检查房间状态是否为等待状态
	if room.State.Status != RoomStatus_ROOM_STATUS_WAITING {
		return
	}

	// 开始游戏
	room.State.Status = RoomStatus_ROOM_STATUS_RUNNING
	room.State.StartTime = time.Now().Unix()
	Log.Debug("Room %s: Game STARTED with %d players (%d ready)", room.ID, len(room.Players), readyPlayers)

	// 广播房间状态变更
	server.broadcastRoomState(room, "Game started")

	// 广播游戏开始消息
	gameStartMsg := GameStartMessage{
		CurrentFrameId:     0,
		GameAlreadyRunning: false,
		ReadyPlayers:       readyPlayers,
		MinPlayers:         minPlayers,
	}

	startMsg := &LockStepMessage{
		Type: LockStepMessage_GAME_START,
		Body: &LockStepMessage_GameStart{
			GameStart: &gameStartMsg,
		},
	}
	server.broadcastToRoom(room, startMsg)
}

// RemoveRoom 移除房间
func (rm *RoomManager) RemoveRoom(roomID RoomID) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	if room, exists := rm.rooms[roomID]; exists {
		rm.stopRoom(room)
		delete(rm.rooms, roomID)
		Log.Debug("Room %s removed", roomID)
	}
}

// Room 网络统计相关方法

// GetNetworkStats 获取房间网络统计信息
func (r *Room) GetNetworkStats() *NetworkStats {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()
	return r.NetworkStats
}

// UpdateNetworkStats 更新房间网络统计信息
func (r *Room) UpdateNetworkStats(totalPackets, lostPackets, bytesReceived, bytesSent uint64, latency time.Duration) {
	if r.NetworkStats == nil {
		return
	}
	r.NetworkStats.UpdateNetworkStats(totalPackets, lostPackets, bytesReceived, bytesSent, latency)
}

// IncrementNetworkStats 增量更新房间网络统计信息
func (r *Room) IncrementNetworkStats(packetsReceived, packetsSent, bytesReceived, bytesSent uint64, lost bool, latency time.Duration) {
	if r.NetworkStats == nil {
		return
	}
	r.NetworkStats.IncrementNetworkStats(packetsReceived, packetsSent, bytesReceived, bytesSent, lost, latency)
}

// Room 帧统计相关方法

// GetFrameStats 获取房间帧统计信息
func (r *Room) GetFrameStats() *FrameStats {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()
	return r.FrameStats
}

// IncrementFrameStats 增量更新房间帧统计信息
func (r *Room) IncrementFrameStats(late, empty bool, frameDuration time.Duration) {
	if r.FrameStats == nil {
		return
	}
	r.FrameStats.IncrementFrameStats(false, late, empty, frameDuration)
}

// GetRoomMonitoringInfo 获取单个房间的监控信息
func (r *Room) GetRoomMonitoringInfo() map[string]interface{} {
	// 分段获取数据，减少锁持有时间
	r.Mutex.RLock()
	// 快速复制基本信息
	roomID := r.ID
	port := r.Port
	currentFrame := r.CurrentFrameID
	maxFrame := r.MaxFrameID
	createdAt := r.CreatedAt
	running := r.running
	playerCount := len(r.Players)
	maxPlayers := r.Config.MaxPlayers
	minPlayers := r.Config.MinPlayers
	frameRate := r.Config.FrameRate
	retryWindow := r.Config.RetryWindow

	// 复制玩家信息
	players := make([]map[string]interface{}, 0, len(r.Players))
	for _, player := range r.Players {
		playerInfo := map[string]interface{}{
			"id":            player.Player.PlayerId,
			"connection_id": player.ConnectionID,
			"status":        player.Player.Status,
		}
		players = append(players, playerInfo)
	}
	r.Mutex.RUnlock()

	// 构建返回信息（不持有锁）
	roomInfo := map[string]interface{}{
		"room_id":       roomID,
		"port":          port,
		"state":         r.State.String(),
		"current_frame": currentFrame,
		"max_frame":     maxFrame,
		"created_at":    createdAt.Format(time.RFC3339),
		"running":       running,
		"player_count":  playerCount,
		"max_players":   maxPlayers,
		"players":       players,
	}

	// 添加统计信息（可能需要短暂锁定）
	if r.FrameStats != nil {
		frameStats := r.GetFrameStats()
		roomInfo["frame_stats"] = frameStats
	}

	if r.NetworkStats != nil {
		networkStats := r.GetNetworkStats()
		roomInfo["network_stats"] = networkStats
	}

	// 添加房间配置信息
	roomInfo["config"] = map[string]interface{}{
		"max_players":  maxPlayers,
		"min_players":  minPlayers,
		"frame_rate":   frameRate,
		"retry_window": retryWindow,
	}

	return roomInfo
}
