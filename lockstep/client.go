package lockstep

import (
	"fmt"
	"log"
	"sync"
	"time"

	kcp2k "github.com/O-Keh-Hunter/kcp2k-go"
	"google.golang.org/protobuf/proto"
)

// LockStepClient 帧同步客户端
type LockStepClient struct {
	kcpClient      *kcp2k.KcpClient
	config         LockStepConfig
	playerID       PlayerID
	roomID         RoomID
	currentFrameID FrameID
	lastFrameID    FrameID
	frameBuffer    map[FrameID]*Frame
	inputBuffer    map[FrameID][]byte
	mutex          sync.RWMutex
	connected      bool
	running        bool
	stopChan       chan struct{}
	logger         *log.Logger

	// 回调函数
	onPlayerJoined func(playerID PlayerID)
	onPlayerLeft   func(playerID PlayerID)

	onGameStarted func()
	onGameEnded   func()

	onConnectedCallback    func()
	onDisconnectedCallback func()

	onErrorCallback func(error)
}

// ClientCallbacks 客户端回调函数
type ClientCallbacks struct {
	OnPlayerJoined func(playerID PlayerID)
	OnPlayerLeft   func(playerID PlayerID)

	OnGameStarted func()
	OnGameEnded   func()

	OnConnected    func()
	OnDisconnected func()

	OnError func(error)
}

// NewLockStepClient 创建新的帧同步客户端
func NewLockStepClient(config *LockStepConfig, playerID PlayerID, callbacks ClientCallbacks) *LockStepClient {
	client := &LockStepClient{
		config:         *config,
		playerID:       playerID,
		currentFrameID: 0,
		lastFrameID:    0,
		frameBuffer:    make(map[FrameID]*Frame),
		inputBuffer:    make(map[FrameID][]byte),
		stopChan:       make(chan struct{}),
		logger:         log.New(log.Writer(), "[LockStepClient] ", log.LstdFlags),
		onPlayerJoined: callbacks.OnPlayerJoined,
		onPlayerLeft:   callbacks.OnPlayerLeft,
		onGameStarted:  callbacks.OnGameStarted,
		onGameEnded:    callbacks.OnGameEnded,

		onConnectedCallback:    callbacks.OnConnected,
		onDisconnectedCallback: callbacks.OnDisconnected,

		onErrorCallback: callbacks.OnError,
	}

	// 创建KCP客户端
	kcpClient := kcp2k.NewKcpClient(
		client.onConnected,
		client.onData,
		client.onDisconnected,
		client.onError,
		config.KcpConfig,
	)

	client.kcpClient = kcpClient
	return client
}

// Connect 连接到服务器
func (c *LockStepClient) Connect(serverAddress string, port uint16) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.connected {
		return fmt.Errorf("client is already connected")
	}

	// 重新创建stopChan以支持重连
	c.stopChan = make(chan struct{})

	// 连接到服务器
	err := c.kcpClient.Connect(serverAddress, port)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}

	// 启动KCP Tick循环
	go func() {
		ticker := time.NewTicker(time.Duration(c.config.KcpConfig.Interval) * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.mutex.RLock()
				kcpClient := c.kcpClient
				c.mutex.RUnlock()

				if kcpClient != nil {
					kcpClient.Tick()
				}
			case <-c.stopChan:
				return
			}
		}
	}()

	c.logger.Printf("Connecting to server %s:%d", serverAddress, port)
	return nil
}

// Disconnect 断开连接
func (c *LockStepClient) Disconnect() {
	c.mutex.Lock()
	if !c.connected {
		c.mutex.Unlock()
		return
	}

	c.connected = false
	c.running = false

	if c.stopChan != nil {
		close(c.stopChan)
	}

	// 保存 kcpClient 引用并清空，避免死锁
	kcpClient := c.kcpClient
	c.kcpClient = nil
	c.mutex.Unlock()

	// 在释放锁后调用 Disconnect，避免与 onDisconnected 回调死锁
	if kcpClient != nil {
		kcpClient.Disconnect()
	}
	c.logger.Println("Disconnected from server")
}

