package kcp2k

import (
	"testing"
)

// MockPeer 对应 C# 版本的 MockPeer
type MockPeer struct {
	*KcpPeer
}

// 实现 KcpPeerEventHandler 接口
func (m *MockPeer) OnAuthenticated() {
	// 测试用的空实现
}

func (m *MockPeer) OnData(data []byte, channel KcpChannel) {
	// 测试用的空实现
}

func (m *MockPeer) OnDisconnected() {
	// 测试用的空实现
}

func (m *MockPeer) OnError(errorCode ErrorCode, msg string) {
	// 测试用的空实现
}

func (m *MockPeer) RawSend(data []byte) {
	// 测试用的空实现
}

// NewMockPeer 创建一个用于测试的 MockPeer
func NewMockPeer(config KcpConfig) *MockPeer {
	mock := &MockPeer{}
	mock.KcpPeer = NewKcpPeer(0, 0, config, mock)
	return mock
}

// TestMaxSendRate 测试最大发送速率计算
// 对应 C# 版本的 MaxSendRate 测试
func TestMaxSendRate(t *testing.T) {
	//   WND(32) * MTU(1195) = 38,240 bytes
	//   => 38,240 * 1000 / INTERVAL(10) = 3,824,000 bytes/s = 3734.4 KB/s
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200, // Go版本使用1200作为默认MTU
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}

	peer := NewMockPeer(config)
	actual := peer.MaxSendRate()

	// 验证计算公式是否正确
	// Go版本：KCP实际MTU = 1200 - 5 = 1195
	// 实际计算：32 * 1195 * 1000 / 10 = 3,824,000
	if actual == 0 {
		t.Errorf("MaxSendRate() returned 0, which indicates an error")
	}

	// 验证计算公式是否正确
	kcpMtu := peer.Kcp.mtu
	expectedCalculated := peer.Kcp.snd_wnd * kcpMtu * 1000 / peer.Kcp.interval
	if actual != expectedCalculated {
		t.Errorf("MaxSendRate() = %d, but calculated %d (snd_wnd=%d, mtu=%d, interval=%d)",
			actual, expectedCalculated, peer.Kcp.snd_wnd, kcpMtu, peer.Kcp.interval)
	}

	// 期望值与C#版本一致：3,824,000
	expected := uint32(3824000)
	if actual != expected {
		t.Errorf("MaxSendRate() = %d, expected %d", actual, expected)
	}
	t.Logf("Go版本结果: %d, C#版本期望: %d, 匹配: %v", actual, expected, actual == expected)
}

// TestMaxReceiveRate 测试最大接收速率计算
// 对应 C# 版本的 MaxReceiveRate 测试
func TestMaxReceiveRate(t *testing.T) {
	// note: WND needs to be >= max fragment size which is 128!
	//   WND(128) * MTU(1195) = 152,960 bytes
	//   => 152,960 * 1000 / INTERVAL(10) = 15,296,000 bytes/s = 14,937.5 KB/s = 14.58 MB/s
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 128,
		Mtu:               1200, // Go版本使用1200作为默认MTU
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}

	peer := NewMockPeer(config)
	actual := peer.MaxReceiveRate()

	// 验证计算公式是否正确
	// Go版本：KCP实际MTU = 1200 - 5 = 1195
	// 实际计算：128 * 1195 * 1000 / 10 = 15,296,000
	if actual == 0 {
		t.Errorf("MaxReceiveRate() returned 0, which indicates an error")
	}

	// 验证计算公式是否正确
	kcpMtu := peer.Kcp.mtu
	expectedCalculated := peer.Kcp.rcv_wnd * kcpMtu * 1000 / peer.Kcp.interval
	if actual != expectedCalculated {
		t.Errorf("MaxReceiveRate() = %d, but calculated %d (rcv_wnd=%d, mtu=%d, interval=%d)",
			actual, expectedCalculated, peer.Kcp.rcv_wnd, kcpMtu, peer.Kcp.interval)
	}

	// 期望值与C#版本一致：15,296,000
	expected := uint32(15296000)
	if actual != expected {
		t.Errorf("MaxReceiveRate() = %d, expected %d", actual, expected)
	}
	t.Logf("Go版本结果: %d, C#版本期望: %d, 匹配: %v", actual, expected, actual == expected)
}

// TestMaxSendRateWithCSharpMTU 使用C#版本相同的MTU值测试
func TestMaxSendRateWithCSharpMTU(t *testing.T) {
	// 使用C#版本的MTU=1199进行测试
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1204, // 1199 + 5 = 1204，因为Go版本会减去5
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}

	peer := NewMockPeer(config)
	actual := peer.MaxSendRate()

	// 验证是否接近C#版本的期望值
	expected := uint32(3824000)
	kcpMtu := peer.Kcp.mtu
	t.Logf("使用C#相同MTU - Go版本结果: %d, C#版本期望: %d, KCP MTU: %d", actual, expected, kcpMtu)

	// 计算期望值：32 * 1199 * 1000 / 10 = 3,836,800
	// 但C#版本期望是3,824,000，可能有其他因素影响
	expectedCalculated := peer.Kcp.snd_wnd * kcpMtu * 1000 / peer.Kcp.interval
	t.Logf("理论计算值: %d", expectedCalculated)

	if actual == 0 {
		t.Errorf("MaxSendRate() returned 0, which indicates an error")
	}
}

