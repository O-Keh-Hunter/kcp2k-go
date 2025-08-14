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

// GoClient Go客户端测试程序
type GoClient struct {
	client           *kcp2k.KcpClient
	receivedMessages []TestMessage
	messageCounter   int
	connected        bool
	testResults      map[string]bool
	pendingTests     []string
	currentTestIndex int
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

func NewGoClient() *GoClient {
	return &GoClient{
		receivedMessages: make([]TestMessage, 0),
		messageCounter:   0,
		connected:        false,
		testResults:      make(map[string]bool),
		pendingTests: []string{
			"PING",
			"ECHO_TEST",
			"RELIABLE_TEST",
			"UNRELIABLE_TEST",
			"LARGE_MESSAGE_TEST",
		},
		currentTestIndex: 0,
	}
}

func (gc *GoClient) Connect(hostname string, port uint16) error {
	fmt.Printf("[Go Client] Connecting to %s:%d...\n", hostname, port)

	gc.client = kcp2k.NewKcpClient(
		gc.onConnected,
		gc.onData,
		gc.onDisconnected,
		gc.onError,
		testConfig,
	)

	gc.client.Connect(hostname, port)

	// 等待连接建立
	for i := 0; i < 100; i++ { // 最多等待10秒
		gc.client.Tick()
		if gc.connected {
			fmt.Println("[Go Client] Connected successfully!")
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("connection timeout")
}

func (gc *GoClient) Disconnect() {
	fmt.Println("[Go Client] Disconnecting...")
	if gc.client != nil {
		gc.client.Disconnect()
	}
	gc.connected = false
}

func (gc *GoClient) Update() {
	if gc.client != nil {
		gc.client.Tick()
	}
}

func (gc *GoClient) onConnected() {
	gc.connected = true
	fmt.Printf("[Go Client] Connected at %s\n", time.Now().Format("15:04:05.000"))
}

func (gc *GoClient) onData(data []byte, channel kcp2k.KcpChannel) {
	try := func() {
		var message TestMessage
		if err := json.Unmarshal(data, &message); err != nil {
			fmt.Printf("[Go Client] Error parsing message: %v\n", err)
			return
		}

		message.Channel = channel
		gc.receivedMessages = append(gc.receivedMessages, message)

		fmt.Printf("[Go Client] Received: ID=%d, Content='%s', Channel=%v, Size=%d bytes\n",
			message.Id, message.Content, channel, len(data))

		// 处理测试响应
		gc.handleTestResponse(message)
	}
	try()
}

func (gc *GoClient) onDisconnected() {
	gc.connected = false
	fmt.Printf("[Go Client] Disconnected at %s\n", time.Now().Format("15:04:05.000"))
	fmt.Printf("[Go Client] Total messages received: %d\n", len(gc.receivedMessages))
}

func (gc *GoClient) onError(error kcp2k.ErrorCode, reason string) {
	fmt.Printf("[Go Client] Error: %v, Reason: %s\n", error, reason)
}

func (gc *GoClient) handleTestResponse(message TestMessage) {
	switch {
	case message.Content == "PONG":
		gc.testResults["PING"] = true
		fmt.Println("[Go Client] ✓ PING test passed")

	case len(message.Content) > 5 && message.Content[:5] == "ECHO:":
		gc.testResults["ECHO_TEST"] = true
		fmt.Println("[Go Client] ✓ ECHO test passed")

	case len(message.Content) > 8 && message.Content[:8] == "Reliable":
		if !gc.testResults["RELIABLE_TEST"] {
			gc.testResults["RELIABLE_TEST"] = true
			fmt.Println("[Go Client] ✓ RELIABLE test passed")
		}

	case len(message.Content) > 10 && message.Content[:10] == "Unreliable":
		if !gc.testResults["UNRELIABLE_TEST"] {
			gc.testResults["UNRELIABLE_TEST"] = true
			fmt.Println("[Go Client] ✓ UNRELIABLE test passed")
		}

	case len(message.Content) >= 1000: // 大消息测试
		gc.testResults["LARGE_MESSAGE_TEST"] = true
		fmt.Println("[Go Client] ✓ LARGE_MESSAGE test passed")
	}
}

func (gc *GoClient) SendMessage(message TestMessage) error {
	if !gc.connected {
		return fmt.Errorf("not connected")
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	gc.client.Send(data, message.Channel)
	fmt.Printf("[Go Client] Sent: ID=%d, Content='%s', Channel=%v, Size=%d bytes\n",
		message.Id, message.Content, message.Channel, len(data))

	return nil
}

func (gc *GoClient) RunTests() {
	fmt.Println("[Go Client] Starting automated tests...")

	// 等待欢迎消息
	time.Sleep(500 * time.Millisecond)
	gc.Update()

	// 运行所有测试
	for _, testName := range gc.pendingTests {
		fmt.Printf("[Go Client] Running test: %s\n", testName)

		message := TestMessage{
			Id:        gc.messageCounter + 1,
			Content:   testName,
			Timestamp: time.Now().UnixMilli(),
			Channel:   kcp2k.KcpReliable,
		}

		if testName == "UNRELIABLE_TEST" {
			message.Channel = kcp2k.KcpUnreliable
		}

		gc.messageCounter++
		if err := gc.SendMessage(message); err != nil {
			fmt.Printf("[Go Client] Failed to send test %s: %v\n", testName, err)
			continue
		}

		// 等待响应
		for i := 0; i < 50; i++ { // 最多等待5秒
			gc.Update()
			if gc.testResults[testName] {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		if !gc.testResults[testName] {
			fmt.Printf("[Go Client] ✗ Test %s failed or timed out\n", testName)
		}

		time.Sleep(500 * time.Millisecond) // 测试间隔
	}
}

func (gc *GoClient) RunInteractiveMode() {
	fmt.Println("[Go Client] Interactive mode started")
	fmt.Println("Commands: ping, echo, reliable, unreliable, large, disconnect, quit")

	for {
		gc.Update()

		fmt.Print("> ")
		var input string
		fmt.Scanln(&input)

		switch input {
		case "quit", "q":
			return
		case "ping":
			message := TestMessage{
				Id:        gc.messageCounter + 1,
				Content:   "PING",
				Timestamp: time.Now().UnixMilli(),
				Channel:   kcp2k.KcpReliable,
			}
			gc.messageCounter++
			gc.SendMessage(message)
		case "echo":
			message := TestMessage{
				Id:        gc.messageCounter + 1,
				Content:   "ECHO_TEST",
				Timestamp: time.Now().UnixMilli(),
				Channel:   kcp2k.KcpReliable,
			}
			gc.messageCounter++
			gc.SendMessage(message)
		case "reliable":
			message := TestMessage{
				Id:        gc.messageCounter + 1,
				Content:   "RELIABLE_TEST",
				Timestamp: time.Now().UnixMilli(),
				Channel:   kcp2k.KcpReliable,
			}
			gc.messageCounter++
			gc.SendMessage(message)
		case "unreliable":
			message := TestMessage{
				Id:        gc.messageCounter + 1,
				Content:   "UNRELIABLE_TEST",
				Timestamp: time.Now().UnixMilli(),
				Channel:   kcp2k.KcpUnreliable,
			}
			gc.messageCounter++
			gc.SendMessage(message)
		case "large":
			message := TestMessage{
				Id:        gc.messageCounter + 1,
				Content:   "LARGE_MESSAGE_TEST",
				Timestamp: time.Now().UnixMilli(),
				Channel:   kcp2k.KcpReliable,
			}
			gc.messageCounter++
			gc.SendMessage(message)
		case "disconnect":
			message := TestMessage{
				Id:        gc.messageCounter + 1,
				Content:   "DISCONNECT_TEST",
				Timestamp: time.Now().UnixMilli(),
				Channel:   kcp2k.KcpReliable,
			}
			gc.messageCounter++
			gc.SendMessage(message)
		default:
			fmt.Println("Unknown command. Available: ping, echo, reliable, unreliable, large, disconnect, quit")
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func (gc *GoClient) PrintTestResults() {
	fmt.Println("\n[Go Client] Test Results:")
	fmt.Println("=========================")
	passed := 0
	total := len(gc.pendingTests)

	for _, testName := range gc.pendingTests {
		if gc.testResults[testName] {
			fmt.Printf("✓ %s: PASSED\n", testName)
			passed++
		} else {
			fmt.Printf("✗ %s: FAILED\n", testName)
		}
	}

	fmt.Printf("\nSummary: %d/%d tests passed\n", passed, total)
	fmt.Printf("Total messages received: %d\n", len(gc.receivedMessages))
}

func main() {
	// 解析命令行参数
	hostname := "127.0.0.1"
	port := uint16(7777)
	autoTest := false

	for i, arg := range os.Args[1:] {
		switch {
		case arg == "-h" || arg == "--host":
			if i+1 < len(os.Args[1:]) {
				hostname = os.Args[i+2]
			}
		case arg == "-p" || arg == "--port":
			if i+1 < len(os.Args[1:]) {
				if p, err := strconv.Atoi(os.Args[i+2]); err == nil {
					port = uint16(p)
				}
			}
		case arg == "-a" || arg == "--auto":
			autoTest = true
		}
	}

	client := NewGoClient()

	// 连接到服务器
	if err := client.Connect(hostname, port); err != nil {
		log.Fatalf("[Go Client] Failed to connect: %v", err)
	}

	defer client.Disconnect()

	if autoTest {
		// 自动测试模式
		client.RunTests()
		client.PrintTestResults()
	} else {
		// 交互模式
		client.RunInteractiveMode()
	}

	fmt.Println("[Go Client] Exiting...")
}