// JoinRoom 加入房间
func (c *LockStepClient) JoinRoom(roomID RoomID) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected {
		return fmt.Errorf("client is not connected")
	}

	c.roomID = roomID

	// 发送加入房间请求
	joinReq := JoinRoomRequest{
		RoomId:   string(roomID),
		PlayerId: uint32(c.playerID),
	}

	joinPayload, _ := proto.Marshal(&joinReq)
	joinMsg := &LockStepMessage{
		Type:    LockStepMessage_JOIN_ROOM,
		Payload: joinPayload,
	}

	c.sendMessage(joinMsg)
	return nil
}

// SendInput 发送玩家输入
func (c *LockStepClient) SendInput(frameID FrameID, inputData []byte) error {
	c.mutex.Lock()
	// 将输入数据缓存起来，供后续冗余输入使用
	c.inputBuffer[frameID] = make([]byte, len(inputData))
	copy(c.inputBuffer[frameID], inputData)
	c.mutex.Unlock()

	return c.sendInputWithFlag(frameID, inputData, uint32(InputFlagNormal)) // 普通输入，flag为0
}

// sendInputWithFlag 发送带标志的输入数据
func (c *LockStepClient) sendInputWithFlag(frameID FrameID, inputData []byte, flag uint32) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected || !c.running {
		return fmt.Errorf("client is not ready")
	}

	// 缓存输入数据
	c.inputBuffer[frameID] = inputData

	// 创建输入消息
	input := InputMessage{
		FrameId: uint32(frameID),
		Data:    inputData,
		Flag:    flag,
	}

	inputPayload, _ := proto.Marshal(&input)
	inputMsg := &LockStepMessage{
		Type:    LockStepMessage_INPUT,
		Payload: inputPayload,
	}

	c.sendMessage(inputMsg)
	return nil
}

// RequestFrames 请求补帧
func (c *LockStepClient) RequestFrames(startFrameID, endFrameID FrameID) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.requestFramesInternal(startFrameID, endFrameID)
}

// requestFramesInternal 内部方法，不加锁，用于已经持有锁的上下文
func (c *LockStepClient) requestFramesInternal(startFrameID, endFrameID FrameID) error {
	if !c.connected {
		return fmt.Errorf("client is not connected")
	}

	req := FrameRequest{
		PlayerId:  uint32(c.playerID),
		StartId:   uint32(startFrameID),
		EndId:     uint32(endFrameID),
		Timestamp: time.Now().UnixMilli(),
	}

	reqPayload, err := proto.Marshal(&req)
	if err != nil {
		return err
	}

	reqMsg := &LockStepMessage{
		Type:    LockStepMessage_FRAME_REQ,
		Payload: reqPayload,
	}

	c.sendMessage(reqMsg)
	return nil
}

// SendPing 发送Ping消息
func (c *LockStepClient) SendPing() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected {
		return fmt.Errorf("client is not connected")
	}

	pingMsg := PingMessage{
		Timestamp: time.Now().UnixMilli(),
	}
	pingPayload, _ := proto.Marshal(&pingMsg)
	lockStepMsg := &LockStepMessage{
		Type:    LockStepMessage_PING,
		Payload: pingPayload,
	}

	c.sendMessage(lockStepMsg)
	return nil
}

// sendMessage 发送消息
func (c *LockStepClient) sendMessage(msg *LockStepMessage) {
	msgData, err := proto.Marshal(msg)
	if err != nil {
		c.logger.Printf("Failed to marshal message: %v", err)
		return
	}

	c.kcpClient.Send(msgData, kcp2k.KcpReliable)
}

// startFrameProcessing 开始帧处理
func (c *LockStepClient) startFrameProcessing() {
	c.mutex.Lock()
	c.running = true
	c.stopChan = make(chan struct{})
	c.mutex.Unlock()

	// 只启动ping处理协程，移除frameProcessor
	go c.pingProcessor()
}

// pingProcessor Ping处理协程
func (c *LockStepClient) pingProcessor() {
	ticker := time.NewTicker(time.Second * 2) // 2秒发送一次Ping
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if c.connected {
				c.SendPing()
			}
		case <-c.stopChan:
			return
		}
	}
}

