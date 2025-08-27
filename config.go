package kcp2k

import (
	kcp "github.com/xtaci/kcp-go/v5"
)

// KcpConfig 包含KCP高层协议的全部配置项，对应C#版KcpConfig
type KcpConfig struct {
	DualMode          bool
	RecvBufferSize    int
	SendBufferSize    int
	Mtu               int
	NoDelay           bool
	Interval          uint
	FastResend        int
	CongestionWindow  bool
	SendWindowSize    uint
	ReceiveWindowSize uint
	Timeout           int
	MaxRetransmits    uint
}

// DefaultKcpConfig 返回与C#默认构造函数等价的默认配置
func DefaultKcpConfig() KcpConfig {
	return KcpConfig{
		DualMode:          false,
		RecvBufferSize:    1024 * 1024 * 7,
		SendBufferSize:    1024 * 1024 * 7,
		Mtu:               kcp.IKCP_MTU_DEF,
		NoDelay:           true,
		Interval:          10,
		FastResend:        0,
		CongestionWindow:  false,
		SendWindowSize:    kcp.IKCP_WND_SND,
		ReceiveWindowSize: kcp.IKCP_WND_RCV,
		Timeout:           DEFAULT_TIMEOUT,
		MaxRetransmits:    kcp.IKCP_DEADLINK,
	}
}

// KcpConfigOption 为可选参数设置器
// 使用方式：cfg := NewKcpConfig(WithMtu(1200), WithSendWindowSize(128))

type KcpConfigOption func(*KcpConfig)

func NewKcpConfig(opts ...KcpConfigOption) KcpConfig {
	cfg := DefaultKcpConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

func WithDualMode(v bool) KcpConfigOption      { return func(c *KcpConfig) { c.DualMode = v } }
func WithRecvBufferSize(v int) KcpConfigOption { return func(c *KcpConfig) { c.RecvBufferSize = v } }
func WithSendBufferSize(v int) KcpConfigOption { return func(c *KcpConfig) { c.SendBufferSize = v } }
func WithMtu(v int) KcpConfigOption            { return func(c *KcpConfig) { c.Mtu = v } }
func WithNoDelay(v bool) KcpConfigOption       { return func(c *KcpConfig) { c.NoDelay = v } }
func WithInterval(v uint) KcpConfigOption      { return func(c *KcpConfig) { c.Interval = v } }
func WithFastResend(v int) KcpConfigOption     { return func(c *KcpConfig) { c.FastResend = v } }
func WithCongestionWindow(v bool) KcpConfigOption {
	return func(c *KcpConfig) { c.CongestionWindow = v }
}
func WithSendWindowSize(v uint) KcpConfigOption { return func(c *KcpConfig) { c.SendWindowSize = v } }
func WithReceiveWindowSize(v uint) KcpConfigOption {
	return func(c *KcpConfig) { c.ReceiveWindowSize = v }
}
func WithTimeout(v int) KcpConfigOption         { return func(c *KcpConfig) { c.Timeout = v } }
func WithMaxRetransmits(v uint) KcpConfigOption { return func(c *KcpConfig) { c.MaxRetransmits = v } }
