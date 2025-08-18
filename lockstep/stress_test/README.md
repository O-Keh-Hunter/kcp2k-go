# Lockstep 压测系统

基于 KCP 协议的帧同步网络游戏框架压力测试系统。

## 系统概述

本压测系统专门为 lockstep 模块设计，支持以下测试场景：

- **正常负载测试**: 5000个客户端，500个房间，每房间10个玩家
- **峰值负载测试**: 超负荷运行，测试系统极限
- **断线重连测试**: 模拟网络不稳定环境下的重连机制
- **网络抖动测试**: 模拟各种网络条件下的性能表现

## 技术规格

### 网络参数
- **客户端上行**: 33ms间隔 (30帧/秒)
- **服务端下行**: 66ms间隔 (15帧/秒)
- **Input包大小**: 40字节
- **房间配置**: 500个房间，每房间10个玩家
- **总连接数**: 5000个客户端连接

### 系统架构
- **多协程并发**: 支持大量并发连接
- **内存池优化**: 减少GC压力
- **性能监控**: 实时系统指标收集
- **自动化报告**: 详细的性能分析报告

## 目录结构

```
stress_test/
├── server/                 # 压测服务器
│   └── main.go            # 服务器主程序
├── client/                 # 压测客户端
│   └── main.go            # 客户端主程序
├── scripts/                # 脚本和工具
│   ├── run_scenario.sh    # 测试场景脚本
│   ├── monitor.go         # 性能监控器
│   └── report_generator.go # 报告生成器
├── configs/                # 配置文件
│   ├── default.yaml       # 默认配置
│   ├── peak.yaml          # 峰值测试配置
│   ├── reconnect.yaml     # 重连测试配置
│   └── network_jitter.yaml # 网络抖动配置
├── results/                # 测试结果
│   ├── logs/              # 日志文件
│   └── reports/           # 测试报告
├── Makefile               # 构建和运行脚本
└── README.md              # 本文档
```

## 快速开始

### 1. 环境要求

- Go 1.19+
- macOS/Linux 系统
- 至少 8GB 内存
- 网络带宽 > 100Mbps

### 2. 构建系统

```bash
# 构建所有组件
make build

# 或者单独构建
make build-server  # 只构建服务器
make build-client  # 只构建客户端
```

### 3. 运行测试

#### 正常负载测试
```bash
make run-normal
```

#### 峰值负载测试
```bash
make run-peak
```

#### 断线重连测试
```bash
make run-reconnect
```

#### 网络抖动测试
```bash
make run-jitter
```

### 4. 自定义测试

```bash
# 自定义参数运行
make run-custom SCENARIO=normal ARGS='-d 15m -r 100'

# 直接使用脚本
cd scripts
./run_scenario.sh normal -d 10m -r 500 -c 10
```

## 配置说明

### YAML配置文件使用

系统支持通过YAML配置文件进行参数配置，配置文件位于 `configs/` 目录下：

- `default.yaml` - 默认配置，用于正常负载测试
- `peak.yaml` - 峰值测试配置
- `reconnect.yaml` - 重连测试配置
- `network_jitter.yaml` - 网络抖动测试配置

#### 使用配置文件

**服务器端**:
```bash
# 使用指定配置文件启动服务器
./stress_server -config=../configs/default.yaml

# 配置文件参数会覆盖默认值，命令行参数会覆盖配置文件
./stress_server -config=../configs/default.yaml -port=9999
```

**客户端**:
```bash
# 使用指定配置文件启动客户端
./stress_client -config=../configs/default.yaml

# 组合使用配置文件和命令行参数
./stress_client -config=../configs/peak.yaml -clients=6000
```

**脚本自动选择**:
`run_scenario.sh` 脚本会根据测试场景自动选择对应的配置文件：
```bash
./run_scenario.sh normal    # 使用 default.yaml
./run_scenario.sh peak      # 使用 peak.yaml
./run_scenario.sh reconnect # 使用 reconnect.yaml
./run_scenario.sh network_jitter # 使用 network_jitter.yaml
```

### 配置文件格式

#### 服务器配置

```yaml
server:
  address: "127.0.0.1"     # 服务器地址
  port: 8888               # 服务器端口
  rooms: 500               # 房间数量
  players_per_room: 10     # 每房间玩家数
  frame_rate: 15           # 帧率 (Hz)
  metrics_port: 8887       # 监控端口
```

#### 客户端配置

```yaml
client:
  total_clients: 5000      # 总客户端数
  input_interval: 33       # 输入间隔 (ms)
  input_size: 40           # 输入包大小 (字节)
  connect_timeout: 10      # 连接超时 (秒)
  max_reconnects: 3        # 最大重连次数
  reconnect_rate: 0.01     # 重连率
```

