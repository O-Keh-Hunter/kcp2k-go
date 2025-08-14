package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	kcp2k "github.com/O-Keh-Hunter/kcp2k-go"
)

// TestMessage 对应C#的TestMessage结构
type TestMessage struct {
	Id        int              `json:"Id"`
	Content   string           `json:"Content"`
	Timestamp int64            `json:"Timestamp"`
	Channel   kcp2k.KcpChannel `json:"Channel"`
}

// GoServer Go服务端测试程序
type GoServer struct {
	server           *kcp2k.KcpServer
	connectionTimes  map[int]time.Time
	receivedMessages map[int][]TestMessage
	messageCounter   int
	isRunning        bool
}

// 测试配置 - 与C#版本保持一致
var testConfig = kcp2k.KcpConfig{
	DualMode:          false,
	RecvBufferSize:    1024 * 1024 * 7,
	SendBufferSize:    1024 * 1024 * 7,
	Mtu:               1400,
	NoDelay:           true,
	Interval:          1,
	FastResend:        0,
	CongestionWindow:  false,
	SendWindowSize:    32000,
	ReceiveWindowSize: 32000,
	Timeout:           10000,
	MaxRetransmits:    40,
}

func NewGoServer() *GoServer {
	return &GoServer{
		connectionTimes:  make(map[int]time.Time),
		receivedMessages: make(map[int][]TestMessage),
		messageCounter:   0,
		isRunning:        false,
	}
}

func (gs *GoServer) Start(port uint16) error {
	fmt.Printf("[Go Server] Starting server on port %d...\n", port)

	gs.server = kcp2k.NewKcpServer(
		gs.onConnected,
		gs.onData,
		gs.onDisconnected,
		gs.onError,
		testConfig,
	)

	if err := gs.server.Start(port); err != nil {
		return fmt.Errorf("failed to start server: %v", err)
	}

	gs.isRunning = true
	fmt.Printf("[Go Server] Server started successfully on port %d\n", port)
	return nil
}

func (gs *GoServer) Stop() {
	fmt.Println("[Go Server] Stopping server...")
	gs.isRunning = false
	if gs.server != nil {
		gs.server.Stop()
	}
	fmt.Println("[Go Server] Server stopped")
}

func (gs *GoServer) Update() {
	if gs.server != nil {
		gs.server.Tick()
	}
}

func (gs *GoServer) onConnected(connectionId int) {
	gs.connectionTimes[connectionId] = time.Now()
	gs.receivedMessages[connectionId] = make([]TestMessage, 0)
	fmt.Printf("[Go Server] Client %d connected at %s\n", connectionId, time.Now().Format("15:04:05.000"))

	// 发送欢迎消息
	welcomeMsg := TestMessage{
		Id:        gs.messageCounter + 1,
		Content:   "Welcome from Go Server!",
		Timestamp: time.Now().UnixMilli(),
		Channel:   kcp2k.KcpReliable,
	}
	gs.messageCounter++

	gs.SendMessage(connectionId, welcomeMsg)
}

func (gs *GoServer) onData(connectionId int, data []byte, channel kcp2k.KcpChannel) {
	var message TestMessage
	if err := json.Unmarshal(data, &message); err != nil {
		fmt.Printf("[Go Server] Error parsing message from %d: %v\n", connectionId, err)
		return
	}

	message.Channel = channel
	gs.receivedMessages[connectionId] = append(gs.receivedMessages[connectionId], message)

	fmt.Printf("[Go Server] Received from %d: ID=%d, Content='%s', Channel=%v, Size=%d bytes\n",
		connectionId, message.Id, message.Content, channel, len(data))

	// 根据消息内容执行不同的测试响应
	gs.handleTestMessage(connectionId, message)
}

func (gs *GoServer) onDisconnected(connectionId int) {
	if connectTime, exists := gs.connectionTimes[connectionId]; exists {
		duration := time.Since(connectTime)
		fmt.Printf("[Go Server] Client %d disconnected after %.2f seconds\n", connectionId, duration.Seconds())
		delete(gs.connectionTimes, connectionId)
	}

	if messages, exists := gs.receivedMessages[connectionId]; exists {
		fmt.Printf("[Go Server] Client %d received %d messages total\n", connectionId, len(messages))
		delete(gs.receivedMessages, connectionId)
	}
}

func (gs *GoServer) onError(connectionId int, error kcp2k.ErrorCode, reason string) {
	fmt.Printf("[Go Server] Client %d Error: %v, Reason: %s\n", connectionId, error, reason)
}

