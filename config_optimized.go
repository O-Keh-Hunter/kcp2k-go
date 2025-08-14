package kcp2k

import (
	kcp "github.com/xtaci/kcp-go/v5"
)

// OptimizedKcpConfig 返回性能优化后的KCP配置
// 基于性能分析报告的建议进行优化
func OptimizedKcpConfig() KcpConfig {
	return KcpConfig{
		// 保持双模式支持IPv4和IPv6
		DualMode: true,
		
		// 优化缓冲区大小：从7MB增加到16MB
		// 这可以减少网络拥塞时的丢包率
		RecvBufferSize: 1024 * 1024 * 16, // 16MB
		SendBufferSize: 1024 * 1024 * 16, // 16MB
		
		// 优化MTU：从1200增加到1400字节
		// 减少数据包分片，提高传输效率
		Mtu: 1400,
		
		// 启用无延迟模式
		NoDelay: true,
		
		// 优化tick间隔：从10ms减少到5ms
		// 更频繁的更新可以减少延迟
		Interval: 5,
		
		// 启用快速重传：设置为2
		// 当收到2个重复ACK时立即重传
		FastResend: 2,
		
		// 禁用拥塞控制窗口
		// 在可靠网络环境下可以提高吞吐量
		CongestionWindow: false,
		
		// 增加窗口大小：从32增加到128
		// 允许更多未确认的数据包在传输中
		SendWindowSize: 128,
		ReceiveWindowSize: 128,
		
		// 保持默认超时时间
		Timeout: DEFAULT_TIMEOUT,
		
		// 保持默认最大重传次数
		MaxRetransmits: kcp.IKCP_DEADLINK,
	}
}

// HighPerformanceKcpConfig 返回高性能场景下的KCP配置
// 适用于低延迟要求的实时应用
func HighPerformanceKcpConfig() KcpConfig {
	return KcpConfig{
		DualMode: true,
		
		// 更大的缓冲区用于高吞吐量场景
		RecvBufferSize: 1024 * 1024 * 32, // 32MB
		SendBufferSize: 1024 * 1024 * 32, // 32MB
		
		// 最优MTU设置
		Mtu: 1400,
		
		// 启用无延迟模式
		NoDelay: true,
		
		// 最小tick间隔
		Interval: 1,
		
		// 激进的快速重传
		FastResend: 1,
		
		// 禁用拥塞控制
		CongestionWindow: false,
		
		// 最大窗口大小
		SendWindowSize: 256,
		ReceiveWindowSize: 256,
		
		// 较短的超时时间
		Timeout: 5000, // 5秒
		
		// 减少重传次数
		MaxRetransmits: 10,
	}
}

// ConservativeKcpConfig 返回保守的KCP配置
// 适用于不稳定网络环境
func ConservativeKcpConfig() KcpConfig {
	return KcpConfig{
		DualMode: true,
		
		// 适中的缓冲区大小
		RecvBufferSize: 1024 * 1024 * 8, // 8MB
		SendBufferSize: 1024 * 1024 * 8, // 8MB
		
		// 保守的MTU设置
		Mtu: 1200,
		
		// 启用无延迟模式
		NoDelay: true,
		
		// 标准tick间隔
		Interval: 10,
		
		// 保守的快速重传
		FastResend: 0,
		
		// 启用拥塞控制
		CongestionWindow: true,
		
		// 标准窗口大小
		SendWindowSize: 64,
		ReceiveWindowSize: 64,
		
		// 较长的超时时间
		Timeout: 15000, // 15秒
		
		// 更多重传次数
		MaxRetransmits: 30,
	}
}