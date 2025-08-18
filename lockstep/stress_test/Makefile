# Lockstep 压测 Makefile

.PHONY: all build clean server client test-normal test-peak test-reconnect test-jitter help

# 默认目标
all: build

# 构建所有程序
build: build-server build-client

# 构建服务器
build-server:
	@echo "构建压测服务器..."
	@cd server && go build -o stress_server main.go config.go
	@echo "服务器构建完成"

# 构建客户端
build-client:
	@echo "构建压测客户端..."
	@cd client && go build -o stress_client main.go config.go
	@echo "客户端构建完成"

# 清理构建文件
clean:
	@echo "清理构建文件..."
	@rm -f server/stress_server
	@rm -f client/stress_client
	@rm -rf results/logs/*
	@rm -rf results/reports/*
	@echo "清理完成"

# 创建目录结构
setup:
	@echo "创建目录结构..."
	@mkdir -p results/logs
	@mkdir -p results/reports
	@mkdir -p configs
	@echo "目录结构创建完成"

# 运行正常负载测试
run-normal: clean build setup
	@echo "运行正常负载测试..."
	@chmod +x scripts/run_scenario.sh
	@cd scripts && ./run_scenario.sh normal

# 运行峰值负载测试
run-peak: clean build setup
	@echo "运行峰值负载测试..."
	@chmod +x scripts/run_scenario.sh
	@cd scripts && ./run_scenario.sh peak -d 10m

# 运行断线重连测试
run-reconnect: clean build setup
	@echo "运行断线重连测试..."
	@chmod +x scripts/run_scenario.sh
	@cd scripts && ./run_scenario.sh reconnect --reconnect 0.05

# 运行网络抖动测试
run-jitter: clean build setup
	@echo "运行网络抖动测试..."
	@chmod +x scripts/run_scenario.sh
	@cd scripts && ./run_scenario.sh network_jitter

# 运行自定义测试
run-custom: clean build setup
	@echo "运行自定义测试..."
	@echo "用法: make run-custom SCENARIO=<场景> ARGS='<额外参数>'"
	@echo "示例: make run-custom SCENARIO=normal ARGS='-d 15m -r 100'"
	@chmod +x scripts/run_scenario.sh
	@cd scripts && ./run_scenario.sh $(SCENARIO) $(ARGS)

# 只启动服务器
start-server: build-server setup
	@echo "启动压测服务器..."
	@cd server && ./stress_server -addr=127.0.0.1 -port=8888 -rooms=500 -players=10

# 只启动客户端
start-client: build-client
	@echo "启动压测客户端..."
	@cd client && ./stress_client -addr=127.0.0.1 -port=8888 -clients=5000 -duration=5m

# 查看服务器日志
logs-server:
	@echo "查看服务器日志..."
	@tail -f results/logs/server.log

# 查看客户端日志
logs-client:
	@echo "查看客户端日志..."
	@tail -f results/logs/client.log

# 查看最新报告
show-report:
	@echo "查看最新测试报告..."
	@ls -t results/reports/*.md | head -1 | xargs cat

# 监控系统资源
monitor:
	@echo "监控系统资源 (按 Ctrl+C 停止)..."
	@while true; do \
		clear; \
		echo "=== 系统资源监控 ==="; \
		echo "时间: $$(date)"; \
		echo ""; \
		echo "=== CPU 和内存 ==="; \
		top -l 1 -n 0 | head -10; \
		echo ""; \
		echo "=== 网络连接 (端口 8888) ==="; \
		netstat -an | grep 8888 | wc -l | xargs echo "连接数:"; \
		echo ""; \
		echo "=== 进程状态 ==="; \
		ps aux | grep -E '(stress_server|stress_client)' | grep -v grep; \
		sleep 5; \
	done

# 性能分析
profile-server: build-server
	@echo "启动服务器性能分析..."
	@cd server && go run -race main.go config.go -addr=127.0.0.1 -port=8888 -rooms=100 -players=10

profile-client: build-client
	@echo "启动客户端性能分析..."
	@cd client && go run -race main.go config.go -addr=127.0.0.1 -port=8888 -clients=1000 -duration=2m

# 基准测试
benchmark: build setup
	@echo "运行基准测试套件..."
	@echo "1. 小规模测试 (100 客户端)"
	@cd scripts && ./run_scenario.sh normal -r 10 -c 10 -d 2m
	@sleep 5
	@echo "2. 中等规模测试 (1000 客户端)"
	@cd scripts && ./run_scenario.sh normal -r 100 -c 10 -d 3m
	@sleep 5
	@echo "3. 大规模测试 (5000 客户端)"
	@cd scripts && ./run_scenario.sh normal -r 500 -c 10 -d 5m

# 压力测试
stress: build setup
	@echo "运行压力测试..."
	@echo "警告: 这将运行高强度测试，可能影响系统性能"
	@read -p "继续? [y/N] " -n 1 -r; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		echo ""; \
		cd scripts && ./run_scenario.sh peak -d 15m; \
	else \
		echo "\n取消压力测试"; \
	fi

# 检查环境
check-env:
	@echo "检查环境依赖..."
	@echo "Go 版本:"
	@go version
	@echo ""
	@echo "系统信息:"
	@uname -a
	@echo ""
	@echo "内存信息:"
	@free -h 2>/dev/null || vm_stat | head -5
	@echo ""
	@echo "网络接口:"
	@ifconfig | grep -E '^[a-z]|inet ' | head -10

# 安装依赖
install-deps:
	@echo "安装 Go 依赖..."
	@cd server && go mod tidy
	@cd client && go mod tidy
	@echo "依赖安装完成"

# 显示帮助信息
help:
	@echo "Lockstep 压测 Makefile"
	@echo ""
	@echo "构建命令:"
	@echo "  make build          构建所有程序"
	@echo "  make build-server   只构建服务器"
	@echo "  make build-client   只构建客户端"
	@echo "  make clean          清理构建文件"
	@echo ""
	@echo "测试命令:"
	@echo "  make run-normal     运行正常负载测试"
	@echo "  make run-peak       运行峰值负载测试"
	@echo "  make run-reconnect  运行断线重连测试"
	@echo "  make run-jitter     运行网络抖动测试"
	@echo "  make benchmark      运行基准测试套件"
	@echo "  make stress         运行压力测试"
	@echo ""
	@echo "运行命令:"
	@echo "  make start-server   只启动服务器"
	@echo "  make start-client   只启动客户端"
	@echo ""
	@echo "监控命令:"
	@echo "  make logs-server    查看服务器日志"
	@echo "  make logs-client    查看客户端日志"
	@echo "  make show-report    查看最新报告"
	@echo "  make monitor        监控系统资源"
	@echo ""
	@echo "分析命令:"
	@echo "  make profile-server 服务器性能分析"
	@echo "  make profile-client 客户端性能分析"
	@echo ""
	@echo "工具命令:"
	@echo "  make check-env      检查环境"
	@echo "  make install-deps   安装依赖"
	@echo "  make setup          创建目录结构"
	@echo "  make help           显示帮助信息"
	@echo ""
	@echo "自定义测试:"
	@echo "  make run-custom SCENARIO=normal ARGS='-d 15m -r 100'"
	@echo ""