#!/bin/bash

# 测试第三个玩家持续接收帧数据
echo "Testing continuous frame reception for third player..."

# 设置信号处理，确保脚本被中断时也能清理环境
trap 'echo "\nScript interrupted. Cleaning up..."; cleanup_processes; check_port 8888; check_port 8889; check_port 8890; check_port 8891; exit 1' INT TERM

# 函数：检查端口是否被占用
check_port() {
    local port=$1
    if lsof -i :$port > /dev/null 2>&1; then
        echo "Port $port is in use. Killing processes..."
        lsof -ti :$port | xargs kill -9 2>/dev/null
        sleep 1
    fi
}

# 函数：清理相关进程
cleanup_processes() {
    echo "Cleaning up existing processes..."
    # 查找并终止所有lockstep相关进程
    pkill -f "lockstep-server" 2>/dev/null
    pkill -f "./lockstep-server" 2>/dev/null
    pkill -f "main.go" 2>/dev/null
    sleep 2
    
    # 强制终止仍在运行的进程
    pkill -9 -f "lockstep-server" 2>/dev/null
    pkill -9 -f "./lockstep-server" 2>/dev/null
    pkill -9 -f "main.go" 2>/dev/null
    sleep 1
}

# 清理环境
echo "Cleaning up environment..."
cleanup_processes

# 检查并清理端口
echo "Checking ports..."
check_port 8888
check_port 8889
check_port 8890
check_port 8891

# 清理之前的日志文件
rm -f server.log client1.log client2.log client3.log

# 编译程序
echo "Building lockstep-server..."
go build -o lockstep-server main.go
if [ $? -ne 0 ]; then
    echo "Build failed!"
    exit 1
fi

# 启动服务器
echo "Starting server..."
./lockstep-server server 8888 > server.log 2>&1 &
SERVER_PID=$!

# 等待服务器启动
sleep 2

# 启动第一个客户端
echo "Starting client 1..."
./lockstep-server client 127.0.0.1 8888 1 > client1.log 2>&1 &
CLIENT1_PID=$!

# 等待第一个客户端连接
sleep 2

# 启动第二个客户端（游戏开始）
echo "Starting client 2..."
./lockstep-server client 127.0.0.1 8888 2 > client2.log 2>&1 &
CLIENT2_PID=$!

# 等待游戏开始并运行一段时间
echo "Waiting for game to start and run..."
sleep 5

# 启动第三个客户端
echo "Starting client 3..."
./lockstep-server client 127.0.0.1 8888 3 > client3.log 2>&1 &
CLIENT3_PID=$!

# 让第三个客户端运行更长时间以接收持续的帧数据
echo "Letting client 3 run to receive continuous frames..."
sleep 10

# 停止所有进程
echo "Stopping all processes..."
kill $CLIENT3_PID $CLIENT2_PID $CLIENT1_PID $SERVER_PID 2>/dev/null

# 等待进程结束，但设置超时
echo "Waiting for processes to terminate..."
for i in {1..5}; do
    if ! kill -0 $CLIENT3_PID $CLIENT2_PID $CLIENT1_PID $SERVER_PID 2>/dev/null; then
        break
    fi
    sleep 1
done

# 强制终止仍在运行的进程
kill -9 $CLIENT3_PID $CLIENT2_PID $CLIENT1_PID $SERVER_PID 2>/dev/null

# 最终清理：确保所有相关进程都被终止
echo "Final cleanup..."
cleanup_processes

# 清理端口
check_port 8888
check_port 8889
check_port 8890
check_port 8891

echo "Test completed. Analyzing results..."

# 分析第三个客户端的日志
echo "\n=== Client 3 Frame Reception Analysis ==="
echo "Initial sync frames:"
grep "Received.*frames from server" client3.log

echo "\nContinuous frame reception:"
grep "PopFrame got frame" client3.log | tail -10

echo "\nTotal frames received by client 3:"
FRAME_COUNT=$(grep -c "PopFrame got frame" client3.log)
echo "Total frames: $FRAME_COUNT"

if [ $FRAME_COUNT -gt 10 ]; then
    echo "✅ SUCCESS: Client 3 received continuous frames ($FRAME_COUNT total)"
    exit 0
else
    echo "❌ FAILED: Client 3 only received initial sync frames ($FRAME_COUNT total)"
    echo "\n=== Server Log (last 20 lines) ==="
    tail -20 server.log
    echo "\n=== Client 3 Log (last 20 lines) ==="
    tail -20 client3.log
    exit 1
fi