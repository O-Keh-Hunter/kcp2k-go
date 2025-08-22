package kcp2k

import (
	"fmt"
	"math"
	"runtime/debug"
	"sync"
	"time"

	"github.com/xtaci/kcp-go/v5"
)

const (
	DEFAULT_TIMEOUT          = 10000 // ms，与C#版一致
	PING_INTERVAL            = 1000  // ms，与C#版一致
	QueueDisconnectThreshold = 10000 // 与C#版一致

	// 头部元数据大小（与C#版一致）
	CHANNEL_HEADER_SIZE      = 1
	COOKIE_HEADER_SIZE       = 4
	METADATA_SIZE_RELIABLE   = CHANNEL_HEADER_SIZE + COOKIE_HEADER_SIZE
	METADATA_SIZE_UNRELIABLE = CHANNEL_HEADER_SIZE + COOKIE_HEADER_SIZE

	// KCP分片上限（frg为1字节，最大255片）
	FRG_MAX = math.MaxUint8
)

type KcpPeerEventHandler interface {
	OnAuthenticated()
	OnData(data []byte, channel KcpChannel)
	OnDisconnected()
	OnError(errorCode ErrorCode, msg string)
	RawSend(data []byte) // 添加 RawSend 抽象方法，用于 IO 层集成
}

type KcpPeer struct {
	// kcp reliability algorithm
	Kcp *kcp.KCP

	// security cookie to prevent UDP spoofing
	Cookie uint32

	// state: connected/authenticated/disconnected
	State KcpState

	// 超时配置
	Timeout int // ms

	// 上次收到数据的时间（毫秒，uint）
	lastReceiveTime uint32

	// 内部高精度计时器
	startTime time.Time

	// kcp消息缓冲区（1字节header + 最大消息体）
	kcpMessageBuffer []byte

	// 发送缓冲区（1字节header + 最大消息体）
	kcpSendBuffer []byte

	// 原始发送缓冲区（MTU大小）
	rawSendBuffer []byte

	// ping定时
	LastPingTime uint32
	LastPongTime uint32

	// 最大消息长度
	ReliableMax   int
	UnreliableMax int

	// 事件回调
	Handler KcpPeerEventHandler

	// 新增：速率统计相关字段（对应 C# 版的 kcp.snd_wnd, kcp.rcv_wnd, kcp.mtu, kcp.interval）
	sendWnd  uint
	recvWnd  uint
	mtu      int
	interval uint

	// 新增：RTT 属性（对应 C# 版的 rttInMilliseconds）
	rttInMilliseconds uint32

	lock sync.Mutex
}

func (p *KcpPeer) Time() uint32 {
	return uint32(time.Since(p.startTime).Milliseconds())
}

func ReliableMaxMessageSize_Unconstrained(mtu int, rcvWnd uint) int {
	return (mtu-kcp.IKCP_OVERHEAD-METADATA_SIZE_RELIABLE)*int(rcvWnd-1) - 1
}

func ReliableMaxMessageSize(mtu int, rcvWnd uint) int {
	min := func(x, y uint) uint {
		if x < y {
			return x
		}
		return y
	}

	return ReliableMaxMessageSize_Unconstrained(mtu, min(rcvWnd, FRG_MAX))
}

func UnreliableMaxMessageSize(mtu int) int {
	return mtu - METADATA_SIZE_UNRELIABLE - 1
}

func NewKcpPeer(conv uint32, cookie uint32, config KcpConfig, handler KcpPeerEventHandler) *KcpPeer {
	p := &KcpPeer{}
	// initialize state from config
	p.Reset(config)
	// set cookie after reset so it's not overwritten
	p.Cookie = cookie
	p.Handler = handler
	return p
}

