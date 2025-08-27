package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
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
	ID             int
	ServerID       int
	KcpClient      *kcp2k.KcpClient
	Stats          *ClientStats
	mu             sync.RWMutex
	stopChan       chan struct{}
	connected      bool
	pendingPackets map[int]*PendingPacket
	packetID       int
}

type ClientStats struct {
	PacketsSent     int64
	PacketsReceived int64
	PacketsLost     int64
	BytesSent       int64
	BytesReceived   int64
	Latency         time.Duration
	StartTime       time.Time
	Connected       bool
}

type PendingPacket struct {
	ID       int
	SentTime time.Time
}

func NewClient(id, serverID int) *Client {
	return &Client{
		ID:             id,
		ServerID:       serverID,
		Stats:          &ClientStats{StartTime: time.Now()},
		stopChan:       make(chan struct{}),
		pendingPackets: make(map[int]*PendingPacket),
		packetID:       0,
	}
}

func (c *Client) Connect(serverAddr string) error {
	// 使用优化后的KCP配置
	config := kcp2k.HighPerformanceKcpConfig()

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
	go c.tickLoop(time.Duration(config.Interval) * time.Millisecond)

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
	defer c.mu.Unlock()

	c.Stats.PacketsReceived++
	c.Stats.BytesReceived += int64(len(data))

	// 尝试解析响应数据包以计算延迟
	dataStr := string(data)
	if strings.HasPrefix(dataStr, "ECHO_") {
		// 解析数据包ID: ECHO_clientID_serverID_packetID_timestamp
		parts := strings.Split(dataStr, "_")
		if len(parts) >= 4 {
			if packetID, err := strconv.Atoi(parts[3]); err == nil {
				if pending, exists := c.pendingPackets[packetID]; exists {
					// 计算真实的网络往返延迟
					c.Stats.Latency = time.Since(pending.SentTime)
					delete(c.pendingPackets, packetID)
				}
			}
		}
	}
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

func (c *Client) tickLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
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

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			if !c.connected {
				continue
			}

			c.mu.Lock()
			c.packetID++
			currentPacketID := c.packetID
			c.mu.Unlock()

			// 随机选择发送可靠或不可靠数据包
			isReliable := rand.IntN(2) == 0
			sendTime := time.Now()

			if isReliable {
				// 发送可靠数据包 (记录延迟追踪)
				relPacket := fmt.Sprintf("PACKET_%d_%d_%d_%s",
					c.ID, c.ServerID, currentPacketID, time.Now().Format("15:04:05.000"))

				c.KcpClient.Send([]byte(relPacket), kcp2k.KcpReliable)

				c.mu.Lock()
				c.Stats.PacketsSent++
				c.Stats.BytesSent += int64(len(relPacket))
				// 记录待响应的数据包用于延迟计算
				c.pendingPackets[currentPacketID] = &PendingPacket{
					ID:       currentPacketID,
					SentTime: sendTime,
				}
				c.mu.Unlock()
			} else {
				// 发送不可靠数据包 (无延迟追踪)
				unrelPacket := fmt.Sprintf("UPACKET_%d_%d_%d_%s",
					c.ID, c.ServerID, currentPacketID, time.Now().Format("15:04:05.000"))

				c.KcpClient.Send([]byte(unrelPacket), kcp2k.KcpUnreliable)

				c.mu.Lock()
				c.Stats.PacketsSent++
				c.Stats.BytesSent += int64(len(unrelPacket))
				// 记录待响应的数据包用于延迟计算
				c.pendingPackets[currentPacketID] = &PendingPacket{
					ID:       currentPacketID,
					SentTime: sendTime,
				}
				c.mu.Unlock()
			}

			// 清理超时的待响应数据包（超过5秒认为丢失）
			c.cleanupTimeoutPackets()
		}
	}
}

func (c *Client) cleanupTimeoutPackets() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for id, pending := range c.pendingPackets {
		if now.Sub(pending.SentTime) > 5*time.Second {
			// 统计丢包
			c.Stats.PacketsLost++
			delete(c.pendingPackets, id)
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
	TotalPacketsLost     int64
	TotalBytesSent       int64
	TotalBytesReceived   int64
	AverageLatency       time.Duration
	PacketLossRate       float64
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

	var totalPacketsSent, totalPacketsReceived, totalPacketsLost int64
	var totalBytesSent, totalBytesReceived int64
	var totalLatency time.Duration
	var connectedClients int64

	for _, client := range cm.Clients {
		stats := client.GetStats()
		if stats.Connected {
			connectedClients++
			totalPacketsSent += stats.PacketsSent
			totalPacketsReceived += stats.PacketsReceived
			totalPacketsLost += stats.PacketsLost
			totalBytesSent += stats.BytesSent
			totalBytesReceived += stats.BytesReceived
			totalLatency += stats.Latency
		}
	}

	var avgLatency time.Duration
	if connectedClients > 0 {
		avgLatency = totalLatency / time.Duration(connectedClients)
	}

	// 计算丢包率
	var packetLossRate float64
	if totalPacketsSent > 0 {
		packetLossRate = float64(totalPacketsLost) / float64(totalPacketsSent) * 100.0
	}

	return TotalStats{
		TotalConnections:     connectedClients,
		TotalPacketsSent:     totalPacketsSent,
		TotalPacketsReceived: totalPacketsReceived,
		TotalPacketsLost:     totalPacketsLost,
		TotalBytesSent:       totalBytesSent,
		TotalBytesReceived:   totalBytesReceived,
		AverageLatency:       avgLatency,
		PacketLossRate:       packetLossRate,
	}
}

func main() {
	var (
		serverHost       = flag.String("host", "localhost", "Server host")
		startPort        = flag.Int("start-port", 10000, "Starting port for servers")
		numServers       = flag.Int("servers", 500, "Number of servers")
		clientsPerServer = flag.Int("clients-per-server", 10, "Number of clients per server")
		fps              = flag.Int("fps", 15, "Frames per second")
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
			time.Sleep(10 * time.Millisecond)
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
				log.Printf("Total Packets Lost: %d", stats.TotalPacketsLost)
				log.Printf("Packet Loss Rate: %.2f%%", stats.PacketLossRate)
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
	log.Printf("Total Packets Lost: %d", finalStats.TotalPacketsLost)
	log.Printf("Packet Loss Rate: %.2f%%", finalStats.PacketLossRate)
	log.Printf("Total Bytes Sent: %d", finalStats.TotalBytesSent)
	log.Printf("Total Bytes Received: %d", finalStats.TotalBytesReceived)
	log.Printf("Average Latency: %v", finalStats.AverageLatency)
	log.Printf("========================")

	log.Println("All clients stopped")
}
