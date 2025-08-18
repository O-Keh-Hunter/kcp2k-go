#!/bin/bash

# Lockstep 压测场景脚本
# 支持多种测试场景：正常负载、峰值负载、断线重连、网络抖动等

set -e

# 配置参数
SERVER_ADDR="127.0.0.1"
SERVER_PORT=8888
SERVER_METRICS_PORT=8887  # 监控指标端口
ROOM_COUNT=1
PLAYERS_PER_ROOM=10
TOTAL_CLIENTS=$((ROOM_COUNT * PLAYERS_PER_ROOM))
UPSTREAM_INTERVAL="33ms"  # 33ms上行间隔 (30帧/秒)
INPUT_PACKET_SIZE=40      # 40字节input包

# 测试场景配置
SCENARIO="normal"  # normal, peak, reconnect, network_jitter
TEST_DURATION="5m"
RECONNECT_RATE=0.01

# 日志和结果目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="$SCRIPT_DIR/../results"
LOGS_DIR="$RESULTS_DIR/logs"
REPORTS_DIR="$RESULTS_DIR/reports"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印带颜色的消息
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 显示帮助信息
show_help() {
    echo "Lockstep 压测场景脚本"
    echo ""
    echo "用法: $0 [选项] <场景>"
    echo ""
    echo "场景:"
    echo "  normal          正常负载测试 (默认)"
    echo "  peak            峰值负载测试"
    echo "  reconnect       断线重连测试"
    echo "  network_jitter  网络抖动测试"
    echo ""
    echo "选项:"
    echo "  -h, --help      显示帮助信息"
    echo "  -a, --addr      服务器地址 (默认: $SERVER_ADDR)"
    echo "  -p, --port      服务器端口 (默认: $SERVER_PORT)"
    echo "  -m, --metrics   监控端口 (默认: $SERVER_METRICS_PORT)"
    echo "  -r, --rooms     房间数量 (默认: $ROOM_COUNT)"
    echo "  -c, --clients   每房间客户端数 (默认: $PLAYERS_PER_ROOM)"
    echo "  -d, --duration  测试持续时间 (默认: $TEST_DURATION)"
    echo "  --reconnect     重连率 (默认: $RECONNECT_RATE)"
    echo ""
    echo "示例:"
    echo "  $0 normal                    # 运行正常负载测试"
    echo "  $0 peak -d 10m              # 运行10分钟峰值负载测试"
    echo "  $0 reconnect --reconnect 0.05 # 运行5%重连率的重连测试"
}

# 解析命令行参数
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            -a|--addr)
                SERVER_ADDR="$2"
                shift 2
                ;;
            -p|--port)
                SERVER_PORT="$2"
                shift 2
                ;;
            -m|--metrics)
                SERVER_METRICS_PORT="$2"
                shift 2
                ;;
            -r|--rooms)
                ROOM_COUNT="$2"
                TOTAL_CLIENTS=$((ROOM_COUNT * PLAYERS_PER_ROOM))
                shift 2
                ;;
            -c|--clients)
                PLAYERS_PER_ROOM="$2"
                TOTAL_CLIENTS=$((ROOM_COUNT * PLAYERS_PER_ROOM))
                shift 2
                ;;
            -d|--duration)
                TEST_DURATION="$2"
                shift 2
                ;;
            --reconnect)
                RECONNECT_RATE="$2"
                shift 2
                ;;
            normal|peak|reconnect|network_jitter)
                SCENARIO="$1"
                shift
                ;;
            *)
                print_error "未知参数: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

# 检查依赖
check_dependencies() {
    print_info "检查依赖..."
    
    # 检查Go环境
    if ! command -v go &> /dev/null; then
        print_error "Go 未安装或不在PATH中"
        exit 1
    fi
    
    # 检查必要的工具
    for tool in lsof pkill; do
        if ! command -v $tool &> /dev/null; then
            print_warning "$tool 未找到，某些功能可能不可用"
        fi
    done
    
    print_success "依赖检查完成"
}

