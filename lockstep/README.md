# KCP2K-Go LockStep 帧同步服务

基于 kcp2k-go 和腾讯 GCloud 帧同步技术文档实现的高性能帧同步服务。

## 特性

- **高可靠性**: 基于 KCP 协议，提供可靠的 UDP 传输
- **低延迟**: 针对弱网环境优化，支持快速重传和拥塞控制
- **帧同步**: 实现严格的帧同步机制，保证游戏状态一致性
- **补帧机制**: 支持丢帧检测和自动补帧
- **断线重连**: 支持客户端断线重连和状态恢复
- **性能监控**: 提供延迟、丢包率等性能指标监控

## 架构设计

### 核心组件

1. **LockStepServer**: 帧同步服务器
   - 房间管理
   - 帧数据分发
   - 补帧处理
   - 玩家状态管理

2. **LockStepClient**: 帧同步客户端
   - 帧接收处理
   - 输入发送
   - 补帧请求
   - 断线重连

3. **Frame**: 帧数据结构
   - 帧ID管理
   - 玩家输入收集
   - 元数据记录

### 消息类型

- `MessageTypeFrame`: 帧数据广播
- `MessageTypeInput`: 玩家输入
- `MessageTypeFrameReq`: 补帧请求
- `MessageTypeFrameResp`: 补帧响应
- `MessageTypeReady`: 准备信号
- `MessageTypeStart`: 开始信号
- `MessageTypePing/Pong`: 延迟检测

## 快速开始

### 安装

```bash
go get github.com/O-Keh-Hunter/kcp2k-go/lockstep
```

### 服务器端

```go
package main

import (
    "github.com/O-Keh-Hunter/kcp2k-go/lockstep"
)

func main() {
    // 创建服务器配置
    config := lockstep.DefaultLockStepConfig()
    config.ServerPort = 8888
    
    // 创建服务器
    server := lockstep.NewLockStepServer(config)
    
    // 启动服务器
    err := server.Start("0.0.0.0:8888")
    if err != nil {
        panic(err)
    }
    
    // 创建房间
    roomConfig := lockstep.DefaultRoomConfig()
    roomConfig.MaxPlayers = 4
    roomConfig.FrameRate = 30
    
    _, err = server.CreateRoom("room1", roomConfig)
    if err != nil {
        panic(err)
    }
    
    // 等待...
    select {}
}
```

### 客户端

```go
package main

import (
    "github.com/O-Keh-Hunter/kcp2k-go/lockstep"
)

func main() {
    // 设置回调函数
    callbacks := lockstep.ClientCallbacks{
        OnFrameReceived: func(frame *lockstep.Frame) {
            // 处理接收到的帧数据
            fmt.Printf("Received frame %d\n", frame.ID)
        },
        OnGameStarted: func() {
            fmt.Println("Game started!")
        },
        OnError: func(err error) {
            fmt.Printf("Error: %v\n", err)
        },
    }
    
    // 创建客户端
    config := lockstep.DefaultLockStepConfig()
    client := lockstep.NewLockStepClient(config, 1, callbacks)
    
    // 连接服务器
    err := client.Connect("127.0.0.1", 8888)
    if err != nil {
        panic(err)
    }
    
    // 加入房间
    err = client.JoinRoom("room1")
    if err != nil {
        panic(err)
    }
    
    // 发送输入
    inputData := []byte("move_right")
    err = client.SendInput(client.GetCurrentFrameID()+1, inputData)
    if err != nil {
        panic(err)
    }
    
    // 等待...
    select {}
}
```

## 运行示例

### 启动服务器

```bash
cd lockstep/example
go run main.go server 8888
```

### 启动客户端

```bash
# 客户端1
go run main.go client 127.0.0.1 8888 1

# 客户端2
go run main.go client 127.0.0.1 8888 2
```

## 配置说明

### LockStepConfig

```go
type LockStepConfig struct {
    KcpConfig   kcp2k.KcpConfig // KCP协议配置
    RoomConfig  RoomConfig      // 房间配置
    ServerPort  uint16          // 服务器端口
    LogLevel    string          // 日志级别
    MetricsPort uint16          // 监控端口
}
```

### RoomConfig

```go
type RoomConfig struct {
    MaxPlayers      int           // 最大玩家数
    FrameRate       int           // 帧率（FPS）
    FrameInterval   time.Duration // 帧间隔
    RetryWindow     int           // 补帧窗口
}
```

## 技术特性

### 帧同步机制

1. **固定帧率**: 服务器以固定帧率（如30FPS）运行
2. **输入收集**: 每帧收集所有玩家的输入
3. **帧广播**: 将完整帧数据广播给所有客户端
4. **确定性**: 保证所有客户端接收到相同的帧序列

### 补帧机制

1. **丢帧检测**: 客户端检测帧序列中的缺失
2. **补帧请求**: 向服务器请求缺失的帧数据
3. **帧缓存**: 服务器维护帧数据缓存用于补帧
4. **重传机制**: 支持多次重传确保数据完整性

### 弱网优化

1. **KCP协议**: 基于UDP的可靠传输协议
2. **快速重传**: 检测到丢包时快速重传
3. **拥塞控制**: 根据网络状况调整发送速率
4. **冗余发送**: 关键数据支持冗余发送

### 性能监控

1. **延迟监控**: 通过Ping/Pong测量网络延迟
2. **丢包统计**: 统计数据包丢失率
3. **帧率监控**: 监控实际帧率和目标帧率差异
4. **连接状态**: 监控玩家连接状态和在线时长

## 最佳实践

### 服务器端

1. **房间管理**: 合理设置房间最大玩家数和帧率
2. **资源清理**: 及时清理空房间和离线玩家
3. **负载均衡**: 大规模部署时考虑负载均衡
4. **监控告警**: 设置性能监控和告警机制

### 客户端

1. **输入预测**: 实现客户端输入预测减少延迟感知
2. **状态回滚**: 在收到权威帧时进行状态回滚
3. **网络适配**: 根据网络状况调整发送频率
4. **错误处理**: 妥善处理网络错误和重连逻辑

### 游戏逻辑

1. **确定性**: 确保游戏逻辑完全确定性
2. **浮点数**: 避免使用浮点数运算
3. **随机数**: 使用固定种子的伪随机数生成器
4. **状态同步**: 定期进行状态校验和同步

## 性能指标

- **延迟**: < 100ms（局域网 < 10ms）
- **丢包率**: < 0.1%
- **帧率稳定性**: 99%以上帧按时到达
- **并发支持**: 单房间支持8-16玩家
- **吞吐量**: 支持1000+并发连接

## 故障排除

### 常见问题

1. **连接失败**: 检查网络连通性和端口开放
2. **帧丢失**: 检查网络质量和缓冲区设置
3. **延迟过高**: 优化网络路径和KCP参数
4. **状态不一致**: 检查游戏逻辑确定性

### 调试工具

1. **日志分析**: 启用详细日志进行问题定位
2. **性能监控**: 使用内置监控接口
3. **网络抓包**: 使用Wireshark等工具分析网络包
4. **压力测试**: 使用多客户端进行压力测试

## 许可证

MIT License