#### 测试配置

```yaml
test:
  duration: "5m"           # 测试时长
  warmup: "30s"            # 预热时间
  cooldown: "10s"          # 冷却时间
```

#### 监控配置

```yaml
monitoring:
  enabled: true            # 是否启用监控
  interval: "1s"           # 监控间隔
  metrics_port: 8887       # 监控端口
```

## 监控和分析

### 实时监控

```bash
# 查看系统资源
make monitor

# 查看服务器日志
make logs-server

# 查看客户端日志
make logs-client
```

### 性能指标

系统会自动收集以下指标：

- **系统指标**: CPU使用率、内存使用率、协程数量
- **网络指标**: 连接数、延迟、吞吐量、丢包率
- **游戏指标**: 帧率、帧处理时间、成功率、重连率

### 报告生成

测试完成后会自动生成详细报告：

```bash
# 查看最新报告
make show-report

# 手动生成报告
cd scripts
go run report_generator.go ../results/logs ../results/reports test_name markdown true
```

## 测试场景详解

### 1. 正常负载测试

**目标**: 验证系统在设计负载下的稳定性

**参数**:
- 5000个客户端
- 500个房间
- 每房间10个玩家
- 运行5分钟

**通过标准**:
- CPU使用率 < 80%
- 内存使用率 < 85%
- 连接成功率 > 99%
- 平均延迟 < 50ms

### 2. 峰值负载测试

**目标**: 找到系统性能极限

**参数**:
- 6000个客户端 (超负荷20%)
- 更高的帧率和更大的包
- 运行10分钟

**观察指标**:
- 系统崩溃点
- 性能下降拐点
- 资源瓶颈识别

### 3. 断线重连测试

**目标**: 验证网络不稳定环境下的稳定性

**参数**:
- 2000个客户端
- 5%的断线率
- 每30秒触发一次断线
- 运行8分钟

**通过标准**:
- 重连成功率 > 95%
- 数据一致性保持
- 无内存泄漏

### 4. 网络抖动测试

**目标**: 模拟真实网络环境

**参数**:
- 3000个客户端
- 模拟WiFi/4G/拥塞网络
- 延迟抖动 ±50ms
- 运行6分钟

**通过标准**:
- 帧同步准确性 > 98%
- 延迟补偿有效
- 用户体验可接受

## 性能优化建议

### 系统级优化

1. **增加文件描述符限制**:
   ```bash
   ulimit -n 65536
   ```

2. **调整网络参数**:
   ```bash
   sudo sysctl -w net.core.rmem_max=134217728
   sudo sysctl -w net.core.wmem_max=134217728
   ```

3. **设置CPU亲和性**:
   ```bash
   taskset -c 0-7 ./stress_server
   ```

### 应用级优化

1. **内存池使用**: 减少GC压力
2. **协程池管理**: 控制协程数量
3. **批量处理**: 减少系统调用
4. **零拷贝**: 优化数据传输

## 故障排除

### 常见问题

1. **连接失败**
   - 检查端口是否被占用
   - 确认防火墙设置
   - 验证网络连通性

2. **性能下降**
   - 监控系统资源使用
   - 检查网络带宽
   - 分析GC频率

3. **内存泄漏**
   - 使用pprof分析
   - 检查协程泄漏
   - 验证资源释放

### 调试工具

```bash
# 性能分析
make profile-server
make profile-client

# 内存分析
go tool pprof http://localhost:6060/debug/pprof/heap

# CPU分析
go tool pprof http://localhost:6060/debug/pprof/profile
```

## 扩展开发

### 添加新的测试场景

1. 在 `configs/` 目录添加配置文件
2. 在 `run_scenario.sh` 中添加场景处理
3. 更新 `Makefile` 添加快捷命令

### 自定义监控指标

1. 修改 `monitor.go` 添加新指标
2. 更新 `report_generator.go` 处理新数据
3. 调整报告模板显示新指标

### 集成CI/CD

```yaml
# .github/workflows/stress-test.yml
name: Stress Test
on: [push, pull_request]
jobs:
  stress-test:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v2
    - uses: actions/setup-go@v2
      with:
        go-version: 1.19
    - name: Run Stress Test
      run: |
        cd lockstep/stress_test
        make benchmark
```

## 许可证

本项目遵循与主项目相同的许可证。

## 贡献指南

1. Fork 项目
2. 创建特性分支
3. 提交更改
4. 推送到分支
5. 创建 Pull Request

## 联系方式

如有问题或建议，请提交 Issue 或联系项目维护者。