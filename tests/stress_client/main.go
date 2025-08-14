package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	kcp2k "github.com/O-Keh-Hunter/kcp2k-go"
)

type Client struct {
	ID        int
	ServerID  int
	KcpClient *kcp2k.KcpClient
	Stats     *ClientStats
	mu        sync.RWMutex
	stopChan  chan struct{}
	connected bool
}

type ClientStats struct {
	PacketsSent     int64
	PacketsReceived int64
	BytesSent       int64
	BytesReceived   int64
	Latency         time.Duration
	StartTime       time.Time
	Connected       bool
}

func NewClient(id, serverID int) *Client {
	return &Client{
		ID:       id,
		ServerID: serverID,
		Stats:    &ClientStats{StartTime: time.Now()},
		stopChan: make(chan struct{}),
	}
}

func (c *Client) Connect(serverAddr string) error {
	// 使用优化后的KCP配置
	config := kcp2k.OptimizedKcpConfig()
	// 针对压力测试进行微调
	config.DualMode = false       // 压力测试只使用IPv4
	config.SendWindowSize = 32000 // 保持大窗口用于高吞吐量测试
	config.ReceiveWindowSize = 32000
	config.Timeout = 2000      // 压力测试使用较短超时
	config.MaxRetransmits = 80 // 增加重传次数确保可靠性

	// 创建KCP2K客户端
	c.KcpClient = kcp2k.NewKcpClient(
		c.onConnected,
		c.onData,
		c.onDisconnected,
		c.onError,
		config,
	)

	// 连接到服务器 (解析主机和端口)
	host, port := "127.0.0.1", uint16(10000) // 直接使用IPv4地址而不是localhost
	if serverAddr != "" {
		// 解析格式为 "host:port"
		parts := strings.Split(serverAddr, ":")
		if len(parts) == 2 {
			host = parts[0]
			// 如果host是localhost，转换为127.0.0.1确保使用IPv4
			if host == "localhost" {
				host = "127.0.0.1"
			}
			if portNum, err := strconv.Atoi(parts[1]); err == nil {
				port = uint16(portNum)
			}
		}
	}
	err := c.KcpClient.Connect(host, port)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %v", serverAddr, err)
	}

	// 启动Tick循环来处理网络事件
	go c.tickLoop()

	// log.Printf("Client %d connected to server %d at %s", c.ID, c.ServerID, serverAddr)
	return nil
}

// KCP2K回调方法
func (c *Client) onConnected() {
	c.mu.Lock()
	c.Stats.Connected = true
	c.connected = true
	c.mu.Unlock()
	// log.Printf("Client %d connected to server %d", c.ID, c.ServerID)
}

func (c *Client) onData(data []byte, channel kcp2k.KcpChannel) {
	c.mu.Lock()
	c.Stats.PacketsReceived++
	c.Stats.BytesReceived += int64(len(data))
	c.mu.Unlock()
}

func (c *Client) onDisconnected() {
	c.mu.Lock()
	c.Stats.Connected = false
	c.connected = false
	c.mu.Unlock()
	// log.Printf("Client %d disconnected from server %d", c.ID, c.ServerID)
}

func (c *Client) onError(error kcp2k.ErrorCode, reason string) {
	log.Printf("Client %d error: %v - %s", c.ID, error, reason)
}

func (c *Client) tickLoop() {
	ticker := time.NewTicker(10 * time.Millisecond) // KCP建议10ms间隔
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			if c.KcpClient != nil {
				c.KcpClient.Tick()
			}
		}
	}
}

func (c *Client) SendPackets(fps int) {
	ticker := time.NewTicker(time.Duration(1000/fps) * time.Millisecond)
	defer ticker.Stop()

	packetID := 0
	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			if !c.connected {
				continue
			}

			// Create a test packet with timestamp and packet ID
			packet := fmt.Sprintf("PACKET_%d_%d_%d_%s",
				c.ID, c.ServerID, packetID, time.Now().Format("15:04:05.000"))

			start := time.Now()
			c.KcpClient.Send([]byte(packet), kcp2k.KcpReliable)

			c.mu.Lock()
			c.Stats.PacketsSent++
			c.Stats.BytesSent += int64(len(packet))
			c.Stats.Latency = time.Since(start)
			c.mu.Unlock()

			packetID++
		}
	}
}

func (c *Client) Stop() {
	close(c.stopChan)
	if c.KcpClient != nil {
		c.KcpClient.Disconnect()
	}
	c.mu.Lock()
	c.Stats.Connected = false
	c.connected = false
	c.mu.Unlock()
}

func (c *Client) GetStats() ClientStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return *c.Stats
}

type ClientManager struct {
	Clients    []*Client
	TotalStats *TotalStats
	mu         sync.RWMutex
}

