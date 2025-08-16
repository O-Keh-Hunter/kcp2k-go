# 生产环境部署指南

本文档介绍如何在生产环境中部署和配置 LockStep 服务器。

## 配置验证

在启动服务器之前，使用 `ValidateConfig()` 方法验证配置：

```go
config := lockstep.DefaultLockStepConfig()
// 修改配置...

if err := config.ValidateConfig(); err != nil {
    log.Fatalf("Invalid configuration: %v", err)
}
```

## 环境变量配置

推荐使用环境变量来配置生产环境参数：

- `LOCKSTEP_PORT`: 服务器端口 (默认: 8888)
- `METRICS_PORT`: 监控端口 (默认: 9090)
- `LOG_LEVEL`: 日志级别 (debug/info/warn/error/fatal)
- `MAX_PLAYERS`: 房间最大玩家数
- `FRAME_RATE`: 帧率设置

## 优雅关闭

服务器支持优雅关闭，会在收到 SIGINT 或 SIGTERM 信号时：

1. 停止接受新连接
2. 通知所有客户端服务器即将关闭
3. 等待2秒让客户端处理关闭消息
4. 停止所有房间
5. 在30秒超时内完成关闭

```go
server := lockstep.NewLockStepServer(&config)
server.Start()

// 等待关闭信号
server.WaitForShutdown()
```

## 健康检查

服务器提供健康检查接口，返回服务器状态：

```go
health := server.HealthCheck()
// 返回包含以下信息的 map:
// - status: "healthy", "degraded", "unhealthy", "idle"
// - running: 服务器是否运行中
// - uptime_seconds: 运行时间
// - rooms: 房间数量
// - players: 玩家数量
// - packet_loss_rate: 丢包率百分比
// - memory_usage_mb: 内存使用量(MB)
// - timestamp: 时间戳
```

健康状态判断规则：
- `healthy`: 服务器正常运行，丢包率 ≤ 10%
- `degraded`: 服务器运行但丢包率 > 10%
- `idle`: 运行超过5分钟但没有活跃房间
- `unhealthy`: 服务器未运行

## 监控接口

### HTTP 监控端点

可以创建 HTTP 服务器提供监控接口：

```go
// 健康检查 - GET /health
// 返回健康状态，HTTP状态码根据健康状态设置

// 详细统计 - GET /metrics  
// 返回完整的服务器统计信息

// 房间信息 - GET /rooms
// 返回所有房间的基本信息
```

### 统计信息

`GetServerStats()` 方法返回详细的性能统计：

- **运行时间**: 服务器启动时长
- **帧统计**: 总帧数、丢失帧数、延迟帧数、平均帧时间
- **网络统计**: 总包数、丢包数、平均/最大/最小延迟

## 性能监控

服务器自动收集以下性能指标：

### 帧统计
- 总处理帧数
- 丢失帧数（玩家输入缺失）
- 延迟帧数（处理时间超过预期）
- 平均帧处理时间

### 网络统计
- 总收发包数
- 丢包数（包括解析失败、连接错误、断开连接）
- 网络延迟统计（平均、最大、最小）

## 日志配置

支持以下日志级别：
- `debug`: 详细调试信息
- `info`: 一般信息（默认）
- `warn`: 警告信息
- `error`: 错误信息
- `fatal`: 致命错误

## 部署建议

### 资源配置
- **CPU**: 建议至少2核，高并发场景建议4核以上
- **内存**: 建议至少1GB，根据房间数量和玩家数量调整
- **网络**: 确保足够的带宽和低延迟

### 安全配置
- 使用防火墙限制访问端口
- 监控端口仅对内网开放
- 定期更新和安全补丁

### 监控告警
建议监控以下指标并设置告警：
- 丢包率 > 5%
- 平均延迟 > 100ms
- 内存使用率 > 80%
- 服务器状态为 `degraded` 或 `unhealthy`

### 负载均衡
对于高并发场景，可以部署多个服务器实例：
- 使用不同端口运行多个实例
- 通过负载均衡器分发连接
- 考虑房间亲和性，避免跨服务器通信

## 故障排查

### 常见问题

1. **配置验证失败**
   - 检查端口是否冲突
   - 验证数值范围是否合理
   - 确认日志级别拼写正确

2. **连接问题**
   - 检查防火墙设置
   - 验证端口是否被占用
   - 确认网络连通性

3. **性能问题**
   - 监控丢包率和延迟
   - 检查内存使用情况
   - 分析帧处理时间

### 日志分析
关注以下日志信息：
- 连接建立和断开
- 输入超时警告
- 帧处理异常
- 网络错误

## 示例配置

```go
config := lockstep.LockStepConfig{
    ServerPort:  8888,
    MetricsPort: 9090,
    LogLevel:    "info",
    RoomConfig: &lockstep.RoomConfig{
        MaxPlayers:       8,
        MinPlayers:       2,
        FrameRate:        30,
        RetryWindow:      50,
        EnableReconnect:  true,
        ReconnectTimeout: 10000, // 10秒
        InputTimeout:     5000,  // 5秒
    },
}
```