package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	kcp2k "github.com/O-Keh-Hunter/kcp2k-go"
)

// 预分配的固定字符串常量，避免GC
var (
	// 可靠数据包常量
	reliablePackets = [][]byte{
		[]byte("PACKET_RELIABLE_TEST_DATA_001"),
		[]byte("PACKET_RELIABLE_TEST_DATA_002"),
		[]byte("PACKET_RELIABLE_TEST_DATA_003"),
		[]byte("PACKET_RELIABLE_TEST_DATA_004"),
		[]byte("PACKET_RELIABLE_TEST_DATA_005"),
	}
	// 不可靠数据包常量
	unreliablePackets = [][]byte{
		[]byte("UPACKET_UNRELIABLE_TEST_DATA_001"),
		[]byte("UPACKET_UNRELIABLE_TEST_DATA_002"),
		[]byte("UPACKET_UNRELIABLE_TEST_DATA_003"),
		[]byte("UPACKET_UNRELIABLE_TEST_DATA_004"),
		[]byte("UPACKET_UNRELIABLE_TEST_DATA_005"),
	}
	// 对应的ECHO响应常量
	echoReliablePackets = [][]byte{
		[]byte("ECHO_RELIABLE_TEST_DATA_001"),
		[]byte("ECHO_RELIABLE_TEST_DATA_002"),
		[]byte("ECHO_RELIABLE_TEST_DATA_003"),
		[]byte("ECHO_RELIABLE_TEST_DATA_004"),
		[]byte("ECHO_RELIABLE_TEST_DATA_005"),
	}
	echoUnreliablePackets = [][]byte{
		[]byte("ECHO_UNRELIABLE_TEST_DATA_001"),
		[]byte("ECHO_UNRELIABLE_TEST_DATA_002"),
		[]byte("ECHO_UNRELIABLE_TEST_DATA_003"),
		[]byte("ECHO_UNRELIABLE_TEST_DATA_004"),
		[]byte("ECHO_UNRELIABLE_TEST_DATA_005"),
	}
)

type Server struct {
	ID          int
	KcpServer   *kcp2k.KcpServer
	Port        int
	Stats       *ServerStats
	mu          sync.RWMutex
	connections map[int]*Connection
}

type Connection struct {
	ID   int
	Conn *kcp2k.KcpServerConnection
}

type ServerStats struct {
	Connections     int64
	PacketsSent     int64
	PacketsReceived int64
	BytesSent       int64
	BytesReceived   int64
	StartTime       time.Time
}

func NewServer(id, port int) *Server {
	return &Server{
		ID:    id,
		Port:  port,
		Stats: &ServerStats{StartTime: time.Now()},
	}
}

func (s *Server) Start() error {
	s.connections = make(map[int]*Connection)

	// 使用优化后的KCP配置
	config := kcp2k.HighPerformanceKcpConfig()

	// 创建KCP2K服务器
	s.KcpServer = kcp2k.NewKcpServer(
		s.onConnected,
		s.onData,
		s.onDisconnected,
		s.onError,
		config,
	)

	// 启动服务器
	err := s.KcpServer.Start(uint16(s.Port))
	if err != nil {
		return fmt.Errorf("failed to start server on port %d: %v", s.Port, err)
	}

	// 启动Tick循环来处理网络事件
	go s.tickLoop(time.Duration(config.Interval) * time.Millisecond)

	log.Printf("Server %d started on port %d", s.ID, s.Port)
	return nil
}

// KCP2K回调方法
func (s *Server) onConnected(connectionId int) {
	s.mu.Lock()
	s.Stats.Connections++
	s.connections[connectionId] = &Connection{
		ID: connectionId,
	}
	s.mu.Unlock()
	// log.Printf("Server %d: new connection %d (total: %d)", s.ID, connectionId, s.Stats.Connections)
}