# 创建目录结构
setup_directories() {
    print_info "创建目录结构..."
    
    mkdir -p "$LOGS_DIR"
    mkdir -p "$REPORTS_DIR"
    
    print_success "目录结构创建完成"
}

# 清理环境
cleanup_environment() {
    print_info "清理环境..."
    
    # 终止可能存在的进程
    pkill -f "stress_server" 2>/dev/null || true
    pkill -f "stress_client" 2>/dev/null || true
    
    # 等待进程完全终止
    sleep 2
    
    # 检查端口占用
    if command -v lsof &> /dev/null; then
        if lsof -i :$SERVER_PORT &> /dev/null; then
            print_warning "端口 $SERVER_PORT 仍被占用，尝试强制终止..."
            lsof -ti :$SERVER_PORT | xargs kill -9 2>/dev/null || true
            sleep 1
        fi
    fi
    
    print_success "环境清理完成"
}

# 构建程序
build_programs() {
    print_info "构建压测程序..."
    
    # 构建服务器
    cd "$SCRIPT_DIR/../server"
    go build -o stress_server main.go config.go
    if [ $? -ne 0 ]; then
        print_error "服务器构建失败"
        exit 1
    fi
    
    # 构建客户端
    cd "$SCRIPT_DIR/../client"
    go build -o stress_client main.go config.go
    if [ $? -ne 0 ]; then
        print_error "客户端构建失败"
        exit 1
    fi
    
    cd "$SCRIPT_DIR"
    print_success "程序构建完成"
}

# 启动压测服务器
start_server() {
    print_info "启动压测服务器..."
    
    cd ../server
    
    # 根据场景选择配置文件
    CONFIG_FILE=""
    case $SCENARIO in
        "normal")
            CONFIG_FILE="../configs/default.yaml"
            ;;
        "peak")
            CONFIG_FILE="../configs/peak.yaml"
            ;;
        "reconnect")
            CONFIG_FILE="../configs/reconnect.yaml"
            ;;
        "network_jitter")
            CONFIG_FILE="../configs/network_jitter.yaml"
            ;;
    esac
    
    # 启动服务器，优先使用配置文件，命令行参数作为覆盖
    if [ -n "$CONFIG_FILE" ] && [ -f "$CONFIG_FILE" ]; then
        print_info "使用配置文件: $CONFIG_FILE"
        ./stress_server -config="$CONFIG_FILE" -addr="$SERVER_ADDR" -port="$SERVER_PORT" -metrics-port="$SERVER_METRICS_PORT" -rooms="$ROOM_COUNT" -players="$PLAYERS_PER_ROOM" > "$LOGS_DIR/server.log" 2>&1 &
    else
        print_warning "配置文件不存在，使用命令行参数"
        ./stress_server -addr="$SERVER_ADDR" -port="$SERVER_PORT" -metrics-port="$SERVER_METRICS_PORT" -rooms="$ROOM_COUNT" -players="$PLAYERS_PER_ROOM" > "$LOGS_DIR/server.log" 2>&1 &
    fi
    
    SERVER_PID=$!
    
    # 等待服务器启动
    sleep 3
    
    # 检查服务器是否正常运行
    if ! kill -0 $SERVER_PID 2>/dev/null; then
        print_error "服务器启动失败"
        cat "$LOGS_DIR/server.log"
        exit 1
    fi
    
    cd ../scripts
    print_success "压测服务器已启动 (PID: $SERVER_PID)"
}