// KCP回调函数
func (c *LockStepClient) onConnected() {
	c.mutex.Lock()
	c.connected = true
	c.mutex.Unlock()

	c.logger.Println("Connected to server")
	if c.onConnectedCallback != nil {
		c.onConnectedCallback()
	}
}

func (c *LockStepClient) onData(data []byte, channel kcp2k.KcpChannel) {
	var msg LockStepMessage
	err := proto.Unmarshal(data, &msg)
	if err != nil {
		c.logger.Printf("Failed to unmarshal message: %v", err)
		return
	}

	c.handleMessage(&msg)
}

func (c *LockStepClient) onDisconnected() {
	c.mutex.Lock()
	// 只有在还连接状态时才更新状态，避免重复处理
	if c.connected {
		c.connected = false
		c.running = false
		c.logger.Println("Disconnected from server")
	}
	c.mutex.Unlock()
	if c.onDisconnectedCallback != nil {
		c.onDisconnectedCallback()
	}
}

func (c *LockStepClient) onError(error kcp2k.ErrorCode, reason string) {
	c.logger.Printf("Client error: %s, reason: %s", error.String(), reason)

	if c.onErrorCallback != nil {
		c.onErrorCallback(fmt.Errorf("KCP error: %s, reason: %s", error.String(), reason))
	}
}

// handleMessage 处理消息
func (c *LockStepClient) handleMessage(msg *LockStepMessage) {
	switch msg.Type {
	case LockStepMessage_FRAME:
		c.handleFrame(msg.Payload)
	case LockStepMessage_FRAME_RESP:
		c.handleFrameResponse(msg.Payload)
	case LockStepMessage_START:
		c.handleGameStart(msg.Payload)
	case LockStepMessage_PONG:
		c.handlePong(msg.Payload)
	case LockStepMessage_PLAYER_STATE:
		c.handlePlayerState(msg.Payload)
	case LockStepMessage_ROOM_STATE:
		c.handleRoomState(msg.Payload)

	case LockStepMessage_ERROR:
		c.handleError(msg.Payload)
	default:
		c.logger.Printf("Unknown message type: %d", msg.Type)
	}
}

// handleFrame 处理帧数据
func (c *LockStepClient) handleFrame(payload []byte) {
	var frame Frame
	err := proto.Unmarshal(payload, &frame)
	if err != nil {
		c.logger.Printf("Failed to unmarshal frame: %v", err)
		return
	}

	c.mutex.Lock()
	c.frameBuffer[FrameID(frame.Id)] = &frame
	if FrameID(frame.Id) > c.currentFrameID {
		c.currentFrameID = FrameID(frame.Id)
	}
	c.mutex.Unlock()
}

// handleFrameResponse 处理补帧响应
func (c *LockStepClient) handleFrameResponse(payload []byte) {
	var resp FrameResponse
	err := proto.Unmarshal(payload, &resp)
	if err != nil {
		c.logger.Printf("Failed to unmarshal frame response: %v", err)
		return
	}

	if !resp.Success {
		c.logger.Printf("Frame request failed: %s", resp.Error)
		return
	}

	if len(resp.Frames) == 0 {
		return
	}

	c.mutex.Lock()
	for _, frame := range resp.Frames {
		c.frameBuffer[FrameID(frame.Id)] = frame
		if FrameID(frame.Id) > c.currentFrameID {
			c.currentFrameID = FrameID(frame.Id)
		}
	}

	c.mutex.Unlock()
}

// handleGameStart 处理游戏开始
func (c *LockStepClient) handleGameStart(payload []byte) {
	c.logger.Println("Game started")

	// 设置running状态为true
	if !c.running {
		c.mutex.Lock()
		c.running = true
		c.mutex.Unlock()
	}

	c.startFrameProcessing()

	// 解析游戏开始消息
	var gameStartMsg GameStartMessage
	if len(payload) > 0 {
		err := proto.Unmarshal(payload, &gameStartMsg)
		if err != nil {
			c.logger.Printf("Failed to unmarshal game start message: %v", err)
		} else {

			// 如果游戏已经在运行，设置当前帧ID，补帧逻辑由PopFrame触发
			if gameStartMsg.GameAlreadyRunning && gameStartMsg.CurrentFrameId > 0 {
				c.mutex.Lock()

				c.currentFrameID = FrameID(gameStartMsg.CurrentFrameId)
				// 重连时从0开始，补帧逻辑由PopFrame中的滑动窗口处理
				c.lastFrameID = 0
				c.mutex.Unlock()

			} else {

			}

		}
	}

	if c.onGameStarted != nil {
		c.onGameStarted()
	}
}

