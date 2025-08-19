package main

import (
	"fmt"
	"io/ioutil"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 压测配置结构体
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Client  ClientConfig  `yaml:"client"`
	Test    TestConfig    `yaml:"test"`
	Network NetworkConfig `yaml:"network"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Address        string `yaml:"address"`
	Port           uint16 `yaml:"port"`
	MetricsPort    uint16 `yaml:"metrics_port"`
	Rooms          int    `yaml:"rooms"`
	PlayersPerRoom int    `yaml:"players_per_room"`
	FrameRate      int    `yaml:"frame_rate"`
}

// ClientConfig 客户端配置
type ClientConfig struct {
	TotalClients      int `yaml:"total_clients"`
	InputInterval     int `yaml:"input_interval"`     // ms
	InputSize         int `yaml:"input_size"`         // bytes
	ConnectTimeout    int `yaml:"connect_timeout"`    // seconds
	ReconnectInterval int `yaml:"reconnect_interval"` // seconds
	MaxReconnects     int `yaml:"max_reconnects"`
}

// TestConfig 测试配置
type TestConfig struct {
	Duration string `yaml:"duration"`
	Warmup   string `yaml:"warmup"`
	Cooldown string `yaml:"cooldown"`
}

// NetworkConfig 网络配置
type NetworkConfig struct {
	KCP KCPConfig `yaml:"kcp"`
}

// KCPConfig KCP协议配置
type KCPConfig struct {
	Nodelay  int `yaml:"nodelay"`
	Interval int `yaml:"interval"`
	Resend   int `yaml:"resend"`
	NC       int `yaml:"nc"`
	Sndwnd   int `yaml:"sndwnd"`
	Rcvwnd   int `yaml:"rcvwnd"`
	MTU      int `yaml:"mtu"`
}

// LoadConfigFromFile 从YAML文件加载配置
func LoadConfigFromFile(filename string) (*Config, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %v", filename, err)
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %v", filename, err)
	}

	return &config, nil
}

// GetTestDuration 获取测试持续时间
func (c *Config) GetTestDuration() (time.Duration, error) {
	return time.ParseDuration(c.Test.Duration)
}

// GetWarmupDuration 获取预热时间
func (c *Config) GetWarmupDuration() (time.Duration, error) {
	return time.ParseDuration(c.Test.Warmup)
}

// GetCooldownDuration 获取冷却时间
func (c *Config) GetCooldownDuration() (time.Duration, error) {
	return time.ParseDuration(c.Test.Cooldown)
}

// GetInputInterval 获取输入间隔
func (c *Config) GetInputInterval() time.Duration {
	return time.Duration(c.Client.InputInterval) * time.Millisecond
}

// Validate 验证配置的有效性
func (c *Config) Validate() error {
	if c.Server.Port == 0 {
		return fmt.Errorf("server port cannot be 0")
	}
	if c.Server.MetricsPort == 0 {
		return fmt.Errorf("metrics port cannot be 0")
	}
	if c.Server.Port == c.Server.MetricsPort {
		return fmt.Errorf("server port and metrics port cannot be the same")
	}
	if c.Server.Rooms <= 0 {
		return fmt.Errorf("room count must be positive")
	}
	if c.Server.PlayersPerRoom <= 0 {
		return fmt.Errorf("players per room must be positive")
	}
	if c.Client.TotalClients <= 0 {
		return fmt.Errorf("total clients must be positive")
	}
	if c.Client.InputInterval <= 0 {
		return fmt.Errorf("input interval must be positive")
	}
	if c.Client.InputSize <= 0 {
		return fmt.Errorf("input size must be positive")
	}

	// 验证时间格式
	if _, err := c.GetTestDuration(); err != nil {
		return fmt.Errorf("invalid test duration: %v", err)
	}
	if _, err := c.GetWarmupDuration(); err != nil {
		return fmt.Errorf("invalid warmup duration: %v", err)
	}
	if _, err := c.GetCooldownDuration(); err != nil {
		return fmt.Errorf("invalid cooldown duration: %v", err)
	}

	return nil
}
