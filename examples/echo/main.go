package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	kcp2k "github.com/O-Keh-Hunter/kcp2k-go"
)

var (
	port    = flag.Int("port", 8888, "Server port")
	host    = flag.String("host", "127.0.0.1", "Server host")
	message = flag.String("message", "", "Custom test message (random if empty)")
	timeout = flag.Int("timeout", 5, "Connection timeout in seconds")
	verbose = flag.Bool("verbose", false, "Enable verbose logging")
	help    = flag.Bool("help", false, "Show help information")
)

func printUsage() {
	fmt.Println("=== KCP Echo Example ===")
	fmt.Println("Usage:")
	fmt.Printf("  %s [options] <mode>\n", os.Args[0])
	fmt.Println()
	fmt.Println("Modes:")
	fmt.Println("  server    Start echo server")
	fmt.Println("  client    Start echo client")
	fmt.Println()
	fmt.Println("Options:")
	flag.PrintDefaults()
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Printf("  %s server                              # Start server on default port\n", os.Args[0])
	fmt.Printf("  %s -port 9999 server                   # Start server on port 9999\n", os.Args[0])
	fmt.Printf("  %s client                              # Connect to default server\n", os.Args[0])
	fmt.Printf("  %s -host 192.168.1.100 -port 9999 client  # Connect to custom server\n", os.Args[0])
	fmt.Printf("  %s -message \"Hello World\" client        # Send custom message\n", os.Args[0])
}

func validateArgs() (string, error) {
	flag.Parse()

	if *help {
		printUsage()
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) == 0 {
		return "", fmt.Errorf("mode is required")
	}

	if len(args) > 1 {
		return "", fmt.Errorf("too many arguments")
	}

	mode := strings.ToLower(args[0])
	if mode != "server" && mode != "client" {
		return "", fmt.Errorf("invalid mode: %s (supported: server, client)", mode)
	}

	// Validate port range
	if *port < 1 || *port > 65535 {
		return "", fmt.Errorf("invalid port: %d (must be 1-65535)", *port)
	}

	// Validate timeout
	if *timeout < 1 || *timeout > 300 {
		return "", fmt.Errorf("invalid timeout: %d (must be 1-300 seconds)", *timeout)
	}

	// Validate host
	if strings.TrimSpace(*host) == "" {
		return "", fmt.Errorf("host cannot be empty")
	}

	return mode, nil
}

func main() {
	mode, err := validateArgs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		printUsage()
		os.Exit(1)
	}

	switch mode {
	case "server":
		fmt.Printf("Starting KCP Echo Server on %s:%d...\n", *host, *port)
		EchoServer()
	case "client":
		fmt.Printf("Starting KCP Echo Client connecting to %s:%d...\n", *host, *port)
		EchoClient()
	}
}

// EchoServer implements a simple echo server example
func EchoServer() {
	fmt.Println("=== KCP Echo Server ===")
	fmt.Println("Starting KCP echo server...")

	// Create server configuration
	cfg := kcp2k.NewKcpConfig(kcp2k.WithDualMode(false))

	// Connection statistics
	connectedCount := 0
	messageCount := 0

	// Declare server variable
	var server *kcp2k.KcpServer

	// Create server with callbacks
	server = kcp2k.NewKcpServer(
		// Connection callback
		func(id int) {
			connectedCount++
			logMsg := fmt.Sprintf("[%s] Client %d connected (total connections: %d)",
				time.Now().Format("15:04:05"), id, connectedCount)
			if *verbose {
				fmt.Println(logMsg)
			} else {
				fmt.Printf("Client %d connected\n", id)
			}
		},
		// Message callback - echo messages
		func(id int, data []byte, ch kcp2k.KcpChannel) {
			messageCount++
			channelStr := "reliable"
			if ch == kcp2k.KcpUnreliable {
				channelStr = "unreliable"
			}

			if *verbose {
				fmt.Printf("[%s] Received from client %d: %s (channel: %s, length: %d)\n",
					time.Now().Format("15:04:05"), id, string(data), channelStr, len(data))
			} else {
				fmt.Printf("Client %d: %s\n", id, string(data))
			}

			// Echo message back to client
			response := fmt.Sprintf("Echo: %s", string(data))
			server.Send(id, []byte(response), ch)

			if *verbose {
				fmt.Printf("[%s] Echoed message to client %d\n", time.Now().Format("15:04:05"), id)
			}
		},
		// Disconnection callback
		func(id int) {
			connectedCount--
			logMsg := fmt.Sprintf("[%s] Client %d disconnected (total connections: %d)",
				time.Now().Format("15:04:05"), id, connectedCount)
			if *verbose {
				fmt.Println(logMsg)
			} else {
				fmt.Printf("Client %d disconnected\n", id)
			}
		},
		// Error callback
		func(id int, errorCode kcp2k.ErrorCode, msg string) {
			fmt.Printf("[%s] Client %d error: %v %s\n", time.Now().Format("15:04:05"), id, errorCode, msg)
		},
		cfg,
	)

	// Start server
	serverPort := uint16(*port)
	if err := server.Start(serverPort); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	fmt.Printf("[%s] Server started on port %d\n", time.Now().Format("15:04:05"), serverPort)

	// Start tick loop
	go func() {
		tickCount := 0
		for {
			server.TickIncoming()
			server.TickOutgoing()
			time.Sleep(10 * time.Millisecond)

			tickCount++
			if *verbose && tickCount%100 == 0 { // Print status every 1 second
				fmt.Printf("[%s] Server running... connections: %d, messages: %d\n",
					time.Now().Format("15:04:05"), connectedCount, messageCount)
			}
		}
	}()

	// Wait for interrupt signals (cross-platform)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan,
		syscall.SIGINT,  // Ctrl+C
		syscall.SIGTERM, // kill command
		syscall.SIGQUIT, // Ctrl+\
		syscall.SIGHUP,  // terminal closed
	)

	fmt.Println("Server is running, press Ctrl+C to stop...")
	<-sigChan

	fmt.Println("\nShutting down server...")
	server.Stop()
	fmt.Println("Server stopped")
}