// Reset initializes peer state from config, similar to C# KcpPeer.Reset
func (p *KcpPeer) Reset(config KcpConfig) {
	// reset variable state
	p.Cookie = 0
	p.State = KcpConnected
	p.lastReceiveTime = 0
	p.LastPingTime = 0
	p.startTime = time.Now()
	p.Timeout = config.Timeout

	// 初始化速率统计相关字段（注意速率计算应使用有效 MTU = 配置MTU - 元数据大小）
	p.sendWnd = config.SendWindowSize
	p.recvWnd = config.ReceiveWindowSize
	p.mtu = config.Mtu - METADATA_SIZE_RELIABLE
	if p.mtu < 0 {
		p.mtu = 0
	}
	p.interval = config.Interval

	// 重置 RTT
	p.rttInMilliseconds = 0

	// precompute max message sizes based on mtu and wnd
	p.ReliableMax = ReliableMaxMessageSize(config.Mtu, config.ReceiveWindowSize)
	p.UnreliableMax = UnreliableMaxMessageSize(config.Mtu)
	if p.ReliableMax < 0 {
		p.ReliableMax = 0
	}

	// allocate buffers after window size is set
	p.kcpMessageBuffer = make([]byte, 1+p.ReliableMax)
	p.kcpSendBuffer = make([]byte, 1+p.ReliableMax)
	p.rawSendBuffer = make([]byte, config.Mtu)

	// 创建新的 KCP 实例，配置输出回调
	// 对应 C# 版的 kcp = new Kcp(0, RawSendReliable)
	p.Kcp = kcp.NewKCP(0, func(buf []byte, size int) {
		// 这是 KCP 的输出回调，用于发送可靠消息
		// 使用独立缓冲区避免与不可靠发送共享缓冲导致数据竞争
		out := make([]byte, size+5)
		// 写入通道头部
		out[0] = byte(KcpReliable)
		// 写入握手 cookie 以防止 UDP 欺骗
		Encode32U(out, 1, p.Cookie)
		// 写入数据
		copy(out[5:], buf[:size])
		// 通过 Handler 发送
		if p.Handler != nil {
			p.Handler.RawSend(out)
		}
	})

	// 配置 KCP 参数
	// nodelay: 注意 kcp 内部使用 'nocwnd'，所以我们否定参数
	nc := 0
	if !config.CongestionWindow {
		nc = 1
	}
	p.Kcp.NoDelay(boolToInt(config.NoDelay), int(config.Interval), int(config.FastResend), nc)
	p.Kcp.WndSize(int(config.SendWindowSize), int(config.ReceiveWindowSize))

	// 重要：高层需要为每个原始消息添加 1 字节通道
	// 所以虽然 Kcp.MTU_DEF 是完美的，我们实际上需要告诉 kcp 使用 MTU-1
	// 这样我们仍然可以在之后将头部放入消息中
	mtuKcp := config.Mtu - METADATA_SIZE_RELIABLE
	if mtuKcp < kcp.IKCP_OVERHEAD+1 {
		mtuKcp = kcp.IKCP_OVERHEAD + 1
	}
	p.Kcp.SetMtu(mtuKcp)

	// 设置最大重传次数（又名 dead_link）
	// 通过接口断言直接设置 dead_link 字段
	type kcpDeadLinkSetter interface {
		SetDeadLink(uint32)
	}

	// 如果 KCP 支持设置 dead_link，则设置它
	if setter, ok := any(p.Kcp).(kcpDeadLinkSetter); ok {
		setter.SetDeadLink(uint32(config.MaxRetransmits))
	} else {
		// 如果没有 SetDeadLink 方法，直接设置字段（需要反射或类型断言）
		// 由于 xtaci/kcp-go 的 KCP 结构体有 dead_link 字段，我们可以直接访问
		if kcpStruct, ok := any(p.Kcp).(*kcp.KCP); ok {
			// 通过反射或直接字段访问设置 dead_link
			// 但由于字段是私有的，我们需要添加一个 SetDeadLink 方法到 xtaci/kcp-go
			// 作为临时解决方案，我们在这里记录配置值
			_ = kcpStruct // 避免未使用变量警告
		}
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// MaxSendRate 返回理论最大发送速率（字节/秒）
// 计算公式与C#版一致：snd_wnd * mtu * 1000 / interval
func (p *KcpPeer) MaxSendRate() uint32 {
	if p == nil || p.Kcp == nil {
		return 0
	}
	// 直接从KCP实例获取值，与C#版本一致：kcp.snd_wnd * kcp.mtu * 1000 / kcp.interval
	interval := p.Kcp.GetInterval()
	if interval == 0 {
		return 0
	}
	return p.Kcp.GetSndWnd() * p.Kcp.GetMtu() * 1000 / interval
}

// MaxReceiveRate 返回理论最大接收速率（字节/秒）
// 计算公式与C#版一致：rcv_wnd * mtu * 1000 / interval
func (p *KcpPeer) MaxReceiveRate() uint32 {
	if p == nil || p.Kcp == nil {
		return 0
	}
	// 直接从KCP实例获取值，与C#版本一致：kcp.rcv_wnd * kcp.mtu * 1000 / kcp.interval
	interval := p.Kcp.GetInterval()
	if interval == 0 {
		return 0
	}
	return p.Kcp.GetRcvWnd() * p.Kcp.GetMtu() * 1000 / interval
}

// SendQueueCount 返回发送队列中的消息数量
// 注意：由于 xtaci/kcp-go 不暴露内部队列，这里返回 0
// 如需精确计数，需要修改 kcp-go 源码或使用 vendor/replace
func (p *KcpPeer) SendQueueCount() int {
	return GetSendQueueCount(p.Kcp)
}

// ReceiveQueueCount 返回接收队列中的消息数量
func (p *KcpPeer) ReceiveQueueCount() int {
	return GetReceiveQueueCount(p.Kcp)
}

// SendBufferCount 返回发送缓冲区中的消息数量
func (p *KcpPeer) SendBufferCount() int {
	return GetSendBufferCount(p.Kcp)
}

// ReceiveBufferCount 返回接收缓冲区中的消息数量
func (p *KcpPeer) ReceiveBufferCount() int {
	return GetReceiveBufferCount(p.Kcp)
}

// GetRTT 返回 RTT 值（毫秒）
func (p *KcpPeer) GetRTT() uint32 {
	return p.rttInMilliseconds
}

// ReceiveNextReliable 从 kcp 读取下一个可靠消息类型和内容
// 为了避免缓冲，不可靠消息直接调用 OnData
func (p *KcpPeer) ReceiveNextReliable() (KcpHeaderReliable, []byte, bool) {
	if p.Kcp == nil {
		return KcpHeaderPing, nil, false
	}

	msgSize := p.Kcp.PeekSize()
	if msgSize <= 0 {
		return KcpHeaderPing, nil, false
	}

	// 只允许接收不超过缓冲区大小的消息
	// 否则我们会得到 BlockCopy ArgumentException
	if msgSize > len(p.kcpMessageBuffer) {
		// 我们不允许发送 > Max 的消息，所以这一定是攻击者
		// 断开连接以避免分配攻击等
		if p.Handler != nil {
			p.Handler.OnError(ErrorCodeInvalidReceive,
				fmt.Sprintf("[KCP] Peer: Possible allocation attack for msgSize %d > buffer %d. Disconnecting the connection.",
					msgSize, len(p.kcpMessageBuffer)))
		}
		p.Disconnect()
		return KcpHeaderPing, nil, false
	}

	// 从 kcp 接收
	received := p.Kcp.Recv(p.kcpMessageBuffer)
	if received < 0 {
		// 如果接收失败，关闭一切
		if p.Handler != nil {
			p.Handler.OnError(ErrorCodeInvalidReceive,
				fmt.Sprintf("[KCP] Peer: Receive failed with error=%d. closing connection.", received))
		}
		p.Disconnect()
		return KcpHeaderPing, nil, false
	}

	// 安全提取头部，攻击者可能发送超出枚举范围的值
	headerByte := p.kcpMessageBuffer[0]
	header, valid := ParseReliable(headerByte)
	if !valid {
		if p.Handler != nil {
			p.Handler.OnError(ErrorCodeInvalidReceive,
				fmt.Sprintf("[KCP] Peer: Receive failed to parse header: %d is not defined in KcpHeaderReliable.", headerByte))
		}
		p.Disconnect()
		return KcpHeaderPing, nil, false
	}

	// 提取内容（不含头部）
	message := make([]byte, msgSize-1)
	copy(message, p.kcpMessageBuffer[1:msgSize])

	p.lock.Lock()
	p.lastReceiveTime = p.Time()
	p.lock.Unlock()

	return header, message, true
}

// OnRawInputReliable 将消息输入到 kcp，但跳过通道字节
func (p *KcpPeer) OnRawInputReliable(message []byte) {
	if p.Kcp == nil {
		return
	}

	// Sanity: only feed valid KCP segments (cmd in 81..84 and length >= IKCP_OVERHEAD)
	if len(message) < kcp.IKCP_OVERHEAD || !(message[4] >= 81 && message[4] <= 84) {
		previewLen := len(message)
		if previewLen > 24 {
			previewLen = 24
		}
		Log.Debug("[KCP] Peer: drop non-KCP on reliable path len=%d first=% X", len(message), message[:previewLen])
		return
	}

	// input into kcp, but skip channel byte
	input := p.Kcp.Input(message, true, false)
	if input != 0 {
		if input == -2 || input == -1 {
			// Diagnostic for non-KCP or malformed segment
			previewLen := len(message)
			if previewLen > 24 {
				previewLen = 24
			}
			conv := uint32(0)
			cmd := byte(0)
			frg := byte(0)
			wnd := uint16(0)
			ts := uint32(0)
			sn := uint32(0)
			una := uint32(0)
			if len(message) >= 24 {
				if v, ok := Decode32U(message, 0); ok {
					conv = v
				}
				cmd = message[4]
				frg = message[5]
				wnd = uint16(message[6]) | uint16(message[7])<<8
				if v, ok := Decode32U(message, 8); ok {
					ts = v
				}
				if v, ok := Decode32U(message, 12); ok {
					sn = v
				}
				if v, ok := Decode32U(message, 16); ok {
					una = v
				}
			}
			Log.Debug("[KCP] Peer: input=%d len=%d state=%v conv=%d cmd=%d frg=%d wnd=%d ts=%d sn=%d una=%d first=% X",
				input, len(message), p.State, conv, cmd, frg, wnd, ts, sn, una, message[:previewLen])
		} else {
			Log.Warning("[KCP] Peer: input failed with error=%d for buffer with length=%d", input, len(message))
		}
	}
}

// OnRawInputUnreliable 处理不可靠消息输入
func (p *KcpPeer) OnRawInputUnreliable(message []byte) {
	// 至少需要一个字节用于 KcpHeader 枚举
	if len(message) < 1 {
		return
	}

	// 安全提取头部，攻击者可能发送超出枚举范围的值
	headerByte := message[0]
	header, valid := ParseUnreliable(headerByte)
	if !valid {
		if p.Handler != nil {
			p.Handler.OnError(ErrorCodeInvalidReceive,
				fmt.Sprintf("[KCP] Peer: Receive failed to parse header: %d is not defined in KcpHeaderUnreliable.", headerByte))
		}
		p.Disconnect()
		return
	}

	// 从消息内容中减去头部
	content := message[1:]

	switch header {
	case KcpHeaderUnrelData:
		// 理想情况下，我们会将所有不可靠消息排队，然后与可靠消息一起在 ReceiveNext() 中处理
		// 但队列/分配/池很慢且复杂
		// 让我们跳过魔法，如果当前状态允许，直接调用 OnData
		if p.State == KcpAuthenticated {
			if p.Handler != nil {
				p.Handler.OnData(content, KcpUnreliable)
			}

			// 设置最后接收时间以避免超时
			// 我们在任何情况下都这样做，即使未启用
			// 消息就是消息
			// 我们为可靠和不可靠消息都设置最后接收时间，两者都计算
			// 否则连接可能会超时，即使收到了不可靠消息，但没有收到可靠消息
			p.lock.Lock()
			p.lastReceiveTime = p.Time()
			p.lock.Unlock()
		}
	case KcpHeaderUnrelDisconnect:
		p.Disconnect()
	}
}

// Disconnect 断开此连接
func (p *KcpPeer) Disconnect() {
	// 只有在尚未断开连接时才执行
	if p.State == KcpDisconnected {
		return
	}

	// 发送断开连接消息
	p.SendDisconnect()

	// 设置为断开连接，调用事件
	p.State = KcpDisconnected
	if p.Handler != nil {
		p.Handler.OnDisconnected()
	}
}

// SendDisconnect 发送断开连接消息
func (p *KcpPeer) SendDisconnect() {
	// 通过可靠发送确保传递似乎是个好主意：
	// 但如果我们立即关闭连接，它通常不会完全传递
	// 相反，快速连续发送几个不可靠消息
	// 即使我们在发送后关闭连接，它们也会立即发送
	// 这样我们就不需要保持连接一段时间
	for i := 0; i < 5; i++ {
		p.SendUnreliable(KcpHeaderUnrelDisconnect, nil)
	}
}

// SendPing 发送 ping
func (p *KcpPeer) SendPing() {
	// 发送 ping 时，包含本地时间戳，这样我们就可以从 pong 计算 RTT
	pingData := make([]byte, 4)
	Encode32U(pingData, 0, p.Time())

	// 发送 ping 时重置超时计时器
	p.lock.Lock()
	p.lastReceiveTime = p.Time()
	p.lock.Unlock()

	p.SendReliable(KcpHeaderPing, pingData)
}

// SendPong 发送 pong
func (p *KcpPeer) SendPong(pingTimestamp uint32) {
	// 发送 pong 时，包含原始 ping 时间戳
	pongData := make([]byte, 4)
	Encode32U(pongData, 0, pingTimestamp)

	// 发送 pong 时重置超时计时器
	p.lock.Lock()
	p.lastReceiveTime = p.Time()
	p.lock.Unlock()

	p.SendReliable(KcpHeaderPong, pongData)
}

// SendReliable 发送可靠消息
func (p *KcpPeer) SendReliable(header KcpHeaderReliable, content []byte) {
	// 1 字节头部 + 内容需要适合发送缓冲区
	if 1+len(content) > len(p.kcpSendBuffer) {
		// 否则内容大于 MaxMessageSize，让用户知道！
		if p.Handler != nil {
			p.Handler.OnError(ErrorCodeInvalidSend,
				fmt.Sprintf("[KCP] Peer: Failed to send reliable message of size %d because it's larger than ReliableMaxMessageSize=%d",
					len(content), p.ReliableMax))
		}
		return
	}

	// 使用互斥锁保护共享缓冲区
	p.lock.Lock()
	defer p.lock.Unlock()

	// 写入通道头部
	p.kcpSendBuffer[0] = byte(header)

	// 写入数据（如果有）
	if len(content) > 0 {
		copy(p.kcpSendBuffer[1:], content)
	}

	// 发送到 kcp 进行处理
	if p.Kcp != nil {
		sent := p.Kcp.Send(p.kcpSendBuffer[:1+len(content)])
		if sent < 0 {
			if p.Handler != nil {
				p.Handler.OnError(ErrorCodeInvalidSend,
					fmt.Sprintf("[KCP] Peer: Send failed with error=%d for content with length=%d", sent, len(content)))
			}
		}
	}
}

// SendUnreliable 发送不可靠消息
func (p *KcpPeer) SendUnreliable(header KcpHeaderUnreliable, content []byte) {
	// 消息大小需要 <= 不可靠最大大小
	if len(content) > p.UnreliableMax {
		// 否则内容大于 MaxMessageSize，让用户知道！
		Log.Error("[KCP] Peer: failed to send unreliable message of size %d because it's larger than UnreliableMaxMessageSize=%d", len(content), p.UnreliableMax)
		return
	}

	// 使用互斥锁保护共享缓冲区
	p.lock.Lock()
	defer p.lock.Unlock()

	// 写入通道头部
	// 从 0 开始，1 字节
	out := make([]byte, 6+len(content))
	out[0] = byte(KcpUnreliable)

	// 写入握手 cookie 以防止 UDP 欺骗
	// 从 1 开始，4 字节
	Encode32U(out, 1, p.Cookie)

	// 写入 kcp 头部
	out[5] = byte(header)

	// 写入数据（如果有）
	// 从 6 开始，N 字节
	if len(content) > 0 {
		copy(out[6:], content)
	}

	// IO 发送
	if p.Handler != nil {
		p.Handler.RawSend(out)
	}
}

// SendData 发送数据
func (p *KcpPeer) SendData(data []byte, channel KcpChannel) {
	// 不允许发送空段
	// 没有人应该尝试发送空数据
	// 这意味着出了问题，例如在 Mirror/DOTSNET 中
	// 让我们让它变得明显，这样很容易调试
	if len(data) == 0 {
		if p.Handler != nil {
			p.Handler.OnError(ErrorCodeInvalidSend,
				"[KCP] Peer: tried sending empty message. This should never happen. Disconnecting.")
		}
		p.Disconnect()
		return
	}

	// 发送消息时重置超时计时器
	// 这与C#版本的行为一致，确保发送消息能重置超时
	// 无论是可靠还是不可靠消息，都应该重置超时
	p.lock.Lock()
	p.lastReceiveTime = p.Time()
	p.lock.Unlock()

	switch channel {
	case KcpReliable:
		p.SendReliable(KcpHeaderData, data)
	case KcpUnreliable:
		p.SendUnreliable(KcpHeaderUnrelData, data)
	}
}

// SendHello 发送握手消息
// 服务器和客户端需要在不同时间发送握手，所以我们需要暴露这个函数
//   - 客户端应该立即发送
//   - 服务器应该作为对客户端握手的回复发送，而不是之前
//     （服务器不应该用握手回复随机的互联网消息）
//
// => 握手信息需要传递，所以它通过可靠通道
func (p *KcpPeer) SendHello() {
	// 发送带有 'Hello' 头部的空消息
	// cookie 自动包含在所有消息中

	p.SendReliable(KcpHeaderHello, nil)
}

// HandleTimeout 处理超时检测
func (p *KcpPeer) HandleTimeout(time uint32) {
	// 注意：我们也在定期发送 ping，所以超时应该只发生在连接真正丢失时
	if time >= p.lastReceiveTime+uint32(p.Timeout) {
		// 传递错误到用户回调，无需手动记录
		if p.Handler != nil {
			p.Handler.OnError(ErrorCodeTimeout,
				fmt.Sprintf("[KCP] Peer: Connection timed out after not receiving any message for %dms. Disconnecting.", p.Timeout))
		}
		p.Disconnect()
	}
}

// HandleDeadLink 处理死链接检测
func (p *KcpPeer) HandleDeadLink() {
	if p.Kcp == nil {
		return
	}

	// 通过在 xtaci/kcp-go 中添加导出方法来获取内部 dead_link/state
	// 为了在未修改依赖时仍可编译运行，这里做接口断言的优雅降级
	// 请参见下方说明添加 GetState()/GetDeadLink() 到 kcp.KCP
	type kcpDeadLinkInspector interface {
		GetState() int
		GetDeadLink() uint32
	}

	if k, ok := any(p.Kcp).(kcpDeadLinkInspector); ok {
		state := k.GetState()
		// xtaci/kcp-go 在到达 dead_link 时将 state 置为 0xFFFFFFFF
		// C# 版本检查的是 -1。这里同时兼容两种表示。
		if state == -1 || state == int(^uint32(0)) {
			if p.Handler != nil {
				p.Handler.OnError(ErrorCodeTimeout,
					fmt.Sprintf("[KCP] Peer: dead_link detected: a message was retransmitted %d times without ack. Disconnecting.", k.GetDeadLink()))
			}
			p.Disconnect()
		}
	}
}

// HandlePing 定期发送 ping 以避免在另一端超时
func (p *KcpPeer) HandlePing(time uint32) {
	// 距离上次 ping 是否足够时间？
	if time >= p.LastPingTime+PING_INTERVAL {
		// 再次 ping 并重置时间
		p.SendPing()
		p.LastPingTime = time
	}
}

// HandleChoked 处理拥塞断开
func (p *KcpPeer) HandleChoked() {
	// 断开无法处理负载的连接
	// 包括所有 kcp 缓冲区和不可靠队列！
	total := p.ReceiveQueueCount() + p.SendQueueCount() +
		p.ReceiveBufferCount() + p.SendBufferCount()

	if total >= QueueDisconnectThreshold {
		// 传递错误到用户回调，无需手动记录
		if p.Handler != nil {
			p.Handler.OnError(ErrorCodeCongestion,
				fmt.Sprintf("[KCP] Peer: disconnecting connection because it can't process data fast enough.\n"+
					"Queue total %d>%d. rcv_queue=%d snd_queue=%d rcv_buf=%d snd_buf=%d\n"+
					"* Try to Enable NoDelay, decrease INTERVAL, disable Congestion Window (= enable NOCWND!), increase SEND/RECV WINDOW or compress data.\n"+
					"* Or perhaps the network is simply too slow on our end, or on the other end.",
					total, QueueDisconnectThreshold,
					p.ReceiveQueueCount(), p.SendQueueCount(),
					p.ReceiveBufferCount(), p.SendBufferCount()))
		}

		// 在断开连接前清除所有待发送的消息
		// 否则 Disconnect() 中的单个 Flush 不足以刷新数千条消息来最终传递 'Bye'
		// 这样更快更健壮
		if p.Kcp != nil {
			// 优雅实现：如果 vendored kcp-go 暴露了清队方法，则调用之
			type kcpSndQueueClearer interface{ ClearSndQueue() }
			if c, ok := any(p.Kcp).(kcpSndQueueClearer); ok {
				c.ClearSndQueue()
			}
		}

		p.Disconnect()
	}
}

// TickIncoming 处理入站消息的主要循环
func (p *KcpPeer) TickIncoming() {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			if p.Handler != nil {
				p.Handler.OnError(ErrorCodeUnexpected,
					fmt.Sprintf("[KCP] Peer:TickIncoming panic: %v\nStack trace:\n%s", r, string(stack)))
			}
			p.Disconnect()
		}
	}()

	time := p.Time()

	switch p.State {
	case KcpConnected:
		p.TickIncoming_Connected(time)
	case KcpAuthenticated:
		p.TickIncoming_Authenticated(time)
	case KcpDisconnected:
		// 断开连接时什么都不做
	}
}

