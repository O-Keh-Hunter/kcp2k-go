# KCP2K-Go

一个高性能的KCP协议Go语言实现，与C# KCP2K完全兼容。

## 项目简介

KCP2K-Go是基于KCP协议的可靠UDP传输库的Go语言实现，专为游戏和实时应用设计。它提供了与Mirror Networking的KCP2K完全兼容的API，支持跨语言通信。

### 主要特性

- 🚀 **高性能**: 优化的KCP实现，低延迟高吞吐量
- 🔄 **跨语言兼容**: 与C# KCP2K完全兼容
- 🛡️ **可靠传输**: 基于UDP的可靠数据传输
- 📊 **性能监控**: 内置性能统计和监控
- 🧪 **全面测试**: 包含压力测试和跨语言兼容性测试
- ⚙️ **可配置**: 丰富的配置选项适应不同场景

## 快速开始

### 环境要求

- Go 1.19+
- .NET 8.0+ (用于跨语言测试)

### 安装

```bash
go get github.com/your-username/kcp2k-go
```

### 基本使用

#### 服务端

```go
package main

import (
    "fmt"
    "log"
    kcp "github.com/your-username/kcp2k-go"
)

func main() {
    config := kcp.DefaultConfig()
    server := kcp.NewServer(config)
    
    server.OnConnected = func(connectionId int) {
        fmt.Printf("Client %d connected\n", connectionId)
    }
    
    server.OnData = func(connectionId int, data []byte, channel kcp.KcpChannel) {
        fmt.Printf("Received from %d: %s\n", connectionId, string(data))
        // Echo back
        server.Send(connectionId, data, channel)
    }
    
    server.OnDisconnected = func(connectionId int) {
        fmt.Printf("Client %d disconnected\n", connectionId)
    }
    
    if err := server.Start(7777); err != nil {
        log.Fatal(err)
    }
    
    select {} // Keep running
}
```

#### 客户端

```go
package main

import (
    "fmt"
    "log"
    "time"
    kcp "github.com/your-username/kcp2k-go"
)

func main() {
    config := kcp.DefaultConfig()
    client := kcp.NewClient(config)
    
    client.OnConnected = func() {
        fmt.Println("Connected to server")
        client.Send([]byte("Hello Server!"), kcp.Reliable)
    }
    
    client.OnData = func(data []byte, channel kcp.KcpChannel) {
        fmt.Printf("Received: %s\n", string(data))
    }
    
    client.OnDisconnected = func() {
        fmt.Println("Disconnected from server")
    }
    
    if err := client.Connect("127.0.0.1", 7777); err != nil {
        log.Fatal(err)
    }
    
    time.Sleep(5 * time.Second)
    client.Disconnect()
}
```

## 配置选项

```go
config := &kcp.KcpConfig{
    DualMode:         false,
    RecvBufferSize:   1024 * 1024 * 7,  // 7MB
    SendBufferSize:   1024 * 1024 * 7,  // 7MB
    Timeout:          10000,             // 10秒
    Interval:         10,                // 10毫秒
    FastResend:       2,
    CongestionWindow: false,
    SendWindowSize:   4096,
    ReceiveWindowSize: 4096,
    Mtu:              1200,
}
```

## 测试

### 运行单元测试

```bash
go test ./...
```

### 跨语言兼容性测试

```bash
# 运行所有跨语言测试
./tools/scripts/testing/run_all_tests.sh

# 运行特定场景
./tools/scripts/testing/run_csharp_server_go_client.sh
./tools/scripts/testing/run_go_server_csharp_client.sh
```

### 压力测试

```bash
# 小规模压力测试
./tools/scripts/performance/test_small.sh

# 大规模压力测试
./tools/scripts/performance/test_full.sh
```

## 项目结构

```
kcp2k-go/
├── README.md                    # 项目说明
├── go.mod                       # Go模块定义
├── *.go                         # 核心实现文件
├── examples/                    # 示例代码
│   └── echo/                    # Echo服务器示例
├── tests/                       # 测试套件
│   ├── csharp_server_go_client/ # C#服务端+Go客户端测试
│   ├── go_server_csharp_client/ # Go服务端+C#客户端测试
│   ├── stress_client/           # 压力测试客户端
│   ├── stress_server/           # 压力测试服务端
│   └── stress_test/             # 压力测试主控程序
├── third_party/                 # 第三方依赖
│   └── kcp2k/                   # C# KCP2K实现
└── tools/                       # 工具脚本
    └── scripts/
        ├── testing/             # 测试脚本
        └── performance/         # 性能测试脚本
```

## 性能特性

- **低延迟**: 针对实时应用优化的传输延迟
- **高吞吐量**: 支持大量并发连接
- **内存优化**: 高效的内存使用和垃圾回收友好
- **可扩展**: 支持水平扩展的架构设计

## 兼容性

- ✅ 与C# KCP2K完全兼容
- ✅ 支持所有KCP2K的消息类型和配置
- ✅ 跨平台支持 (Windows, macOS, Linux)

## 贡献

欢迎提交Issue和Pull Request！

### 开发环境设置

1. 克隆仓库
```bash
git clone https://github.com/your-username/kcp2k-go.git
cd kcp2k-go
```

2. 初始化子模块
```bash
git submodule update --init --recursive
```

3. 运行测试
```bash
go test ./...
./tools/scripts/testing/run_all_tests.sh
```

## 许可证

MIT License - 详见 [LICENSE](LICENSE) 文件

## 相关项目

- [KCP2K](https://github.com/MirrorNetworking/kcp2k) - C# KCP实现
- [Mirror Networking](https://github.com/MirrorNetworking/Mirror) - Unity网络库
- [KCP](https://github.com/skywind3000/kcp) - 原始KCP协议实现