# KCP2K 测试套件

本目录包含 KCP2K 的各种测试程序，用于验证协议的正确性和性能。

## 目录结构

### 跨语言兼容性测试

#### 场景 1: C# 服务端 + Go 客户端
- 服务端：C# KcpServer
- 客户端：Go KcpClient
- 测试目录：`csharp_server_go_client/`

#### 场景 2: Go 服务端 + C# 客户端
- 服务端：Go KcpServer
- 客户端：C# KcpClient
- 测试目录：`go_server_csharp_client/`

### 压力测试程序

#### stress_server/
压力测试服务器
- **功能**: 专门用于压力测试的高性能服务器
- **特点**: 支持大量并发连接，优化内存使用

#### stress_client/
压力测试客户端
- **功能**: 模拟大量客户端连接进行压力测试
- **特点**: 可配置连接数、发送频率等参数

#### stress_test/
压力测试主控程序
- **功能**: 协调多个服务器和客户端进行压力测试
- **用法**: 
  ```bash
  go run tests/stress_test/main.go -servers 10 -clients-per-server 5
  ```

## 测试内容

1. **连接建立和断开**
   - 基本连接/断开
   - 超时断开
   - 主动踢出

2. **消息传输**
   - 可靠消息 (Reliable)
   - 不可靠消息 (Unreliable)
   - 空消息
   - 最大尺寸消息
   - 多消息批量发送

3. **错误处理**
   - 无效消息
   - 超大消息
   - 网络异常

4. **性能测试**
   - 延迟测试
   - 吞吐量测试
   - 并发连接测试

5. **压力测试**
   - 大量并发连接测试
   - 长时间稳定性测试
   - 资源使用监控
   - 极限负载测试

## 运行测试

### 手动运行测试组件

#### 跨语言兼容性测试
```bash
# C# 服务端 + Go 客户端
cd tests/csharp_server_go_client
dotnet run &
go run go_client.go

# Go 服务端 + C# 客户端
cd tests/go_server_csharp_client
go run go_server.go &
dotnet run
```

#### 压力测试组件
```bash
# 构建压力测试程序
go build -o tests/stress_server/stress_server ./tests/stress_server
go build -o tests/stress_client/stress_client ./tests/stress_client
go build -o tests/stress_test/stress_test ./tests/stress_test

# 手动运行压力测试
./tests/stress_test/stress_test -servers 500 -clients-per-server 10 -fps 15
```

### 使用测试脚本

#### 跨语言兼容性测试脚本
```bash
# 运行所有跨语言测试
./tools/scripts/testing/run_all_tests.sh

# 运行特定场景测试
./tools/scripts/testing/run_csharp_server_go_client.sh
./tools/scripts/testing/run_go_server_csharp_client.sh
```

#### 性能测试脚本
```bash
# 小规模测试 (推荐先运行)
./tools/scripts/performance/test_small.sh

# 完整规模测试 (需要大量系统资源)
./tools/scripts/performance/test_full.sh
```

#### 脚本参数说明
- **test_small.sh**: 10服务器 × 5客户端 × 15FPS，适合功能验证
- **test_full.sh**: 500服务器 × 10客户端 × 15FPS，适合性能压力测试
- **测试时长**: 默认60秒，可通过脚本内参数调整
- **端口范围**: 10000-10499 (小规模), 10000-10499 (完整规模)

## 注意事项

### 环境要求
- 运行测试前请确保在项目根目录执行
- 确保系统已安装 .NET 8.0 和 Go 1.19+
- 测试结果和日志文件保存在 `tests/test_results/` 目录

### 脚本使用注意事项
1. **权限**: 确保脚本有执行权限 (`chmod +x script_name.sh`)
2. **依赖**: 性能测试需要先构建相关的测试程序
3. **资源**: 完整规模测试需要大量 CPU 和内存资源
4. **平台**: 脚本主要在 Unix-like 系统上测试 (Linux/macOS)

## 测试结果

测试结果将保存在 `test_results/` 目录中，包括：
- 连接性测试报告
- 消息传输测试报告
- 跨语言兼容性测试报告
- 压力测试报告
- 性能基准测试报告
- 错误日志