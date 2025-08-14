#!/bin/bash

# KCP2K 跨语言兼容性测试脚本
# 场景：C# 服务端 + Go 客户端

echo "======================================"
echo "KCP2K Cross-Language Compatibility Test"
echo "Scenario: C# Server + Go Client"
echo "======================================"

# 配置
PORT=7777
TEST_TIMEOUT=30
RESULT_DIR="test_results"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
LOG_PREFIX="tests/${RESULT_DIR}/csharp_server_go_client_${TIMESTAMP}"
SERVER_LOG="${LOG_PREFIX}_server.log"
CLIENT_LOG="${LOG_PREFIX}_client.log"

# 设置工作目录到项目根目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
cd "$PROJECT_ROOT"

# 创建结果目录
mkdir -p "tests/$RESULT_DIR"

echo "Test configuration:"
echo "  Port: $PORT"
echo "  Timeout: ${TEST_TIMEOUT}s"
echo "  Log prefix: $LOG_PREFIX"
echo ""

# 清理函数
cleanup() {
    echo "\nCleaning up..."
    if [ ! -z "$SERVER_PID" ]; then
        echo "Stopping C# server (PID: $SERVER_PID)"
        kill $SERVER_PID 2>/dev/null || true
        wait $SERVER_PID 2>/dev/null || true
    fi
    if [ ! -z "$CLIENT_PID" ]; then
        echo "Stopping Go client (PID: $CLIENT_PID)"
        kill $CLIENT_PID 2>/dev/null || true
        wait $CLIENT_PID 2>/dev/null || true
    fi
}

# 设置信号处理
trap cleanup EXIT INT TERM

# 检查端口是否被占用
if lsof -Pi :$PORT -sTCP:LISTEN -t >/dev/null 2>&1; then
    echo "Error: Port $PORT is already in use"
    exit 1
fi

echo "Step 1: Building C# server..."
if ! dotnet build tests/csharp_server_go_client/CSharpServer.csproj --configuration Release; then
    echo "Error: Failed to build C# server"
    exit 1
fi
echo "✓ C# server built successfully"
echo "\nStep 2: Building Go client..."
cd tests/csharp_server_go_client
if ! go build -o go_client go_client.go; then
    echo "Error: Failed to build Go client"
    exit 1
fi
echo "✓ Go client built successfully"
cd ..

echo "Step 3: Starting C# server..."
cd "$PROJECT_ROOT/tests/csharp_server_go_client"
dotnet run --project CSharpServer.csproj --configuration Release -- $PORT > "$PROJECT_ROOT/${SERVER_LOG}" 2>&1 &
SERVER_PID=$!
echo "C# server started (PID: $SERVER_PID)"
cd "$PROJECT_ROOT"

# 等待服务器启动
echo "Waiting for server to start..."
sleep 3

# 检查服务器是否正在运行
if ! kill -0 $SERVER_PID 2>/dev/null; then
    echo "Error: C# server failed to start"
    cat "${LOG_PREFIX}_server.log"
    exit 1
fi

# 检查端口是否监听
for i in {1..10}; do
    if lsof -Pi :$PORT -sTCP:LISTEN -t >/dev/null 2>&1; then
        echo "✓ Server is listening on port $PORT"
        break
    fi
    if [ $i -eq 10 ]; then
        echo "Error: Server is not listening on port $PORT after 10 seconds"
        exit 1
    fi
    sleep 1
done

echo "\nStep 4: Running Go client tests..."
cd "$PROJECT_ROOT/tests/csharp_server_go_client"
./go_client --host 127.0.0.1 --port $PORT --auto > "$PROJECT_ROOT/${CLIENT_LOG}" 2>&1
CLIENT_EXIT_CODE=$?
echo "Go client finished (exit code: $CLIENT_EXIT_CODE)"
cd "$PROJECT_ROOT"

# 检查客户端执行结果
if [ "$CLIENT_EXIT_CODE" -eq 0 ]; then
    echo "✓ Go client tests completed successfully"
else
    echo "✗ Go client tests failed with exit code $CLIENT_EXIT_CODE"
fi

echo "\nStep 5: Analyzing test results..."

# 分析服务器日志
echo "Server log analysis:"
if grep -q "Client.*connected" "${LOG_PREFIX}_server.log"; then
    echo "  ✓ Client connection established"
else
    echo "  ✗ No client connection found"
fi

if grep -q "Received from" "${LOG_PREFIX}_server.log"; then
    echo "  ✓ Server received messages from client"
else
    echo "  ✗ Server did not receive messages from client"
fi

if grep -q "Sent to" "${LOG_PREFIX}_server.log"; then
    echo "  ✓ Server sent messages to client"
else
    echo "  ✗ Server did not send messages to client"
fi

# 分析客户端日志
echo "\nClient log analysis:"
if grep -q "Connected successfully" "${LOG_PREFIX}_client.log"; then
    echo "  ✓ Client connected to server"
else
    echo "  ✗ Client failed to connect to server"
fi

if grep -q "Received:" "${LOG_PREFIX}_client.log"; then
    echo "  ✓ Client received messages from server"
else
    echo "  ✗ Client did not receive messages from server"
fi

# 统计测试结果
PASSED_TESTS=$(grep -c "✓.*test passed" "${LOG_PREFIX}_client.log" 2>/dev/null)
if [ -z "$PASSED_TESTS" ] || [ "$PASSED_TESTS" = "" ]; then
    PASSED_TESTS=0
fi
FAILED_TESTS=$(grep -c "✗.*test failed" "${LOG_PREFIX}_client.log" 2>/dev/null)
if [ -z "$FAILED_TESTS" ] || [ "$FAILED_TESTS" = "" ]; then
    FAILED_TESTS=0
fi
TOTAL_TESTS=$((PASSED_TESTS + FAILED_TESTS))

echo "\nTest Results Summary:"
echo "  Passed: $PASSED_TESTS"
echo "  Failed: $FAILED_TESTS"
echo "  Total: $TOTAL_TESTS"

# 生成测试报告
REPORT_FILE="${LOG_PREFIX}_report.txt"
cat > "$REPORT_FILE" << EOF
KCP2K Cross-Language Compatibility Test Report
Scenario: C# Server + Go Client
Timestamp: $(date)
Port: $PORT

=== Test Results ===
Passed Tests: $PASSED_TESTS
Failed Tests: $FAILED_TESTS
Total Tests:  $TOTAL_TESTS
Client Exit Code: $CLIENT_EXIT_CODE

=== Server Log ===
$(cat "${LOG_PREFIX}_server.log")

=== Client Log ===
$(cat "${LOG_PREFIX}_client.log")
EOF

echo "\nTest report saved to: $REPORT_FILE"

# 确定最终结果
if [ "$CLIENT_EXIT_CODE" -eq 0 ] && [ "$PASSED_TESTS" -gt 0 ] && [ "$FAILED_TESTS" -eq 0 ]; then
    echo "\n🎉 All tests PASSED! C# server and Go client are compatible."
    exit 0
else
    echo "\n❌ Some tests FAILED. Check the logs for details."
    echo "Server log: ${LOG_PREFIX}_server.log"
    echo "Client log: ${LOG_PREFIX}_client.log"
    echo "Full report: $REPORT_FILE"
    exit 1
fi