// generateTestMessage generates a test message based on configuration
func generateTestMessage() string {
	if *message != "" {
		return *message
	}

	// Generate random test message
	messages := []string{
		"Hello KCP Test!",
		"Random message test",
		"Echo test message",
		"KCP reliability check",
		"Network connectivity test",
		fmt.Sprintf("Test at %s", time.Now().Format("15:04:05")),
		fmt.Sprintf("Random number: %d", rand.Intn(10000)),
	}

	return messages[rand.Intn(len(messages))]
}

// EchoClient implements a simple echo client example
func EchoClient() {
	fmt.Println("=== KCP Echo Client ===")
	fmt.Println("Starting KCP echo client...")

	// Initialize random seed
	rand.Seed(time.Now().UnixNano())

	// Create client configuration
	cfg := kcp2k.NewKcpConfig(kcp2k.WithDualMode(false))

	// Connection state
	connected := false
	messageCount := 0
	messageReceived := make(chan bool, 1)
	disconnected := make(chan bool, 1)
	errorOccurred := make(chan bool, 1)

	// Create client with callbacks
	client := kcp2k.NewKcpClient(
		// Connection success callback
		func() {
			connected = true
			if *verbose {
				fmt.Printf("[%s] Connected to server\n", time.Now().Format("15:04:05"))
			} else {
				fmt.Println("Connected to server")
			}
		},
		// Message receive callback
		func(data []byte, ch kcp2k.KcpChannel) {
			messageCount++
			channelStr := "reliable"
			if ch == kcp2k.KcpUnreliable {
				channelStr = "unreliable"
			}

			if *verbose {
				fmt.Printf("[%s] Received from server: %s (channel: %s, length: %d)\n",
					time.Now().Format("15:04:05"), string(data), channelStr, len(data))
			} else {
				fmt.Printf("Server: %s\n", string(data))
			}

			// Send signal indicating message received
			select {
			case messageReceived <- true:
			default:
			}
		},
		// Disconnection callback
		func() {
			connected = false
			if *verbose {
				fmt.Printf("[%s] Disconnected from server\n", time.Now().Format("15:04:05"))
			} else {
				fmt.Println("Disconnected from server")
			}

			// Send signal indicating disconnection complete
			select {
			case disconnected <- true:
			default:
			}
		},
		// Error callback
		func(errorCode kcp2k.ErrorCode, msg string) {
			fmt.Printf("[%s] Client error: %v %s\n", time.Now().Format("15:04:05"), errorCode, msg)

			// Send signal indicating error occurred
			select {
			case errorOccurred <- true:
			default:
			}
		},
		cfg,
	)

	// Connect to server
	serverAddr := *host
	serverPort := uint16(*port)

	fmt.Printf("[%s] Connecting to server %s:%d...\n", time.Now().Format("15:04:05"), serverAddr, serverPort)

	if err := client.Connect(serverAddr, serverPort); err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}

	// Start tick loop
	go func() {
		for {
			client.Tick()
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Wait for connection establishment
	connectionTimeout := time.Duration(*timeout) * time.Second
	time.Sleep(1 * time.Second)
	if !connected {
		fmt.Printf("Connection timeout after %v, please check if server is running\n", connectionTimeout)
		return
	}

	fmt.Println("\n=== Automatic Test Mode ===")
	fmt.Println("Connection successful, will send test message and exit")

	// Send test message
	testMessage := generateTestMessage()
	fmt.Printf("[%s] Sending test message: %s\n", time.Now().Format("15:04:05"), testMessage)
	client.Send([]byte(testMessage), kcp2k.KcpReliable)

	// Wait for echo response or error with timeout
	fmt.Printf("[%s] Waiting for server echo...\n", time.Now().Format("15:04:05"))

	// Use channel to wait for message reception or error with timeout
	select {
	case <-messageReceived:
		fmt.Printf("[%s] Received server echo, test successful\n", time.Now().Format("15:04:05"))
	case <-errorOccurred:
		fmt.Printf("[%s] Error occurred, preparing to exit\n", time.Now().Format("15:04:05"))
		return
	case <-time.After(time.Duration(*timeout) * time.Second):
		fmt.Printf("[%s] Timeout waiting for server echo\n", time.Now().Format("15:04:05"))
	}

	// Disconnect and wait for completion or error
	fmt.Printf("[%s] Test complete, disconnecting...\n", time.Now().Format("15:04:05"))
	client.Disconnect()

	// Wait for disconnection callback completion or error
	select {
	case <-disconnected:
		fmt.Printf("[%s] Disconnection complete\n", time.Now().Format("15:04:05"))
	case <-errorOccurred:
		fmt.Printf("[%s] Error occurred during disconnection\n", time.Now().Format("15:04:05"))
	case <-time.After(3 * time.Second):
		fmt.Printf("[%s] Timeout waiting for disconnection\n", time.Now().Format("15:04:05"))
	}

	fmt.Printf("[%s] Client exited\n", time.Now().Format("15:04:05"))
}
