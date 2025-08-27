package kcp2k

// HighPerformanceKcpConfig 返回高性能场景下的KCP配置
// 适用于低延迟要求的实时应用
func HighPerformanceKcpConfig() KcpConfig {
	return KcpConfig{
		DualMode: false,

		// 更大的缓冲区用于高吞吐量场景
		RecvBufferSize: 4 * 1024 * 1024, // 4MB
		SendBufferSize: 4 * 1024 * 1024, // 4MB

		// 最优MTU设置
		Mtu: 1400,

		// 启用无延迟模式
		NoDelay: true,

		// 最小tick间隔
		Interval: 10,

		// 激进的快速重传
		FastResend: 2,

		// 禁用拥塞控制
		CongestionWindow: false,

		// 最大窗口大小
		SendWindowSize:    512,
		ReceiveWindowSize: 512,

		// 较短的超时时间
		Timeout: 10 * 1000, // 10秒

		// 减少重传次数
		MaxRetransmits: 20,
	}
}

// ConservativeKcpConfig 返回保守的KCP配置
// 适用于不稳定网络环境
func ConservativeKcpConfig() KcpConfig {
	return KcpConfig{
		DualMode: false,

		// 更大的缓冲区用于高吞吐量场景
		RecvBufferSize: 4 * 1024 * 1024, // 4MB
		SendBufferSize: 4 * 1024 * 1024, // 4MB

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
		SendWindowSize:    64,
		ReceiveWindowSize: 64,

		// 较长的超时时间
		Timeout: 15000, // 15秒

		// 更多重传次数
		MaxRetransmits: 30,
	}
}
