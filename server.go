package kcp2k

import (
	"errors"
	"net"
	"sync"
	"time"
)

// sendTask represents a UDP send task
type sendTask struct {
	data   []byte
	remote *net.UDPAddr
}

// receiveTask represents a UDP receive task
type receiveTask struct {
	data   []byte
	remote *net.UDPAddr
}

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

	// async send queue and worker
	sendQueue  *LockFreeQueue[sendTask]
	sendSignal chan struct{}
	sendDone   chan struct{}
	// async receive queue and worker
	receiveQueue  *LockFreeQueue[receiveTask]
	receiveSignal chan struct{}
	receiveDone   chan struct{}
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

	return s
}

func (s *KcpServer) IsActive() bool { return s.conn != nil }

// sendWorker processes async send tasks from the lock-free queue
func (s *KcpServer) sendWorker() {
	for {
		select {
		case <-s.sendDone:
			return
		case <-s.sendSignal:
			for {
				// Try to dequeue a task
				if task, ok := s.sendQueue.Dequeue(); ok {
					if s.conn != nil {
						_, e := s.conn.WriteToUDP(task.data, task.remote)
						if e != nil {
							// only log error if connection is not closed
							if !errors.Is(e, net.ErrClosed) {
								Log.Error("[KCP] Server: async sendTo failed: %v", e)
							}
						}
					}
					// return buffer to pool after sending
					s.PutBuf(task.data)
				} else {
					break
				}
			}
		}
	}
}

// GetBuf gets a buffer from the pool
func (s *KcpServer) GetBuf(size int) []byte {
	buf := s.bufferPool.Get()
	if cap(buf) >= size {
		return buf[:size]
	}
	return make([]byte, size)
}

// PutBuf returns a buffer to the pool
func (s *KcpServer) PutBuf(buf []byte) {
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

	// Initialize sendDone channel and start send worker
	s.sendSignal = make(chan struct{}, 1)
	s.sendDone = make(chan struct{})
	s.sendQueue = NewLockFreeQueue[sendTask]()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				Log.Error("[KCP] Server: sendWorker panic: %v", r)
			}
		}()
		s.sendWorker()
	}()

	// initialize receive queue and start workers
	s.receiveSignal = make(chan struct{}, 1)
	s.receiveDone = make(chan struct{})
	s.receiveQueue = NewLockFreeQueue[receiveTask]()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				Log.Error("[KCP] Server: receiveWorker panic: %v", r)
			}
		}()
		s.receiveWorker()
	}()

	return nil
}

func (s *KcpServer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already stopped
	if s.conn == nil {
		return
	}

	for id := range s.connections {
		delete(s.connections, id)
	}
	s.toRemove = make(map[int]struct{})

	_ = s.conn.Close()
	s.conn = nil

	// stop send worker
	if s.sendDone != nil {
		close(s.sendDone)
		s.sendDone = nil
	}

	// stop receive worker
	if s.receiveDone != nil {
		close(s.receiveDone)
		s.receiveDone = nil
	}
}

func (s *KcpServer) Send(connectionId int, data []byte, channel KcpChannel) {
	s.mu.RLock()
	c, ok := s.connections[connectionId]
	s.mu.RUnlock()
	if ok {
		c.Send(data, channel)
	} else {
		Log.Warning("[KCP] Server: Send failed, connection %d not found", connectionId)
	}
}

func (s *KcpServer) GetSendQueueCount(connectionId int) int {
	s.mu.RLock()
	c, ok := s.connections[connectionId]
	s.mu.RUnlock()
	if ok {
		return c.peer.SendQueueCount()
	} else {
		Log.Warning("[KCP] Server: GetSendQueueCount failed, connection %d not found", connectionId)
	}
	return 0
}

func (s *KcpServer) GetSendBufferCount(connectionId int) int {
	s.mu.RLock()
	c, ok := s.connections[connectionId]
	s.mu.RUnlock()
	if ok {
		return c.peer.SendBufferCount()
	} else {
		Log.Warning("[KCP] Server: GetSendBufferCount failed, connection %d not found", connectionId)
	}
	return 0
}

func (s *KcpServer) GetReceiveQueueCount(connectionId int) int {
	s.mu.RLock()
	c, ok := s.connections[connectionId]
	s.mu.RUnlock()
	if ok {
		return c.peer.ReceiveQueueCount()
	} else {
		Log.Warning("[KCP] Server: GetReceiveQueueCount failed, connection %d not found", connectionId)
	}
	return 0
}

func (s *KcpServer) GetReceiveBufferCount(connectionId int) int {
	s.mu.RLock()
	c, ok := s.connections[connectionId]
	s.mu.RUnlock()
	if ok {
		return c.peer.ReceiveBufferCount()
	} else {
		Log.Warning("[KCP] Server: GetReceiveBufferCount failed, connection %d not found", connectionId)
	}
	return 0
}