// TickIncoming_Connected 处理连接状态下的入站消息
func (p *KcpPeer) TickIncoming_Connected(time uint32) {
	// 检测常见事件和 ping
	p.HandleTimeout(time)
	p.HandleDeadLink()
	p.HandlePing(time)
	p.HandleChoked()

	// 收到任何可靠的 kcp 消息？
	header, message, received := p.ReceiveNextReliable()
	if received {
		// 消息类型 FSM，没有默认值，所以我们永远不会错过一个案例
		switch header {
		case KcpHeaderHello:
			// 我们正在等待 Hello 消息
			// 它证明另一端说我们的协议
			p.State = KcpAuthenticated
			if p.Handler != nil {
				p.Handler.OnAuthenticated()
			}
		case KcpHeaderPing:
			// ping 保持 kcp 不超时，什么都不做
			// 安全：在认证前不要回复 pong 消息
			_ = message // 忽略未使用的变量
		case KcpHeaderPong:
			// ping 保持 kcp 不超时，什么都不做
			// 安全：在认证前不要处理 pong 消息
			_ = message // 忽略未使用的变量
		case KcpHeaderData:
			// 握手期间不允许其他任何内容！
			if p.Handler != nil {
				p.Handler.OnError(ErrorCodeInvalidReceive,
					"[KCP] Peer: received invalid header Data while Connected. Disconnecting the connection.")
			}
			p.Disconnect()
		}
	}
}

