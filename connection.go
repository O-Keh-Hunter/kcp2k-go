package kcp2k

import (
	"net"
)

// KcpServerConnection mirrors C# KcpServerConnection.
// It owns a protocol peer for one remote endpoint and forwards callbacks.
type KcpServerConnection struct {
	// remote endpoint
	remoteAddr *net.UDPAddr

	// config and peer
	config KcpConfig
	peer   *KcpPeer

	// callbacks
	onConnected    func(*KcpServerConnection)
	onData         func([]byte, KcpChannel)
	onDisconnected func()
	onError        func(ErrorCode, string)
	onRawSend      func([]byte)
}

func NewKcpServerConnection(
	onConnected func(*KcpServerConnection),
	onData func([]byte, KcpChannel),
	onDisconnected func(),
	onError func(ErrorCode, string),
	onRawSend func([]byte),
	config KcpConfig,
	cookie uint32,
	remote *net.UDPAddr,
) *KcpServerConnection {
	c := &KcpServerConnection{
		onConnected:    onConnected,
		onData:         onData,
		onDisconnected: onDisconnected,
		onError:        onError,
		onRawSend:      onRawSend,
		config:         config,
		remoteAddr:     remote,
	}
	// create a peer with server-assigned cookie
	c.peer = NewKcpPeer(0, cookie, config, c)
	return c
}

// OnAuthenticated: once we receive first client hello, reply with hello so client learns cookie, then raise connected.
func (c *KcpServerConnection) OnAuthenticated() {
	c.peer.SendHello()
	if c.onConnected != nil {
		c.onConnected(c)
	}
}

func (c *KcpServerConnection) OnData(message []byte, channel KcpChannel) {
	if c.onData != nil {
		c.onData(message, channel)
	}
}

func (c *KcpServerConnection) OnDisconnected() {
	if c.onDisconnected != nil {
		c.onDisconnected()
	}
}

func (c *KcpServerConnection) OnError(errorCode ErrorCode, msg string) {
	if c.onError != nil {
		c.onError(errorCode, msg)
	}
}

// RawSend is called by peer's reliable output callback after placing channel+cookie.
// Server should send this datagram through its UDP socket to c.remoteAddr via the provided callback.
func (c *KcpServerConnection) RawSend(data []byte) {
	if c.onRawSend != nil {
		c.onRawSend(data)
	}
}

// RemoteAddr returns the remote UDP address of this connection.
func (c *KcpServerConnection) RemoteAddr() *net.UDPAddr { return c.remoteAddr }

// Peer exposes the underlying peer for ticking and sending.
func (c *KcpServerConnection) Peer() *KcpPeer { return c.peer }

// RawInput inserts raw UDP datagram into the peer after validating cookie and extracting channel.
func (c *KcpServerConnection) RawInput(segment []byte) {
	// ensure valid size: at least 1 byte channel + 4 bytes cookie
	if len(segment) <= CHANNEL_HEADER_SIZE+COOKIE_HEADER_SIZE {
		Log.Error("[KCP] ServerConnection: received message too short: %d bytes", len(segment))
		return
	}
	channel := segment[0]
	messageCookie, ok := Decode32U(segment, 1)
	if !ok {
		Log.Error("[KCP] ServerConnection: failed to decode cookie from message")
		return
	}

	// security: after authentication we expect cookie to match to protect against UDP spoofing.
	// During handshake (KcpConnected state), we accept any cookie from client
	switch c.peer.State {
	case KcpAuthenticated:
		if messageCookie != c.peer.Cookie {
			Log.Warning("[KCP] ServerConnection: dropped message with invalid cookie: %d from %v expected: %d state: %v. this can happen with retransmitted hello or spoofing.", messageCookie, c.remoteAddr, c.peer.Cookie, c.peer.State)
			return
		}
	case KcpConnected:
		// During handshake, accept any cookie from client
		// The client will send its own cookie, which is fine during handshake

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
			Log.Debug("[KCP] ServerConnection: drop non-KCP on reliable path len=%d first=% X", len(message), message[:previewLen])
			return
		}
		c.peer.OnRawInputReliable(message)
	case KcpUnreliable:
		c.peer.OnRawInputUnreliable(message)
	default:
		Log.Warning("[KCP] ServerConnection: invalid channel header: %d from %v, likely internet noise", channel, c.remoteAddr)
	}
}

// Convenience: forward ticks and send
func (c *KcpServerConnection) TickIncoming()                        { c.peer.TickIncoming() }
func (c *KcpServerConnection) TickOutgoing()                        { c.peer.TickOutgoing() }
func (c *KcpServerConnection) Send(data []byte, channel KcpChannel) { c.peer.SendData(data, channel) }
func (c *KcpServerConnection) Disconnect()                          { c.peer.Disconnect() }
