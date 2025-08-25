# KCP2K Examples

This directory contains basic example programs for the KCP2K protocol to help developers understand and use KCP2K.

## Directory Structure

### echo/
KCP echo server and client example
- **Features**: Demonstrates basic KCP connection, message sending and receiving
- **Usage**: 
  ```bash
  # Start server
  go run examples/echo/main.go server
  
  # Start client
  go run examples/echo/main.go client
  
  # Custom configuration examples
  go run examples/echo/main.go -port 9999 server
  go run examples/echo/main.go -host 127.0.0.1 -port 9999 client
  go run examples/echo/main.go -message "Hello World" -verbose client
  ```

## Usage Instructions

1. **Basic Examples**: Start with `echo/` to understand the basic usage of KCP2K
2. **Performance Testing**: Stress testing programs have been moved to the `tests/` directory
3. **Custom Development**: Refer to the example code to develop your own applications

## Build Instructions

```bash
# Build echo example
go build -o examples/echo/echo ./examples/echo

# Run with help to see all options
go run examples/echo/main.go -help
```

## Command Line Options

The echo example supports various configuration options:

- `-host string`: Server host (default "127.0.0.1")
- `-port int`: Server port (default 8888)
- `-message string`: Custom test message (random if empty)
- `-timeout int`: Connection timeout in seconds (default 5)
- `-verbose`: Enable verbose logging
- `-help`: Show help information

## Example Scenarios

### Basic Echo Test
```bash
# Terminal 1: Start server
go run examples/echo/main.go server

# Terminal 2: Start client
go run examples/echo/main.go client
```

### Custom Configuration
```bash
# Start server on custom port with verbose logging
go run examples/echo/main.go -port 9999 -verbose server

# Connect client with custom message and timeout
go run examples/echo/main.go -host localhost -port 9999 -message "Custom Test" -timeout 10 -verbose client
```

## Other Testing

- **Stress Testing**: Please refer to the stress testing programs in the `tests/` directory
- **Cross-Language Testing**: Please refer to the cross-language compatibility tests in the `tests/` directory
- **Performance Scripts**: Use `tools/scripts/performance/` for performance evaluation