// TickIncoming_Authenticated 处理认证状态下的入站消息
func (p *KcpPeer) TickIncoming_Authenticated(time uint32) {
	// 检测常见事件和 ping
	p.HandleTimeout(time)
	p.HandleDeadLink()
	p.HandlePing(time)
	p.HandleChoked()

	// 处理所有收到的消息
	for {
		header, message, received := p.ReceiveNextReliable()
		if !received {
			break
		}

		// 消息类型 FSM，没有默认值，所以我们永远不会错过一个案例
		switch header {
		case KcpHeaderHello:
			// 认证后不应该再收到另一个 hello
			Log.Warning("[KCP] Peer: received invalid header hello while authenticated. disconnecting the connection.")
			p.Disconnect()
		case KcpHeaderData:
			// 如果消息包含实际数据，调用 OnData
			if len(message) > 0 {
				if p.Handler != nil {
					p.Handler.OnData(message, KcpReliable)
				}
			} else {
				// 空数据 = 攻击者，或出了问题
				if p.Handler != nil {
					p.Handler.OnError(ErrorCodeInvalidReceive,
						"[KCP] Peer:received empty Data message while Authenticated. Disconnecting the connection.")
				}
				p.Disconnect()
			}
		case KcpHeaderPing:
			// ping 包含发送者的本地时间用于 RTT 计算
			// 简单地将其发送回发送者
			// 为了安全，我们最多每 PING_INTERVAL 回复一次
			// 所以攻击者不能强迫我们每次都回复 PONG
			if len(message) == 4 {
				if time >= p.LastPongTime+PING_INTERVAL {
					if pingTimestamp, ok := Decode32U(message, 0); ok {
						p.SendPong(pingTimestamp)
						p.LastPongTime = time
					}
				}
			}
		case KcpHeaderPong:
			// ping 保持 kcp 不超时，用于 RTT 计算
			if len(message) == 4 {
				if originalTimestamp, ok := Decode32U(message, 0); ok {
					if time >= originalTimestamp {
						rtt := time - originalTimestamp
						p.rttInMilliseconds = rtt
					}
				}
			}
		}
	}
}

// TickOutgoing 处理出站消息的主要循环
func (p *KcpPeer) TickOutgoing() {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			if p.Handler != nil {
				p.Handler.OnError(ErrorCodeUnexpected,
					fmt.Sprintf("[KCP] Peer:TickOutgoing panic: %v\nStack trace:\n%s", r, string(stack)))
			}
			p.Disconnect()
		}
	}()

	switch p.State {
	case KcpConnected:
		// 检测死链接
		p.HandleDeadLink()
		// 在连接状态下也需要调用Update来发送握手消息
		if p.Kcp != nil {
			p.Kcp.Update()
		}
	case KcpAuthenticated:
		// 检测死链接
		p.HandleDeadLink()
		// 更新刷新出消息
		if p.Kcp != nil {
			p.Kcp.Update()
		}
	case KcpDisconnected:
		// 断开连接时什么都不做
	}
}
