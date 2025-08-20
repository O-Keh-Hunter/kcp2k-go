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

	// 本地输入序列号
	nextSequenceID uint32

	// 序列号跟踪（用于丢包检测）
	lastReceivedSeqID uint32 // 自己最后接收到的SequenceId

	// 初始化统计信息
	FrameStats   *FrameStats
	NetworkStats *NetworkStats
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

	// Optional logger for the client
	Logger *log.Logger
}

// NewLockStepClient 创建新的帧同步客户端
func NewLockStepClient(config *LockStepConfig, playerID PlayerID, callbacks ClientCallbacks) *LockStepClient {
	// Use provided logger or create default one
	var logger *log.Logger
	if callbacks.Logger != nil {
		logger = callbacks.Logger
	} else {
		logger = log.New(log.Writer(), "[LockStepClient] ", log.LstdFlags)
	}

	client := &LockStepClient{
		config:         *config,
		playerID:       playerID,
		currentFrameID: 0,
		lastFrameID:    0,
		frameBuffer:    make(map[FrameID]*Frame),
		stopChan:       make(chan struct{}),
		logger:         logger,
		onPlayerJoined: callbacks.OnPlayerJoined,
		onPlayerLeft:   callbacks.OnPlayerLeft,
		onGameStarted:  callbacks.OnGameStarted,
		onGameEnded:    callbacks.OnGameEnded,

		onConnectedCallback:    callbacks.OnConnected,
		onDisconnectedCallback: callbacks.OnDisconnected,

		onErrorCallback: callbacks.OnError,
		nextSequenceID:  0, // 初始化nextSequenceID
		// 初始化丢包统计
		lastReceivedSeqID: 0,
		// 初始化统计信息
		FrameStats:   &FrameStats{},
		NetworkStats: &NetworkStats{},
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

	c.sendMessage(joinMsg, kcp2k.KcpReliable)
	return nil
}

// SendInput 发送玩家输入
func (c *LockStepClient) SendInput(inputData []byte, inputFlag InputMessage_InputFlag) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected || !c.running {
		return fmt.Errorf("client is not ready")
	}

	// 创建输入消息
	c.nextSequenceID++
	input := InputMessage{
		SequenceId: c.nextSequenceID,
		Timestamp:  uint64(time.Now().UnixMilli()),
		Data:       inputData,
		Flag:       inputFlag,
	}

	inputPayload, _ := proto.Marshal(&input)
	inputMsg := &LockStepMessage{
		Type:    LockStepMessage_INPUT,
		Payload: inputPayload,
	}

	c.sendMessage(inputMsg, kcp2k.KcpReliable)
	return nil
}

// RequestFrames 请求补帧
func (c *LockStepClient) RequestFrames(startFrameID, count uint32) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected {
		return fmt.Errorf("client is not connected")
	}

	req := FrameRequest{
		FrameId: uint32(startFrameID),
		Count:   count,
	}

	reqPayload, err := proto.Marshal(&req)
	if err != nil {
		return err
	}

	reqMsg := &LockStepMessage{
		Type:    LockStepMessage_FRAME_REQ,
		Payload: reqPayload,
	}

	c.sendMessage(reqMsg, kcp2k.KcpReliable)
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

	c.sendMessage(lockStepMsg, kcp2k.KcpUnreliable)
	return nil
}

// sendMessage 发送消息
func (c *LockStepClient) sendMessage(msg *LockStepMessage, channel kcp2k.KcpChannel) {
	msgData, err := proto.Marshal(msg)
	if err != nil {
		c.logger.Printf("Failed to marshal message: %v", err)
		return
	}

	c.kcpClient.Send(msgData, channel)

	// 更新网络统计信息（发送数据）
	c.IncrementNetworkStats(0, 1, 0, uint64(len(msgData)), false, 0)
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

	c.logger.Println("[LockStepClient] Connected to server")
	c.logger.Printf("[LockStepClient] Logger test - PlayerID: %d", c.playerID)
	if c.onConnectedCallback != nil {
		c.onConnectedCallback()
	}
}

