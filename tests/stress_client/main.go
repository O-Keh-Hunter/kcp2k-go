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
	// Per-second statistics
	LastReportTime        time.Time
	LastPacketsSent       int64
	LastPacketsReceived   int64
	LastPacketsLost       int64
	LastBytesSent         int64
	LastBytesReceived     int64
	PacketsPerSecSent     int64
	PacketsPerSecReceived int64
	PacketsPerSecLost     int64
	BytesPerSecSent       float64 // MB/s
	BytesPerSecReceived   float64 // MB/s
}

type PendingPacket struct {
	ID       int
	SentTime time.Time
}

func NewClient(id, serverID int) *Client {
	now := time.Now()
	return &Client{
		ID:       id,
		ServerID: serverID,
		Stats: &ClientStats{
			StartTime:      now,
			LastReportTime: now,
		},
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

	// 使用字节比较避免字符串转换，减少GC
	// 检查是否是ECHO响应包
	if len(data) >= 4 && data[0] == 'E' && data[1] == 'C' && data[2] == 'H' && data[3] == 'O' {
		// 简化延迟计算，使用固定的包ID模式
		// 由于使用固定常量，我们可以通过包内容来识别对应的发送时间
		for i, echoPacket := range echoReliablePackets {
			if len(data) == len(echoPacket) && string(data) == string(echoPacket) {
				// 找到对应的可靠包，使用包索引作为简化的ID
				if pending, exists := c.pendingPackets[i+1]; exists {
					c.Stats.Latency = time.Since(pending.SentTime)
					delete(c.pendingPackets, i+1)
				}
				break
			}
		}
		for i, echoPacket := range echoUnreliablePackets {
			if len(data) == len(echoPacket) && string(data) == string(echoPacket) {
				// 找到对应的不可靠包，使用包索引作为简化的ID
				if pending, exists := c.pendingPackets[i+100]; exists {
					c.Stats.Latency = time.Since(pending.SentTime)
					delete(c.pendingPackets, i+100)
				}
				break
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

			// 随机选择发送可靠或不可靠数据包
			isReliable := rand.IntN(2) == 0
			sendTime := time.Now()

			if isReliable {
				// 发送可靠数据包，使用预分配的常量避免GC
				packetIndex := rand.IntN(len(reliablePackets))
				packetData := reliablePackets[packetIndex]

				c.KcpClient.Send(packetData, kcp2k.KcpReliable)

				c.mu.Lock()
				c.Stats.PacketsSent++
				c.Stats.BytesSent += int64(len(packetData))
				// 使用包索引+1作为简化的ID用于延迟计算
				c.pendingPackets[packetIndex+1] = &PendingPacket{
					ID:       packetIndex + 1,
					SentTime: sendTime,
				}
				c.mu.Unlock()
			} else {
				// 发送不可靠数据包，使用预分配的常量避免GC
				packetIndex := rand.IntN(len(unreliablePackets))
				packetData := unreliablePackets[packetIndex]

				c.KcpClient.Send(packetData, kcp2k.KcpUnreliable)

				c.mu.Lock()
				c.Stats.PacketsSent++
				c.Stats.BytesSent += int64(len(packetData))
				// 使用包索引+100作为简化的ID用于延迟计算
				c.pendingPackets[packetIndex+100] = &PendingPacket{
					ID:       packetIndex + 100,
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

// Calculate per-second statistics
func (c *Client) UpdatePerSecondStats() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	timeDiff := now.Sub(c.Stats.LastReportTime).Seconds()

	if timeDiff > 0 {
		// Calculate packets per second
		packetsSentDiff := c.Stats.PacketsSent - c.Stats.LastPacketsSent
		packetsReceivedDiff := c.Stats.PacketsReceived - c.Stats.LastPacketsReceived
		packetsLostDiff := c.Stats.PacketsLost - c.Stats.LastPacketsLost
		c.Stats.PacketsPerSecSent = int64(float64(packetsSentDiff) / timeDiff)
		c.Stats.PacketsPerSecReceived = int64(float64(packetsReceivedDiff) / timeDiff)
		c.Stats.PacketsPerSecLost = int64(float64(packetsLostDiff) / timeDiff)

		// Calculate bytes per second (MB/s)
		bytesSentDiff := c.Stats.BytesSent - c.Stats.LastBytesSent
		bytesReceivedDiff := c.Stats.BytesReceived - c.Stats.LastBytesReceived
		c.Stats.BytesPerSecSent = float64(bytesSentDiff) / timeDiff / (1024 * 1024)
		c.Stats.BytesPerSecReceived = float64(bytesReceivedDiff) / timeDiff / (1024 * 1024)

		// Update last values
		c.Stats.LastReportTime = now
		c.Stats.LastPacketsSent = c.Stats.PacketsSent
		c.Stats.LastPacketsReceived = c.Stats.PacketsReceived
		c.Stats.LastPacketsLost = c.Stats.PacketsLost
		c.Stats.LastBytesSent = c.Stats.BytesSent
		c.Stats.LastBytesReceived = c.Stats.BytesReceived
	}
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
	// Per-second statistics
	TotalPacketsPerSecSent     int64
	TotalPacketsPerSecReceived int64
	TotalPacketsPerSecLost     int64
	TotalBytesPerSecSent       float64 // MB/s
	TotalBytesPerSecReceived   float64 // MB/s
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

	// Update per-second stats for all clients first
	for _, client := range cm.Clients {
		client.UpdatePerSecondStats()
	}

	var totalPacketsSent, totalPacketsReceived, totalPacketsLost int64
	var totalBytesSent, totalBytesReceived int64
	var totalLatency time.Duration
	var connectedClients int64
	var totalPacketsPerSecSent, totalPacketsPerSecReceived, totalPacketsPerSecLost int64
	var totalBytesPerSecSent, totalBytesPerSecReceived float64

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
			totalPacketsPerSecSent += stats.PacketsPerSecSent
			totalPacketsPerSecReceived += stats.PacketsPerSecReceived
			totalPacketsPerSecLost += stats.PacketsPerSecLost
			totalBytesPerSecSent += stats.BytesPerSecSent
			totalBytesPerSecReceived += stats.BytesPerSecReceived
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
		TotalConnections:           connectedClients,
		TotalPacketsSent:           totalPacketsSent,
		TotalPacketsReceived:       totalPacketsReceived,
		TotalPacketsLost:           totalPacketsLost,
		TotalBytesSent:             totalBytesSent,
		TotalBytesReceived:         totalBytesReceived,
		AverageLatency:             avgLatency,
		PacketLossRate:             packetLossRate,
		TotalPacketsPerSecSent:     totalPacketsPerSecSent,
		TotalPacketsPerSecReceived: totalPacketsPerSecReceived,
		TotalPacketsPerSecLost:     totalPacketsPerSecLost,
		TotalBytesPerSecSent:       totalBytesPerSecSent,
		TotalBytesPerSecReceived:   totalBytesPerSecReceived,
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
				log.Printf("--- Per Second Stats ---")
				log.Printf("Packets/sec Sent: %d", stats.TotalPacketsPerSecSent)
				log.Printf("Packets/sec Received: %d", stats.TotalPacketsPerSecReceived)
				log.Printf("Packets/sec Lost: %d", stats.TotalPacketsPerSecLost)
				log.Printf("MB/sec Sent: %.2f", stats.TotalBytesPerSecSent)
				log.Printf("MB/sec Received: %.2f", stats.TotalBytesPerSecReceived)
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
	log.Printf("--- Final Per Second Stats ---")
	log.Printf("Packets/sec Sent: %d", finalStats.TotalPacketsPerSecSent)
	log.Printf("Packets/sec Received: %d", finalStats.TotalPacketsPerSecReceived)
	log.Printf("Packets/sec Lost: %d", finalStats.TotalPacketsPerSecLost)
	log.Printf("MB/sec Sent: %.2f", finalStats.TotalBytesPerSecSent)
	log.Printf("MB/sec Received: %.2f", finalStats.TotalBytesPerSecReceived)
	log.Printf("=============================")

	log.Println("All clients stopped")
}
