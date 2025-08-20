package lockstep

import (
	"sync"
	"time"
)

// NetworkStats 网络统计信息
type NetworkStats struct {
	totalPackets  uint64
	lostPackets   uint64
	bytesReceived uint64
	bytesSent     uint64
	latencySum    time.Duration
	latencyCount  uint64
	maxLatency    time.Duration
	minLatency    time.Duration

	// 输入延时统计
	inputLatencySum   time.Duration
	inputLatencyCount uint64
	maxInputLatency   time.Duration
	minInputLatency   time.Duration

	mutex sync.RWMutex
}

// GetTotalPackets 获取总包数
func (ns *NetworkStats) GetTotalPackets() uint64 {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.totalPackets
}

// GetLostPackets 获取丢包数
func (ns *NetworkStats) GetLostPackets() uint64 {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.lostPackets
}

// GetAverageLatency 获取平均延迟
func (ns *NetworkStats) GetAverageLatency() time.Duration {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	if ns.latencyCount == 0 {
		return 0
	}
	return ns.latencySum / time.Duration(ns.latencyCount)
}

// GetMaxLatency 获取最大延迟
func (ns *NetworkStats) GetMaxLatency() time.Duration {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.maxLatency
}

// GetMinLatency 获取最小延迟
func (ns *NetworkStats) GetMinLatency() time.Duration {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.minLatency
}

// GetBytesReceived 获取接收字节数
func (ns *NetworkStats) GetBytesReceived() uint64 {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.bytesReceived
}

// GetBytesSent 获取发送字节数
func (ns *NetworkStats) GetBytesSent() uint64 {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.bytesSent
}

// GetAverageInputLatency 获取平均输入延迟
func (ns *NetworkStats) GetAverageInputLatency() time.Duration {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	if ns.inputLatencyCount == 0 {
		return 0
	}
	return ns.inputLatencySum / time.Duration(ns.inputLatencyCount)
}

// GetMaxInputLatency 获取最大输入延迟
func (ns *NetworkStats) GetMaxInputLatency() time.Duration {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.maxInputLatency
}

// GetMinInputLatency 获取最小输入延迟
func (ns *NetworkStats) GetMinInputLatency() time.Duration {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.minInputLatency
}

// GetInputLatencyCount 获取输入延迟样本数量
func (ns *NetworkStats) GetInputLatencyCount() uint64 {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.inputLatencyCount
}

// UpdateNetworkStats 更新网络统计信息
func (ns *NetworkStats) UpdateNetworkStats(totalPackets, lostPackets, bytesReceived, bytesSent uint64, latency time.Duration) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	ns.totalPackets = totalPackets
	ns.lostPackets = lostPackets
	ns.bytesReceived = bytesReceived
	ns.bytesSent = bytesSent
	if latency > 0 {
		ns.latencySum += latency
		ns.latencyCount++
		if latency > ns.maxLatency {
			ns.maxLatency = latency
		}
		if latency < ns.minLatency || ns.minLatency == 0 {
			ns.minLatency = latency
		}
	}
}

// IncrementNetworkStats 增量更新网络统计信息
func (ns *NetworkStats) IncrementNetworkStats(packetsReceived, packetsSent, bytesReceived, bytesSent uint64, lost bool, latency time.Duration) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	ns.totalPackets += packetsReceived + packetsSent
	if lost {
		ns.lostPackets++
	}
	ns.bytesReceived += bytesReceived
	ns.bytesSent += bytesSent
	if latency > 0 {
		ns.latencySum += latency
		ns.latencyCount++
		if latency > ns.maxLatency {
			ns.maxLatency = latency
		}
		if latency < ns.minLatency || ns.minLatency == 0 {
			ns.minLatency = latency
		}
	}
}

// IncrementInputLatencyStats 增量更新输入延时统计信息
func (ns *NetworkStats) IncrementInputLatencyStats(inputLatency time.Duration) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()

	ns.inputLatencySum += inputLatency
	ns.inputLatencyCount++
	if inputLatency > ns.maxInputLatency {
		ns.maxInputLatency = inputLatency
	}
	if inputLatency < ns.minInputLatency || ns.minInputLatency == 0 {
		ns.minInputLatency = inputLatency
	}
}

// Reset 重置网络统计信息
func (ns *NetworkStats) Reset() {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	ns.totalPackets = 0
	ns.lostPackets = 0
	ns.bytesReceived = 0
	ns.bytesSent = 0
	ns.latencySum = 0
	ns.latencyCount = 0
	ns.maxLatency = 0
	ns.minLatency = 0

	// 重置输入延时统计
	ns.inputLatencySum = 0
	ns.inputLatencyCount = 0
	ns.maxInputLatency = 0
	ns.minInputLatency = 0
}
