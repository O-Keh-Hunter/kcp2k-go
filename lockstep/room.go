package lockstep

import (
	"fmt"
	"log"
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
	logger      *log.Logger
	kcpConfig   *kcp2k.KcpConfig
	stopChan    chan struct{}
}

// NewRoomManager 创建房间管理器
func NewRoomManager(portManager *PortManager, logger *log.Logger, kcpConfig *kcp2k.KcpConfig) *RoomManager {
	return &RoomManager{
		rooms:       make(map[RoomID]*Room),
		portManager: portManager,
		logger:      logger,
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
		Players:        make(map[PlayerID]*Player),
		Frames:         make(map[FrameID]*Frame),
		CurrentFrameID: 0,
		MaxFrameID:     0,
		State: &RoomState{
			Status:         uint32(RoomStatusWaiting),
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
	rm.logger.Printf("Room %s created on port %d", roomID, port)

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
func (rm *RoomManager) JoinRoom(roomID RoomID, playerID PlayerID, connectionID int, server *LockStepServer) error {
	room, exists := rm.GetRoom(roomID)
	if !exists {
		return fmt.Errorf("room %s not found", roomID)
	}

	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	// 先检查玩家是否已在房间中，如果存在则更新连接信息（支持重连）
	if existingPlayer, exists := room.Players[playerID]; exists {
		// 玩家重连，更新连接信息
		// 使用锁确保状态更新的原子性
		existingPlayer.Mutex.Lock()
		existingPlayer.ConnectionID = connectionID
		existingPlayer.State.Online = true
		existingPlayer.Mutex.Unlock()

		// 广播玩家重新上线状态
		server.broadcastPlayerState(room, playerID, existingPlayer.State, "Player reconnected")

		// 如果游戏已经在运行，发送游戏开始消息
		if room.State.Status == uint32(RoomStatusRunning) {
			gameStartMsg := GameStartMessage{
				CurrentFrameId:     uint32(room.CurrentFrameID),
				GameAlreadyRunning: true,
			}
			gameStartPayload, err := proto.Marshal(&gameStartMsg)
			if err == nil {
				startMsg := LockStepMessage{
					Type:    LockStepMessage_START,
					Payload: gameStartPayload,
				}
				msgData, err := proto.Marshal(&startMsg)
				if err == nil {
					room.KcpServer.Send(connectionID, msgData, kcp2k.KcpReliable)
					rm.logger.Printf("Sent game start message to reconnected player %d in room %s (current frame: %d)", playerID, roomID, room.CurrentFrameID)
				}
			}
		}

		rm.logger.Printf("Player %d reconnected to room %s", playerID, roomID)
		return nil
	}

	// 检查房间是否已满（仅对新玩家）
	if len(room.Players) >= int(room.Config.MaxPlayers) {
		return fmt.Errorf("room %s is full", roomID)
	}

	// 创建玩家
	player := &Player{
		ID:           playerID,
		ConnectionID: connectionID,
		State: &PlayerState{
			Online: true,
		},
		LastFrameID: 0, // 初始设置为0，后面会根据游戏状态调整
		InputBuffer: make(map[FrameID][]byte),
		Mutex:       sync.RWMutex{},
	}

	// 设置新玩家的LastFrameID
	// 如果游戏已经运行，设置为同步帧的起始帧ID-1，这样客户端可以从同步帧开始处理
	if room.State.Status == uint32(RoomStatusRunning) && room.CurrentFrameID > 0 {
		syncStartFrame := room.CurrentFrameID - 9
		if syncStartFrame < 1 {
			syncStartFrame = 1
		}
		player.LastFrameID = syncStartFrame - 1
	} else {
		player.LastFrameID = 0
	}

	// 添加到房间
	room.Players[playerID] = player

	// 广播玩家状态 - 确保向所有现有玩家通知新玩家加入
	server.broadcastPlayerState(room, playerID, player.State, "Player joined room")

	// 如果游戏已经在运行，发送游戏开始消息，包含当前帧ID信息
	if room.State.Status == uint32(RoomStatusRunning) {
		gameStartMsg := GameStartMessage{
			CurrentFrameId:     uint32(room.CurrentFrameID),
			GameAlreadyRunning: true,
		}
		gameStartPayload, err := proto.Marshal(&gameStartMsg)
		if err == nil {
			startMsg := LockStepMessage{
				Type:    LockStepMessage_START,
				Payload: gameStartPayload,
			}
			msgData, err := proto.Marshal(&startMsg)
			if err == nil {
				room.KcpServer.Send(connectionID, msgData, kcp2k.KcpReliable)
				rm.logger.Printf("Sent game start message to new player %d in room %s (current frame: %d)", playerID, roomID, room.CurrentFrameID)
			}
		}
	}

	// 根据玩家数量更新房间状态（注意：此时已经持有room.Mutex锁）
	rm.updateRoomStatusLocked(room, server)

	return nil
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
				room.KcpServer.TickIncoming()
				room.KcpServer.TickOutgoing()
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

	rm.logger.Printf("Room %s stopped", room.ID)
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

	for roomID, room := range rm.rooms {
		room.Mutex.RLock()
		onlineCount := 0
		for _, player := range room.Players {
			if player.State.Online {
				onlineCount++
			}
		}
		createdAt := room.CreatedAt
		room.Mutex.RUnlock()

		// 只清理创建时间超过5分钟且没有在线玩家的房间
		if onlineCount == 0 && time.Since(createdAt) > 5*time.Minute {
			rm.stopRoom(room)
			delete(rm.rooms, roomID)
			rm.logger.Printf("Room %s cleaned up (empty for %v)", roomID, time.Since(createdAt))
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
	currentStatus := RoomStatus(room.State.Status)

	// 更新当前玩家数量
	room.State.CurrentPlayers = uint32(playerCount)

	// 状态转换逻辑
	switch currentStatus {
	case RoomStatusWaiting:
		if playerCount >= int(room.Config.MinPlayers) {
			// 达到最小玩家数，转为准备状态
			room.State.Status = uint32(RoomStatusReady)
			rm.logger.Printf("Room %s: Status changed to READY (%d/%d players)", room.ID, playerCount, room.Config.MaxPlayers)
			server.broadcastRoomState(room, "Room ready to start")

			// 自动开始游戏（可配置延迟）
			go func() {
				rm.startGame(room, server)
			}()
		}

	case RoomStatusReady:
		if playerCount < int(room.Config.MinPlayers) {
			// 玩家数不足，回到等待状态
			room.State.Status = uint32(RoomStatusWaiting)
			rm.logger.Printf("Room %s: Status changed to WAITING (insufficient players: %d/%d)", room.ID, playerCount, room.Config.MinPlayers)
			server.broadcastRoomState(room, "Waiting for more players")
		}

	case RoomStatusRunning:
		if playerCount == 0 {
			// 无玩家，结束游戏
			room.State.Status = uint32(RoomStatusEnded)
			rm.logger.Printf("Room %s: Game ENDED (no players)", room.ID)
			server.broadcastRoomState(room, "Game ended - no players")
		}
	}
}

// startGame 开始游戏
func (rm *RoomManager) startGame(room *Room, server *LockStepServer) {
	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	// 检查房间状态是否仍为READY
	if room.State.Status != uint32(RoomStatusReady) {
		return
	}

	// 检查玩家数量是否仍满足要求
	if len(room.Players) < int(room.Config.MinPlayers) {
		room.State.Status = uint32(RoomStatusWaiting)
		server.broadcastRoomState(room, "Waiting for more players")
		return
	}

	// 开始游戏
	room.State.Status = uint32(RoomStatusRunning)
	room.State.StartTime = time.Now().Unix()
	rm.logger.Printf("Room %s: Game STARTED with %d players", room.ID, len(room.Players))

	// 广播房间状态变更
	server.broadcastRoomState(room, "Game started")

	// 广播游戏开始消息
	gameStartMsg := GameStartMessage{
		CurrentFrameId:     0,
		GameAlreadyRunning: false,
	}
	gameStartPayload, err := proto.Marshal(&gameStartMsg)
	if err == nil {
		startMsg := &LockStepMessage{
			Type:    LockStepMessage_START,
			Payload: gameStartPayload,
		}
		server.broadcastToRoom(room, startMsg)
	}
}

// RemoveRoom 移除房间
func (rm *RoomManager) RemoveRoom(roomID RoomID) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	if room, exists := rm.rooms[roomID]; exists {
		rm.stopRoom(room)
		delete(rm.rooms, roomID)
		rm.logger.Printf("Room %s removed", roomID)
	}
}

// GetRoomMonitoringInfo 获取单个房间的监控信息
func (r *Room) GetRoomMonitoringInfo() map[string]interface{} {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()

	// 获取房间基本信息
	roomInfo := map[string]interface{}{
		"room_id":         r.ID,
		"port":           r.Port,
		"state":          r.State.String(),
		"current_frame":  r.CurrentFrameID,
		"max_frame":      r.MaxFrameID,
		"created_at":     r.CreatedAt.Format(time.RFC3339),
		"running":        r.running,
		"player_count":   len(r.Players),
		"max_players":    r.Config.MaxPlayers,
	}

	// 添加玩家信息
	players := make([]map[string]interface{}, 0, len(r.Players))
	for _, player := range r.Players {
		playerInfo := map[string]interface{}{
			"id":            player.ID,
			"connection_id": player.ConnectionID,
		}
		if player.State != nil {
			playerInfo["online"] = player.State.Online
			playerInfo["last_frame_id"] = player.State.LastFrameId
			playerInfo["ping"] = player.State.Ping
			playerInfo["last_ping_time"] = player.State.LastPingTime
		}
		players = append(players, playerInfo)
	}
	roomInfo["players"] = players

	// 添加帧统计信息
	if r.FrameStats != nil {
		frameStats := r.GetFrameStats()
		roomInfo["frame_stats"] = frameStats
	}

	// 添加网络统计信息
	if r.NetworkStats != nil {
		networkStats := r.GetNetworkStats()
		roomInfo["network_stats"] = networkStats
	}

	// 添加房间配置信息
	roomInfo["config"] = map[string]interface{}{
		"max_players":  r.Config.MaxPlayers,
		"min_players":  r.Config.MinPlayers,
		"frame_rate":   r.Config.FrameRate,
		"retry_window": r.Config.RetryWindow,
	}

	return roomInfo
}
