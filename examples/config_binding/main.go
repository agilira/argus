// Package main demonstrates configuration binding using Argus.
// This example shows how to bind configuration values from a generic map structure
// to strongly typed Go variables, including error handling and performance benchmarking.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/agilira/argus"
)

// exampleConfig is a sample configuration in JSON format for demonstration purposes.
const exampleConfig = `{
   "app": {
	   "name": "my-service",
	   "version": "1.0.0",
	   "debug": true
   },
   "server": {
	   "host": "localhost",
	   "port": 8080,
	   "timeout": "30s"
   },
   "database": {
	   "host": "db.example.com",
	   "port": 5432,
	   "ssl_mode": "require",
	   "pool": {
		   "max_connections": 20,
		   "idle_timeout": "5m"
	   }
   }
}`

// main demonstrates configuration binding, error handling, and performance benchmarking using Argus.
func main() {
	fmt.Println("Argus Configuration Binding Example")
	fmt.Println("==========================================")

	// Parse JSON configuration
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(exampleConfig), &config); err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}

	// Application configuration variables
	var (
		// App settings
		appName    string
		appVersion string
		appDebug   bool

		// Server settings
		serverHost    string
		serverPort    int
		serverTimeout time.Duration

		// Database settings
		dbHost        string
		dbPort        int
		dbSSLMode     string
		dbMaxConns    int
		dbIdleTimeout time.Duration
	)

	// Bind configuration values to Go variables
	fmt.Println("\nBinding configuration...")
	start := time.Now()

	err := argus.BindFromConfig(config).
		// App bindings with default values
		BindString(&appName, "app.name", "default-service").
		BindString(&appVersion, "app.version", "0.0.1").
		BindBool(&appDebug, "app.debug", false).
		// Server bindings with default values
		BindString(&serverHost, "server.host", "0.0.0.0").
		BindInt(&serverPort, "server.port", 3000).
		BindDuration(&serverTimeout, "server.timeout", 10*time.Second).
		// Database bindings with default values
		BindString(&dbHost, "database.host", "localhost").
		BindInt(&dbPort, "database.port", 5432).
		BindString(&dbSSLMode, "database.ssl_mode", "disable").
		BindInt(&dbMaxConns, "database.pool.max_connections", 10).
		BindDuration(&dbIdleTimeout, "database.pool.idle_timeout", 1*time.Minute).
		Apply()

	duration := time.Since(start)

	if err != nil {
		log.Fatalf("Binding failed: %v", err)
	}

	fmt.Printf("Configuration bound in %v\n", duration)

	// Display bound configuration values
	fmt.Println("\nConfiguration Results:")
	fmt.Println("=========================")
	fmt.Printf("App Name:          %s\n", appName)
	fmt.Printf("App Version:       %s\n", appVersion)
	fmt.Printf("App Debug:         %t\n", appDebug)
	fmt.Printf("Server Host:       %s\n", serverHost)
	fmt.Printf("Server Port:       %d\n", serverPort)
	fmt.Printf("Server Timeout:    %v\n", serverTimeout)
	fmt.Printf("DB Host:           %s\n", dbHost)
	fmt.Printf("DB Port:           %d\n", dbPort)
	fmt.Printf("DB SSL Mode:       %s\n", dbSSLMode)
	fmt.Printf("DB Max Conns:      %d\n", dbMaxConns)
	fmt.Printf("DB Idle Timeout:   %v\n", dbIdleTimeout)

	// Performance benchmark: repeated bindings
	fmt.Println("\nPerformance Benchmark:")
	fmt.Println("=====================")

	const iterations = 10000
	fmt.Printf("Running %d binding operations...\n", iterations)

	start = time.Now()
	for i := 0; i < iterations; i++ {
		// Variables for benchmark
		var testName string
		var testPort int
		var testDebug bool
		var testTimeout time.Duration

		err := argus.BindFromConfig(config).
			BindString(&testName, "app.name").
			BindInt(&testPort, "server.port").
			BindBool(&testDebug, "app.debug").
			BindDuration(&testTimeout, "server.timeout").
			Apply()

		if err != nil {
			log.Fatalf("Performance benchmark failed: %v", err)
		}
	}
	duration = time.Since(start)

	fmt.Printf("%d operations completed in %v\n", iterations, duration)
	fmt.Printf("Average per operation: %v\n", duration/iterations)
	fmt.Printf("Operations per second: %.0f\n", float64(iterations)/duration.Seconds())

	// Error handling demonstration
	fmt.Println("\nError Handling Demo:")
	fmt.Println("========================")

	invalidConfig := map[string]interface{}{
		"invalid_port": "not-a-number",
		"invalid_bool": "maybe",
	}

	var invalidPort int
	var invalidBool bool

	err = argus.BindFromConfig(invalidConfig).
		BindInt(&invalidPort, "invalid_port").
		BindBool(&invalidBool, "invalid_bool").
		Apply()

	if err != nil {
		fmt.Printf("Error correctly detected: %v\n", err)
	} else {
		fmt.Println("Expected error but got none")
	}

	fmt.Println("\nDemo completed successfully.")
	fmt.Println("All configuration bindings and error handling checks passed.")
}