// handlePong 处理Pong消息
func (c *LockStepClient) handlePong(payload []byte) {
	var pongMsg PongMessage
	err := proto.Unmarshal(payload, &pongMsg)
	if err != nil {
		return
	}

	latency := time.Now().UnixMilli() - pongMsg.Timestamp
	c.logger.Printf("Ping: %dms", latency)
}

// handlePlayerState 处理玩家状态
func (c *LockStepClient) handlePlayerState(payload []byte) {
	var playerStateMsg PlayerStateMessage
	err := proto.Unmarshal(payload, &playerStateMsg)
	if err != nil {
		c.logger.Printf("Failed to unmarshal player state message: %v", err)
		return
	}

	// c.logger.Printf("Player %d state changed: online=%v, lastFrame=%d, ping=%dms, reason: %s",
	// 	playerStateMsg.PlayerId,
	// 	playerStateMsg.State.Online,
	// 	playerStateMsg.State.LastFrameId,
	// 	playerStateMsg.State.Ping,
	// 	playerStateMsg.Reason)

	// 如果是其他玩家的状态变更，触发相应的回调
	if PlayerID(playerStateMsg.PlayerId) != c.playerID {
		if playerStateMsg.State.Online {
			// 玩家加入或重连
			if c.onPlayerJoined != nil {
				c.onPlayerJoined(PlayerID(playerStateMsg.PlayerId))
			}
		} else {
			// 玩家离线
			if c.onPlayerLeft != nil {
				c.onPlayerLeft(PlayerID(playerStateMsg.PlayerId))
			}
		}
	}

	// 打印玩家状态变更信息（取消注释以便调试）
	c.logger.Printf("Player %d state changed: online=%v, reason: %s",
		playerStateMsg.PlayerId,
		playerStateMsg.State.Online,
		playerStateMsg.Reason)
}

// handleRoomState 处理房间状态
func (c *LockStepClient) handleRoomState(payload []byte) {
	var roomStateMsg RoomStateMessage
	err := proto.Unmarshal(payload, &roomStateMsg)
	if err != nil {
		c.logger.Printf("Failed to unmarshal room state message: %v", err)
		return
	}

	c.logger.Printf("Room %s state changed: status=%d, players=%d/%d, currentFrame=%d, reason: %s",
		roomStateMsg.RoomId,
		roomStateMsg.State.Status,
		roomStateMsg.State.CurrentPlayers,
		roomStateMsg.State.MaxPlayers,
		roomStateMsg.State.CurrentFrameId,
		roomStateMsg.Reason)

	// 根据房间状态变更触发相应的回调
	switch RoomStatus(roomStateMsg.State.Status) {
	case RoomStatusRunning:
		if !c.running && c.onGameStarted != nil {
			c.mutex.Lock()
			c.running = true
			c.mutex.Unlock()
			c.onGameStarted()
		}
	case RoomStatusEnded:
		if c.running && c.onGameEnded != nil {
			c.mutex.Lock()
			c.running = false
			c.mutex.Unlock()
			c.onGameEnded()
		}
	case RoomStatusWaiting:
		if c.running {
			c.mutex.Lock()
			c.running = false
			c.mutex.Unlock()
			c.logger.Println("Game paused - waiting for more players")
		}
	}
}