func (gs *GoServer) handleTestMessage(connectionId int, message TestMessage) {
	switch message.Content {
	case "PING":
		// 响应PING消息
		pongMsg := TestMessage{
			Id:        gs.messageCounter + 1,
			Content:   "PONG",
			Timestamp: time.Now().UnixMilli(),
			Channel:   message.Channel,
		}
		gs.messageCounter++
		gs.SendMessage(connectionId, pongMsg)

	case "ECHO_TEST":
		// 回显测试
		echoMsg := TestMessage{
			Id:        message.Id, // 保持相同ID
			Content:   fmt.Sprintf("ECHO: %s", message.Content),
			Timestamp: time.Now().UnixMilli(),
			Channel:   message.Channel,
		}
		gs.SendMessage(connectionId, echoMsg)

	case "RELIABLE_TEST":
		// 可靠消息测试
		for i := 0; i < 5; i++ {
			reliableMsg := TestMessage{
				Id:        gs.messageCounter + 1,
				Content:   fmt.Sprintf("Reliable message %d/5", i+1),
				Timestamp: time.Now().UnixMilli(),
				Channel:   kcp2k.KcpReliable,
			}
			gs.messageCounter++
			gs.SendMessage(connectionId, reliableMsg)
		}

	case "UNRELIABLE_TEST":
		// 不可靠消息测试
		for i := 0; i < 5; i++ {
			unreliableMsg := TestMessage{
				Id:        gs.messageCounter + 1,
				Content:   fmt.Sprintf("Unreliable message %d/5", i+1),
				Timestamp: time.Now().UnixMilli(),
				Channel:   kcp2k.KcpUnreliable,
			}
			gs.messageCounter++
			gs.SendMessage(connectionId, unreliableMsg)
		}

	case "LARGE_MESSAGE_TEST":
		// 大消息测试
		largeContent := make([]byte, 1000) // 1KB消息
		for i := range largeContent {
			largeContent[i] = 'A'
		}
		largeMsg := TestMessage{
			Id:        gs.messageCounter + 1,
			Content:   string(largeContent),
			Timestamp: time.Now().UnixMilli(),
			Channel:   kcp2k.KcpReliable,
		}
		gs.messageCounter++
		gs.SendMessage(connectionId, largeMsg)

	case "DISCONNECT_TEST":
		// 断开连接测试
		fmt.Printf("[Go Server] Disconnecting client %d as requested\n", connectionId)
		gs.server.Disconnect(connectionId)

	default:
		// 默认回显
		defaultMsg := TestMessage{
			Id:        gs.messageCounter + 1,
			Content:   fmt.Sprintf("Server received: %s", message.Content),
			Timestamp: time.Now().UnixMilli(),
			Channel:   message.Channel,
		}
		gs.messageCounter++
		gs.SendMessage(connectionId, defaultMsg)
	}
}

func (gs *GoServer) SendMessage(connectionId int, message TestMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	gs.server.Send(connectionId, data, message.Channel)
	fmt.Printf("[Go Server] Sent to %d: ID=%d, Content='%s', Channel=%v, Size=%d bytes\n",
		connectionId, message.Id, message.Content, message.Channel, len(data))

	return nil
}

func (gs *GoServer) PrintStats() {
	fmt.Printf("[Go Server] Active connections: %d\n", len(gs.connectionTimes))
	for connectionId, messages := range gs.receivedMessages {
		fmt.Printf("[Go Server] Connection %d: %d messages received\n", connectionId, len(messages))
	}
}

func main() {
	// 解析命令行参数
	port := uint16(7777)
	if len(os.Args) > 1 {
		if p, err := strconv.Atoi(os.Args[1]); err == nil {
			port = uint16(p)
		}
	}

	server := NewGoServer()

	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[Go Server] Panic recovered: %v\n", r)
		}
		server.Stop()
	}()

	if err := server.Start(port); err != nil {
		log.Fatalf("[Go Server] Failed to start server: %v", err)
	}

	fmt.Printf("[Go Server] Server started on port %d\n", port)
	fmt.Println("[Go Server] Running for 60 seconds...")

	// 运行60秒后自动退出（用于测试）
	startTime := time.Now()
	for time.Since(startTime) < 60*time.Second {
		server.Update()
		time.Sleep(1 * time.Millisecond)
	}

	fmt.Println("[Go Server] Test completed, shutting down...")
}
