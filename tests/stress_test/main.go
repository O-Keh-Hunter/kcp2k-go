package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
)

type StressTest struct {
	NumServers       int
	ClientsPerServer int
	FPS              int
	StartPort        int
	ServerHost       string
	TestDuration     time.Duration
	ServerProcess    *exec.Cmd
	ClientProcess    *exec.Cmd
	ctx              context.Context
	cancel           context.CancelFunc
}

func NewStressTest() *StressTest {
	ctx, cancel := context.WithCancel(context.Background())
	return &StressTest{
		ctx:    ctx,
		cancel: cancel,
	}
}

func (st *StressTest) StartServers() error {
	// Get the path to the stress_server binary
	serverBin := filepath.Join("tests", "stress_server", "stress_server")
	if runtime.GOOS == "windows" {
		serverBin += ".exe"
	}

	// Build the server if it doesn't exist
	if _, err := os.Stat(serverBin); os.IsNotExist(err) {
		log.Println("Building stress server...")
		buildCmd := exec.Command("go", "build", "-o", serverBin, "./tests/stress_server")
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("failed to build server: %v", err)
		}
	}

	// Start the server process
	st.ServerProcess = exec.CommandContext(st.ctx, serverBin,
		"-servers", fmt.Sprintf("%d", st.NumServers),
		"-start-port", fmt.Sprintf("%d", st.StartPort),
		"-report", "5s")

	st.ServerProcess.Stdout = os.Stdout
	st.ServerProcess.Stderr = os.Stderr

	log.Printf("Starting %d servers on ports %d-%d...",
		st.NumServers, st.StartPort, st.StartPort+st.NumServers-1)

	if err := st.ServerProcess.Start(); err != nil {
		return fmt.Errorf("failed to start server process: %v", err)
	}

	// Wait a bit for servers to start
	time.Sleep(10 * time.Second)

	return nil
}

func (st *StressTest) StartClients() error {
	// Get the path to the stress_client binary
	clientBin := filepath.Join("tests", "stress_client", "stress_client")
	if runtime.GOOS == "windows" {
		clientBin += ".exe"
	}

	// Build the client if it doesn't exist
	if _, err := os.Stat(clientBin); os.IsNotExist(err) {
		log.Println("Building stress client...")
		buildCmd := exec.Command("go", "build", "-o", clientBin, "./tests/stress_client")
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("failed to build client: %v", err)
		}
	}

	// Start the client process
	st.ClientProcess = exec.CommandContext(st.ctx, clientBin,
		"-host", st.ServerHost,
		"-servers", fmt.Sprintf("%d", st.NumServers),
		"-clients-per-server", fmt.Sprintf("%d", st.ClientsPerServer),
		"-fps", fmt.Sprintf("%d", st.FPS),
		"-start-port", fmt.Sprintf("%d", st.StartPort),
		"-report", "5s")

	st.ClientProcess.Stdout = os.Stdout
	st.ClientProcess.Stderr = os.Stderr

	totalClients := st.NumServers * st.ClientsPerServer
	log.Printf("Starting %d clients (%d per server) at %d FPS...",
		totalClients, st.ClientsPerServer, st.FPS)

	if err := st.ClientProcess.Start(); err != nil {
		return fmt.Errorf("failed to start client process: %v", err)
	}

	return nil
}

func (st *StressTest) Run() error {
	var wg sync.WaitGroup

	// Start servers
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := st.StartServers(); err != nil {
			log.Printf("Server error: %v", err)
			st.cancel()
		}
	}()

	// Wait for servers to be ready
	time.Sleep(15 * time.Second)

	// Start clients
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := st.StartClients(); err != nil {
			log.Printf("Client error: %v", err)
			st.cancel()
		}
	}()

	// Wait for test duration or interrupt
	if st.TestDuration > 0 {
		log.Printf("Running stress test for %v...", st.TestDuration)
		select {
		case <-time.After(st.TestDuration):
			log.Println("Test duration completed")
		case <-st.ctx.Done():
			log.Println("Test interrupted")
		}
	} else {
		log.Println("Running stress test indefinitely (press Ctrl+C to stop)...")
		<-st.ctx.Done()
	}

	// Stop all processes
	st.Stop()

	// Wait for processes to finish
	wg.Wait()

	return nil
}

func (st *StressTest) Stop() {
	log.Println("Stopping stress test...")

	st.cancel()

	if st.ClientProcess != nil && st.ClientProcess.Process != nil {
		log.Println("Stopping client process...")
		st.ClientProcess.Process.Signal(syscall.SIGTERM)
		time.Sleep(5 * time.Second)
		if st.ClientProcess.Process != nil {
			st.ClientProcess.Process.Kill()
		}
	}

	if st.ServerProcess != nil && st.ServerProcess.Process != nil {
		log.Println("Stopping server process...")
		st.ServerProcess.Process.Signal(syscall.SIGTERM)
		time.Sleep(5 * time.Second)
		if st.ServerProcess.Process != nil {
			st.ServerProcess.Process.Kill()
		}
	}
}

func main() {
	var (
		numServers       = flag.Int("servers", 500, "Number of servers")
		clientsPerServer = flag.Int("clients-per-server", 10, "Number of clients per server")
		fps              = flag.Int("fps", 15, "Frames per second")
		startPort        = flag.Int("start-port", 10000, "Starting port for servers")
		serverHost       = flag.String("host", "localhost", "Server host")
		testDuration     = flag.Duration("duration", 0, "Test duration (0 = run indefinitely)")
	)
	flag.Parse()

	// Validate parameters
	if *numServers <= 0 {
		log.Fatal("Number of servers must be positive")
	}
	if *clientsPerServer <= 0 {
		log.Fatal("Number of clients per server must be positive")
	}
	if *fps <= 0 {
		log.Fatal("FPS must be positive")
	}
	if *startPort <= 0 {
		log.Fatal("Start port must be positive")
	}

	// Calculate expected load
	totalConnections := *numServers * *clientsPerServer
	totalPacketsPerSecond := totalConnections * *fps

	log.Printf("=== STRESS TEST CONFIGURATION ===")
	log.Printf("Servers: %d", *numServers)
	log.Printf("Clients per server: %d", *clientsPerServer)
	log.Printf("Total connections: %d", totalConnections)
	log.Printf("FPS per client: %d", *fps)
	log.Printf("Total packets/second: %d", totalPacketsPerSecond)
	log.Printf("Server ports: %d-%d", *startPort, *startPort+*numServers-1)
	log.Printf("Test duration: %v", *testDuration)
	log.Printf("================================")

	// Create and run stress test
	stressTest := NewStressTest()
	stressTest.NumServers = *numServers
	stressTest.ClientsPerServer = *clientsPerServer
	stressTest.FPS = *fps
	stressTest.StartPort = *startPort
	stressTest.ServerHost = *serverHost
	stressTest.TestDuration = *testDuration

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received interrupt signal")
		stressTest.Stop()
	}()

	// Run the test
	if err := stressTest.Run(); err != nil {
		log.Fatalf("Stress test failed: %v", err)
	}

	log.Println("Stress test completed")
}
