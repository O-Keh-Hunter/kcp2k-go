package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	kcp2k "github.com/O-Keh-Hunter/kcp2k-go"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("=== KCP Echo 示例 ===")
		fmt.Println("使用方法:")
		fmt.Println("  go run cmd/echo/main.go server  # 启动服务器")
		fmt.Println("  go run cmd/echo/main.go client  # 启动客户端")
		fmt.Println()
		fmt.Println("示例:")
		fmt.Println("  1. 先启动服务器: go run cmd/echo/main.go server")
		fmt.Println("  2. 再启动客户端: go run cmd/echo/main.go client")
		fmt.Println("  3. 在客户端输入消息，服务器会回显")
		return
	}

	mode := os.Args[1]

	switch mode {
	case "server":
		fmt.Println("启动KCP Echo服务器...")
		EchoServer()
	case "client":
		fmt.Println("启动KCP Echo客户端...")
		EchoClient()
	default:
		fmt.Printf("未知模式: %s\n", mode)
		fmt.Println("支持的模式: server, client")
	}
}

// EchoServer 简单的echo服务器示例
func EchoServer() {
	fmt.Println("=== KCP Echo Server ===")
	fmt.Println("启动KCP echo服务器...")

	// 创建服务器配置
	cfg := kcp2k.NewKcpConfig(kcp2k.WithDualMode(false))

	// 连接统计
	connectedCount := 0
	messageCount := 0

	// 声明服务器变量
	var server *kcp2k.KcpServer

	// 创建服务器
	server = kcp2k.NewKcpServer(
		// 连接回调
		func(id int) {
			connectedCount++
			fmt.Printf("[%s] 客户端 %d 已连接 (总连接数: %d)\n", time.Now().Format("15:04:05"), id, connectedCount)
		},
		// 消息回调 - 回显消息
		func(id int, data []byte, ch kcp2k.KcpChannel) {
			messageCount++
			channelStr := "可靠"
			if ch == kcp2k.KcpUnreliable {
				channelStr = "不可靠"
			}
			fmt.Printf("[%s] 收到客户端 %d 消息: %s (通道: %s, 长度: %d)\n",
				time.Now().Format("15:04:05"), id, string(data), channelStr, len(data))

			// 回显消息给客户端
			response := fmt.Sprintf("Echo: %s", string(data))
			server.Send(id, []byte(response), ch)
			fmt.Printf("[%s] 已回显消息给客户端 %d\n", time.Now().Format("15:04:05"), id)
		},
		// 断开连接回调
		func(id int) {
			connectedCount--
			fmt.Printf("[%s] 客户端 %d 已断开 (总连接数: %d)\n", time.Now().Format("15:04:05"), id, connectedCount)
		},
		// 错误回调
		func(id int, errorCode kcp2k.ErrorCode, msg string) {
			fmt.Printf("[%s] 客户端 %d 错误: %v %s\n", time.Now().Format("15:04:05"), id, errorCode, msg)
		},
		cfg,
	)

	// 启动服务器
	port := uint16(8888)
	if err := server.Start(port); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}

	fmt.Printf("[%s] 服务器已启动在端口 %d\n", time.Now().Format("15:04:05"), port)

	// 启动tick循环
	go func() {
		tickCount := 0
		for {
			server.TickIncoming()
			server.TickOutgoing()
			time.Sleep(10 * time.Millisecond)

			tickCount++
			if tickCount%100 == 0 { // 每1秒打印一次状态
				fmt.Printf("[%s] 服务器运行中... 连接数: %d, 消息数: %d\n",
					time.Now().Format("15:04:05"), connectedCount, messageCount)
			}
		}
	}()

	// 等待中断信号 (macOS优化)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan,
		syscall.SIGINT,  // Ctrl+C
		syscall.SIGTERM, // kill命令
		syscall.SIGQUIT, // Ctrl+\
		syscall.SIGHUP,  // 终端关闭
	)

	fmt.Println("服务器正在运行，按 Ctrl+C 停止...")
	<-sigChan

	fmt.Println("\n正在关闭服务器...")
	server.Stop()
	fmt.Println("服务器已关闭")
}

