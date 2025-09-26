package kcp2k

import (
	"errors"
	"net"
	"runtime"
	"runtime/debug"
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

	// async send queue and worker
	sendQueue  *LockFreeQueue[ipv4.Message]
	sendSignal chan struct{}
	sendDone   chan struct{}
	// async receive queue and worker
	receiveQueue  *LockFreeQueue[ipv4.Message]
	receiveSignal chan struct{}
	receiveDone   chan struct{}

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

	// initialize batch operation buffers
	batchSize := 1
	if config.EnableBatchOps && runtime.GOOS == "linux" {
		batchSize = config.BatchSize
	}
	// Initialize send batch buffers
	s.sendBatchMessages = make([]ipv4.Message, batchSize)

	// Initialize receive batch buffers
	s.recvBatchMessages = make([]ipv4.Message, batchSize)
	s.recvBatchBuffers = make([][]byte, batchSize)
	for i := range s.recvBatchBuffers {
		s.recvBatchBuffers[i] = make([]byte, config.Mtu)
		s.recvBatchMessages[i].Buffers = [][]byte{s.recvBatchBuffers[i]}
	}

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
			batchCount := 0
			messages := s.sendBatchMessages[:0] // reuse slice, reset length

			// Collect tasks for batching
			for batchCount < len(s.sendBatchMessages) {
				if msg, ok := s.sendQueue.Dequeue(); ok {
					messages = append(messages, msg)
					batchCount++
				} else {
					break
				}
			}

			// Send batch if we have messages
			if batchCount > 0 {
				// Use WriteBatch if supported
				if s.ipv4Conn != nil {
					n, err := s.ipv4Conn.WriteBatch(messages, 0)
					if err != nil {
						if !errors.Is(err, net.ErrClosed) {
							Log.Error("[KCP] Server: WriteBatch failed: %v, sent %d/%d messages", err, n, batchCount)
						}
					}
				} else {
					// Fallback to single writes
					for _, msg := range messages {
						data := msg.Buffers[0][:msg.N]
						udpAddr := msg.Addr.(*net.UDPAddr)
						_, err := s.conn.WriteToUDP(data, udpAddr)
						if err != nil && !errors.Is(err, net.ErrClosed) {
							Log.Error("[KCP] Server: async sendTo failed: %v", err)
						}
					}
				}

				// Return all buffers to pool
				for _, msg := range messages {
					s.PutBuf(msg.Buffers[0])
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

	// Set deadlines to zero for blocking I/O.
	if err := s.conn.SetReadDeadline(time.Time{}); err != nil {
		Log.Warning("[KCP] Server: SetReadDeadline failed: %v", err)
	}
	if err := s.conn.SetWriteDeadline(time.Time{}); err != nil {
		Log.Warning("[KCP] Server: SetWriteDeadline failed: %v", err)
	}

	// Initialize IPv4 packet connection for batch operations
	if s.config.EnableBatchOps {
		s.ipv4Conn = ipv4.NewPacketConn(s.conn)
		if s.ipv4Conn == nil {
			Log.Warning("[KCP] Server: Failed to create IPv4 packet connection, falling back to standard operations")
			s.config.EnableBatchOps = false
		} else {
			// Set deadlines to zero for blocking I/O for the packet conn.
			if err := s.ipv4Conn.SetReadDeadline(time.Time{}); err != nil {
				Log.Warning("[KCP] Server: SetReadDeadline for ipv4Conn failed: %v", err)
			}
			if err := s.ipv4Conn.SetWriteDeadline(time.Time{}); err != nil {
				Log.Warning("[KCP] Server: SetWriteDeadline for ipv4Conn failed: %v", err)
			}
		}
	}

	// Initialize sendDone channel and start send worker
	s.sendDone = make(chan struct{})
	s.sendSignal = make(chan struct{}, 1)
	s.sendQueue = NewLockFreeQueue[ipv4.Message]()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				Log.Error("[KCP] Server: sendWorker panic: %v\n%s", r, debug.Stack())
			}
		}()
		s.sendWorker()
	}()

	// initialize receive queue and worker
	s.receiveDone = make(chan struct{})
	s.receiveSignal = make(chan struct{}, 1)
	s.receiveQueue = NewLockFreeQueue[ipv4.Message]()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				Log.Error("[KCP] Server: receiveWorker panic: %v\n%s", r, debug.Stack())
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

	_ = s.conn.Close()
	s.conn = nil
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

