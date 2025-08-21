package lockstep

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// StartMetricsServer 启动指标HTTP服务器
func (s *LockStepServer) StartMetricsServer() {
	mux := http.NewServeMux()

	// Metrics endpoint
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		stats := s.GetServerStats()
		json.NewEncoder(w).Encode(stats)
	})

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		health := s.HealthCheck()
		json.NewEncoder(w).Encode(health)
	})

	// Create HTTP server
	s.metricsServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.MetricsPort),
		Handler: mux,
	}

	// Start server in goroutine
	server := s.metricsServer
	go func() {
		Log.Info("Starting metrics server on port %d", s.config.MetricsPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			Log.Error("Metrics server error: %v", err)
		}
	}()
}

// StopMetricsServer 停止指标HTTP服务器
func (s *LockStepServer) StopMetricsServer() {
	if s.metricsServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.metricsServer.Shutdown(ctx); err != nil {
			Log.Error("Error stopping metrics server: %v", err)
		} else {
			Log.Info("Metrics server stopped")
		}
		s.metricsServer = nil
	}
}
