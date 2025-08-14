#!/bin/bash

# 小规模压力测试脚本
# 用于验证系统功能

echo "=== 小规模压力测试 ==="
echo "服务器数量: 10"
echo "每服务器客户端数: 5"
echo "FPS: 15"
echo "测试时长: 60秒"
echo "=================="

# 构建所有组件
echo "构建组件..."
go build -o tests/stress_server/stress_server ./tests/stress_server
go build -o tests/stress_client/stress_client ./tests/stress_client
go build -o tests/stress_test/stress_test ./tests/stress_test

# 运行测试
echo "开始测试..."
./tests/stress_test/stress_test \
    -servers 10 \
    -clients-per-server 5 \
    -fps 15 \
    -start-port 10000 \
    -duration 60s

echo "测试完成"