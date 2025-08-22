package lockstep

import (
	"fmt"
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
	frameBuffer    map[FrameID]*FrameMessage
	mutex          sync.RWMutex
	connected      bool
	running        bool
	stopChan       chan struct{}

	// 回调函数
	onPlayerJoined  func(playerID PlayerID)
	onPlayerReadyed func(playerID PlayerID)
	onPlayerLeft    func(playerID PlayerID)

	onGameStarted func()
	onGameEnded   func()

	onConnectedCallback    func()
	onDisconnectedCallback func()

	onLoginResponseCallback      func(errorCode ErrorCode, errorMessage string)
	onLogoutResponseCallback     func(errorCode ErrorCode, errorMessage string)
	onJoinRoomResponseCallback   func(errorCode ErrorCode, errorMessage string)
	onReadyResponseCallback      func(errorCode ErrorCode, errorMessage string)
	onBroadcastReceivedCallback  func(playerID PlayerID, data []byte)
	onPlayerStateChangedCallback func(playerID PlayerID, status PlayerStatus, reason string)
	onRoomStateChangedCallback   func(status RoomStatus)

	onErrorCallback func(error)

	// 本地输入序列号
	nextSequenceID int32

	// 序列号跟踪（用于丢包检测）- 只记录自己的序列号
	lastSeqID int32 // 最后接收到的自己的SequenceId

	// 初始化统计信息
	FrameStats   *FrameStats
	NetworkStats *NetworkStats
}

// ClientCallbacks 客户端回调函数
type ClientCallbacks struct {
	OnPlayerJoined  func(playerID PlayerID)
	OnPlayerReadyed func(playerID PlayerID)
	OnPlayerLeft    func(playerID PlayerID)

	OnGameStarted func()
	OnGameEnded   func()

	OnConnected    func()
	OnDisconnected func()

	OnLoginResponse      func(errorCode ErrorCode, errorMessage string)
	OnLogoutResponse     func(errorCode ErrorCode, errorMessage string)
	OnJoinRoomResponse   func(errorCode ErrorCode, errorMessage string)
	OnReadyResponse      func(errorCode ErrorCode, errorMessage string)
	OnBroadcastReceived  func(playerID PlayerID, data []byte)
	OnPlayerStateChanged func(playerID PlayerID, status PlayerStatus, reason string)
	OnRoomStateChanged   func(status RoomStatus)

	OnError func(error)
}

// NewLockStepClient 创建新的帧同步客户端
func NewLockStepClient(config *LockStepConfig, playerID PlayerID, callbacks ClientCallbacks) *LockStepClient {
	client := &LockStepClient{
		config:          *config,
		playerID:        playerID,
		currentFrameID:  0,
		lastFrameID:     0,
		frameBuffer:     make(map[FrameID]*FrameMessage),
		stopChan:        make(chan struct{}),
		onPlayerJoined:  callbacks.OnPlayerJoined,
		onPlayerReadyed: callbacks.OnPlayerReadyed,
		onPlayerLeft:    callbacks.OnPlayerLeft,
		onGameStarted:   callbacks.OnGameStarted,
		onGameEnded:     callbacks.OnGameEnded,

		onConnectedCallback:    callbacks.OnConnected,
		onDisconnectedCallback: callbacks.OnDisconnected,

		onLoginResponseCallback:      callbacks.OnLoginResponse,
		onLogoutResponseCallback:     callbacks.OnLogoutResponse,
		onReadyResponseCallback:      callbacks.OnReadyResponse,
		onBroadcastReceivedCallback:  callbacks.OnBroadcastReceived,
		onPlayerStateChangedCallback: callbacks.OnPlayerStateChanged,
		onRoomStateChangedCallback:   callbacks.OnRoomStateChanged,

		onErrorCallback: callbacks.OnError,
		nextSequenceID:  0, // 初始化nextSequenceID
		// 初始化序列号跟踪器
		lastSeqID: 0,
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

	Log.Debug("Connecting to server %s:%d", serverAddress, port)
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
	Log.Debug("Disconnected from server")
}

// JoinRoom 加入房间
func (c *LockStepClient) JoinRoom(roomID RoomID, playerID PlayerID) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected {
		return fmt.Errorf("client is not connected")
	}

	c.roomID = roomID

	// 发送加入房间请求
	joinReq := &JoinRoomRequest{
		RoomId:   string(roomID),
		PlayerId: playerID,
	}

	joinMsg := &LockStepMessage{
		Type: LockStepMessage_JOIN_ROOM_REQ,
		Body: &LockStepMessage_JoinRoomReq{
			JoinRoomReq: joinReq,
		},
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
		SequenceId: int32(c.nextSequenceID),
		Timestamp:  int64(time.Now().UnixMilli()),
		Data:       inputData,
		Flag:       inputFlag,
	}

	inputMsg := &LockStepMessage{
		Type: LockStepMessage_INPUT,
		Body: &LockStepMessage_Input{
			Input: &input,
		},
	}

	c.sendMessage(inputMsg, kcp2k.KcpReliable)
	return nil
}