func (s *KcpServer) Disconnect(connectionId int) {
	s.mu.RLock()
	c, ok := s.connections[connectionId]
	s.mu.RUnlock()
	if ok {
		c.Disconnect()
	} else {
		Log.Warning("[KCP] Server: Disconnect failed, connection %d not found", connectionId)
	}
}

func (s *KcpServer) GetClientEndPoint(connectionId int) *net.UDPAddr {
	s.mu.RLock()
	c, ok := s.connections[connectionId]
	s.mu.RUnlock()
	if ok {
		return c.RemoteAddr()
	} else {
		Log.Warning("[KCP] Server: GetClientEndPoint failed, connection %d not found", connectionId)
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
	} else {
		Log.Warning("[KCP] Server: GetConnection failed, connection %d not found", connectionId)
	}
	return nil
}

// ConnectionsCount returns the number of active connections
func (s *KcpServer) ConnectionsCount() int {
	s.mu.RLock()
	count := len(s.connections)
	s.mu.RUnlock()
	return count
}

// receiveWorker continuously reads UDP packets and queues them for async processing
func (s *KcpServer) receiveWorker() {
	for {
		select {
		case <-s.receiveDone:
			return
		default:
			// use mutex to safely access conn
			s.mu.RLock()
			conn := s.conn
			s.mu.RUnlock()

			n, addr, err := conn.ReadFromUDP(s.recvBuf)
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				if isTimeoutError(err) {
					continue
				}
				Log.Error("[KCP] Server: ReadFromUDP error: %v", err)
				continue
			}
			if n <= 0 {
				continue
			}

			// get buffer from pool and copy data
			buf := s.GetBuf(n)
			copy(buf, s.recvBuf[:n])

			// queue the received data for async processing
			s.receiveQueue.Enqueue(receiveTask{data: buf, remote: addr})
			select {
			case s.receiveSignal <- struct{}{}:
			default:
			}
		}
	}
}

func isTimeoutError(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}

// TickIncoming: tick all connections
func (s *KcpServer) TickIncoming() {
	if s.receiveQueue == nil {
		return
	}

	// input all received messages
	for {
		select {
		case <-s.receiveSignal:
			for {
				// Try to dequeue a task
				if task, ok := s.receiveQueue.Dequeue(); ok {
					s.processMessage(task.data, task.remote)
					// return buffer to pool after processing
					s.PutBuf(task.data)
				} else {
					break
				}
			}
		default:
			goto DRAIN_DONE
		}
	}
DRAIN_DONE:

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
	// if connection is being removed, ignore all incoming messages until it's fully removed.
	if _, ok := s.toRemove[id]; ok {
		s.mu.RUnlock()
		return
	}
	c, ok := s.connections[id]
	s.mu.RUnlock()
	if ok {
		c.RawInput(segment)
		return
	}

	// Ignore stray disconnect messages.
	// If a client sends 5 unreliable disconnects, the server might have
	// already closed the connection after the first one. The next 4 would
	// be for a non-existent connection, causing the server to create a new
	// peer that is immediately disconnected again.
	//
	// Note: this is KcpUnreliable specific.
	// Reliable disconnects are not a thing.
	//
	// Packet structure:
	// [0] = channel: KcpUnreliable
	// [1..4] = cookie
	// [5] = header: KcpHeaderUnrelDisconnect
	if len(segment) >= 6 &&
		KcpChannel(segment[0]) == KcpUnreliable &&
		KcpHeaderUnreliable(segment[5]) == KcpHeaderUnrelDisconnect {
		Log.Debug("[KCP] Server: ignored stray disconnect message for non-existent connectionId %d", id)
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
		s.mu.Lock()
		// check if connection exists and not already being removed
		if _, exists := s.connections[connectionId]; !exists {
			s.mu.Unlock()
			return
		}
		if _, removing := s.toRemove[connectionId]; removing {
			s.mu.Unlock()
			return
		}

		Log.Debug("[KCP] Server: OnDisconnected connectionId: %d", connectionId)
		// schedule removal
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
		// performance monitoring for onRawSend
		start := time.Now()

		// async send using channel to avoid blocking KCP processing
		if s.conn == nil {
			return
		}

		// get buffer from pool to avoid GC pressure
		dataCopy := s.GetBuf(len(data))
		copy(dataCopy, data)

		// send task to worker via lock-free queue
		s.sendQueue.Enqueue(sendTask{data: dataCopy, remote: remote})
		// signal send worker
		select {
		case s.sendSignal <- struct{}{}:
		default:
		}

		totalDuration := time.Since(start)

		// log onRawSend performance breakdown
		if totalDuration > 10*time.Millisecond {
			Log.Warning("[KcpServerConnection] OnRawSend breakdown: total=%v, size=%d",
				totalDuration, len(data))
		}
	}

	Log.Debug("[KCP] Server: createConnection connectionId: %d, cookie: %d, remote: %s", connectionId, cookie, remote.String())
	return NewKcpServerConnection(onConnected, onData, onDisconnected, onError, onRawSend, s.config, cookie, remote)
}
