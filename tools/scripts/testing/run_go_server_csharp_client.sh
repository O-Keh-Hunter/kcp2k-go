#!/bin/bash

# KCP2K 跨语言兼容性测试脚本
# 场景：Go 服务端 + C# 客户端

echo "======================================"
echo "KCP2K Cross-Language Compatibility Test"
echo "Scenario: Go Server + C# Client"
echo "======================================"

# 设置工作目录到项目根目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
cd "$PROJECT_ROOT"

# 配置
PORT=7778
TEST_TIMEOUT=30
RESULT_DIR="tests/test_results"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
LOG_PREFIX="${RESULT_DIR}/go_server_csharp_client_${TIMESTAMP}"
SERVER_LOG="${LOG_PREFIX}_server.log"
CLIENT_LOG="${LOG_PREFIX}_client.log"

# 创建结果目录
mkdir -p "$RESULT_DIR"

echo "Test configuration:"
echo "  Port: $PORT"
echo "  Timeout: ${TEST_TIMEOUT}s"
echo "  Log prefix: $LOG_PREFIX"
echo ""

# 清理函数
cleanup() {
    echo "\nCleaning up..."
    if [ ! -z "$SERVER_PID" ]; then
        echo "Stopping Go server (PID: $SERVER_PID)"
        kill $SERVER_PID 2>/dev/null || true
        wait $SERVER_PID 2>/dev/null || true
    fi
    if [ ! -z "$CLIENT_PID" ]; then
        echo "Stopping C# client (PID: $CLIENT_PID)"
        kill $CLIENT_PID 2>/dev/null || true
        wait $CLIENT_PID 2>/dev/null || true
    fi
}

# 设置信号处理
trap cleanup EXIT INT TERM

# 清理可能占用的端口
echo "Cleaning up ports..."
lsof -ti:$PORT | xargs -r kill -9 2>/dev/null || true
sleep 1

# 检查端口是否被占用
if lsof -i UDP:$PORT -t >/dev/null 2>&1; then
    echo "Error: Port $PORT is still in use after cleanup"
    exit 1
fi

echo "Step 1: Building Go server..."
cd "$PROJECT_ROOT/tests/go_server_csharp_client"
if ! go build -o go_server go_server.go; then
    echo "Error: Failed to build Go server"
    exit 1
fi
echo "✓ Go server built successfully"

echo "\nStep 2: Building C# client..."
cd "$PROJECT_ROOT/tests/go_server_csharp_client"
if ! dotnet build CSharpClient.csproj --configuration Release; then
    echo "Error: Failed to build C# client"
    exit 1
fi
echo "✓ C# client built successfully"
cd "$PROJECT_ROOT"

echo "\nStep 3: Starting Go server..."
cd "$PROJECT_ROOT/tests/go_server_csharp_client"
./go_server $PORT > "$PROJECT_ROOT/$SERVER_LOG" 2>&1 &
SERVER_PID=$!
echo "Go server started (PID: $SERVER_PID)"
cd "$PROJECT_ROOT"

# 等待服务器启动
echo "Waiting for server to start..."
sleep 5

# 检查服务器是否正在运行
if ! kill -0 $SERVER_PID 2>/dev/null; then
    echo "Error: Go server failed to start"
    cat "${LOG_PREFIX}_server.log"
    exit 1
fi

# 检查端口是否监听
for i in {1..10}; do
    if lsof -i UDP:$PORT -t >/dev/null 2>&1; then
        echo "✓ Server is listening on port $PORT"
        break
    fi
    if [ $i -eq 10 ]; then
        echo "Error: Server is not listening on port $PORT after 10 seconds"
        exit 1
    fi
    sleep 1
done

echo "\nStep 4: Running C# client tests..."
cd "$PROJECT_ROOT/tests/go_server_csharp_client"
dotnet run --project CSharpClient.csproj --configuration Release -- --host 127.0.0.1 --port $PORT --auto > "$PROJECT_ROOT/$CLIENT_LOG" 2>&1 &
CLIENT_PID=$!
echo "C# client started (PID: $CLIENT_PID)"
cd "$PROJECT_ROOT"

# 等待客户端完成测试
echo "Waiting for client tests to complete..."
if wait $CLIENT_PID; then
    CLIENT_EXIT_CODE=0
    echo "✓ C# client tests completed successfully"
else
    CLIENT_EXIT_CODE=$?
    echo "✗ C# client tests failed with exit code $CLIENT_EXIT_CODE"
fi
CLIENT_PID=""  # 清空PID，避免重复清理

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
PASSED_TESTS=$(grep -c "test passed" "${LOG_PREFIX}_client.log" 2>/dev/null | head -1 || echo "0")
FAILED_TESTS=$(grep -c "test failed" "${LOG_PREFIX}_client.log" 2>/dev/null | head -1 || echo "0")
TOTAL_TESTS=$((PASSED_TESTS + FAILED_TESTS))

echo "\nTest Results Summary:"
echo "  Passed: $PASSED_TESTS"
echo "  Failed: $FAILED_TESTS"
echo "  Total:  $TOTAL_TESTS"

# 生成测试报告
REPORT_FILE="${LOG_PREFIX}_report.txt"
cat > "$REPORT_FILE" << EOF
KCP2K Cross-Language Compatibility Test Report
Scenario: Go Server + C# Client
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
    echo "\n🎉 All tests PASSED! Go server and C# client are compatible."
    exit 0
else
    echo "\n❌ Some tests FAILED. Check the logs for details."
    echo "Server log: ${LOG_PREFIX}_server.log"
    echo "Client log: ${LOG_PREFIX}_client.log"
    echo "Full report: $REPORT_FILE"
    exit 1
fi