// SendLogin 发送登录请求
func (c *LockStepClient) SendLogin(token string, playerID int32) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected {
		return fmt.Errorf("client is not connected")
	}

	// 创建登录请求
	loginReq := &LoginRequest{
		Token:    token,
		PlayerId: playerID,
	}

	msg := &LockStepMessage{
		Type: LockStepMessage_LOGIN_REQ,
		Body: &LockStepMessage_LoginReq{
			LoginReq: loginReq,
		},
	}

	c.sendMessage(msg, kcp2k.KcpReliable)
	Log.Debug("[Player %d] Sent login request", c.playerID)
	return nil
}

// SendLogout 发送登出请求
func (c *LockStepClient) SendLogout() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected {
		return fmt.Errorf("client is not connected")
	}

	// 创建登出请求
	logoutReq := &LogoutRequest{}

	msg := &LockStepMessage{
		Type: LockStepMessage_LOGOUT_REQ,
		Body: &LockStepMessage_LogoutReq{
			LogoutReq: logoutReq,
		},
	}

	c.sendMessage(msg, kcp2k.KcpReliable)
	Log.Debug("[Player %d] Sent logout request", c.playerID)
	return nil
}

// SendReady 发送玩家准备状态
func (c *LockStepClient) SendReady(ready bool) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected {
		return fmt.Errorf("client is not connected")
	}

	// 创建准备消息
	readyReq := &ReadyRequest{
		Ready: ready,
	}

	msg := &LockStepMessage{
		Type: LockStepMessage_READY_REQ,
		Body: &LockStepMessage_ReadyReq{
			ReadyReq: readyReq,
		},
	}

	c.sendMessage(msg, kcp2k.KcpReliable)
	Log.Debug("[Player %d] Sent ready status: %v", c.playerID, ready)
	return nil
}

// SendBroadcast 发送广播消息
func (c *LockStepClient) SendBroadcast(data []byte) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected {
		return fmt.Errorf("client is not connected")
	}

	// 创建广播请求
	broadcastReq := &BroadcastRequest{
		Data: data,
	}

	msg := &LockStepMessage{
		Type: LockStepMessage_BROADCAST_REQ,
		Body: &LockStepMessage_BroadcastReq{
			BroadcastReq: broadcastReq,
		},
	}

	c.sendMessage(msg, kcp2k.KcpReliable)
	Log.Debug("[Player %d] Sent broadcast message", c.playerID)
	return nil
}

// RequestFrames 请求补帧
func (c *LockStepClient) RequestFrames(startFrameID, count int32) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected {
		return fmt.Errorf("client is not connected")
	}

	req := &FrameRequest{
		FrameId: int32(startFrameID),
		Count:   int32(count),
	}

	reqMsg := &LockStepMessage{
		Type: LockStepMessage_FRAME_REQ,
		Body: &LockStepMessage_FrameReq{
			FrameReq: req,
		},
	}

	c.sendMessage(reqMsg, kcp2k.KcpReliable)
	return nil
}

// sendMessage 发送消息
func (c *LockStepClient) sendMessage(msg *LockStepMessage, channel kcp2k.KcpChannel) {
	msgData, err := proto.Marshal(msg)
	if err != nil {
		Log.Error("Failed to marshal message: %v", err)
		return
	}

	c.kcpClient.Send(msgData, channel)

	// 获取当前RTT作为网络延迟
	rtt := c.GetRTT()

	// 更新网络统计信息（发送数据）
	c.IncrementNetworkStats(0, 1, 0, uint64(len(msgData)), false, rtt)
}

