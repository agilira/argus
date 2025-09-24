// config_binding_test.go: Configuration Binding Test
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/agilira/argus"
)

func TestConfigBinding_BasicFunctionality(t *testing.T) {
	// Test configuration
	config := map[string]interface{}{
		"app": map[string]interface{}{
			"name":    "test-service",
			"version": "2.0.0",
			"debug":   true,
		},
		"server": map[string]interface{}{
			"host":    "0.0.0.0",
			"port":    9090,
			"timeout": "45s",
		},
		"database": map[string]interface{}{
			"host":     "test-db.example.com",
			"port":     3306,
			"ssl_mode": "prefer",
			"pool": map[string]interface{}{
				"max_connections": 50,
				"idle_timeout":    "10m",
			},
		},
	}

	// Test variables
	var (
		appName       string
		appVersion    string
		appDebug      bool
		serverHost    string
		serverPort    int
		serverTimeout time.Duration
		dbHost        string
		dbPort        int
		dbSSLMode     string
		dbMaxConns    int
		dbIdleTimeout time.Duration
	)

	// Test binding
	err := argus.BindFromConfig(config).
		BindString(&appName, "app.name", "default-service").
		BindString(&appVersion, "app.version", "0.0.1").
		BindBool(&appDebug, "app.debug", false).
		BindString(&serverHost, "server.host", "localhost").
		BindInt(&serverPort, "server.port", 3000).
		BindDuration(&serverTimeout, "server.timeout", 10*time.Second).
		BindString(&dbHost, "database.host", "localhost").
		BindInt(&dbPort, "database.port", 5432).
		BindString(&dbSSLMode, "database.ssl_mode", "disable").
		BindInt(&dbMaxConns, "database.pool.max_connections", 10).
		BindDuration(&dbIdleTimeout, "database.pool.idle_timeout", 1*time.Minute).
		Apply()

	if err != nil {
		t.Fatalf("Binding failed: %v", err)
	}

	// Verify results
	if appName != "test-service" {
		t.Errorf("Expected appName 'test-service', got '%s'", appName)
	}
	if appVersion != "2.0.0" {
		t.Errorf("Expected appVersion '2.0.0', got '%s'", appVersion)
	}
	if !appDebug {
		t.Errorf("Expected appDebug true, got %t", appDebug)
	}
	if serverHost != "0.0.0.0" {
		t.Errorf("Expected serverHost '0.0.0.0', got '%s'", serverHost)
	}
	if serverPort != 9090 {
		t.Errorf("Expected serverPort 9090, got %d", serverPort)
	}
	if serverTimeout != 45*time.Second {
		t.Errorf("Expected serverTimeout 45s, got %v", serverTimeout)
	}
	if dbHost != "test-db.example.com" {
		t.Errorf("Expected dbHost 'test-db.example.com', got '%s'", dbHost)
	}
	if dbPort != 3306 {
		t.Errorf("Expected dbPort 3306, got %d", dbPort)
	}
	if dbSSLMode != "prefer" {
		t.Errorf("Expected dbSSLMode 'prefer', got '%s'", dbSSLMode)
	}
	if dbMaxConns != 50 {
		t.Errorf("Expected dbMaxConns 50, got %d", dbMaxConns)
	}
	if dbIdleTimeout != 10*time.Minute {
		t.Errorf("Expected dbIdleTimeout 10m, got %v", dbIdleTimeout)
	}
}

