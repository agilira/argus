// demo_app.go: Practical Demo Application showing Argus + FlashFlags Integration
//
// Copyright (c) 2025 AGILira
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

// This file demonstrates a complete application using the integrated
// Argus configuration management with FlashFlags for ultra-fast parsing.

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/agilira/argus" // Import the unified package
)

func main() {
	// Create unified configuration manager with fluent interface
	config := argus.NewConfigManager("demo-server").
		SetDescription("Demo web server showcasing Argus + FlashFlags integration").
		SetVersion("2.0.0").
		// Server configuration
		StringFlag("host", "localhost", "Server host address").
		IntFlag("port", 8080, "Server port").
		StringFlag("tls-cert", "", "TLS certificate file (enables HTTPS)").
		StringFlag("tls-key", "", "TLS private key file").
		// Operational configuration
		BoolFlag("debug", false, "Enable debug logging").
		DurationFlag("read-timeout", 30*time.Second, "HTTP read timeout").
		DurationFlag("write-timeout", 30*time.Second, "HTTP write timeout").
		IntFlag("max-connections", 1000, "Maximum concurrent connections").
		// Real-time configuration
		StringFlag("config-file", "", "Configuration file for real-time updates").
		DurationFlag("config-reload-interval", 5*time.Second, "Config reload check interval").
		// Security and CORS
		StringSliceFlag("allowed-origins", []string{"*"}, "Allowed CORS origins").
		StringFlag("api-key", "", "API key for authentication").
		// Performance tuning
		IntFlag("worker-count", 4, "Number of worker goroutines").
		DurationFlag("graceful-timeout", 15*time.Second, "Graceful shutdown timeout")

	// Parse command-line arguments and environment variables
	if err := config.ParseArgs(); err != nil {
		fmt.Printf("Configuration error: %v\n", err)
		config.PrintUsage()
		os.Exit(1)
	}

	// Show configuration precedence in action
	fmt.Println("=== Configuration Precedence Demo ===")
	fmt.Printf("Host: %s (precedence: CLI > ENV:%s > default)\n",
		config.GetString("host"), config.FlagToEnvKey("host"))
	fmt.Printf("Port: %d (precedence: CLI > ENV:%s > default)\n",
		config.GetInt("port"), config.FlagToEnvKey("port"))
	fmt.Printf("Debug: %t (precedence: CLI > ENV:%s > default)\n",
		config.GetBool("debug"), config.FlagToEnvKey("debug"))

	// Show configuration statistics
	total, valid := config.GetStats()
	fmt.Printf("Configuration entries: %d/%d (100%% valid)\n", valid, total)

	// Show bound flags for debugging
	if config.GetBool("debug") {
		boundFlags := config.GetBoundFlags()
		fmt.Printf("Bound flags: %d\n", len(boundFlags))
		for configKey, flagName := range boundFlags {
			fmt.Printf("  %s -> %s\n", flagName, configKey)
		}
	}

	// Setup real-time configuration watching if config file is specified
	configFile := config.GetString("config-file")
	if configFile != "" {
		fmt.Printf("Enabling real-time config watching for: %s\n", configFile)

		if err := config.WatchConfigFile(configFile, func() {
			log.Printf("Configuration file changed: %s", configFile)

			// Example: Update log level dynamically
			if config.GetBool("debug") {
				log.Println("Debug mode enabled via config file")
			}

			// Example: Update connection limits dynamically
			maxConn := config.GetInt("max-connections")
			log.Printf("Max connections updated to: %d", maxConn)
		}); err != nil {
			log.Printf("Warning: Cannot setup config watching: %v", err)
		}

		if err := config.StartWatching(); err != nil {
			log.Printf("Warning: Cannot start config watcher: %v", err)
		}
		defer config.StopWatching()
	}

	// Create HTTP server with configuration
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.GetString("host"), config.GetInt("port")),
		ReadTimeout:  config.GetDuration("read-timeout"),
		WriteTimeout: config.GetDuration("write-timeout"),
		Handler:      createHandler(config),
	}

	// Setup graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Start server
	go func() {
		fmt.Printf("\n=== Starting Demo Server ===\n")
		fmt.Printf("URL: http://%s\n", server.Addr)
		fmt.Printf("Debug: %t\n", config.GetBool("debug"))
		fmt.Printf("Workers: %d\n", config.GetInt("worker-count"))
		fmt.Printf("Max connections: %d\n", config.GetInt("max-connections"))
		fmt.Printf("Allowed origins: %v\n", config.GetStringSlice("allowed-origins"))

		if config.GetString("tls-cert") != "" {
			fmt.Println("Starting HTTPS server...")
			if err := server.ListenAndServeTLS(
				config.GetString("tls-cert"),
				config.GetString("tls-key")); err != http.ErrServerClosed {
				log.Fatalf("HTTPS server error: %v", err)
			}
		} else {
			fmt.Println("Starting HTTP server...")
			if err := server.ListenAndServe(); err != http.ErrServerClosed {
				log.Fatalf("HTTP server error: %v", err)
			}
		}
	}()

	// Wait for shutdown signal
	<-shutdown

	// Graceful shutdown
	fmt.Println("\nShutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), config.GetDuration("graceful-timeout"))
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	fmt.Println("Server stopped gracefully")
}