// startFrameProcessing 开始帧处理
func (c *LockStepClient) startFrameProcessing() {
	c.mutex.Lock()
	c.running = true
	c.stopChan = make(chan struct{})
	c.mutex.Unlock()
}

// KCP回调函数
func (c *LockStepClient) onConnected() {
	c.mutex.Lock()
	c.connected = true
	c.mutex.Unlock()

	Log.Debug("[LockStepClient] Connected to server - PlayerID: %d", c.playerID)
	if c.onConnectedCallback != nil {
		c.onConnectedCallback()
	}
}

func (c *LockStepClient) onData(data []byte, channel kcp2k.KcpChannel) {
	if len(data) == 0 {
		Log.Warning("Received empty message, channel: %d", channel)
		return
	}

	var msg LockStepMessage
	err := proto.Unmarshal(data, &msg)
	if err != nil {
		Log.Error("Failed to unmarshal message, channel: %d, error: %v", channel, err)
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
		Log.Debug("Disconnected from server")
	}
	c.mutex.Unlock()
	if c.onDisconnectedCallback != nil {
		c.onDisconnectedCallback()
	}
}

func (c *LockStepClient) onError(error kcp2k.ErrorCode, reason string) {
	Log.Error("Client error: %s, reason: %s", error.String(), reason)

	if c.onErrorCallback != nil {
		c.onErrorCallback(fmt.Errorf("KCP error: %s, reason: %s", error.String(), reason))
	}
}

// handleMessage 处理消息
func (c *LockStepClient) handleMessage(msg *LockStepMessage) {
	switch msg.Type {
	case LockStepMessage_FRAME:
		if frame := msg.GetFrame(); frame != nil {
			data, _ := proto.Marshal(frame)
			c.handleFrame(data)
		}
	case LockStepMessage_BROADCAST:
		if broadcast := msg.GetBroadcast(); broadcast != nil {
			c.handleBroadcast(broadcast)
		}
	case LockStepMessage_GAME_START:
		if gameStart := msg.GetGameStart(); gameStart != nil {
			data, _ := proto.Marshal(gameStart)
			c.handleGameStart(data)
		}
	case LockStepMessage_ROOM_STATE:
		if roomState := msg.GetRoomState(); roomState != nil {
			data, _ := proto.Marshal(roomState)
			c.handleRoomState(data)
		}
	case LockStepMessage_PLAYER_STATE:
		if playerState := msg.GetPlayerState(); playerState != nil {
			data, _ := proto.Marshal(playerState)
			c.handlePlayerState(data)
		}
	case LockStepMessage_LOGIN_RESP:
		if loginResp := msg.GetLoginResp(); loginResp != nil {
			c.handleLoginResponse(loginResp)
		}
	case LockStepMessage_LOGOUT_RESP:
		if logoutResp := msg.GetLogoutResp(); logoutResp != nil {
			c.handleLogoutResponse(logoutResp)
		}
	case LockStepMessage_JOIN_ROOM_RESP:
		if joinRoomResp := msg.GetJoinRoomResp(); joinRoomResp != nil {
			c.handleJoinRoomResponse(joinRoomResp)
		}
	case LockStepMessage_READY_RESP:
		if readyResp := msg.GetReadyResp(); readyResp != nil {
			c.handleReadyResponse(readyResp)
		}
	case LockStepMessage_FRAME_RESP:
		if frameResp := msg.GetFrameResp(); frameResp != nil {
			data, _ := proto.Marshal(frameResp)
			c.handleFrameResponse(data)
		}
	case LockStepMessage_BROADCAST_RESP:
		if broadcastResp := msg.GetBroadcastResp(); broadcastResp != nil {
			c.handleBroadcastResponse(broadcastResp)
		}
	default:
		Log.Warning("Unknown message type: %d", msg.Type)
	}
}

