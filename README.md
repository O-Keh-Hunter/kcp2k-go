# KCP2K-Go

[English](README.md) | [中文](README_CN.md)

A Go implementation of https://github.com/MirrorNetworking/kcp2k, fully compatible with C# KCP2K.

## Project Overview

KCP2K-Go is a Go implementation of https://github.com/MirrorNetworking/kcp2k, built on top of github.com/xtaci/kcp-go, providing complete KCP2K functionality. Designed for games and real-time applications, it offers a fully compatible API with Mirror Networking's KCP2K and supports cross-language communication.

### Key Features

- 🚀 **High Performance**: Optimized KCP implementation with low latency and high throughput
- 🔄 **Cross-Language Compatible**: Fully compatible with C# KCP2K
- 🛡️ **Reliable Transport**: UDP-based reliable data transmission
- 📊 **Performance Monitoring**: Built-in performance statistics and monitoring
- 🧪 **Comprehensive Testing**: Includes stress testing and cross-language compatibility tests
- ⚙️ **Configurable**: Rich configuration options for different scenarios

## Quick Start

### Requirements

- Go 1.19+
- .NET 8.0+ (for cross-language testing)

### Installation

```bash
go get https://github.com/O-Keh-Hunter/kcp2k-go
```

### Basic Usage

#### Server

```go
package main

import (
    "fmt"
    "log"
    kcp "https://github.com/O-Keh-Hunter/kcp2k-go"
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

#### Client

```go
package main

import (
    "fmt"
    "log"
    "time"
    kcp "https://github.com/O-Keh-Hunter/kcp2k-go"
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

### Example Usage

The project includes a comprehensive echo example with modern CLI interface:

```bash
# Start server with default settings
go run examples/echo/main.go server

# Start client with default settings
go run examples/echo/main.go client

# Custom configuration examples
go run examples/echo/main.go -port 9999 -verbose server
go run examples/echo/main.go -host 192.168.1.100 -port 9999 -message "Hello World" client

# Show help for all options
go run examples/echo/main.go -help
```

## Configuration Options

```go
config := &kcp.KcpConfig{
    DualMode:         false,
    RecvBufferSize:   1024 * 1024 * 7,  // 7MB
    SendBufferSize:   1024 * 1024 * 7,  // 7MB
    Timeout:          10000,             // 10 seconds
    Interval:         10,                // 10 milliseconds
    FastResend:       2,
    CongestionWindow: false,
    SendWindowSize:   4096,
    ReceiveWindowSize: 4096,
    Mtu:              1200,
}
```

## Testing

### Run Unit Tests

```bash
go test ./...
```

### Cross-Language Compatibility Tests

```bash
# Run all cross-language tests
./tools/scripts/testing/run_all_tests.sh

# Run specific scenarios
./tools/scripts/testing/run_csharp_server_go_client.sh
./tools/scripts/testing/run_go_server_csharp_client.sh
```

### Stress Testing

```bash
# Small-scale stress test
./tools/scripts/performance/test_small.sh

# Large-scale stress test
./tools/scripts/performance/test_full.sh
```

## Project Structure

```
kcp2k-go/
├── README.md                    # Project documentation (English)
├── README_CN.md                 # Project documentation (Chinese)
├── go.mod                       # Go module definition
├── *.go                         # Core implementation files
├── examples/                    # Example code
│   └── echo/                    # Echo server example
├── tests/                       # Test suites
│   ├── csharp_server_go_client/ # C# server + Go client tests
│   ├── go_server_csharp_client/ # Go server + C# client tests
│   ├── stress_client/           # Stress test client
│   ├── stress_server/           # Stress test server
│   └── stress_test/             # Stress test controller
├── third_party/                 # Third-party dependencies
│   └── kcp2k/                   # C# KCP2K implementation
└── tools/                       # Tool scripts
    └── scripts/
        ├── testing/             # Test scripts
        └── performance/         # Performance test scripts
```

## Performance Characteristics

- **Low Latency**: Optimized transmission delay for real-time applications
- **High Throughput**: Supports large numbers of concurrent connections
- **Memory Optimized**: Efficient memory usage and garbage collection friendly
- **Scalable**: Architecture designed for horizontal scaling

## Compatibility

- ✅ Fully compatible with C# KCP2K
- ✅ Supports all KCP2K message types and configurations
- ✅ Cross-platform support (Windows, macOS, Linux)

## Contributing

Issues and Pull Requests are welcome!

### Development Environment Setup

1. Clone the repository
```bash
git clone https://github.com/O-Keh-Hunter/kcp2k-go.git
cd kcp2k-go
```

2. Initialize submodules
```bash
git submodule update --init --recursive
```

3. Run tests
```bash
go test ./...
./tools/scripts/testing/run_all_tests.sh
```

## License

MIT License - see [LICENSE](LICENSE) file for details

## Related Projects

- [KCP2K](https://github.com/MirrorNetworking/kcp2k) - C# KCP implementation
- [Mirror Networking](https://github.com/MirrorNetworking/Mirror) - Unity networking library
- [KCP](https://github.com/skywind3000/kcp) - Original KCP protocol implementation