// createHandler creates an HTTP handler that uses the configuration
func createHandler(config *argus.ConfigManager) http.Handler {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","debug":%t,"connections":%d}`,
			config.GetBool("debug"), config.GetInt("max-connections"))
	})

	// Configuration endpoint (for debugging)
	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		// Simple API key authentication
		if apiKey := config.GetString("api-key"); apiKey != "" {
			if r.Header.Get("X-API-Key") != apiKey {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
  "host": "%s",
  "port": %d,
  "debug": %t,
  "read_timeout": "%s",
  "write_timeout": "%s",
  "max_connections": %d,
  "worker_count": %d,
  "allowed_origins": %v
}`,
			config.GetString("host"),
			config.GetInt("port"),
			config.GetBool("debug"),
			config.GetDuration("read-timeout"),
			config.GetDuration("write-timeout"),
			config.GetInt("max-connections"),
			config.GetInt("worker-count"),
			config.GetStringSlice("allowed-origins"))
	})

	// Demo endpoint showing real-time config access
	mux.HandleFunc("/demo", func(w http.ResponseWriter, r *http.Request) {
		// CORS handling using configuration
		origins := config.GetStringSlice("allowed-origins")
		origin := r.Header.Get("Origin")

		for _, allowedOrigin := range origins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}

		// Response varies based on current configuration
		if config.GetBool("debug") {
			w.Header().Set("X-Debug", "true")
			w.Header().Set("X-Worker-Count", fmt.Sprintf("%d", config.GetInt("worker-count")))
		}

		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "Demo endpoint - Debug mode: %t, Worker count: %d\n",
			config.GetBool("debug"), config.GetInt("worker-count"))
	})

	// Statistics endpoint
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		total, valid := config.GetStats()
		boundFlags := config.GetBoundFlags()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
  "config_entries": {"total": %d, "valid": %d},
  "bound_flags": %d,
  "performance": "ultra-fast lock-free access"
}`, total, valid, len(boundFlags))
	})

	return mux
}

// Example usage commands:
//
// Basic usage:
//   go run demo_app.go --port=8080 --debug
//
// With environment variables:
//   DEMO_SERVER_HOST=0.0.0.0 DEMO_SERVER_PORT=3000 go run demo_app.go
//
// With config file watching:
//   go run demo_app.go --config-file=server.json --debug
//
// With HTTPS:
//   go run demo_app.go --tls-cert=server.crt --tls-key=server.key --port=8443
//
// With API key:
//   go run demo_app.go --api-key=secret123 --debug
//
// Test endpoints:
//   curl http://localhost:8080/health
//   curl http://localhost:8080/config -H "X-API-Key: secret123"
//   curl http://localhost:8080/demo
//   curl http://localhost:8080/stats