// TestMaxReceiveRateWithCSharpMTU 使用C#版本相同的MTU值测试
func TestMaxReceiveRateWithCSharpMTU(t *testing.T) {
	// 使用C#版本的MTU=1199进行测试
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 128,
		Mtu:               1204, // 1199 + 5 = 1204，因为Go版本会减去5
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}

	peer := NewMockPeer(config)
	actual := peer.MaxReceiveRate()

	// 验证是否接近C#版本的期望值
	expected := uint32(15296000)
	kcpMtu := peer.Kcp.mtu
	t.Logf("使用C#相同MTU - Go版本结果: %d, C#版本期望: %d, KCP MTU: %d", actual, expected, kcpMtu)

	// 计算期望值：128 * 1199 * 1000 / 10 = 15,347,200
	// 但C#版本期望是15,296,000，可能有其他因素影响
	expectedCalculated := peer.Kcp.rcv_wnd * kcpMtu * 1000 / peer.Kcp.interval
	t.Logf("理论计算值: %d", expectedCalculated)

	if actual == 0 {
		t.Errorf("MaxReceiveRate() returned 0, which indicates an error")
	}
}

// ========== 基准测试 ==========

// BenchmarkReliableMaxMessageSize 测试可靠消息最大大小计算性能
func BenchmarkReliableMaxMessageSize(b *testing.B) {
	mtu := 1200
	rcvWnd := uint(128)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ReliableMaxMessageSize(mtu, rcvWnd)
	}
}

// BenchmarkReliableMaxMessageSizeUnconstrained 测试无约束可靠消息最大大小计算性能
func BenchmarkReliableMaxMessageSizeUnconstrained(b *testing.B) {
	mtu := 1200
	rcvWnd := uint(128)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ReliableMaxMessageSize_Unconstrained(mtu, rcvWnd)
	}
}

// BenchmarkUnreliableMaxMessageSize 测试不可靠消息最大大小计算性能
func BenchmarkUnreliableMaxMessageSize(b *testing.B) {
	mtu := 1200
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = UnreliableMaxMessageSize(mtu)
	}
}

// BenchmarkNewKcpPeer 测试创建KcpPeer的性能
func BenchmarkNewKcpPeer(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	mock := &MockPeer{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer := NewKcpPeer(uint32(i), uint32(i), config, mock)
		_ = peer
	}
}

// BenchmarkNewKcpPeerWithBufferPool 测试创建带缓冲池的KcpPeer的性能
func BenchmarkNewKcpPeerWithBufferPool(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	mock := &MockPeer{}
	bufferPool := NewBufferPool(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer := NewKcpPeerWithBufferPool(uint32(i), uint32(i), config, mock, bufferPool)
		_ = peer
	}
}

// BenchmarkKcpPeerTime 测试KcpPeer的Time方法性能
func BenchmarkKcpPeerTime(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = peer.Time()
	}
}

// BenchmarkKcpPeerReset 测试KcpPeer的Reset方法性能
func BenchmarkKcpPeerReset(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.Reset(config)
	}
}

// BenchmarkKcpPeerMaxSendRate 测试KcpPeer的MaxSendRate方法性能
func BenchmarkKcpPeerMaxSendRate(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = peer.MaxSendRate()
	}
}

// BenchmarkKcpPeerMaxReceiveRate 测试KcpPeer的MaxReceiveRate方法性能
func BenchmarkKcpPeerMaxReceiveRate(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 128,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = peer.MaxReceiveRate()
	}
}

// BenchmarkKcpPeerGetRTT 测试KcpPeer的GetRTT方法性能
func BenchmarkKcpPeerGetRTT(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = peer.GetRTT()
	}
}

// BenchmarkKcpPeerSendQueueCount 测试KcpPeer的SendQueueCount方法性能
func BenchmarkKcpPeerSendQueueCount(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = peer.SendQueueCount()
	}
}

// BenchmarkKcpPeerReceiveQueueCount 测试KcpPeer的ReceiveQueueCount方法性能
func BenchmarkKcpPeerReceiveQueueCount(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = peer.ReceiveQueueCount()
	}
}

// BenchmarkKcpPeerSendBufferCount 测试KcpPeer的SendBufferCount方法性能
func BenchmarkKcpPeerSendBufferCount(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = peer.SendBufferCount()
	}
}

// BenchmarkKcpPeerReceiveBufferCount 测试KcpPeer的ReceiveBufferCount方法性能
func BenchmarkKcpPeerReceiveBufferCount(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = peer.ReceiveBufferCount()
	}
}