// handleFrame 处理帧数据
func (c *LockStepClient) handleFrame(payload []byte) {
	var frame FrameMessage
	err := proto.Unmarshal(payload, &frame)
	if err != nil {
		Log.Error("Failed to unmarshal frame: %v", err)
		return
	}

	frameStartTime := time.Now()

	// 更新Jitter统计
	c.updateJitterStats(&frame)

	// 存储帧数据并检测丢包
	packetLossDetected := c.storeFrameAndDetectLoss(&frame)

	// 更新统计信息
	c.updateFrameAndNetworkStats(frameStartTime, len(payload), len(frame.DataCollection) == 0, packetLossDetected)
}

// updateJitterStats 更新Jitter统计
func (c *LockStepClient) updateJitterStats(frame *FrameMessage) {
	if frame.RecvTickMs > 0 {
		// 将RecvTickMS转换为时间
		recvTime := time.Unix(0, int64(frame.RecvTickMs)*int64(time.Millisecond))
		// 计算期望的帧间隔（基于配置的帧率）
		expectedInterval := time.Duration(1000/c.config.RoomConfig.FrameRate) * time.Millisecond
		c.NetworkStats.UpdateJitterStats(recvTime, expectedInterval)
	}
}

// storeFrameAndDetectLoss 存储帧数据并检测丢包
func (c *LockStepClient) storeFrameAndDetectLoss(frame *FrameMessage) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// 存储帧数据
	c.frameBuffer[FrameID(frame.FrameId)] = frame
	if FrameID(frame.FrameId) > c.currentFrameID {
		c.currentFrameID = FrameID(frame.FrameId)
	}

	// 检测丢包
	return c.detectPacketLoss(frame.DataCollection)
}

// detectPacketLoss 检测丢包
func (c *LockStepClient) detectPacketLoss(dataCollection []*RelayData) bool {
	packetLossDetected := false
	for _, relayData := range dataCollection {
		playerID := PlayerID(relayData.PlayerId)
		sequenceID := relayData.SequenceId

		// 只处理自己的数据包
		if playerID == c.playerID {
			// 记录输入延迟统计
			inputLatency := time.Duration(relayData.DelayMs) * time.Millisecond
			c.IncrementInputLatencyStats(inputLatency)

			// 检测丢包
			if c.processSequenceID(sequenceID) {
				packetLossDetected = true
			}
		}
	}
	return packetLossDetected
}

// processSequenceID 处理序列号并检测丢包
func (c *LockStepClient) processSequenceID(sequenceID int32) bool {
	if c.lastSeqID == 0 {
		// 第一次收到自己的包，直接记录
		c.lastSeqID = sequenceID
		return false
	}

	if sequenceID > c.lastSeqID {
		// 检查是否有序列号跳跃（丢包）
		gap := sequenceID - c.lastSeqID
		if gap > 1 {
			// 检测到丢包
			Log.Warning("Packet loss detected: expected seq %d, got %d, gap: %d",
				c.lastSeqID+1, sequenceID, gap-1)
			c.lastSeqID = sequenceID
			return true
		}
		// 更新最后接收序列号
		c.lastSeqID = sequenceID
	} else if sequenceID < c.lastSeqID {
		// 收到乱序包
		Log.Warning("Out-of-order packet received: seq %d, expected > %d",
			sequenceID, c.lastSeqID)
	} else {
		// 重复包
		Log.Warning("Duplicate packet received: seq %d", sequenceID)
	}
	return false
}

// updateFrameAndNetworkStats 更新帧和网络统计信息
func (c *LockStepClient) updateFrameAndNetworkStats(frameStartTime time.Time, payloadSize int, isEmpty bool, packetLossDetected bool) {
	// 更新帧统计信息
	frameDuration := time.Since(frameStartTime)
	c.IncrementFrameStats(false, false, isEmpty, frameDuration)

	// 更新网络统计信息
	if packetLossDetected {
		// 检测到丢包时记录网络延迟和丢包统计
		rtt := c.GetRTT()
		c.IncrementNetworkStats(0, 0, 0, 0, true, rtt)
	}
	// 更新网络统计信息（接收到帧数据）
	c.IncrementNetworkStats(1, 0, uint64(payloadSize), 0, false, 0)
}

