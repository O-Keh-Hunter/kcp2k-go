package kcp2k

import (
	"bytes"
	"testing"
	"time"
)

// 多客户端测试端口，避免与单客户端测试冲突
const MultiClientTestPort = 7778

// MultiClientServerTest 对应 C# 的 MultipleClientServerTests
type MultiClientServerTest struct {
	server          *KcpServer
	clientA         *KcpClient
	clientB         *KcpClient
	serverReceived  []Message
	clientReceivedA []Message
	clientReceivedB []Message

	// 连接ID记录
	connectionIdA int
	connectionIdB int

	// 连接/断开连接计数器
	onClientAConnectedCalled    int
	onClientADisconnectedCalled int
	onClientBConnectedCalled    int
	onClientBDisconnectedCalled int
	onServerConnectedCalled     int
	onServerDisconnectedCalled  int
}

// 测试配置，对应 C# 版本的配置
var multiClientTestConfig = KcpConfig{
	DualMode:          false, // 禁用双模式，不是所有平台都支持
	RecvBufferSize:    1024 * 1024 * 7,
	SendBufferSize:    1024 * 1024 * 7,
	Mtu:               1400,
	NoDelay:           true, // 强制 NoDelay 和最小间隔
	Interval:          1,    // 1ms 间隔，这样 UpdateSeveralTimes() 不需要等待很长时间，测试运行更快
	FastResend:        0,
	CongestionWindow:  false, // 拥塞窗口严重限制发送/接收窗口大小，发送最大大小的消息需要数千次更新
	SendWindowSize:    32000, // 大窗口大小，这样大消息可以用很少的更新调用刷新，否则测试需要太长时间
	ReceiveWindowSize: 32000, // 大窗口大小
	Timeout:           2000,
	MaxRetransmits:    80, // 最大重传尝试次数直到检测到 dead_link，默认值 * 2 来检查配置是否工作
}

// 客户端A数据回调
func (t *MultiClientServerTest) clientOnDataA(data []byte, channel KcpChannel) {
	msg := Message{data: make([]byte, len(data)), channel: channel}
	copy(msg.data, data)
	t.clientReceivedA = append(t.clientReceivedA, msg)
}

// 客户端B数据回调
func (t *MultiClientServerTest) clientOnDataB(data []byte, channel KcpChannel) {
	msg := Message{data: make([]byte, len(data)), channel: channel}
	copy(msg.data, data)
	t.clientReceivedB = append(t.clientReceivedB, msg)
}

// 服务器数据回调
func (t *MultiClientServerTest) serverOnData(connectionId int, data []byte, channel KcpChannel) {
	msg := Message{data: make([]byte, len(data)), channel: channel}
	copy(msg.data, data)
	t.serverReceived = append(t.serverReceived, msg)
}

// 创建服务器
func (t *MultiClientServerTest) createServer() {
	t.server = NewKcpServer(
		func(connectionId int) {
			t.onServerConnectedCalled++
			// Record connection IDs based on connection order
			switch t.onServerConnectedCalled {
			case 1:
				t.connectionIdA = connectionId
			case 2:
				t.connectionIdB = connectionId
			}
		},
		t.serverOnData,
		func(connectionId int) {
			t.onServerDisconnectedCalled++
		},
		func(connectionId int, errorCode ErrorCode, message string) {
			// 错误处理
		},
		multiClientTestConfig,
	)
}

// 创建客户端
func (t *MultiClientServerTest) createClients() {
	t.clientA = NewKcpClient(
		func() {
			t.onClientAConnectedCalled++
		},
		t.clientOnDataA,
		func() {
			t.onClientADisconnectedCalled++
		},
		func(errorCode ErrorCode, message string) {
			// 错误处理
		},
		multiClientTestConfig,
	)

	t.clientB = NewKcpClient(
		func() {
			t.onClientBConnectedCalled++
		},
		t.clientOnDataB,
		func() {
			t.onClientBDisconnectedCalled++
		},
		func(errorCode ErrorCode, message string) {
			// 错误处理
		},
		multiClientTestConfig,
	)
}

// 设置测试环境
func (t *MultiClientServerTest) setUp() {
	// 重置所有状态
	t.serverReceived = make([]Message, 0)
	t.clientReceivedA = make([]Message, 0)
	t.clientReceivedB = make([]Message, 0)
	t.connectionIdA = -1
	t.connectionIdB = -1
	t.onClientAConnectedCalled = 0
	t.onClientADisconnectedCalled = 0
	t.onClientBConnectedCalled = 0
	t.onClientBDisconnectedCalled = 0
	t.onServerConnectedCalled = 0
	t.onServerDisconnectedCalled = 0

	// 创建服务器和客户端
	t.createServer()
	t.createClients()
}

