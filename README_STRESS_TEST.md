# 压力测试系统

这是一个基于KCP协议的高性能压力测试系统，支持大规模并发连接和数据包传输测试。

## 系统架构

- **压力测试服务器** (`cmd/stress_server/`): 启动多个KCP服务器实例
- **压力测试客户端** (`cmd/stress_client/`): 连接到服务器并以指定FPS发送数据包
- **主控制程序** (`cmd/stress_test/`): 协调整个测试过程

## 功能特性

- 支持1000个服务器实例
- 每个服务器支持10个客户端连接
- 每个客户端以30FPS速率发送数据包
- 实时统计连接数、数据包发送/接收情况
- 自动重连机制
- 优雅关闭和资源清理

## 快速开始

### 1. 小规模测试（推荐先运行）

```bash
# 运行小规模测试：10个服务器，每个5个客户端，60秒
./scripts/test_small.sh
```

### 2. 完整规模测试

```bash
# 运行完整规模测试：1000个服务器，每个10个客户端
./scripts/test_full.sh
```

### 3. 手动运行

```bash
# 构建所有组件
go build -o cmd/stress_server/stress_server ./cmd/stress_server
go build -o cmd/stress_client/stress_client ./cmd/stress_client
go build -o cmd/stress_test/stress_test ./cmd/stress_test

# 运行测试
./cmd/stress_test/stress_test \
    -servers 1000 \
    -clients-per-server 10 \
    -fps 30 \
    -start-port 10000
```

## 参数说明

### 主控制程序参数

- `-servers`: 服务器数量 (默认: 1000)
- `-clients-per-server`: 每个服务器的客户端数量 (默认: 10)
- `-fps`: 每个客户端的帧率 (默认: 30)
- `-start-port`: 服务器起始端口 (默认: 10000)
- `-host`: 服务器主机地址 (默认: localhost)
- `-duration`: 测试持续时间 (默认: 0，表示无限运行)

### 服务器参数

- `-servers`: 服务器数量
- `-start-port`: 起始端口
- `-report`: 统计报告间隔 (默认: 5s)

### 客户端参数

- `-host`: 服务器主机地址
- `-servers`: 服务器数量
- `-clients-per-server`: 每个服务器的客户端数量
- `-fps`: 帧率
- `-start-port`: 服务器起始端口
- `-report`: 统计报告间隔

## 性能指标

### 预期负载

- **总连接数**: 1000 × 10 = 10,000 个连接
- **总数据包/秒**: 10,000 × 30 = 300,000 个数据包/秒
- **端口范围**: 10000-10999

### 监控指标

- 连接数统计
- 数据包发送/接收统计
- 字节传输统计
- 平均延迟
- 系统资源使用情况

## 系统要求

### 硬件要求

- **CPU**: 建议8核心以上
- **内存**: 建议16GB以上
- **网络**: 千兆网络连接

### 软件要求

- Go 1.24.6+
- 支持KCP协议的网络环境

## 故障排除

### 常见问题

1. **端口被占用**
   ```bash
   # 检查端口使用情况
   lsof -i :10000-10999
   
   # 使用不同的起始端口
   ./cmd/stress_test/stress_test -start-port 20000
   ```

2. **内存不足**
   ```bash
   # 减少服务器数量进行测试
   ./cmd/stress_test/stress_test -servers 100 -clients-per-server 5
   ```

3. **连接失败**
   ```bash
   # 检查防火墙设置
   # 确保端口范围开放
   ```

### 性能调优

1. **增加系统文件描述符限制**
   ```bash
   # Linux
   ulimit -n 65536
   
   # macOS
   sudo launchctl limit maxfiles 65536 200000
   ```

2. **调整网络参数**
   ```bash
   # Linux
   echo 65536 > /proc/sys/net/core/somaxconn
   ```

## 日志分析

系统会输出详细的日志信息，包括：

- 服务器启动和连接信息
- 客户端连接和断开信息
- 定期统计报告
- 错误和异常信息

### 日志示例

```
=== STRESS TEST CONFIGURATION ===
Servers: 1000
Clients per server: 10
Total connections: 10000
FPS per client: 30
Total packets/second: 300000
Server ports: 10000-10999
Test duration: 0s
================================

=== STATS REPORT ===
Total Connections: 10000
Total Packets Sent: 1500000
Total Packets Received: 1500000
Total Bytes Sent: 45000000
Total Bytes Received: 45000000
===================
```

## 扩展功能

### 自定义数据包格式

可以修改客户端代码中的数据包格式：

```go
// 在 cmd/stress_client/main.go 中修改
packet := fmt.Sprintf("CUSTOM_PACKET_%d_%d_%d_%s", 
    c.ID, c.ServerID, packetID, time.Now().Format("15:04:05.000"))
```

### 添加更多监控指标

可以在统计结构中添加更多指标：

```go
type ServerStats struct {
    // 现有字段...
    AverageLatency    time.Duration
    PacketLossRate    float64
    ConnectionErrors  int64
}
```

## 许可证

本项目使用与主项目相同的许可证。 