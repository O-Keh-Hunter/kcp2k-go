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
	config := kcp2k.OptimizedKcpConfig()
	// 针对压力测试进行微调
	config.DualMode = false       // 压力测试只使用IPv4
	config.SendWindowSize = 32000 // 保持大窗口用于高吞吐量测试
	config.ReceiveWindowSize = 32000
	config.Timeout = 2000      // 压力测试使用较短超时
	config.MaxRetransmits = 80 // 增加重传次数确保可靠性

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
	go s.tickLoop()

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

	// Echo back the data
	s.KcpServer.Send(connectionId, data, channel)

	s.mu.Lock()
	s.Stats.PacketsSent++
	s.Stats.BytesSent += int64(len(data))
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

func (s *Server) tickLoop() {
	ticker := time.NewTicker(10 * time.Millisecond) // KCP建议10ms间隔
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