# 运行正常负载测试
run_normal_load() {
    print_info "运行正常负载测试..."
    print_info "配置: $TOTAL_CLIENTS 客户端, $ROOM_COUNT 房间, 每房间 $PLAYERS_PER_ROOM 玩家"
    print_info "上行间隔: $UPSTREAM_INTERVAL, 包大小: $INPUT_PACKET_SIZE 字节, 持续时间: $TEST_DURATION"
    
    cd ../client
    
    # 根据场景选择配置文件
    CONFIG_FILE=""
    case $SCENARIO in
        "normal")
            CONFIG_FILE="../configs/default.yaml"
            ;;
        "peak")
            CONFIG_FILE="../configs/peak.yaml"
            ;;
        "reconnect")
            CONFIG_FILE="../configs/reconnect.yaml"
            ;;
        "network_jitter")
            CONFIG_FILE="../configs/network_jitter.yaml"
            ;;
    esac
    
    # 启动客户端，优先使用配置文件，命令行参数作为覆盖
    if [ -n "$CONFIG_FILE" ] && [ -f "$CONFIG_FILE" ]; then
        print_info "客户端使用配置文件: $CONFIG_FILE"
        ./stress_client -config="$CONFIG_FILE" -addr="$SERVER_ADDR" -port="$SERVER_PORT" -clients="$TOTAL_CLIENTS" -duration="$TEST_DURATION" -reconnect="$RECONNECT_RATE" > "$LOGS_DIR/client.log" 2>&1 &
    else
        print_warning "配置文件不存在，客户端使用命令行参数"
        ./stress_client -addr="$SERVER_ADDR" -port="$SERVER_PORT" -clients="$TOTAL_CLIENTS" -duration="$TEST_DURATION" -reconnect="$RECONNECT_RATE" > "$LOGS_DIR/client.log" 2>&1 &
    fi
    
    CLIENT_PID=$!
    
    cd ../scripts
    print_success "正常负载测试已启动 (PID: $CLIENT_PID)"
}

# 运行峰值负载测试
run_peak_load() {
    print_info "运行峰值负载测试..."
    
    # 峰值负载：增加20%的客户端数量
    PEAK_CLIENTS=$((TOTAL_CLIENTS * 120 / 100))
    print_info "配置: $PEAK_CLIENTS 客户端 (峰值), $ROOM_COUNT 房间"
    
    cd ../client
    
    # 根据场景选择配置文件
    CONFIG_FILE="../configs/peak.yaml"
    
    # 启动客户端，优先使用配置文件，命令行参数作为覆盖
    if [ -f "$CONFIG_FILE" ]; then
        print_info "峰值测试使用配置文件: $CONFIG_FILE"
        ./stress_client -config="$CONFIG_FILE" -addr="$SERVER_ADDR" -port="$SERVER_PORT" -clients="$PEAK_CLIENTS" -duration="$TEST_DURATION" -reconnect="$RECONNECT_RATE" > "$LOGS_DIR/client.log" 2>&1 &
    else
        print_warning "配置文件不存在，峰值测试使用命令行参数"
        ./stress_client -addr="$SERVER_ADDR" -port="$SERVER_PORT" -clients="$PEAK_CLIENTS" -duration="$TEST_DURATION" -reconnect="$RECONNECT_RATE" > "$LOGS_DIR/client.log" 2>&1 &
    fi
    
    CLIENT_PID=$!
    
    cd ../scripts
    print_success "峰值负载测试已启动 (PID: $CLIENT_PID)"
}

# 运行断线重连测试
run_reconnect_test() {
    print_info "运行断线重连测试..."
    
    # 断线重连测试：提高重连率到5%
    RECONNECT_RATE=0.05
    print_info "配置: $TOTAL_CLIENTS 客户端, 重连率: $RECONNECT_RATE"
    
    cd ../client
    
    # 根据场景选择配置文件
    CONFIG_FILE="../configs/reconnect.yaml"
    
    # 启动客户端，优先使用配置文件，命令行参数作为覆盖
    if [ -f "$CONFIG_FILE" ]; then
        print_info "重连测试使用配置文件: $CONFIG_FILE"
        ./stress_client -config="$CONFIG_FILE" -addr="$SERVER_ADDR" -port="$SERVER_PORT" -clients="$TOTAL_CLIENTS" -duration="$TEST_DURATION" -reconnect="$RECONNECT_RATE" > "$LOGS_DIR/client.log" 2>&1 &
    else
        print_warning "配置文件不存在，重连测试使用命令行参数"
        ./stress_client -addr="$SERVER_ADDR" -port="$SERVER_PORT" -clients="$TOTAL_CLIENTS" -duration="$TEST_DURATION" -reconnect="$RECONNECT_RATE" > "$LOGS_DIR/client.log" 2>&1 &
    fi
    
    CLIENT_PID=$!
    
    cd ../scripts
    print_success "断线重连测试已启动 (PID: $CLIENT_PID)"
}

