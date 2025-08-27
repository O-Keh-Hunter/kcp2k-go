package kcp2k

import (
	"testing"
)

func TestDefaultKcpConfig(t *testing.T) {
	config := DefaultKcpConfig()

	// 验证默认值
	if config.DualMode {
		t.Error("Default DualMode should be false")
	}

	if config.RecvBufferSize != 1024*1024*7 {
		t.Errorf("Default RecvBufferSize should be 7MB, got %d", config.RecvBufferSize)
	}

	if config.SendBufferSize != 1024*1024*7 {
		t.Errorf("Default SendBufferSize should be 7MB, got %d", config.SendBufferSize)
	}

	if config.Mtu != 1400 {
		t.Errorf("Default MTU should be 1400, got %d", config.Mtu)
	}

	if !config.NoDelay {
		t.Error("Default NoDelay should be true")
	}

	if config.Interval != 10 {
		t.Errorf("Default Interval should be 10, got %d", config.Interval)
	}

	if config.FastResend != 0 {
		t.Errorf("Default FastResend should be 0, got %d", config.FastResend)
	}

	if config.CongestionWindow {
		t.Error("Default CongestionWindow should be false")
	}

	if config.SendWindowSize != 32 {
		t.Errorf("Default SendWindowSize should be 32, got %d", config.SendWindowSize)
	}

	if config.ReceiveWindowSize != 32 {
		t.Errorf("Default ReceiveWindowSize should be 32, got %d", config.ReceiveWindowSize)
	}

	if config.Timeout != 10000 {
		t.Errorf("Default Timeout should be 10000, got %d", config.Timeout)
	}

	if config.MaxRetransmits != 20 {
		t.Errorf("Default MaxRetransmits should be 20, got %d", config.MaxRetransmits)
	}
}

func TestNewKcpConfig_WithOptions(t *testing.T) {
	// 测试单个选项
	config := NewKcpConfig(WithMtu(1200))
	if config.Mtu != 1200 {
		t.Errorf("Expected MTU 1200, got %d", config.Mtu)
	}

	// 测试多个选项
	config = NewKcpConfig(
		WithMtu(1500),
		WithSendWindowSize(64),
		WithReceiveWindowSize(256),
		WithNoDelay(false),
		WithInterval(20),
		WithFastResend(2),
		WithCongestionWindow(true),
		WithTimeout(5000),
		WithMaxRetransmits(20),
		WithRecvBufferSize(1024*1024*10),
		WithSendBufferSize(1024*1024*10),
		WithDualMode(false),
	)

	if config.Mtu != 1500 {
		t.Errorf("Expected MTU 1500, got %d", config.Mtu)
	}

	if config.SendWindowSize != 64 {
		t.Errorf("Expected SendWindowSize 64, got %d", config.SendWindowSize)
	}

	if config.ReceiveWindowSize != 256 {
		t.Errorf("Expected ReceiveWindowSize 256, got %d", config.ReceiveWindowSize)
	}

	if config.NoDelay {
		t.Error("Expected NoDelay false")
	}

	if config.Interval != 20 {
		t.Errorf("Expected Interval 20, got %d", config.Interval)
	}

	if config.FastResend != 2 {
		t.Errorf("Expected FastResend 2, got %d", config.FastResend)
	}

	if !config.CongestionWindow {
		t.Error("Expected CongestionWindow true")
	}

	if config.Timeout != 5000 {
		t.Errorf("Expected Timeout 5000, got %d", config.Timeout)
	}

	if config.MaxRetransmits != 20 {
		t.Errorf("Expected MaxRetransmits 20, got %d", config.MaxRetransmits)
	}

	if config.RecvBufferSize != 1024*1024*10 {
		t.Errorf("Expected RecvBufferSize 10MB, got %d", config.RecvBufferSize)
	}

	if config.SendBufferSize != 1024*1024*10 {
		t.Errorf("Expected SendBufferSize 10MB, got %d", config.SendBufferSize)
	}

	if config.DualMode {
		t.Error("Expected DualMode false")
	}
}

func TestNewKcpConfig_EmptyOptions(t *testing.T) {
	// 测试空选项应该返回默认配置
	config := NewKcpConfig()
	defaultConfig := DefaultKcpConfig()

	if config.Mtu != defaultConfig.Mtu {
		t.Errorf("Empty options should return default MTU %d, got %d", defaultConfig.Mtu, config.Mtu)
	}

	if config.SendWindowSize != defaultConfig.SendWindowSize {
		t.Errorf("Empty options should return default SendWindowSize %d, got %d", defaultConfig.SendWindowSize, config.SendWindowSize)
	}

	if config.ReceiveWindowSize != defaultConfig.ReceiveWindowSize {
		t.Errorf("Empty options should return default ReceiveWindowSize %d, got %d", defaultConfig.ReceiveWindowSize, config.ReceiveWindowSize)
	}
}
