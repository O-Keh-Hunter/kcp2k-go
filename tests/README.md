# KCP2K Test Suite

[English](README.md) | [中文](README_CN.md)

This directory contains various test programs for KCP2K to verify protocol correctness and performance.

## Directory Structure

### Cross-Language Compatibility Tests

#### Scenario 1: C# Server + Go Client
- Server: C# KcpServer
- Client: Go KcpClient
- Test Directory: `csharp_server_go_client/`

#### Scenario 2: Go Server + C# Client
- Server: Go KcpServer
- Client: C# KcpClient
- Test Directory: `go_server_csharp_client/`

### Stress Testing Programs

#### stress_server/
Stress test server
- **Function**: High-performance server designed specifically for stress testing
- **Features**: Supports large numbers of concurrent connections with optimized memory usage

#### stress_client/
Stress test client
- **Function**: Simulates large numbers of client connections for stress testing
- **Features**: Configurable connection count, send frequency, and other parameters

#### stress_test/
Stress test controller
- **Function**: Coordinates multiple servers and clients for stress testing
- **Usage**: 
  ```bash
  go run tests/stress_test/main.go -servers 10 -clients-per-server 5
  ```

## Test Coverage

1. **Connection Establishment and Termination**
   - Basic connect/disconnect
   - Timeout disconnection
   - Active client removal

2. **Message Transmission**
   - Reliable messages (Reliable)
   - Unreliable messages (Unreliable)
   - Empty messages
   - Maximum size messages
   - Batch message sending

3. **Error Handling**
   - Invalid messages
   - Oversized messages
   - Network exceptions

4. **Performance Testing**
   - Latency testing
   - Throughput testing
   - Concurrent connection testing

5. **Stress Testing**
   - Large-scale concurrent connection testing
   - Long-term stability testing
   - Resource usage monitoring
   - Maximum load testing

## Running Tests

### Manual Test Component Execution

#### Cross-Language Compatibility Tests
```bash
# C# Server + Go Client
cd tests/csharp_server_go_client
dotnet run &
go run go_client.go

# Go Server + C# Client
cd tests/go_server_csharp_client
go run go_server.go &
dotnet run
```

#### Stress Test Components
```bash
# Build stress test programs
go build -o tests/stress_server/stress_server ./tests/stress_server
go build -o tests/stress_client/stress_client ./tests/stress_client
go build -o tests/stress_test/stress_test ./tests/stress_test

# Run stress test manually
./tests/stress_test/stress_test -servers 500 -clients-per-server 10 -fps 15
```

### Using Test Scripts

#### Cross-Language Compatibility Test Scripts
```bash
# Run all cross-language tests
./tools/scripts/testing/run_all_tests.sh

# Run specific scenario tests
./tools/scripts/testing/run_csharp_server_go_client.sh
./tools/scripts/testing/run_go_server_csharp_client.sh
```

#### Performance Test Scripts
```bash
# Small-scale testing (recommended to run first)
./tools/scripts/performance/test_small.sh

# Full-scale testing (requires significant system resources)
./tools/scripts/performance/test_full.sh
```

#### Script Parameter Description
- **test_small.sh**: 10 servers × 5 clients × 15 FPS, suitable for functionality verification
- **test_full.sh**: 500 servers × 10 clients × 15 FPS, suitable for performance stress testing
- **Test Duration**: Default 60 seconds, adjustable via script parameters
- **Port Range**: 10000-10499 (small-scale), 10000-10499 (full-scale)

## Important Notes

### Environment Requirements
- Ensure tests are executed from the project root directory
- Verify that .NET 8.0 and Go 1.19+ are installed on the system
- Test results and log files are saved in the `tests/test_results/` directory

### Script Usage Guidelines
1. **Permissions**: Ensure scripts have execution permissions (`chmod +x script_name.sh`)
2. **Dependencies**: Performance tests require building the related test programs first
3. **Resources**: Full-scale testing requires significant CPU and memory resources
4. **Platform**: Scripts are primarily tested on Unix-like systems (Linux/macOS)

### Performance Considerations
- **Small-scale tests**: Suitable for development and CI/CD environments
- **Full-scale tests**: Intended for dedicated testing environments
- **Resource monitoring**: Monitor system resources during stress testing
- **Network configuration**: Ensure sufficient network buffers for high-load testing

## Test Results

Test results are saved in the `test_results/` directory, including:
- Connectivity test reports
- Message transmission test reports
- Cross-language compatibility test reports
- Stress test reports
- Performance benchmark reports
- Error logs

## Test Configuration

### Customizing Test Parameters

Most test scripts support environment variables for customization:

```bash
# Example: Custom test duration and connection limits
export TEST_DURATION=120  # 2 minutes
export MAX_CLIENTS=1000   # Maximum clients per server
export TEST_FPS=30        # Messages per second

./tools/scripts/performance/test_full.sh
```

### Troubleshooting

- **Port conflicts**: Ensure test port ranges don't conflict with other services
- **Resource limits**: Check system ulimits for file descriptors and memory
- **Network issues**: Verify firewall settings don't block test traffic
- **Build failures**: Ensure all dependencies are properly installed

For detailed test configuration and advanced usage, refer to the individual test program documentation in their respective directories.