# 运行网络抖动测试
run_network_jitter_test() {
    print_info "运行网络抖动测试..."
    print_warning "网络抖动测试需要额外的网络模拟工具，当前使用基础测试"
    
    # 网络抖动测试：使用较短的测试间隔模拟网络不稳定
    cd ../client
    
    # 根据场景选择配置文件
    CONFIG_FILE="../configs/network_jitter.yaml"
    
    # 启动客户端，优先使用配置文件，命令行参数作为覆盖
    if [ -f "$CONFIG_FILE" ]; then
        print_info "网络抖动测试使用配置文件: $CONFIG_FILE"
        ./stress_client -config="$CONFIG_FILE" -addr="$SERVER_ADDR" -port="$SERVER_PORT" -clients="$TOTAL_CLIENTS" -duration="$TEST_DURATION" -reconnect="0.02" > "$LOGS_DIR/client.log" 2>&1 &
    else
        print_warning "配置文件不存在，网络抖动测试使用命令行参数"
        ./stress_client -addr="$SERVER_ADDR" -port="$SERVER_PORT" -clients="$TOTAL_CLIENTS" -duration="$TEST_DURATION" -reconnect="0.02" > "$LOGS_DIR/client.log" 2>&1 &
    fi
    
    CLIENT_PID=$!
    
    cd ../scripts
    print_success "网络抖动测试已启动 (PID: $CLIENT_PID)"
}

# 等待测试完成
wait_for_completion() {
    print_info "等待测试完成..."
    
    # 等待客户端进程结束
    wait $CLIENT_PID
    CLIENT_EXIT_CODE=$?
    
    print_info "客户端测试完成 (退出码: $CLIENT_EXIT_CODE)"
    
    # 停止服务器
    if kill -0 $SERVER_PID 2>/dev/null; then
        print_info "停止压测服务器..."
        kill $SERVER_PID
        wait $SERVER_PID 2>/dev/null || true
    fi
    
    print_success "测试完成"
}

# 主函数
main() {
    print_info "=== Lockstep 压测场景脚本 ==="
    
    # 解析参数
    parse_args "$@"
    
    print_info "场景: $SCENARIO"
    print_info "服务器: $SERVER_ADDR:$SERVER_PORT"
    print_info "总客户端: $TOTAL_CLIENTS ($ROOM_COUNT 房间 x $PLAYERS_PER_ROOM 玩家)"
    
    # 执行测试流程
    check_dependencies
    setup_directories
    cleanup_environment
    build_programs
    start_server
    
    # 根据场景运行不同的测试
    case $SCENARIO in
        "normal")
            run_normal_load
            ;;
        "peak")
            run_peak_load
            ;;
        "reconnect")
            run_reconnect_test
            ;;
        "network_jitter")
            run_network_jitter_test
            ;;
        *)
            print_error "未知场景: $SCENARIO"
            exit 1
            ;;
    esac
    
    wait_for_completion
    
    print_success "=== 压测完成 ==="
}

# 信号处理
trap 'print_warning "收到中断信号，正在清理..."; kill $SERVER_PID $CLIENT_PID 2>/dev/null || true; exit 1' INT TERM

# 运行主函数
main "$@"