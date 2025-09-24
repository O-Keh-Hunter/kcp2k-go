package kcp2k

import (
	"errors"
	"net"
	"runtime"
	"sync"
	"time"

	"golang.org/x/net/ipv4"
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

	// buffer pool for reducing GC pressure
	bufferPool Pool[[]byte]

	// optimization: reuse connection slice to avoid allocations
	connectionSlice []*KcpServerConnection

	// object pools for memory optimization
	connectionPool *ConnectionPool
	bufferPoolOpt  *BufferPool

	// batch operation support for Linux optimization
	ipv4Conn          *ipv4.PacketConn // IPv4 packet connection for batch operations
	sendBatchMessages []ipv4.Message   // Reusable message slice for send batch operations
	recvBatchMessages []ipv4.Message   // Reusable message slice for receive batch operations
	recvBatchBuffers  [][]byte         // Reusable buffer slice for receive batch operations
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
		bufferPoolOpt:  NewBufferPool(config),
	}
	s.connectionPool = NewConnectionPool(config, s.bufferPoolOpt)

	// initialize buffer pool
	s.bufferPool = New(func() []byte {
		return make([]byte, config.Mtu)
	})

	// initialize batch operation buffers if enabled
	if config.EnableBatchOps && runtime.GOOS == "linux" {
		// Initialize send batch buffers
		s.sendBatchMessages = make([]ipv4.Message, config.BatchSize)

		// Initialize receive batch buffers
		s.recvBatchMessages = make([]ipv4.Message, config.BatchSize)
		s.recvBatchBuffers = make([][]byte, config.BatchSize)
		for i := range s.recvBatchBuffers {
			s.recvBatchBuffers[i] = make([]byte, config.Mtu)
			s.recvBatchMessages[i].Buffers = [][]byte{s.recvBatchBuffers[i]}
		}
	}

	return s
}

func (s *KcpServer) IsActive() bool { return s.conn != nil }

// getBuf gets a buffer from the pool
func (s *KcpServer) getBuf(size int) []byte {
	buf := s.bufferPool.Get()
	if cap(buf) >= size {
		return buf[:size]
	}
	return make([]byte, size)
}

// putBuf returns a buffer to the pool
func (s *KcpServer) putBuf(buf []byte) {
	if cap(buf) > 0 {
		buf = buf[:0] // reset length but keep capacity
		s.bufferPool.Put(buf)
	}
}

func (s *KcpServer) LocalEndPoint() net.Addr {
	if s.conn == nil {
		return nil
	}
	return s.conn.LocalAddr()
}

func (s *KcpServer) Start(port uint16) error {
	if s.conn != nil {
		Log.Warning("[KCP] Server: already started!")
		return errors.New("server already started")
	}

	udpConn, err := s.createServerSocket(s.dualMode, port)
	if err != nil {
		return err
	}
	s.conn = udpConn

	err = ConfigureSocketBuffers(s.conn, s.config.RecvBufferSize, s.config.SendBufferSize)
	if err != nil {
		return err
	}

	// Initialize IPv4 packet connection for batch operations on Linux
	if s.config.EnableBatchOps && runtime.GOOS == "linux" {
		s.ipv4Conn = ipv4.NewPacketConn(s.conn)
		if s.ipv4Conn == nil {
			Log.Warning("[KCP] Server: failed to create IPv4 packet connection for batch operations")
		}
	}

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
	// Clean up IPv4 packet connection
	s.ipv4Conn = nil
}

func (s *KcpServer) Send(connectionId int, data []byte, channel KcpChannel) {
	s.mu.RLock()
	c, ok := s.connections[connectionId]
	s.mu.RUnlock()
	if ok {
		c.Send(data, channel)
	}
}

func (s *KcpServer) GetSendQueueCount(connectionId int) int {
	s.mu.RLock()
	c, ok := s.connections[connectionId]
	s.mu.RUnlock()
	if ok {
		return c.peer.SendQueueCount()
	}
	return 0
}

func (s *KcpServer) GetSendBufferCount(connectionId int) int {
	s.mu.RLock()
	c, ok := s.connections[connectionId]
	s.mu.RUnlock()
	if ok {
		return c.peer.SendBufferCount()
	}
	return 0
}

func (s *KcpServer) GetReceiveQueueCount(connectionId int) int {
	s.mu.RLock()
	c, ok := s.connections[connectionId]
	s.mu.RUnlock()
	if ok {
		return c.peer.ReceiveQueueCount()
	}
	return 0
}