func TestConfigBinding_DefaultValues(t *testing.T) {
	// Test configuration with missing keys
	config := map[string]interface{}{
		"app": map[string]interface{}{
			"name": "partial-service",
		},
		// Missing server and database sections
	}

	// Test variables
	var (
		appName       string
		appVersion    string
		appDebug      bool
		serverHost    string
		serverPort    int
		serverTimeout time.Duration
		dbHost        string
		dbPort        int
		dbSSLMode     string
		dbMaxConns    int
		dbIdleTimeout time.Duration
	)

	// Test binding with defaults
	err := argus.BindFromConfig(config).
		BindString(&appName, "app.name", "default-service").
		BindString(&appVersion, "app.version", "0.0.1").
		BindBool(&appDebug, "app.debug", false).
		BindString(&serverHost, "server.host", "localhost").
		BindInt(&serverPort, "server.port", 3000).
		BindDuration(&serverTimeout, "server.timeout", 10*time.Second).
		BindString(&dbHost, "database.host", "localhost").
		BindInt(&dbPort, "database.port", 5432).
		BindString(&dbSSLMode, "database.ssl_mode", "disable").
		BindInt(&dbMaxConns, "database.pool.max_connections", 10).
		BindDuration(&dbIdleTimeout, "database.pool.idle_timeout", 1*time.Minute).
		Apply()

	if err != nil {
		t.Fatalf("Binding failed: %v", err)
	}

	// Verify results - should use provided values and defaults
	if appName != "partial-service" {
		t.Errorf("Expected appName 'partial-service', got '%s'", appName)
	}
	if appVersion != "0.0.1" {
		t.Errorf("Expected appVersion '0.0.1' (default), got '%s'", appVersion)
	}
	if appDebug {
		t.Errorf("Expected appDebug false (default), got %t", appDebug)
	}
	if serverHost != "localhost" {
		t.Errorf("Expected serverHost 'localhost' (default), got '%s'", serverHost)
	}
	if serverPort != 3000 {
		t.Errorf("Expected serverPort 3000 (default), got %d", serverPort)
	}
	if serverTimeout != 10*time.Second {
		t.Errorf("Expected serverTimeout 10s (default), got %v", serverTimeout)
	}
	if dbHost != "localhost" {
		t.Errorf("Expected dbHost 'localhost' (default), got '%s'", dbHost)
	}
	if dbPort != 5432 {
		t.Errorf("Expected dbPort 5432 (default), got %d", dbPort)
	}
	if dbSSLMode != "disable" {
		t.Errorf("Expected dbSSLMode 'disable' (default), got '%s'", dbSSLMode)
	}
	if dbMaxConns != 10 {
		t.Errorf("Expected dbMaxConns 10 (default), got %d", dbMaxConns)
	}
	if dbIdleTimeout != 1*time.Minute {
		t.Errorf("Expected dbIdleTimeout 1m (default), got %v", dbIdleTimeout)
	}
}

func TestConfigBinding_ErrorHandling(t *testing.T) {
	// Test configuration with invalid values
	config := map[string]interface{}{
		"invalid_port":     "not-a-number",
		"invalid_bool":     "maybe",
		"invalid_duration": "not-a-duration",
	}

	// Test variables
	var (
		invalidPort     int
		invalidBool     bool
		invalidDuration time.Duration
	)

	// Test binding with invalid values
	err := argus.BindFromConfig(config).
		BindInt(&invalidPort, "invalid_port").
		BindBool(&invalidBool, "invalid_bool").
		BindDuration(&invalidDuration, "invalid_duration").
		Apply()

	if err == nil {
		t.Fatal("Expected error for invalid configuration values, but got none")
	}

	// Verify error message contains expected information
	errorMsg := err.Error()
	if !contains(errorMsg, "invalid_port") {
		t.Errorf("Expected error message to mention 'invalid_port', got: %s", errorMsg)
	}
}

func TestConfigBinding_Performance(t *testing.T) {
	// Test configuration
	config := map[string]interface{}{
		"app": map[string]interface{}{
			"name":    "perf-test",
			"version": "1.0.0",
			"debug":   true,
		},
		"server": map[string]interface{}{
			"host":    "localhost",
			"port":    8080,
			"timeout": "30s",
		},
	}

	const iterations = 1000
	start := time.Now()

	for i := 0; i < iterations; i++ {
		var (
			appName       string
			appVersion    string
			appDebug      bool
			serverHost    string
			serverPort    int
			serverTimeout time.Duration
		)

		err := argus.BindFromConfig(config).
			BindString(&appName, "app.name").
			BindString(&appVersion, "app.version").
			BindBool(&appDebug, "app.debug").
			BindString(&serverHost, "server.host").
			BindInt(&serverPort, "server.port").
			BindDuration(&serverTimeout, "server.timeout").
			Apply()

		if err != nil {
			t.Fatalf("Performance test failed at iteration %d: %v", i, err)
		}

		// Verify values are correct
		if appName != "perf-test" || appVersion != "1.0.0" || !appDebug {
			t.Fatalf("Performance test values incorrect at iteration %d", i)
		}
	}

	duration := time.Since(start)
	avgTime := duration / iterations

	t.Logf("Performance test: %d iterations in %v", iterations, duration)
	t.Logf("Average time per operation: %v", avgTime)
	t.Logf("Operations per second: %.0f", float64(iterations)/duration.Seconds())

	// Performance should be very fast (less than 10µs per operation)
	if avgTime > 10*time.Microsecond {
		t.Errorf("Performance too slow: %v per operation (expected < 10µs)", avgTime)
	}
}

