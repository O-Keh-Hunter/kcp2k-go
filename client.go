package kcp2k

import (
	"errors"
	"fmt"
	"net"
)

// sendTask represents a UDP send task for client
type clientSendTask struct {
	data []byte
}

// receiveTask represents a UDP receive task for client
type clientReceiveTask struct {
	data []byte
}

// KcpClient is the Go implementation mirroring C# KcpClient.
// It owns the UDP socket and implements KcpPeerEventHandler for IO-agnostic KcpPeer.
type KcpClient struct {
	// IO
	conn       *net.UDPConn
	remoteAddr *net.UDPAddr

	// config
	config KcpConfig

	// peer (protocol logic)
	peer *KcpPeer

	// state
	active    bool
	connected bool

	// buffers
	rawReceiveBuffer []byte
	bufferPool       Pool[[]byte]

	// async send queue and worker
	sendQueue  *LockFreeQueue[clientSendTask]
	sendSignal chan struct{}
	sendDone   chan struct{}

	// async receive queue and worker
	receiveQueue  *LockFreeQueue[clientReceiveTask]
	receiveSignal chan struct{}
	receiveDone   chan struct{}

	// callbacks
	onConnected    func()
	onData         func([]byte, KcpChannel)
	onDisconnected func()
	onError        func(ErrorCode, string)
}

// NewKcpClient constructs a new client. Call Connect to initiate a session.
func NewKcpClient(onConnected func(), onData func([]byte, KcpChannel), onDisconnected func(), onError func(ErrorCode, string), config KcpConfig) *KcpClient {
	c := &KcpClient{
		config:           config,
		onConnected:      onConnected,
		onData:           onData,
		onDisconnected:   onDisconnected,
		onError:          onError,
		rawReceiveBuffer: make([]byte, config.Mtu),
		bufferPool:       New(func() []byte { return make([]byte, 0, config.Mtu) }),
		sendQueue:        NewLockFreeQueue[clientSendTask](),
		receiveQueue:     NewLockFreeQueue[clientReceiveTask](),
	}
	// client has no cookie yet. it will be assigned from first server message.
	c.peer = NewKcpPeer(0, 0, config, c)
	return c
}

// OnAuthenticated is invoked by peer when the handshake completes.
func (c *KcpClient) OnAuthenticated() {
	Log.Debug("[KCP] Client: OnConnected connectionId: %d", c.peer.Cookie)
	c.connected = true
	if c.onConnected != nil {
		c.onConnected()
	}
}

// OnData forwards data to user callback.
func (c *KcpClient) OnData(data []byte, channel KcpChannel) {
	if c.onData != nil {
		c.onData(data, channel)
	}
}

// OnDisconnected tears down connection and calls user callback.
func (c *KcpClient) OnDisconnected() {
	// stop async workers
	if c.sendDone != nil {
		close(c.sendDone)
		c.sendDone = nil
	}
	// stop receive worker
	if c.receiveDone != nil {
		close(c.receiveDone)
		c.receiveDone = nil
	}

	if c.conn != nil {
		_ = c.conn.Close()
	}

	c.conn = nil
	c.remoteAddr = nil
	c.connected = false

	if c.onDisconnected != nil {
		c.onDisconnected()
	}
	Log.Debug("[KCP] Client: OnDisconnected connectionId: %d", c.peer.Cookie)
}

// OnError forwards error details to user callback.
func (c *KcpClient) OnError(errorCode ErrorCode, message string) {
	Log.Error("[KCP] Client: OnError: %v, %s connectionId: %d", errorCode, message, c.peer.Cookie)
	if c.onError != nil {
		c.onError(errorCode, message)
	}
}

// sendWorker processes async send tasks from the lock-free queue
func (c *KcpClient) sendWorker() {
	for {
		select {
		case <-c.sendDone:
			return
		case <-c.sendSignal:
			for {
				if task, ok := c.sendQueue.Dequeue(); ok {
					if c.conn != nil {
						_, err := c.conn.Write(task.data)
						if err != nil && !errors.Is(err, net.ErrClosed) {
							Log.Error("[KCP] Client: async send failed: %v", err)
						}
					}
					c.putBuf(task.data)
				} else {
					break
				}
			}
		}
	}
}