func (s *KcpServer) GetReceiveBufferCount(connectionId int) int {
	s.mu.RLock()
	c, ok := s.connections[connectionId]
	s.mu.RUnlock()
	if ok {
		return c.peer.ReceiveBufferCount()
	}
	return 0
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

// recvBatch receives multiple UDP packets in a single system call (Linux only)
func (s *KcpServer) recvBatch() []struct {
	data []byte
	addr *net.UDPAddr
} {
	if s.ipv4Conn == nil || runtime.GOOS != "linux" {
		return nil
	}

	// Set read deadline for non-blocking behavior
	err := s.conn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
	if err != nil {
		Log.Error("[KCP] Server: SetReadDeadline error: %v", err)
		return nil
	}

	// Read batch of messages
	n, err := s.ipv4Conn.ReadBatch(s.recvBatchMessages, 0)
	if err != nil {
		if errors.Is(err, net.ErrClosed) || isTimeoutError(err) {
			return nil
		}
		Log.Error("[KCP] Server: ReadBatch error: %v", err)
		return nil
	}

	if n <= 0 {
		return nil
	}

	// Process received messages
	results := make([]struct {
		data []byte
		addr *net.UDPAddr
	}, 0, n)

	for i := 0; i < n; i++ {
		msg := &s.recvBatchMessages[i]
		if msg.N <= 0 {
			continue
		}

		// Extract UDP address from message
		udpAddr, ok := msg.Addr.(*net.UDPAddr)
		if !ok {
			continue
		}

		// Copy data to new buffer from pool
		buf := s.getBuf(msg.N)
		copy(buf, s.recvBatchBuffers[i][:msg.N])

		results = append(results, struct {
			data []byte
			addr *net.UDPAddr
		}{data: buf, addr: udpAddr})
	}

	return results
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
		if errors.Is(err, net.ErrClosed) || isTimeoutError(err) {
			return nil, nil, false
		}
		Log.Error("[KCP] Server: ReadFromUDP error: %v", err)
		return nil, nil, false
	}
	if n <= 0 {
		return nil, nil, false
	}
	// use buffer pool to reduce GC pressure
	buf := s.getBuf(n)
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
	// Try batch receive first (Linux only)
	if s.config.EnableBatchOps && runtime.GOOS == "linux" && s.ipv4Conn != nil {
		batchResults := s.recvBatch()
		if len(batchResults) > 0 {
			// Process all batched messages
			for _, result := range batchResults {
				s.processMessage(result.data, result.addr)
				// return buffer to pool after processing
				s.putBuf(result.data)
			}
		}
	} else {
		// Standard single message receive
		for {
			seg, remote, ok := s.rawReceiveFrom()
			if !ok {
				break
			}
			s.processMessage(seg, remote)
			// return buffer to pool after processing
			s.putBuf(seg)
		}
	}

	// tick all connections - reuse slice to avoid allocation
	s.mu.RLock()
	// reuse existing slice, reset length to 0
	s.connectionSlice = s.connectionSlice[:0]
	for _, c := range s.connections {
		s.connectionSlice = append(s.connectionSlice, c)
	}
	s.mu.RUnlock()

	for _, c := range s.connectionSlice {
		c.TickIncoming()
	}
	// remove disconnected - clear map instead of reallocating
	s.mu.Lock()
	for id := range s.toRemove {
		delete(s.connections, id)
		delete(s.toRemove, id) // clear the key instead of reallocating map
	}
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

// sendBatch sends multiple UDP packets in a single system call (Linux only)
func (s *KcpServer) sendBatch(messages []struct {
	data []byte
	addr *net.UDPAddr
}) error {
	if s.ipv4Conn == nil || runtime.GOOS != "linux" || len(messages) == 0 {
		return errors.New("batch send not available")
	}

	// Prepare batch messages
	batchSize := len(messages)
	if batchSize > len(s.sendBatchMessages) {
		batchSize = len(s.sendBatchMessages)
	}

	for i := 0; i < batchSize; i++ {
		s.sendBatchMessages[i].Buffers = [][]byte{messages[i].data}
		s.sendBatchMessages[i].Addr = messages[i].addr
	}

	// Send batch
	n, err := s.ipv4Conn.WriteBatch(s.sendBatchMessages[:batchSize], 0)
	if err != nil {
		Log.Error("[KCP] Server: WriteBatch error: %v", err)
		return err
	}

	if n != batchSize {
		Log.Warning("[KCP] Server: WriteBatch sent %d/%d messages", n, batchSize)
	}

	return nil
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
		// schedule removal and return connection to pool
		s.mu.Lock()
		s.toRemove[connectionId] = struct{}{}
		if conn, exists := s.connections[connectionId]; exists {
			// return connection to pool for reuse
			s.connectionPool.Put(conn)
		}
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

		// Try batch send if enabled and available (Linux only)
		if s.config.EnableBatchOps && runtime.GOOS == "linux" && s.ipv4Conn != nil {
			// For now, send single message via batch API
			// In a more advanced implementation, we could collect multiple messages
			messages := []struct {
				data []byte
				addr *net.UDPAddr
			}{{data: data, addr: remote}}

			err := s.sendBatch(messages)
			if err != nil {
				// Fallback to regular send if batch fails
				_, e := s.conn.WriteToUDP(data, remote)
				if e != nil {
					Log.Error("[KCP] Server: sendTo failed: %v", e)
				}
			}
		} else {
			// Standard single message send
			_, e := s.conn.WriteToUDP(data, remote)
			if e != nil {
				Log.Error("[KCP] Server: sendTo failed: %v", e)
			}
		}
	}
	// use connection pool to get reusable connection
	return s.connectionPool.Get(onConnected, onData, onDisconnected, onError, onRawSend, cookie, remote)
}
