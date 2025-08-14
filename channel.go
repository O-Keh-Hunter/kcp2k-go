package kcp2k

// KcpChannel 表示KCP高层通道类型
// 与C#版KcpChannel枚举保持一致
// Reliable=1, Unreliable=2

type KcpChannel byte

const (
	KcpReliable   KcpChannel = 1
	KcpUnreliable KcpChannel = 2
)