func TestConfigBinding_JSONParsing(t *testing.T) {
	// Test JSON parsing like in the example
	jsonConfig := `{
		"app": {
			"name": "json-test",
			"version": "1.0.0",
			"debug": true
		},
		"server": {
			"host": "localhost",
			"port": 8080,
			"timeout": "30s"
		}
	}`

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(jsonConfig), &config); err != nil {
		t.Fatalf("JSON parsing failed: %v", err)
	}

	// Test binding from parsed JSON
	var (
		appName       string
		appVersion    string
		appDebug      bool
		serverHost    string
		serverPort    int
		serverTimeout time.Duration
	)

	err := argus.BindFromConfig(config).
		BindString(&appName, "app.name").
		BindString(&appVersion, "app.version").
		BindBool(&appDebug, "app.debug").
		BindString(&serverHost, "server.host").
		BindInt(&serverPort, "server.port").
		BindDuration(&serverTimeout, "server.timeout").
		Apply()

	if err != nil {
		t.Fatalf("JSON binding failed: %v", err)
	}

	// Verify results
	if appName != "json-test" {
		t.Errorf("Expected appName 'json-test', got '%s'", appName)
	}
	if appVersion != "1.0.0" {
		t.Errorf("Expected appVersion '1.0.0', got '%s'", appVersion)
	}
	if !appDebug {
		t.Errorf("Expected appDebug true, got %t", appDebug)
	}
	if serverHost != "localhost" {
		t.Errorf("Expected serverHost 'localhost', got '%s'", serverHost)
	}
	if serverPort != 8080 {
		t.Errorf("Expected serverPort 8080, got %d", serverPort)
	}
	if serverTimeout != 30*time.Second {
		t.Errorf("Expected serverTimeout 30s, got %v", serverTimeout)
	}
}

func TestConfigBinding_EdgeCases(t *testing.T) {
	t.Run("EmptyConfig", func(t *testing.T) {
		config := map[string]interface{}{}

		var (
			appName    string
			serverPort int
			appDebug   bool
		)

		err := argus.BindFromConfig(config).
			BindString(&appName, "app.name", "default").
			BindInt(&serverPort, "server.port", 3000).
			BindBool(&appDebug, "app.debug", false).
			Apply()

		if err != nil {
			t.Fatalf("Empty config binding failed: %v", err)
		}

		if appName != "default" || serverPort != 3000 || appDebug {
			t.Errorf("Default values not applied correctly")
		}
	})

	t.Run("NilConfig", func(t *testing.T) {
		var (
			appName    string
			serverPort int
		)

		err := argus.BindFromConfig(nil).
			BindString(&appName, "app.name", "default").
			BindInt(&serverPort, "server.port", 3000).
			Apply()

		if err != nil {
			t.Fatalf("Nil config binding failed: %v", err)
		}

		if appName != "default" || serverPort != 3000 {
			t.Errorf("Default values not applied correctly for nil config")
		}
	})

	t.Run("NestedKeys", func(t *testing.T) {
		config := map[string]interface{}{
			"database": map[string]interface{}{
				"pool": map[string]interface{}{
					"max_connections": 25,
					"idle_timeout":    "5m",
				},
			},
		}

		var (
			maxConns    int
			idleTimeout time.Duration
		)

		err := argus.BindFromConfig(config).
			BindInt(&maxConns, "database.pool.max_connections").
			BindDuration(&idleTimeout, "database.pool.idle_timeout").
			Apply()

		if err != nil {
			t.Fatalf("Nested keys binding failed: %v", err)
		}

		if maxConns != 25 {
			t.Errorf("Expected maxConns 25, got %d", maxConns)
		}
		if idleTimeout != 5*time.Minute {
			t.Errorf("Expected idleTimeout 5m, got %v", idleTimeout)
		}
	})
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				contains(s[1:], substr))))
}