func (s *Server) onData(connectionId int, data []byte, channel kcp2k.KcpChannel) {
	s.mu.Lock()
	s.Stats.PacketsReceived++
	s.Stats.BytesReceived += int64(len(data))
	s.mu.Unlock()

	// 使用字节比较和预分配常量，避免字符串转换和拼接，减少GC
	var response []byte

	// 检查是否是可靠数据包
	for i, reliablePacket := range reliablePackets {
		if len(data) == len(reliablePacket) && string(data) == string(reliablePacket) {
			// 使用对应的预分配ECHO响应
			response = echoReliablePackets[i]
			break
		}
	}

	// 如果不是可靠包，检查是否是不可靠数据包
	if response == nil {
		for i, unreliablePacket := range unreliablePackets {
			if len(data) == len(unreliablePacket) && string(data) == string(unreliablePacket) {
				// 使用对应的预分配ECHO响应
				response = echoUnreliablePackets[i]
				break
			}
		}
	}

	// 如果都不匹配，直接回显原数据（向后兼容）
	if response == nil {
		response = data
	}

	// 发送响应
	s.KcpServer.Send(connectionId, response, channel)

	s.mu.Lock()
	s.Stats.PacketsSent++
	s.Stats.BytesSent += int64(len(response))
	s.mu.Unlock()
}

func (s *Server) onDisconnected(connectionId int) {
	s.mu.Lock()
	delete(s.connections, connectionId)
	s.Stats.Connections--
	s.mu.Unlock()
	// log.Printf("Server %d: connection %d disconnected", s.ID, connectionId)
}

func (s *Server) onError(connectionId int, error kcp2k.ErrorCode, reason string) {
	log.Printf("Server %d: connection %d error: %v - %s", s.ID, connectionId, error, reason)
}

func (s *Server) tickLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if s.KcpServer != nil {
			s.KcpServer.Tick()
		}
	}
}

func (s *Server) Stop() {
	if s.KcpServer != nil {
		s.KcpServer.Stop()
	}
}

func (s *Server) GetStats() ServerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return *s.Stats
}

func main() {
	var (
		startPort      = flag.Int("start-port", 10000, "Starting port for servers")
		numServers     = flag.Int("servers", 500, "Number of servers to start")
		reportInterval = flag.Duration("report", 5*time.Second, "Stats report interval")
	)
	flag.Parse()

	if *numServers <= 0 {
		log.Fatal("Number of servers must be positive")
	}

	servers := make([]*Server, *numServers)
	var wg sync.WaitGroup

	// Start servers
	for i := 0; i < *numServers; i++ {
		port := *startPort + i
		server := NewServer(i+1, port)
		servers[i] = server

		wg.Add(1)
		go func(s *Server) {
			defer wg.Done()
			if err := s.Start(); err != nil {
				log.Printf("Failed to start server %d: %v", s.ID, err)
			}
		}(server)

		// Small delay to avoid overwhelming the system
		time.Sleep(10 * time.Millisecond)
	}

	// Stats reporting goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ticker := time.NewTicker(*reportInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				reportStats(servers)
			}
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down servers...")
	cancel()

	// Stop all servers
	for _, server := range servers {
		server.Stop()
	}

	// Final stats report
	reportStats(servers)

	log.Println("All servers stopped")
}

func reportStats(servers []*Server) {
	var totalConnections, totalPacketsSent, totalPacketsReceived int64
	var totalBytesSent, totalBytesReceived int64

	for _, server := range servers {
		stats := server.GetStats()
		totalConnections += stats.Connections
		totalPacketsSent += stats.PacketsSent
		totalPacketsReceived += stats.PacketsReceived
		totalBytesSent += stats.BytesSent
		totalBytesReceived += stats.BytesReceived
	}

	log.Printf("=== STATS REPORT ===")
	log.Printf("Total Connections: %d", totalConnections)
	log.Printf("Total Packets Sent: %d", totalPacketsSent)
	log.Printf("Total Packets Received: %d", totalPacketsReceived)
	log.Printf("Total Bytes Sent: %d", totalBytesSent)
	log.Printf("Total Bytes Received: %d", totalBytesReceived)
	log.Printf("===================")
}