// handleError 处理错误消息
func (c *LockStepClient) handleError(payload []byte) {
	var errorMsg ErrorMessage
	err := proto.Unmarshal(payload, &errorMsg)
	if err != nil {
		c.logger.Printf("Failed to unmarshal error message: %v", err)
		return
	}

	c.logger.Printf("Received error %d: %s - %s", errorMsg.Code, errorMsg.Message, errorMsg.Details)

	// 根据错误码执行相应的处理
	switch errorMsg.Code {
	case ErrorCodeRoomNotFound:
		if c.onErrorCallback != nil {
			c.onErrorCallback(fmt.Errorf("room not found: %s", errorMsg.Details))
		}
	case ErrorCodeRoomFull:
		if c.onErrorCallback != nil {
			c.onErrorCallback(fmt.Errorf("room is full: %s", errorMsg.Details))
		}
	case ErrorCodeConnectionLost:
		if c.onErrorCallback != nil {
			c.onErrorCallback(fmt.Errorf("connection lost: %s", errorMsg.Details))
		}
	case ErrorCodeTimeout:
		if c.onErrorCallback != nil {
			c.onErrorCallback(fmt.Errorf("operation timeout: %s", errorMsg.Details))
		}
	case ErrorCodeSyncFailed:
		// 同步失败，可能需要重新请求帧数据
		c.logger.Println("Sync failed, requesting frame data...")
		if c.onErrorCallback != nil {
			c.onErrorCallback(fmt.Errorf("synchronization failed: %s", errorMsg.Details))
		}
	default:
		// 通用错误处理
		if c.onErrorCallback != nil {
			c.onErrorCallback(fmt.Errorf("error %d: %s - %s", errorMsg.Code, errorMsg.Message, errorMsg.Details))
		}
	}
}

// GetCurrentFrameID 获取当前帧ID
func (c *LockStepClient) GetCurrentFrameID() FrameID {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.currentFrameID
}

// GetLastFrameID 获取最后处理的帧ID
func (c *LockStepClient) GetLastFrameID() FrameID {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.lastFrameID
}

// IsConnected 检查是否已连接
func (c *LockStepClient) IsConnected() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.connected
}

// IsRunning 检查是否正在运行
func (c *LockStepClient) IsRunning() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.running
}

// GetPlayerID 获取玩家ID
func (c *LockStepClient) GetPlayerID() PlayerID {
	return c.playerID
}

// GetRoomID 获取房间ID
func (c *LockStepClient) GetRoomID() RoomID {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.roomID
}

// PopFrame 弹出下一帧数据，如果没有可用帧则返回nil
func (c *LockStepClient) PopFrame() *Frame {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// 获取下一帧ID
	nextFrameID := c.lastFrameID + 1

	// 检查是否有该帧数据
	frame, exists := c.frameBuffer[nextFrameID]
	if !exists {
		// 当下一帧不存在时，判断是否需要补帧
		if c.shouldTriggerFrameSync(nextFrameID) {
			// 触发补帧，每次最多补5帧
			c.triggerFrameSync(nextFrameID)
		}
		return nil
	}

	// 移除已处理的帧并更新lastFrameID
	delete(c.frameBuffer, nextFrameID)
	c.lastFrameID = nextFrameID

	return frame
}

// shouldTriggerFrameSync 判断是否需要触发补帧
func (c *LockStepClient) shouldTriggerFrameSync(nextFrameID FrameID) bool {
	// 如果下一帧ID小于等于当前帧ID，说明需要补帧
	return nextFrameID <= c.currentFrameID
}

// triggerFrameSync 触发补帧，每次最多补5帧
func (c *LockStepClient) triggerFrameSync(startFrameID FrameID) {
	// 计算要补充的帧范围，最多5帧
	endFrameID := startFrameID + 4 // 补5帧：startFrameID到startFrameID+4
	if endFrameID > c.currentFrameID {
		endFrameID = c.currentFrameID
	}

	// 检查这些帧是否已经在缓冲区中
	allFramesPresent := true
	for frameID := startFrameID; frameID <= endFrameID; frameID++ {
		if _, exists := c.frameBuffer[frameID]; !exists {
			allFramesPresent = false
			break
		}
	}

	// 如果帧不在缓冲区中，异步请求
	if !allFramesPresent {
		go func() {
			err := c.requestFramesInternal(startFrameID, endFrameID)
			if err != nil {
				c.logger.Printf("Failed to request frames %d-%d: %v", startFrameID, endFrameID, err)
			}
		}()
	}
}