// 清理测试环境
func (t *MultiClientServerTest) tearDown() {
	if t.clientA != nil {
		t.clientA.Disconnect()
		t.clientA = nil
	}
	if t.clientB != nil {
		t.clientB.Disconnect()
		t.clientB = nil
	}
	if t.server != nil {
		t.server.Stop()
		t.server = nil
	}
	// 等待一段时间确保资源完全释放
	time.Sleep(10 * time.Millisecond)
}

// 多次更新服务器和客户端
func (t *MultiClientServerTest) updateSeveralTimes(amount int) {
	for i := 0; i < amount; i++ {
		if t.server != nil {
			t.server.Tick()
		}
		if t.clientA != nil {
			t.clientA.Tick()
		}
		if t.clientB != nil {
			t.clientB.Tick()
		}
		// 短暂休眠以模拟网络延迟
		time.Sleep(time.Millisecond)
	}
}

// 阻塞式连接客户端
func (t *MultiClientServerTest) connectClientsBlocking(hostname string) {
	if hostname == "" {
		hostname = "127.0.0.1"
	}

	// Connect ClientA first and wait for it to be fully connected
	_ = t.clientA.Connect(hostname, MultiClientTestPort)

	// Wait for ClientA to connect completely
	for i := 0; i < 100 && !t.clientA.Connected(); i++ {
		t.updateSeveralTimes(10)
	}

	// Only connect ClientB after ClientA is fully connected
	_ = t.clientB.Connect(hostname, MultiClientTestPort)

	// Wait for ClientB to connect
	for i := 0; i < 100 && !t.clientB.Connected(); i++ {
		t.updateSeveralTimes(10)
	}
}

// 阻塞式断开客户端连接
func (t *MultiClientServerTest) disconnectClientsBlocking() {
	t.clientA.Disconnect()
	t.clientB.Disconnect()
}

// 阻塞式踢出客户端
func (t *MultiClientServerTest) kickClientBlocking(connectionId int) {
	if conn, ok := t.server.connections[connectionId]; ok {
		conn.Disconnect()
	}
}

// 阻塞式客户端到服务器发送消息
func (t *MultiClientServerTest) sendClientToServerBlocking(client *KcpClient, message []byte, channel KcpChannel) {
	client.Send(message, channel)
	t.updateSeveralTimes(10)
}

// 阻塞式服务器到客户端发送消息
func (t *MultiClientServerTest) sendServerToClientBlocking(connectionId int, message []byte, channel KcpChannel) {
	t.server.Send(connectionId, message, channel)
	t.updateSeveralTimes(10)
}

// TestConnectAndDisconnectClients 对应 C# 的 ConnectAndDisconnectClients
func TestConnectAndDisconnectClients(t *testing.T) {
	test := &MultiClientServerTest{}
	test.setUp()
	defer test.tearDown()

	err := test.server.Start(MultiClientTestPort)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// 连接客户端
	test.connectClientsBlocking("")

	// 验证连接
	if !test.clientA.Connected() {
		t.Error("ClientA should be connected")
	}
	if !test.clientB.Connected() {
		t.Error("ClientB should be connected")
	}
	if test.onClientAConnectedCalled != 1 {
		t.Errorf("ClientA connected callback should be called once, got %d", test.onClientAConnectedCalled)
	}
	if test.onClientBConnectedCalled != 1 {
		t.Errorf("ClientB connected callback should be called once, got %d", test.onClientBConnectedCalled)
	}
	if test.onServerConnectedCalled != 2 {
		t.Errorf("Server connected callback should be called twice, got %d", test.onServerConnectedCalled)
	}
	if len(test.server.connections) != 2 {
		t.Errorf("Server should have 2 connections, got %d", len(test.server.connections))
	}

	// 断开连接
	test.disconnectClientsBlocking()
	test.updateSeveralTimes(10)

	// 验证断开连接
	if test.clientA.Connected() {
		t.Error("ClientA should be disconnected")
	}
	if test.clientB.Connected() {
		t.Error("ClientB should be disconnected")
	}
}

// TestConnectAndDisconnectClientsMultipleTimes 对应 C# 的 ConnectAndDisconnectClientsMultipleTimes
func TestConnectAndDisconnectClientsMultipleTimes(t *testing.T) {
	test := &MultiClientServerTest{}
	test.setUp()
	defer test.tearDown()

	err := test.server.Start(MultiClientTestPort)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// 多次连接和断开
	for i := 0; i < 3; i++ {
		// 连接
		test.connectClientsBlocking("")
		if !test.clientA.Connected() || !test.clientB.Connected() {
			t.Errorf("Clients should be connected on iteration %d", i)
		}

		// 断开
		test.disconnectClientsBlocking()
		test.updateSeveralTimes(10)
		if test.clientA.Connected() || test.clientB.Connected() {
			t.Errorf("Clients should be disconnected on iteration %d", i)
		}
	}
}

