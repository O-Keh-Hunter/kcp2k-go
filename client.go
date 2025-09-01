package kcp2k

import (
	"errors"
	"fmt"
	"net"
	"time"

	kcp "github.com/xtaci/kcp-go/v5"
)

// KcpClient is the Go implementation mirroring C# KcpClient.
// It owns the UDP socket and implements KcpPeerEventHandler for IO-agnostic KcpPeer.
type KcpClient struct {
	// IO
	conn       *net.UDPConn
	remoteAddr *net.UDPAddr
	localAddr  net.Addr

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
	Log.Debug("[KCP] Client: OnDisconnected connectionId: %d", c.peer.Cookie)
	c.connected = false
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn = nil
	c.remoteAddr = nil
	c.localAddr = nil
	if c.onDisconnected != nil {
		c.onDisconnected()
	}
	// 不要设置 active = false，这样 Tick 方法仍然可以工作
	// active 只在 Connect/Disconnect 时设置
}

// OnError forwards error details to user callback.
func (c *KcpClient) OnError(errorCode ErrorCode, message string) {
	Log.Error("[KCP] Client: OnError: %v, %s connectionId: %d", errorCode, message, c.peer.Cookie)
	if c.onError != nil {
		c.onError(errorCode, message)
	}
}

// RawSend sends one raw packet over UDP.
func (c *KcpClient) RawSend(data []byte) {
	if c.conn == nil {
		return
	}
	_, err := c.conn.Write(data)
	if err != nil {
		// match C# behavior: treat send errors as info rather than fatal
		Log.Error("[KCP] Client.RawSend: error sending data: %v", err)
		return
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
	return c.localAddr
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

	c.conn.SetWriteBuffer(c.config.SendBufferSize)
	c.conn.SetReadBuffer(c.config.RecvBufferSize)

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
		Log.Debug("[KCP] Client: defer send until cookie learned (channel=%d, len=%d)", channel, len(data))
		return
	}
	c.peer.SendData(data, channel)
}

// RawReceive tries to non-blockingly receive a UDP datagram.
// Returns (segment, true) if something was read, otherwise (nil, false).
func (c *KcpClient) RawReceive() ([]byte, bool) {
	if c.conn == nil {
		return nil, false
	}

	err := c.conn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
	if err != nil {
		Log.Error("[KCP] Client: SetReadDeadline error: %v", err)
		return nil, false
	}

	n, _, err := c.conn.ReadFromUDP(c.rawReceiveBuffer)
	if err != nil {
		if errors.Is(err, net.ErrClosed) || isTimeoutError(err) {
			return nil, false
		}
		Log.Error("[KCP] Client: ReadFromUDP error: %v", err)
		return nil, false
	}
	if n <= 0 {
		return nil, false
	}
	
	// 使用对象池获取缓冲区，避免频繁分配
	buf := c.bufferPool.Get()
	if cap(buf) < n {
		// 如果池中的缓冲区容量不足，创建新的缓冲区
		buf = make([]byte, n)
	} else {
		// 重新切片到正确的长度
		buf = buf[:n]
	}
	copy(buf, c.rawReceiveBuffer[:n])

	return buf, true
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
		if len(message) < kcp.IKCP_OVERHEAD || !(message[4] >= 81 && message[4] <= 84) {
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

// PutBuffer returns a buffer to the pool for reuse.
func (c *KcpClient) PutBuffer(buf []byte) {
	if cap(buf) > 0 {
		// 重置长度为0，但保持容量
		buf = buf[:0]
		c.bufferPool.Put(buf)
	}
}

// TickIncoming polls socket and then lets peer process incoming.
func (c *KcpClient) TickIncoming() {
	if c.active {
		// 限制循环次数以防止无限循环
		maxReceives := 100
		for i := 0; i < maxReceives; i++ {
			seg, ok := c.RawReceive()
			if !ok {
				break
			}
			c.RawInput(seg)
			// 处理完数据后归还缓冲区到池中
			c.PutBuffer(seg)
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