type TotalStats struct {
	TotalConnections     int64
	TotalPacketsSent     int64
	TotalPacketsReceived int64
	TotalBytesSent       int64
	TotalBytesReceived   int64
	AverageLatency       time.Duration
}

func NewClientManager() *ClientManager {
	return &ClientManager{
		TotalStats: &TotalStats{},
	}
}

func (cm *ClientManager) AddClient(client *Client) {
	cm.mu.Lock()
	cm.Clients = append(cm.Clients, client)
	cm.mu.Unlock()
}

func (cm *ClientManager) GetTotalStats() TotalStats {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var totalPacketsSent, totalPacketsReceived int64
	var totalBytesSent, totalBytesReceived int64
	var totalLatency time.Duration
	var connectedClients int64

	for _, client := range cm.Clients {
		stats := client.GetStats()
		if stats.Connected {
			connectedClients++
			totalPacketsSent += stats.PacketsSent
			totalPacketsReceived += stats.PacketsReceived
			totalBytesSent += stats.BytesSent
			totalBytesReceived += stats.BytesReceived
			totalLatency += stats.Latency
		}
	}

	var avgLatency time.Duration
	if connectedClients > 0 {
		avgLatency = totalLatency / time.Duration(connectedClients)
	}

	return TotalStats{
		TotalConnections:     connectedClients,
		TotalPacketsSent:     totalPacketsSent,
		TotalPacketsReceived: totalPacketsReceived,
		TotalBytesSent:       totalBytesSent,
		TotalBytesReceived:   totalBytesReceived,
		AverageLatency:       avgLatency,
	}
}

func main() {
	var (
		serverHost       = flag.String("host", "localhost", "Server host")
		startPort        = flag.Int("start-port", 10000, "Starting port for servers")
		numServers       = flag.Int("servers", 1000, "Number of servers")
		clientsPerServer = flag.Int("clients-per-server", 10, "Number of clients per server")
		fps              = flag.Int("fps", 30, "Frames per second")
		reportInterval   = flag.Duration("report", 5*time.Second, "Stats report interval")
	)
	flag.Parse()

	if *numServers <= 0 || *clientsPerServer <= 0 {
		log.Fatal("Number of servers and clients per server must be positive")
	}

	clientManager := NewClientManager()
	var wg sync.WaitGroup

	// Create and connect clients
	clientID := 1
	for serverID := 1; serverID <= *numServers; serverID++ {
		serverPort := *startPort + serverID - 1
		serverAddr := fmt.Sprintf("%s:%d", *serverHost, serverPort)

		for clientIndex := 0; clientIndex < *clientsPerServer; clientIndex++ {
			client := NewClient(clientID, serverID)
			clientManager.AddClient(client)

			wg.Add(1)
			go func(c *Client) {
				defer wg.Done()

				// Retry connection with exponential backoff
				for retry := 0; retry < 5; retry++ {
					if err := c.Connect(serverAddr); err == nil {
						break
					}
					// log.Printf("Client %d: connection failed, retrying in %d seconds...",
					// 	c.ID, 1<<retry)
					time.Sleep(time.Duration(1<<retry) * time.Second)
				}

				// Start sending packets
				c.SendPackets(*fps)
			}(client)

			clientID++

			// Small delay to avoid overwhelming the system
			time.Sleep(50 * time.Millisecond)
		}
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
				stats := clientManager.GetTotalStats()
				log.Printf("=== CLIENT STATS REPORT ===")
				log.Printf("Connected Clients: %d", stats.TotalConnections)
				log.Printf("Total Packets Sent: %d", stats.TotalPacketsSent)
				log.Printf("Total Packets Received: %d", stats.TotalPacketsReceived)
				log.Printf("Total Bytes Sent: %d", stats.TotalBytesSent)
				log.Printf("Total Bytes Received: %d", stats.TotalBytesReceived)
				log.Printf("Average Latency: %v", stats.AverageLatency)
				log.Printf("===========================")
			}
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down clients...")
	cancel()

	// Stop all clients
	clientManager.mu.RLock()
	for _, client := range clientManager.Clients {
		client.Stop()
	}
	clientManager.mu.RUnlock()

	// Final stats report
	finalStats := clientManager.GetTotalStats()
	log.Printf("=== FINAL CLIENT STATS ===")
	log.Printf("Connected Clients: %d", finalStats.TotalConnections)
	log.Printf("Total Packets Sent: %d", finalStats.TotalPacketsSent)
	log.Printf("Total Packets Received: %d", finalStats.TotalPacketsReceived)
	log.Printf("Total Bytes Sent: %d", finalStats.TotalBytesSent)
	log.Printf("Total Bytes Received: %d", finalStats.TotalBytesReceived)
	log.Printf("Average Latency: %v", finalStats.AverageLatency)
	log.Printf("========================")

	log.Println("All clients stopped")
}