// handleFrameResponse 处理补帧响应
func (c *LockStepClient) handleFrameResponse(payload []byte) {
	var resp FrameResponse
	err := proto.Unmarshal(payload, &resp)
	if err != nil {
		Log.Error("Failed to unmarshal frame response: %v", err)
		return
	}

	if resp.Base.ErrorCode != ErrorCode_ERROR_CODE_SUCC {
		Log.Error("Frame request failed: %s", resp.Base.ErrorCode)
		return
	}

	if len(resp.Frames) == 0 {
		Log.Warning("[LockStepClient] No frames in response")
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
			if relayData.PlayerId == int32(c.playerID) {
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
	Log.Debug("Game started")

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
			Log.Error("Failed to unmarshal game start message: %v", err)
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

// handlePlayerState 处理玩家状态
func (c *LockStepClient) handlePlayerState(payload []byte) {
	var playerStateMsg PlayerStateMessage
	err := proto.Unmarshal(payload, &playerStateMsg)
	if err != nil {
		Log.Error("Failed to unmarshal player state message: %v", err)
		return
	}

	switch playerStateMsg.Status {
	case PlayerStatus_PLAYER_STATUS_ONLINE:
		// 玩家加入或重连
		if c.onPlayerJoined != nil {
			c.onPlayerJoined(PlayerID(playerStateMsg.PlayerId))
		}
	case PlayerStatus_PLAYER_STATUS_READY:
		// 玩家准备
		if c.onPlayerReadyed != nil {
			c.onPlayerReadyed(PlayerID(playerStateMsg.PlayerId))
		}
	default:
		// 玩家离线
		if c.onPlayerLeft != nil {
			c.onPlayerLeft(PlayerID(playerStateMsg.PlayerId))
		}
	}

	// 打印玩家状态变更信息（取消注释以便调试）
	Log.Debug("Player %d state changed: status=%v, reason: %s",
		playerStateMsg.PlayerId,
		playerStateMsg.Status,
		playerStateMsg.Reason)
}

// handleLoginResponse 处理登录响应
func (c *LockStepClient) handleLoginResponse(resp *LoginResponse) {
	if c.onLoginResponseCallback != nil {
		c.onLoginResponseCallback(resp.Base.ErrorCode, resp.Base.ErrorMessage)
	}
}

// handleLogoutResponse 处理登出响应
func (c *LockStepClient) handleLogoutResponse(resp *LogoutResponse) {
	if c.onLogoutResponseCallback != nil {
		c.onLogoutResponseCallback(resp.Base.ErrorCode, resp.Base.ErrorMessage)
	}
}

func (c *LockStepClient) handleJoinRoomResponse(resp *JoinRoomResponse) {
	if c.onJoinRoomResponseCallback != nil {
		c.onJoinRoomResponseCallback(resp.Base.ErrorCode, resp.Base.ErrorMessage)
	}
}

// handleReadyResponse 处理准备响应
func (c *LockStepClient) handleReadyResponse(resp *ReadyResponse) {
	if c.onReadyResponseCallback != nil {
		c.onReadyResponseCallback(resp.Base.ErrorCode, resp.Base.ErrorMessage)
	}
}

// handleBroadcastResponse 处理广播响应
func (c *LockStepClient) handleBroadcastResponse(resp *BroadcastResponse) {
	if resp.Base.ErrorCode != ErrorCode_ERROR_CODE_SUCC {
		Log.Error("Broadcast failed: %s", resp.Base.ErrorMessage)
		return
	}
	Log.Debug("[Player %d] Broadcast sent successfully", c.playerID)
}

// handleBroadcast 处理广播消息
func (c *LockStepClient) handleBroadcast(broadcast *BroadcastMessage) {
	if c.onBroadcastReceivedCallback != nil {
		c.onBroadcastReceivedCallback(PlayerID(broadcast.PlayerId), broadcast.Data)
	}
}

// handleRoomState 处理房间状态
func (c *LockStepClient) handleRoomState(payload []byte) {
	var roomStateMsg RoomStateMessage
	err := proto.Unmarshal(payload, &roomStateMsg)
	if err != nil {
		Log.Error("Failed to unmarshal room state message: %v", err)
		return
	}

	Log.Debug("Room %s state changed: status=%v, players=%d/%d, currentFrame=%d, reason: %s",
		roomStateMsg.RoomId,
		roomStateMsg.Status,
		roomStateMsg.CurrentPlayers,
		roomStateMsg.MaxPlayers,
		roomStateMsg.CurrentFrameId,
		roomStateMsg.Reason)

	// 触发房间状态变更回调
	if c.onRoomStateChangedCallback != nil {
		c.onRoomStateChangedCallback(RoomStatus(roomStateMsg.Status))
	}

	// 根据房间状态变更触发相应的回调
	switch RoomStatus(roomStateMsg.Status) {
	case RoomStatus_ROOM_STATUS_IDLE:
		if c.running {
			c.mutex.Lock()
			c.running = false
			c.mutex.Unlock()
			Log.Debug("Room is idle - no players")
		}
	case RoomStatus_ROOM_STATUS_WAITING:
		if c.running {
			c.mutex.Lock()
			c.running = false
			c.mutex.Unlock()
			Log.Debug("Room is waiting - waiting for more players")
		}
	case RoomStatus_ROOM_STATUS_RUNNING:
		if !c.running && c.onGameStarted != nil {
			c.mutex.Lock()
			c.running = true
			c.mutex.Unlock()
			c.onGameStarted()
		}
	case RoomStatus_ROOM_STATUS_ENDED:
		if c.running && c.onGameEnded != nil {
			c.mutex.Lock()
			c.running = false
			c.mutex.Unlock()
			c.onGameEnded()
		}
	}
}

// handleError 处理错误消息
func (c *LockStepClient) handleError(response *BaseResponse) {
	Log.Error("Received error %d: %s", response.ErrorCode, response.ErrorMessage)

	// 根据错误码执行相应的处理
	switch response.ErrorCode {
	case ErrorCode_ERROR_CODE_ROOM_NOT_FOUND:
		if c.onErrorCallback != nil {
			c.onErrorCallback(fmt.Errorf("room not found"))
		}
	case ErrorCode_ERROR_CODE_ROOM_FULL:
		if c.onErrorCallback != nil {
			c.onErrorCallback(fmt.Errorf("room is full"))
		}
	default:
		// 通用错误处理
		if c.onErrorCallback != nil {
			c.onErrorCallback(fmt.Errorf("error %d: %s", response.ErrorCode, response.ErrorMessage))
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
	// 重置序列号跟踪
	c.lastSeqID = 0
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

// GetRTT 获取KCP连接的RTT（往返时间）
func (c *LockStepClient) GetRTT() time.Duration {
	if c.kcpClient == nil {
		return 0
	}

	// KCP的GetRTT返回毫秒数，转换为time.Duration
	rttMs := c.kcpClient.GetRTT()
	return time.Duration(rttMs) * time.Millisecond
}

// IncrementFrameStats 增量更新帧统计信息
func (c *LockStepClient) IncrementFrameStats(missed, late, empty bool, frameDuration time.Duration) {
	c.FrameStats.IncrementFrameStats(missed, late, empty, frameDuration)
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
		"rtt":              c.GetRTT().String(),
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
			"average_rtt":           networkStats.GetAverageRTT().String(),
			"max_rtt":               networkStats.GetMaxRTT().String(),
			"min_rtt":               networkStats.GetMinRTT().String(),
			"input_latency_count":   networkStats.GetInputLatencyCount(),
			"average_input_latency": networkStats.GetAverageInputLatency().String(),
			"max_input_latency":     networkStats.GetMaxInputLatency().String(),
			"min_input_latency":     networkStats.GetMinInputLatency().String(),
		}
	}

	return stats
}

// PopFrame 弹出下一帧数据，如果没有可用帧则返回nil
func (c *LockStepClient) PopFrame() *FrameMessage {
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
			c.IncrementFrameStats(true, false, false, 0)
		}
		return nil
	}

	// 检查帧是否延迟（简单判断：如果帧ID小于当前帧ID说明可能是延迟到达的）
	isLate := nextFrameID < c.currentFrameID

	// 移除已处理的帧并更新lastFrameID
	delete(c.frameBuffer, nextFrameID)
	c.lastFrameID = nextFrameID

	// 更新帧统计信息（成功处理的帧）
	// 检查帧是否为空帧（没有输入数据）
	isEmpty := len(frame.DataCollection) == 0
	c.IncrementFrameStats(false, isLate, isEmpty, 0)

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
				Log.Error("Failed to request frames %d-%d: %v", startFrameID, endFrameID, err)
			}
		}()
	}
}
