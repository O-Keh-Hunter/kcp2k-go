package kcp2k

import (
	"errors"
	"net"
	"sync"
	"time"
)

// KcpServer manages UDP socket and KcpServerConnection instances.
type KcpServer struct {
	// callbacks
	onConnected    func(int)
	onData         func(int, []byte, KcpChannel)
	onDisconnected func(int)
	onError        func(int, ErrorCode, string)

	// configuration
	config KcpConfig

	// state
	conn     *net.UDPConn
	recvBuf  []byte
	dualMode bool
	mu       sync.RWMutex

	// connections by id
	connections map[int]*KcpServerConnection
	toRemove    map[int]struct{}
}

func NewKcpServer(onConnected func(int), onData func(int, []byte, KcpChannel), onDisconnected func(int), onError func(int, ErrorCode, string), config KcpConfig) *KcpServer {
	s := &KcpServer{
		onConnected:    onConnected,
		onData:         onData,
		onDisconnected: onDisconnected,
		onError:        onError,
		config:         config,
		recvBuf:        make([]byte, config.Mtu),
		dualMode:       config.DualMode,
		connections:    make(map[int]*KcpServerConnection),
		toRemove:       make(map[int]struct{}),
	}
	return s
}

func (s *KcpServer) IsActive() bool { return s.conn != nil }

func (s *KcpServer) LocalEndPoint() net.Addr {
	if s.conn == nil {
		return nil
	}
	return s.conn.LocalAddr()
}

func (s *KcpServer) Start(port uint16) error {
	if s.conn != nil {
		Log.Warning("[KCP] Server: already started!")
		return nil
	}

	udpConn, err := s.createServerSocket(s.dualMode, port)
	if err != nil {
		return err
	}
	s.conn = udpConn

	ConfigureSocketBuffers(s.conn, s.config.RecvBufferSize, s.config.SendBufferSize)

	return nil
}

func (s *KcpServer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.connections {
		delete(s.connections, id)
	}
	s.toRemove = make(map[int]struct{})
	if s.conn != nil {
		_ = s.conn.Close()
	}
	s.conn = nil
}

func (s *KcpServer) Send(connectionId int, data []byte, channel KcpChannel) {
	s.mu.RLock()
	c, ok := s.connections[connectionId]
	s.mu.RUnlock()
	if ok {
		c.Send(data, channel)
	}
}

func (s *KcpServer) Disconnect(connectionId int) {
	s.mu.RLock()
	c, ok := s.connections[connectionId]
	s.mu.RUnlock()
	if ok {
		c.Disconnect()
	}
}

func (s *KcpServer) GetClientEndPoint(connectionId int) *net.UDPAddr {
	s.mu.RLock()
	c, ok := s.connections[connectionId]
	s.mu.RUnlock()
	if ok {
		return c.RemoteAddr()
	}
	return nil
}

// GetConnection returns the KcpServerConnection for the given connection ID.
// Returns nil if the connection doesn't exist.
func (s *KcpServer) GetConnection(connectionId int) *KcpServerConnection {
	s.mu.RLock()
	c, ok := s.connections[connectionId]
	s.mu.RUnlock()
	if ok {
		return c
	}
	return nil
}

// receive one datagram non-blocking-ish using deadlines.
func (s *KcpServer) rawReceiveFrom() ([]byte, *net.UDPAddr, bool) {
	if s.conn == nil {
		return nil, nil, false
	}
	err := s.conn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
	if err != nil {
		Log.Error("[KCP] Server: SetReadDeadline error: %v", err)
		return nil, nil, false
	}

	n, addr, err := s.conn.ReadFromUDP(s.recvBuf)
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			return nil, nil, false
		}

		// 只在非超时错误时记录
		if !isTimeoutError(err) {
			Log.Error("[KCP] Server: ReadFromUDP error: %v", err)
		}
		return nil, nil, false
	}
	if n <= 0 {
		return nil, nil, false
	}
	buf := make([]byte, n)
	copy(buf, s.recvBuf[:n])

	return buf, addr, true
}

func isTimeoutError(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}