// EchoClient 简单的echo客户端示例
func EchoClient() {
	fmt.Println("=== KCP Echo Client ===")
	fmt.Println("启动KCP echo客户端...")

	// 创建客户端配置
	cfg := kcp2k.NewKcpConfig(kcp2k.WithDualMode(false))

	// 连接状态
	connected := false
	messageCount := 0
	messageReceived := make(chan bool, 1)
	disconnected := make(chan bool, 1)
	errorOccurred := make(chan bool, 1)

	// 创建客户端
	client := kcp2k.NewKcpClient(
		// 连接成功回调
		func() {
			connected = true
			fmt.Printf("[%s] 已连接到服务器\n", time.Now().Format("15:04:05"))
		},
		// 消息接收回调
		func(data []byte, ch kcp2k.KcpChannel) {
			messageCount++
			channelStr := "可靠"
			if ch == kcp2k.KcpUnreliable {
				channelStr = "不可靠"
			}
			fmt.Printf("[%s] 收到服务器消息: %s (通道: %s, 长度: %d)\n",
				time.Now().Format("15:04:05"), string(data), channelStr, len(data))

			// 发送信号表示已收到消息
			select {
			case messageReceived <- true:
			default:
			}
		},
		// 断开连接回调
		func() {
			connected = false
			fmt.Printf("[%s] 与服务器断开连接\n", time.Now().Format("15:04:05"))

			// 发送信号表示断开连接完成
			select {
			case disconnected <- true:
			default:
			}
		},
		// 错误回调
		func(errorCode kcp2k.ErrorCode, msg string) {
			fmt.Printf("[%s] 客户端错误: %v %s\n", time.Now().Format("15:04:05"), errorCode, msg)

			// 发送信号表示发生错误
			select {
			case errorOccurred <- true:
			default:
			}
		},
		cfg,
	)

	// 连接到服务器
	serverAddr := "127.0.0.1"
	serverPort := uint16(8888)

	fmt.Printf("[%s] 正在连接到服务器 %s:%d...\n", time.Now().Format("15:04:05"), serverAddr, serverPort)

	if err := client.Connect(serverAddr, serverPort); err != nil {
		log.Fatalf("连接服务器失败: %v", err)
	}

	// 启动tick循环
	go func() {
		for {
			client.Tick()
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// 等待连接建立
	time.Sleep(1 * time.Second)
	if !connected {
		fmt.Println("连接超时，请检查服务器是否运行")
		return
	}

	fmt.Println("\n=== 自动测试模式 ===")
	fmt.Println("连接成功，将自动发送测试消息并退出")

	// 发送测试消息
	testMessage := "Hello KCP Auto Test!"
	fmt.Printf("[%s] 发送测试消息: %s\n", time.Now().Format("15:04:05"), testMessage)
	client.Send([]byte(testMessage), kcp2k.KcpReliable)

	// 等待接收回显消息或发生错误
	fmt.Printf("[%s] 等待服务器回显...\n", time.Now().Format("15:04:05"))

	// 使用通道等待消息接收完成或错误，设置超时
	select {
	case <-messageReceived:
		fmt.Printf("[%s] 已收到服务器回显，测试成功\n", time.Now().Format("15:04:05"))
	case <-errorOccurred:
		fmt.Printf("[%s] 发生错误，准备退出\n", time.Now().Format("15:04:05"))
		return
	case <-time.After(5 * time.Second):
		fmt.Printf("[%s] 等待服务器回显超时\n", time.Now().Format("15:04:05"))
	}

	// 断开连接并等待断开完成或错误
	fmt.Printf("[%s] 测试完成，正在断开连接...\n", time.Now().Format("15:04:05"))
	client.Disconnect()

	// 等待断开连接回调执行完成或发生错误
	select {
	case <-disconnected:
		fmt.Printf("[%s] 断开连接完成\n", time.Now().Format("15:04:05"))
	case <-errorOccurred:
		fmt.Printf("[%s] 断开连接过程中发生错误\n", time.Now().Format("15:04:05"))
	case <-time.After(3 * time.Second):
		fmt.Printf("[%s] 等待断开连接超时\n", time.Now().Format("15:04:05"))
	}

	fmt.Printf("[%s] 客户端已退出\n", time.Now().Format("15:04:05"))
}