func (c *LockStepClient) onData(data []byte, channel kcp2k.KcpChannel) {
	if len(data) == 0 {
		c.logger.Printf("Received empty message, channel: %d", channel)
		return
	}

	var msg LockStepMessage
	err := proto.Unmarshal(data, &msg)
	if err != nil {
		c.logger.Printf("Failed to unmarshal message, channel: %d, error: %v", channel, err)
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

	frameStartTime := time.Now()

	// 基于RecvTickMS计算Jitter统计
	if frame.RecvTickMs > 0 {
		// 将RecvTickMS转换为时间
		recvTime := time.Unix(0, int64(frame.RecvTickMs)*int64(time.Millisecond))
		// 计算期望的帧间隔（基于配置的帧率）
		expectedInterval := time.Duration(1000/c.config.RoomConfig.FrameRate) * time.Millisecond
		c.NetworkStats.UpdateJitterStats(recvTime, expectedInterval)
	}

	c.mutex.Lock()
	c.frameBuffer[FrameID(frame.FrameId)] = &frame
	if FrameID(frame.FrameId) > c.currentFrameID {
		c.currentFrameID = FrameID(frame.FrameId)
	}

	// 检测丢包：遍历帧中的RelayData，检查SequenceId连续性
	for _, relayData := range frame.DataCollection {
		playerID := PlayerID(relayData.PlayerId)
		sequenceID := relayData.SequenceId

		// 如果 RelayData.PlayerId == Self.PlayerId 时，才有意义
		// 表示用户自己的上行包，从调用 InputMessage 到 LockStep 客户端收到这个包的总延时，单位ms
		// 方便游戏收集用户输入延时
		if playerID == c.playerID {
			// 记录输入延迟统计
			inputLatency := time.Duration(relayData.DelayMs) * time.Millisecond
			c.IncrementInputLatencyStats(inputLatency)

			// 检查序列号连续性（只对第一个包之后进行检查）
			if c.lastReceivedSeqID > 0 {
				expected := c.lastReceivedSeqID + 1
				if sequenceID > expected {
					c.IncrementNetworkStats(0, 0, 0, 0, true, 0)
				}
			}

			// 更新最后接收到的序列号
			c.lastReceivedSeqID = sequenceID
		}
	}

	c.mutex.Unlock()

	// 更新帧统计信息
	frameDuration := time.Since(frameStartTime)
	c.IncrementFrameStats(false, false, frameDuration)

	// 更新网络统计信息（接收到帧数据）
	c.IncrementNetworkStats(1, 0, uint64(len(payload)), 0, false, 0)
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
		c.logger.Printf("[LockStepClient] No frames in response")
		return
	}

	c.mutex.Lock()
	for _, frame := range resp.Frames {
		c.frameBuffer[FrameID(frame.FrameId)] = frame
		if FrameID(frame.FrameId) > c.currentFrameID {
			c.currentFrameID = FrameID(frame.FrameId)
		}

		// 处理输入延迟统计
		for _, relayData := range frame.DataCollection {
			if relayData.PlayerId == uint32(c.playerID) {
				// 使用服务器计算好的延迟时间进行统计
				inputLatency := time.Duration(relayData.DelayMs) * time.Millisecond
				c.IncrementInputLatencyStats(inputLatency)
			}
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

	// latency := time.Now().UnixMilli() - pongMsg.Timestamp
	// c.logger.Printf("Ping: %dms", latency)
	latency := time.Duration(time.Now().UnixMilli()-pongMsg.Timestamp) * time.Millisecond
	// c.logger.Printf("Ping: %dms", latency.Milliseconds())

	// 更新网络统计信息（延迟信息）
	c.IncrementNetworkStats(1, 0, uint64(len(payload)), 0, false, latency)
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

// GetPacketLossRate 获取丢包率（0.0-1.0）
func (c *LockStepClient) GetPacketLossRate() float64 {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	networkStats := c.NetworkStats
	if networkStats == nil {
		return 0.0
	}

	totalPackets := networkStats.GetTotalPackets()
	lostPackets := networkStats.GetLostPackets()
	if totalPackets == 0 {
		return 0.0
	}
	return float64(lostPackets) / float64(totalPackets)
}

// GetPacketLossStats 获取详细的丢包统计信息
func (c *LockStepClient) GetPacketLossStats() (sentPackets, receivedPackets, lostPackets uint64, lossRate float64) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	networkStats := c.NetworkStats
	if networkStats == nil {
		return 0, 0, 0, 0.0
	}

	totalPackets := networkStats.GetTotalPackets()
	lostPackets = networkStats.GetLostPackets()
	lossRate = 0.0
	if totalPackets > 0 {
		lossRate = float64(lostPackets) / float64(totalPackets)
	}
	return totalPackets - lostPackets, totalPackets, lostPackets, lossRate
}

// ResetPacketLossStats 重置丢包统计
func (c *LockStepClient) ResetPacketLossStats() {
	c.lastReceivedSeqID = 0
	// 重置网络统计
	if c.NetworkStats != nil {
		c.NetworkStats.Reset()
	}
}

// GetFrameStats 获取客户端帧统计信息
func (c *LockStepClient) GetFrameStats() *FrameStats {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.FrameStats
}

// GetNetworkStats 获取客户端网络统计信息
func (c *LockStepClient) GetNetworkStats() *NetworkStats {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.NetworkStats
}

// UpdateFrameStats 更新帧统计信息
func (c *LockStepClient) UpdateFrameStats(totalFrames, missedFrames, lateFrames uint64, frameTimeSum time.Duration, lastFrameTime time.Time) {
	c.FrameStats.UpdateFrameStats(totalFrames, missedFrames, lateFrames, frameTimeSum, lastFrameTime)
}

// IncrementFrameStats 增量更新帧统计信息
func (c *LockStepClient) IncrementFrameStats(missed, late bool, frameDuration time.Duration) {
	c.FrameStats.IncrementFrameStats(missed, late, frameDuration)
}

// UpdateNetworkStats 更新网络统计信息
func (c *LockStepClient) UpdateNetworkStats(totalPackets, lostPackets, bytesReceived, bytesSent uint64, latency time.Duration) {
	c.NetworkStats.UpdateNetworkStats(totalPackets, lostPackets, bytesReceived, bytesSent, latency)
}

// IncrementNetworkStats 增量更新网络统计信息
func (c *LockStepClient) IncrementNetworkStats(packetsReceived, packetsSent, bytesReceived, bytesSent uint64, lost bool, latency time.Duration) {
	c.NetworkStats.IncrementNetworkStats(packetsReceived, packetsSent, bytesReceived, bytesSent, lost, latency)
}

// IncrementInputLatencyStats 增量更新输入延时统计信息
func (c *LockStepClient) IncrementInputLatencyStats(inputLatency time.Duration) {
	c.NetworkStats.IncrementInputLatencyStats(inputLatency)
}

// GetClientStats 获取客户端统计信息
func (c *LockStepClient) GetClientStats() map[string]interface{} {
	c.mutex.RLock()
	frameStats := c.FrameStats
	networkStats := c.NetworkStats
	c.mutex.RUnlock()

	stats := map[string]interface{}{
		"player_id":        c.playerID,
		"room_id":          c.roomID,
		"connected":        c.connected,
		"running":          c.running,
		"current_frame":    c.currentFrameID,
		"last_frame":       c.lastFrameID,
		"packet_loss_rate": c.GetPacketLossRate(),
	}

	// 添加帧统计信息
	if frameStats != nil {
		stats["frame_stats"] = map[string]interface{}{
			"total_frames":       frameStats.GetTotalFrames(),
			"missed_frames":      frameStats.GetMissedFrames(),
			"late_frames":        frameStats.GetLateFrames(),
			"average_frame_time": frameStats.GetAverageFrameTime().String(),
		}
	}

	// 添加网络统计信息
	if networkStats != nil {
		stats["network_stats"] = map[string]interface{}{
			"total_packets":         networkStats.GetTotalPackets(),
			"lost_packets":          networkStats.GetLostPackets(),
			"bytes_received":        networkStats.GetBytesReceived(),
			"bytes_sent":            networkStats.GetBytesSent(),
			"average_latency":       networkStats.GetAverageLatency().String(),
			"max_latency":           networkStats.GetMaxLatency().String(),
			"min_latency":           networkStats.GetMinLatency().String(),
			"input_latency_count":   networkStats.GetInputLatencyCount(),
			"average_input_latency": networkStats.GetAverageInputLatency().String(),
			"max_input_latency":     networkStats.GetMaxInputLatency().String(),
			"min_input_latency":     networkStats.GetMinInputLatency().String(),
		}
	}

	return stats
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
			// 更新帧统计信息（丢失帧）
			c.IncrementFrameStats(true, false, 0)
		}
		return nil
	}

	// 检查帧是否延迟（简单判断：如果帧ID小于当前帧ID说明可能是延迟到达的）
	isLate := nextFrameID < c.currentFrameID

	// 移除已处理的帧并更新lastFrameID
	delete(c.frameBuffer, nextFrameID)
	c.lastFrameID = nextFrameID

	// 更新帧统计信息（成功处理的帧）
	c.IncrementFrameStats(false, isLate, 0)

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
			err := c.RequestFrames(startFrameID, endFrameID)
			if err != nil {
				c.logger.Printf("Failed to request frames %d-%d: %v", startFrameID, endFrameID, err)
			}
		}()
	}
}
