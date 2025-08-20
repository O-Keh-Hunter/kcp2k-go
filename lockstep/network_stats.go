package lockstep

import (
	"sync"
	"time"
)

// NetworkStats 网络统计信息
type NetworkStats struct {
	// 网络包统计
	totalPackets  uint64
	lostPackets   uint64
	bytesReceived uint64
	bytesSent     uint64

	// 网络 RTT 统计
	rttSum   time.Duration
	rttCount uint64
	maxRTT   time.Duration
	minRTT   time.Duration

	// 输入延时统计
	inputLatencySum   time.Duration
	inputLatencyCount uint64
	maxInputLatency   time.Duration
	minInputLatency   time.Duration

	// Jitter统计
	jitterSum    time.Duration
	jitterCount  uint64
	maxJitter    time.Duration
	minJitter    time.Duration
	lastRecvTime time.Time

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

// GetAverageRTT 获取平均RTT
func (ns *NetworkStats) GetAverageRTT() time.Duration {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	if ns.rttCount == 0 {
		return 0
	}
	return ns.rttSum / time.Duration(ns.rttCount)
}

// GetTotalRTT 获取总RTT
func (ns *NetworkStats) GetTotalRTT() time.Duration {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.rttSum
}

// GetRTTCount 获取RTT样本数量
func (ns *NetworkStats) GetRTTCount() uint64 {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.rttCount
}

// GetMaxRTT 获取最大RTT
func (ns *NetworkStats) GetMaxRTT() time.Duration {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.maxRTT
}

// GetMinRTT 获取最小RTT
func (ns *NetworkStats) GetMinRTT() time.Duration {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.minRTT
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
		ns.rttSum += latency
		ns.rttCount++
		if latency > ns.maxRTT {
			ns.maxRTT = latency
		}
		if latency < ns.minRTT || ns.minRTT == 0 {
			ns.minRTT = latency
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
		ns.rttSum += latency
		ns.rttCount++
		if latency > ns.maxRTT {
			ns.maxRTT = latency
		}
		if latency < ns.minRTT || ns.minRTT == 0 {
			ns.minRTT = latency
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

// GetAverageJitter 获取平均抖动
func (ns *NetworkStats) GetAverageJitter() time.Duration {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	if ns.jitterCount == 0 {
		return 0
	}
	return ns.jitterSum / time.Duration(ns.jitterCount)
}

// GetMaxJitter 获取最大抖动
func (ns *NetworkStats) GetMaxJitter() time.Duration {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.maxJitter
}

// GetMinJitter 获取最小抖动
func (ns *NetworkStats) GetMinJitter() time.Duration {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.minJitter
}

// GetJitterCount 获取抖动样本数量
func (ns *NetworkStats) GetJitterCount() uint64 {
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.jitterCount
}

// UpdateJitterStats 基于期望间隔更新抖动统计信息
// 抖动（Jitter）：连续帧收包时间间隔的不稳定差异（即实际到达间隔 vs. 期望间隔）
// expectedInterval: 期望的帧间隔时间（如30fps对应33.33ms，20fps对应50ms）
func (ns *NetworkStats) UpdateJitterStats(recvTime time.Time, expectedInterval time.Duration) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()

	// 如果这不是第一个包，计算抖动
	if !ns.lastRecvTime.IsZero() {
		// 计算当前包间隔
		currentInterval := recvTime.Sub(ns.lastRecvTime)

		// 计算抖动（当前间隔与期望间隔的差值的绝对值）
		jitter := currentInterval - expectedInterval
		if jitter < 0 {
			jitter = -jitter
		}

		// 更新抖动统计
		ns.jitterSum += jitter
		ns.jitterCount++
		if jitter > ns.maxJitter {
			ns.maxJitter = jitter
		}
		if jitter < ns.minJitter || ns.minJitter == 0 {
			ns.minJitter = jitter
		}
	}

	// 更新最后接收时间
	ns.lastRecvTime = recvTime
}

// Reset 重置网络统计信息
func (ns *NetworkStats) Reset() {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()

	ns.totalPackets = 0
	ns.lostPackets = 0
	ns.bytesReceived = 0
	ns.bytesSent = 0

	ns.rttSum = 0
	ns.rttCount = 0
	ns.maxRTT = 0
	ns.minRTT = 0

	ns.inputLatencySum = 0
	ns.inputLatencyCount = 0
	ns.maxInputLatency = 0
	ns.minInputLatency = 0

	ns.jitterSum = 0
	ns.jitterCount = 0
	ns.maxJitter = 0
	ns.minJitter = 0

	ns.lastRecvTime = time.Time{}
}