// TestClientsToServerMessage 对应 C# 的 ClientsToServerMessage
func TestClientsToServerMessage(t *testing.T) {
	test := &MultiClientServerTest{}
	test.setUp()
	defer test.tearDown()

	err := test.server.Start(MultiClientTestPort)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	test.connectClientsBlocking("")

	messageA := []byte{0x01, 0x02}
	messageB := []byte{0x03, 0x04}

	// 客户端A发送可靠消息
	test.sendClientToServerBlocking(test.clientA, messageA, KcpReliable)
	if len(test.serverReceived) != 1 {
		t.Errorf("Server should receive 1 message, got %d", len(test.serverReceived))
	}
	if !bytes.Equal(test.serverReceived[0].data, messageA) {
		t.Error("Server message data mismatch for clientA")
	}
	if test.serverReceived[0].channel != KcpReliable {
		t.Errorf("Server channel mismatch: expected %v, got %v", KcpReliable, test.serverReceived[0].channel)
	}

	// 客户端B发送不可靠消息
	test.sendClientToServerBlocking(test.clientB, messageB, KcpUnreliable)
	if len(test.serverReceived) != 2 {
		t.Errorf("Server should receive 2 messages, got %d", len(test.serverReceived))
	}
	if !bytes.Equal(test.serverReceived[1].data, messageB) {
		t.Error("Server message data mismatch for clientB")
	}
	if test.serverReceived[1].channel != KcpUnreliable {
		t.Errorf("Server channel mismatch: expected %v, got %v", KcpUnreliable, test.serverReceived[1].channel)
	}
}

// TestServerToClientsReliableMessage 对应 C# 的 ServerToClientsReliableMessage
func TestServerToClientsReliableMessage(t *testing.T) {
	test := &MultiClientServerTest{}
	test.setUp()
	defer test.tearDown()

	err := test.server.Start(MultiClientTestPort)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	test.connectClientsBlocking("")
	// Use recorded connection IDs instead of helper functions
	connectionIdA := test.connectionIdA
	connectionIdB := test.connectionIdB

	message := []byte{0x01, 0x02}

	test.sendServerToClientBlocking(connectionIdA, message, KcpReliable)
	if len(test.clientReceivedA) != 1 {
		t.Errorf("ClientA should receive 1 message, got %d", len(test.clientReceivedA))
	}
	if len(test.clientReceivedA) > 0 {
		if !bytes.Equal(test.clientReceivedA[0].data, message) {
			t.Error("ClientA message data mismatch")
		}
		if test.clientReceivedA[0].channel != KcpReliable {
			t.Errorf("ClientA channel mismatch: expected %v, got %v", KcpReliable, test.clientReceivedA[0].channel)
		}
	}

	test.sendServerToClientBlocking(connectionIdB, message, KcpReliable)
	if len(test.clientReceivedB) != 1 {
		t.Errorf("ClientB should receive 1 message, got %d", len(test.clientReceivedB))
	}
	if len(test.clientReceivedB) > 0 {
		if !bytes.Equal(test.clientReceivedB[0].data, message) {
			t.Error("ClientB message data mismatch")
		}
		if test.clientReceivedB[0].channel != KcpReliable {
			t.Errorf("ClientB channel mismatch: expected %v, got %v", KcpReliable, test.clientReceivedB[0].channel)
		}
	}
}

// TestMultiClientServerToClientUnreliableMessage 对应 C# 的 ServerToClientUnreliableMessage
func TestMultiClientServerToClientUnreliableMessage(t *testing.T) {
	test := &MultiClientServerTest{}
	test.setUp()
	defer test.tearDown()

	err := test.server.Start(MultiClientTestPort)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	test.connectClientsBlocking("")
	// Use recorded connection IDs instead of helper functions
	connectionIdA := test.connectionIdA
	connectionIdB := test.connectionIdB

	message := []byte{0x03, 0x04}

	test.sendServerToClientBlocking(connectionIdA, message, KcpUnreliable)
	if len(test.clientReceivedA) != 1 {
		t.Errorf("ClientA should receive 1 message, got %d", len(test.clientReceivedA))
	}
	if len(test.clientReceivedA) > 0 {
		if !bytes.Equal(test.clientReceivedA[0].data, message) {
			t.Error("ClientA message data mismatch")
		}
		if test.clientReceivedA[0].channel != KcpUnreliable {
			t.Errorf("ClientA channel mismatch: expected %v, got %v", KcpUnreliable, test.clientReceivedA[0].channel)
		}
	}

	test.sendServerToClientBlocking(connectionIdB, message, KcpUnreliable)
	if len(test.clientReceivedB) != 1 {
		t.Errorf("ClientB should receive 1 message, got %d", len(test.clientReceivedB))
	}
	if len(test.clientReceivedB) > 0 {
		if !bytes.Equal(test.clientReceivedB[0].data, message) {
			t.Error("ClientB message data mismatch")
		}
		if test.clientReceivedB[0].channel != KcpUnreliable {
			t.Errorf("ClientB channel mismatch: expected %v, got %v", KcpUnreliable, test.clientReceivedB[0].channel)
		}
	}
}