// receiveWorker continuously reads UDP packets and queues them for async processing
func (c *KcpClient) receiveWorker() {
	for {
		select {
		case <-c.receiveDone:
			return
		default:
			n, _, err := c.conn.ReadFromUDP(c.rawReceiveBuffer)
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				if isTimeoutError(err) {
					continue
				}
				Log.Error("[KCP] Client: ReadFromUDP error: %v", err)
				continue
			}
			if n <= 0 {
				continue
			}

			// get buffer from pool and copy data
			buf := c.getBuf(n)
			copy(buf, c.rawReceiveBuffer[:n])

			// queue the received data for async processing
			c.receiveQueue.Enqueue(clientReceiveTask{data: buf})
			select {
			case c.receiveSignal <- struct{}{}:
			default:
			}
		}
	}
}

// RawSend sends one raw packet over UDP asynchronously.
func (c *KcpClient) RawSend(data []byte) {
	if c.conn == nil {
		return
	}

	// get buffer from pool and copy data
	buf := c.getBuf(len(data))
	copy(buf, data)

	// enqueue for async sending
	c.sendQueue.Enqueue(clientSendTask{data: buf})
	select {
	case c.sendSignal <- struct{}{}:
	default:
	}
}

// Connected returns whether the client is connected.
func (c *KcpClient) Connected() bool {
	return c.connected
}

// LocalEndPoint returns the local UDP address if available.
func (c *KcpClient) LocalEndPoint() net.Addr {
	if c.conn != nil {
		return c.conn.LocalAddr()
	}
	return nil
}

// Connect resolves the address, creates UDP socket and starts a fresh peer.
func (c *KcpClient) Connect(address string, port uint16) error {
	if c.connected {
		Log.Warning("[KCP] Client: already connected!")
		return nil
	}

	ips, success := ResolveHostname(address)
	if !success || len(ips) == 0 {
		c.OnError(ErrorCodeDnsResolve, fmt.Sprintf("Failed to resolve host: %s", address))
		if c.onDisconnected != nil {
			c.onDisconnected()
		}
		return errors.New("failed to resolve hostname")
	}

	// reset peer for a fresh session; cookie will be assigned from first server message
	c.peer.Reset(c.config)
	c.remoteAddr = &net.UDPAddr{IP: ips[0], Port: int(port)}
	conn, err := net.DialUDP("udp", nil, c.remoteAddr)
	if err != nil {
		c.OnError(ErrorCodeUnexpected, fmt.Sprintf("failed to dial %s:%d", address, port))
		return err
	}
	c.conn = conn
	c.active = true

	if err := c.conn.SetWriteBuffer(c.config.SendBufferSize); err != nil {
		Log.Warning("[KCP] Client: SetWriteBuffer failed: %v", err)
	}
	if err := c.conn.SetReadBuffer(c.config.RecvBufferSize); err != nil {
		Log.Warning("[KCP] Client: SetReadBuffer failed: %v", err)
	}

	// Initialize async workers
	c.sendSignal = make(chan struct{}, 1)
	c.sendDone = make(chan struct{})
	c.receiveSignal = make(chan struct{}, 1)
	c.receiveDone = make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				Log.Error("[Client] Client: sendWorker panic: %v", r)
			}
		}()
		c.sendWorker()
	}()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				Log.Error("[KCP] Client: receiveWorker panic: %v", r)
			}
		}()
		c.receiveWorker()
	}()

	// immediately send hello; note cookie is 0 until server responds
	c.peer.SendHello()
	return nil
}

// Send transmits application payload over selected channel if connected.
func (c *KcpClient) Send(data []byte, channel KcpChannel) {
	if !c.connected {
		Log.Warning("[KCP] Client: can't send because not connected!")
		return
	}
	// ensure cookie learned before any send to avoid server drops
	if c.peer != nil && c.peer.Cookie == 0 {
		Log.Warning("[KCP] Client: defer send until cookie learned (channel=%d, len=%d)", channel, len(data))
		return
	}
	c.peer.SendData(data, channel)
}