// receiveWorker continuously reads UDP packets and queues them for async processing.
// It uses ReadBatch for performance if available, otherwise falls back to ReadFromUDP.
func (s *KcpServer) receiveWorker() {
	for {
		select {
		case <-s.receiveDone:
			return
		default:
			if s.conn == nil {
				continue
			}

			// Use ReadBatch if supported and configured
			if s.ipv4Conn != nil {
				messages := s.recvBatchMessages
				for i := range messages {
					messages[i].Buffers[0] = s.recvBatchBuffers[i][:s.config.Mtu]
					messages[i].N = 0
					messages[i].Addr = nil
				}

				// Read batch of messages
				n, err := s.ipv4Conn.ReadBatch(messages, 0)
				if err != nil {
					if !errors.Is(err, net.ErrClosed) && !isTimeoutError(err) {
						Log.Error("[KCP] Server: ReadBatch error: %v", err)
					}
				} else if n > 0 {
					// Process received messages
					for i := 0; i < n; i++ {
						msg := &messages[i]
						if msg.N > 0 {
							// get buffer from pool and copy data
							buf := s.GetBuf(msg.N)
							copy(buf, msg.Buffers[0][:msg.N])

							// Extract UDP address from the message
							var udpAddr *net.UDPAddr
							if addr, ok := msg.Addr.(*net.UDPAddr); ok {
								udpAddr = addr
							} else {
								// Fallback: try to convert from other address types
								if msg.Addr != nil {
									if resolved, err := net.ResolveUDPAddr("udp", msg.Addr.String()); err == nil {
										udpAddr = resolved
									}
								}
							}

							if udpAddr != nil {
								// queue the received data for async processing
								newMsg := ipv4.Message{
									Buffers: [][]byte{buf},
									Addr:    udpAddr,
									N:       msg.N,
								}
								s.receiveQueue.Enqueue(newMsg)
							} else {
								// Return buffer to pool if we can't process the message
								s.PutBuf(buf)
								Log.Warning("[KCP] Server: Failed to extract UDP address from batch message")
							}
						}
					}

					// signal receive worker
					select {
					case s.receiveSignal <- struct{}{}:
					default:
					}
				}
			} else {
				// Fallback to single read
				n, addr, err := s.conn.ReadFromUDP(s.recvBuf)
				if err != nil {
					if !errors.Is(err, net.ErrClosed) && !isTimeoutError(err) {
						Log.Error("[KCP] Server: ReadFromUDP error: %v", err)
					}
				} else if n > 0 {
					// get buffer from pool and copy data
					buf := s.GetBuf(n)
					copy(buf, s.recvBuf[:n])

					// queue the received data for async processing
					msg := ipv4.Message{
						Buffers: [][]byte{buf},
						Addr:    addr,
						N:       n,
					}
					s.receiveQueue.Enqueue(msg)

					// signal receive worker
					select {
					case s.receiveSignal <- struct{}{}:
					default:
					}
				}
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
				if msg, ok := s.receiveQueue.Dequeue(); ok {
					data := msg.Buffers[0][:msg.N]
					udpAddr := msg.Addr.(*net.UDPAddr)
					s.processMessage(data, udpAddr)
					// return buffer to pool after processing
					s.PutBuf(msg.Buffers[0])
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
		// performance monitoring for onRawSend
		start := time.Now()

		// async send using channel to avoid blocking KCP processing
		if s.conn == nil {
			return
		}

		// get buffer from pool to avoid GC pressure
		buf := s.GetBuf(len(data))
		copy(buf, data)

		// send task to worker via lock-free queue
		msg := ipv4.Message{
			Buffers: [][]byte{buf},
			Addr:    remote,
			N:       len(buf),
		}
		s.sendQueue.Enqueue(msg)
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
	// use connection pool to get reusable connection
	return s.connectionPool.Get(onConnected, onData, onDisconnected, onError, onRawSend, cookie, remote)
}
