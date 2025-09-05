#!/bin/bash

# 完整规模压力测试脚本
# 500个服务器，每个服务器10个客户端，15FPS

echo "=== 完整规模压力测试 ==="
echo "服务器数量: 300"
echo "每服务器客户端数: 10"
echo "FPS: 15"
echo "总连接数: 3000"
echo "总数据包/秒: 45000"
echo "======================"

# 检查系统资源
echo "检查系统资源..."
echo "CPU核心数: $(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 'unknown')"
echo "内存: $(free -h 2>/dev/null | grep Mem | awk '{print $2}' || echo 'unknown')"

# 构建所有组件 (Release模式)
echo "构建组件 (Release模式)..."
go build -ldflags="-s -w" -gcflags="-N -l" -o tests/stress_server/stress_server ./tests/stress_server
go build -ldflags="-s -w" -gcflags="-N -l" -o tests/stress_client/stress_client ./tests/stress_client
go build -ldflags="-s -w" -gcflags="-N -l" -o tests/stress_test/stress_test ./tests/stress_test

# 运行测试
echo "开始完整规模测试..."
echo "注意: 这可能需要大量系统资源，请确保系统有足够的内存和CPU"
echo "按 Ctrl+C 停止测试"
echo "启用GC检测以分析内存分配..."
echo ""

# 启用GC检测和内存分析
export GODEBUG=gctrace=1,gcpacertrace=1
export GOMAXPROCS=$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo '4')

./tests/stress_test/stress_test \
    -servers 300 \
    -clients-per-server 10 \
    -fps 15 \
    -start-port 10000 \
    2>&1 | tee gc_analysis.log

echo "测试完成"