// RawInput inserts a single raw datagram into peer.
func (c *KcpClient) RawInput(segment []byte) {
	if len(segment) <= CHANNEL_HEADER_SIZE+COOKIE_HEADER_SIZE {
		return
	}
	channel := segment[0]
	if cookie, ok := Decode32U(segment, 1); ok {
		if cookie == 0 {
			Log.Error("[KCP] Client: received message with cookie=0, this should never happen. server should always include the security cookie.")
		}
		if c.peer.Cookie == 0 {
			c.peer.Cookie = cookie

		} else if c.peer.Cookie != cookie {
			Log.Warning("[KCP] Client: dropping message with mismatching cookie: %d expected: %d.", cookie, c.peer.Cookie)
			return
		}
	}
	message := segment[CHANNEL_HEADER_SIZE+COOKIE_HEADER_SIZE:]
	switch KcpChannel(channel) {
	case KcpReliable:
		// sanity: reliable path must carry KCP segment
		if len(message) < IKCP_OVERHEAD || !(message[4] >= IKCP_CMD_PUSH && message[4] <= IKCP_CMD_WINS) {
			previewLen := len(message)
			if previewLen > 24 {
				previewLen = 24
			}
			Log.Debug("[KCP] Client: drop non-KCP on reliable path len=%d first=% X", len(message), message[:previewLen])
			return
		}
		c.peer.OnRawInputReliable(message)
	case KcpUnreliable:
		c.peer.OnRawInputUnreliable(message)
	default:
		Log.Warning("[KCP] Client: invalid channel header: %d, likely internet noise", channel)
	}
}

// TickIncoming processes async received messages and then lets peer process incoming.
func (c *KcpClient) TickIncoming() {
	if c.active {
		// process all received messages from async queue
		select {
		case <-c.receiveSignal:
			for {
				// Try to dequeue a task
				if task, ok := c.receiveQueue.Dequeue(); ok {
					c.RawInput(task.data)
					c.putBuf(task.data)
				} else {
					break
				}
			}
		default:
		}
	}
	if c.active {
		c.peer.TickIncoming()
	}
}

// TickOutgoing lets peer flush outgoing messages.
func (c *KcpClient) TickOutgoing() {
	if c.active {
		c.peer.TickOutgoing()
	}
}

// Tick convenience processes incoming then outgoing.
func (c *KcpClient) Tick() {
	c.TickIncoming()
	c.TickOutgoing()
}

// getBuf gets a buffer from the pool
func (c *KcpClient) getBuf(size int) []byte {
	buf := c.bufferPool.Get()
	if cap(buf) >= size {
		return buf[:size]
	}
	return make([]byte, size)
}

// putBuf returns a buffer to the pool
func (c *KcpClient) putBuf(buf []byte) {
	if cap(buf) > 0 {
		buf = buf[:0] // reset length but keep capacity
		c.bufferPool.Put(buf)
	}
}

// GetRTT returns the current round-trip time in milliseconds.
// Returns 0 if no RTT measurement is available yet.
func (c *KcpClient) GetRTT() uint32 {
	if c.peer == nil {
		return 0
	}
	return c.peer.GetRTT()
}

// Disconnect closes the connection via peer API.
func (c *KcpClient) Disconnect() {
	c.peer.Disconnect()
}

func (c *KcpClient) GetSendQueueCount(connectionId int) int {
	return c.peer.SendQueueCount()
}

func (c *KcpClient) GetSendBufferCount(connectionId int) int {
	return c.peer.SendBufferCount()
}

func (c *KcpClient) GetReceiveQueueCount(connectionId int) int {
	return c.peer.ReceiveQueueCount()
}

func (c *KcpClient) GetReceiveBufferCount(connectionId int) int {
	return c.peer.ReceiveBufferCount()
}
