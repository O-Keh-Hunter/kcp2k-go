package kcp2k

// KcpHeaderReliable 表示可靠消息的高层协议头
// 与C#版KcpHeaderReliable枚举保持一致

type KcpHeaderReliable byte

const (
	KcpHeaderHello KcpHeaderReliable = 1
	KcpHeaderPing  KcpHeaderReliable = 2
	KcpHeaderData  KcpHeaderReliable = 3
	KcpHeaderPong  KcpHeaderReliable = 4 // 4而不是3，为兼容历史
)

// KcpHeaderUnreliable 表示不可靠消息的高层协议头
// 与C#版KcpHeaderUnreliable枚举保持一致

type KcpHeaderUnreliable byte

const (
	KcpHeaderUnrelData       KcpHeaderUnreliable = 4
	KcpHeaderUnrelDisconnect KcpHeaderUnreliable = 5
)

// ParseReliable 安全解析可靠头
func ParseReliable(value byte) (KcpHeaderReliable, bool) {
	switch KcpHeaderReliable(value) {
	case KcpHeaderHello, KcpHeaderPing, KcpHeaderData, KcpHeaderPong:
		return KcpHeaderReliable(value), true
	default:
		return KcpHeaderPing, false // 默认
	}
}

// ParseUnreliable 安全解析不可靠头
func ParseUnreliable(value byte) (KcpHeaderUnreliable, bool) {
	switch KcpHeaderUnreliable(value) {
	case KcpHeaderUnrelData, KcpHeaderUnrelDisconnect:
		return KcpHeaderUnreliable(value), true
	default:
		return KcpHeaderUnrelDisconnect, false // 默认
	}
}