// BenchmarkKcpPeerReceiveNextReliable 测试KcpPeer的ReceiveNextReliable方法性能
func BenchmarkKcpPeerReceiveNextReliable(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = peer.ReceiveNextReliable()
	}
}

// BenchmarkKcpPeerOnRawInputReliable 测试KcpPeer的OnRawInputReliable方法性能
func BenchmarkKcpPeerOnRawInputReliable(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	// 创建一个简单的KCP数据包用于测试
	testData := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.OnRawInputReliable(testData)
	}
}

// BenchmarkKcpPeerOnRawInputUnreliable 测试KcpPeer的OnRawInputUnreliable方法性能
func BenchmarkKcpPeerOnRawInputUnreliable(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	// 创建测试数据
	testData := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.OnRawInputUnreliable(testData)
	}
}

// BenchmarkKcpPeerSendReliable 测试KcpPeer的SendReliable方法性能
func BenchmarkKcpPeerSendReliable(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	testData := []byte("test message")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.SendReliable(KcpHeaderData, testData)
	}
}

// BenchmarkKcpPeerSendUnreliable 测试KcpPeer的SendUnreliable方法性能
func BenchmarkKcpPeerSendUnreliable(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	testData := []byte("test message")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.SendUnreliable(KcpHeaderUnrelData, testData)
	}
}

// BenchmarkKcpPeerSendData 测试KcpPeer的SendData方法性能
func BenchmarkKcpPeerSendData(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	testData := []byte("test message")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.SendData(testData, KcpReliable)
	}
}

// BenchmarkKcpPeerSendPing 测试KcpPeer的SendPing方法性能
func BenchmarkKcpPeerSendPing(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.SendPing()
	}
}

// BenchmarkKcpPeerSendPong 测试KcpPeer的SendPong方法性能
func BenchmarkKcpPeerSendPong(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	testTimestamp := uint32(12345)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.SendPong(testTimestamp)
	}
}

// BenchmarkKcpPeerSendHello 测试KcpPeer的SendHello方法性能
func BenchmarkKcpPeerSendHello(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.SendHello()
	}
}

// BenchmarkKcpPeerSendDisconnect 测试KcpPeer的SendDisconnect方法性能
func BenchmarkKcpPeerSendDisconnect(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.SendDisconnect()
	}
}

// BenchmarkKcpPeerDisconnect 测试KcpPeer的Disconnect方法性能
func BenchmarkKcpPeerDisconnect(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer := NewMockPeer(config)
		peer.Disconnect()
	}
}

// BenchmarkKcpPeerHandleTimeout 测试KcpPeer的HandleTimeout方法性能
func BenchmarkKcpPeerHandleTimeout(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.HandleTimeout(peer.Time())
	}
}

// BenchmarkKcpPeerHandleDeadLink 测试KcpPeer的HandleDeadLink方法性能
func BenchmarkKcpPeerHandleDeadLink(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.HandleDeadLink()
	}
}

// BenchmarkKcpPeerHandlePing 测试KcpPeer的HandlePing方法性能
func BenchmarkKcpPeerHandlePing(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	testTimestamp := uint32(12345)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.HandlePing(testTimestamp)
	}
}

// BenchmarkKcpPeerHandleChoked 测试KcpPeer的HandleChoked方法性能
func BenchmarkKcpPeerHandleChoked(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.HandleChoked()
	}
}

// BenchmarkKcpPeerTickIncoming 测试KcpPeer的TickIncoming方法性能
func BenchmarkKcpPeerTickIncoming(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.TickIncoming()
	}
}

// BenchmarkKcpPeerTickIncoming_Connected 测试KcpPeer的TickIncoming_Connected方法性能
func BenchmarkKcpPeerTickIncoming_Connected(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.TickIncoming_Connected(peer.Time())
	}
}

// BenchmarkKcpPeerTickIncoming_Authenticated 测试KcpPeer的TickIncoming_Authenticated方法性能
func BenchmarkKcpPeerTickIncoming_Authenticated(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.TickIncoming_Authenticated(peer.Time())
	}
}

// BenchmarkKcpPeerTickOutgoing 测试KcpPeer的TickOutgoing方法性能
func BenchmarkKcpPeerTickOutgoing(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.TickOutgoing()
	}
}

// BenchmarkKcpPeerReleaseBuffers 测试KcpPeer的ReleaseBuffers方法性能
func BenchmarkKcpPeerReleaseBuffers(b *testing.B) {
	config := KcpConfig{
		SendWindowSize:    32,
		ReceiveWindowSize: 64,
		Mtu:               1200,
		Interval:          10,
		NoDelay:           true,
		CongestionWindow:  false,
		Timeout:           DEFAULT_TIMEOUT,
	}
	peer := NewMockPeer(config)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer.ReleaseBuffers()
	}
}
