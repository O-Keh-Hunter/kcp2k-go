package kcp2k

// KcpState 表示KCP连接状态
// 与C#版KcpState枚举保持一致
// Connected=0, Authenticated=1, Disconnected=2

type KcpState int

const (
	KcpConnected KcpState = iota
	KcpAuthenticated
	KcpDisconnected
)