// TickIncoming: receive UDP, process connections, and tick all.
func (s *KcpServer) TickIncoming() {
	// input all received messages
	for {
		seg, remote, ok := s.rawReceiveFrom()
		if !ok {
			break
		}
		s.processMessage(seg, remote)
	}
	// tick all connections
	s.mu.RLock()
	connections := make([]*KcpServerConnection, 0, len(s.connections))
	for _, c := range s.connections {
		connections = append(connections, c)
	}
	s.mu.RUnlock()

	for _, c := range connections {
		c.TickIncoming()
	}
	// remove disconnected
	s.mu.Lock()
	for id := range s.toRemove {
		delete(s.connections, id)
	}
	s.toRemove = make(map[int]struct{})
	s.mu.Unlock()
}

// TickOutgoing: flush all connections
func (s *KcpServer) TickOutgoing() {
	for _, c := range s.connections {
		c.TickOutgoing()
	}
}

// Tick convenience
func (s *KcpServer) Tick() {
	s.TickIncoming()
	s.TickOutgoing()
}

// internal helpers -----------------------------------------------------------

func (s *KcpServer) createServerSocket(dual bool, port uint16) (*net.UDPConn, error) {
	if dual {
		// Try IPv6 dual stack first
		addr6 := &net.UDPAddr{IP: net.IPv6unspecified, Port: int(port)}

		conn, err := net.ListenUDP("udp6", addr6)
		if err == nil {
			return conn, nil
		}
		Log.Warning("[KCP] Server: failed to create IPv6 dual-mode socket: %v, falling back to IPv4", err)
		// Fallback to IPv4
		addr4 := &net.UDPAddr{IP: net.IPv4zero, Port: int(port)}
		conn, err = net.ListenUDP("udp4", addr4)
		if err != nil {
			Log.Error("[KCP] Server: failed to create IPv4 socket: %v", err)
			return nil, err
		}

		return conn, nil
	}

	addr := &net.UDPAddr{IP: net.IPv4zero, Port: int(port)}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		Log.Error("[KCP] Server: failed to create socket: %v", err)
		return nil, err
	}

	return conn, nil
}

func (s *KcpServer) processMessage(segment []byte, remote *net.UDPAddr) {
	id := ConnectionHash(remote)
	s.mu.RLock()
	c, ok := s.connections[id]
	s.mu.RUnlock()
	if ok {
		c.RawInput(segment)
		return
	}

	// create a new connection but don't add yet. only if first message is handshake.
	conn := s.createConnection(id, remote)
	conn.RawInput(segment)
	conn.TickIncoming()
	// if it wasn't a proper handshake, it simply won't be added by OnAuthenticated callback.
}

func (s *KcpServer) createConnection(connectionId int, remote *net.UDPAddr) *KcpServerConnection {
	cookie := GenerateCookie()
	// wrap callbacks to include connectionId and add/remove semantics
	onConnected := func(conn *KcpServerConnection) {
		Log.Debug("[KCP] Server: OnConnected connectionId: %d", connectionId)
		// add to map
		s.mu.Lock()
		s.connections[connectionId] = conn
		s.mu.Unlock()

		// fire user callback
		if s.onConnected != nil {
			s.onConnected(connectionId)
		}
	}
	onData := func(msg []byte, ch KcpChannel) {
		if s.onData != nil {
			s.onData(connectionId, msg, ch)
		}
	}
	onDisconnected := func() {
		Log.Debug("[KCP] Server: OnDisconnected connectionId: %d", connectionId)
		// schedule removal
		s.mu.Lock()
		s.toRemove[connectionId] = struct{}{}
		s.mu.Unlock()

		if s.onDisconnected != nil {
			s.onDisconnected(connectionId)
		}
	}
	onError := func(errorCode ErrorCode, reason string) {
		if errorCode != ErrorCodeTimeout {
			Log.Error("[KCP] Server: OnError connectionId: %d, errorCode: %d, reason: %s", connectionId, errorCode, reason)
		}

		if s.onError != nil {
			s.onError(connectionId, errorCode, reason)
		}
	}
	onRawSend := func(data []byte) {
		// send back to this remote
		if s.conn == nil {
			return
		}
		_, e := s.conn.WriteToUDP(data, remote)
		if e != nil {
			Log.Error("[KCP] Server: sendTo failed: %v", e)
		}
	}
	return NewKcpServerConnection(onConnected, onData, onDisconnected, onError, onRawSend, s.config, cookie, remote)
}
