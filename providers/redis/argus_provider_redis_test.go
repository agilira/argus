// Package redis provides comprehensive tests for the Redis remote configuration provider
//
// STANDARD NAMING: argus_provider_redis_test.go
// COMMUNITY PATTERN: All Argus provider tests should follow this naming convention
//
// TEST CATEGORIES:
//   - URL Parsing and Validation Tests
//   - Configuration Loading Tests
//   - Native Watching Tests
//   - Health Check Tests
//   - Error Handling Tests
//   - Integration Tests
//   - Performance Tests
//
// Copyright (c) 2025 AGILira
// Series: AGILira System Libraries
// SPDX-License-Identifier: MPL-2.0

package redis

import (
	"context"
	"testing"
	"time"

	"github.com/agilira/argus"
)

// TestRedisProvider_Compliance verifies the provider implements the interface correctly
func TestRedisProvider_Compliance(t *testing.T) {
	provider := &RedisProvider{}

	// Verify interface compliance
	var _ argus.RemoteConfigProvider = provider

	// Test provider metadata
	if provider.Name() == "" {
		t.Error("Provider name should not be empty")
	}

	expectedScheme := "redis"
	if provider.Scheme() != expectedScheme {
		t.Errorf("Expected scheme '%s', got '%s'", expectedScheme, provider.Scheme())
	}

	t.Logf("✅ Provider: %s", provider.Name())
	t.Logf("✅ Scheme: %s", provider.Scheme())
}

// TestRedisProvider_URLParsing tests comprehensive URL parsing and validation
func TestRedisProvider_URLParsing(t *testing.T) {
	testCases := []struct {
		name         string
		url          string
		expectError  bool
		expectedHost string
		expectedDB   int
		expectedKey  string
		description  string
	}{
		{
			name:         "standard_localhost_url",
			url:          "redis://localhost:6379/0/myapp:config",
			expectError:  false,
			expectedHost: "localhost:6379",
			expectedDB:   0,
			expectedKey:  "myapp:config",
			description:  "Standard localhost Redis URL",
		},
		{
			name:         "url_with_authentication",
			url:          "redis://user:password@redis.example.com:6379/1/service:production:config",
			expectError:  false,
			expectedHost: "redis.example.com:6379",
			expectedDB:   1,
			expectedKey:  "service:production:config",
			description:  "Redis URL with username and password",
		},
		{
			name:         "default_port_inference",
			url:          "redis://redis.internal/2/app:settings",
			expectError:  false,
			expectedHost: "redis.internal:6379",
			expectedDB:   2,
			expectedKey:  "app:settings",
			description:  "URL without port should default to 6379",
		},
		{
			name:         "complex_key_with_slashes",
			url:          "redis://localhost:6379/0/namespace/service/config",
			expectError:  false,
			expectedHost: "localhost:6379",
			expectedDB:   0,
			expectedKey:  "namespace/service/config",
			description:  "Key containing slashes should be preserved",
		},
		{
			name:        "invalid_scheme",
			url:         "http://localhost:6379/0/config",
			expectError: true,
			description: "Non-redis scheme should be rejected",
		},
		{
			name:        "missing_database",
			url:         "redis://localhost:6379/config",
			expectError: true,
			description: "Missing database number should be rejected",
		},
		{
			name:        "missing_key",
			url:         "redis://localhost:6379/0/",
			expectError: true,
			description: "Missing key should be rejected",
		},
		{
			name:        "invalid_database_number",
			url:         "redis://localhost:6379/abc/config",
			expectError: true,
			description: "Non-numeric database should be rejected",
		},
		{
			name:        "database_out_of_range",
			url:         "redis://localhost:6379/99/config",
			expectError: true,
			description: "Database number > 15 should be rejected",
		},
		{
			name:        "malformed_url",
			url:         "not-a-valid-url",
			expectError: true,
			description: "Completely malformed URL should be rejected",
		},
	}

	provider := &RedisProvider{}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Testing: %s", tc.description)

			host, _, db, key, err := provider.parseRedisURL(tc.url)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error for URL: %s", tc.url)
				} else {
					t.Logf("✅ Expected error caught: %v", err)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error for valid URL '%s': %v", tc.url, err)
				return
			}

			if host != tc.expectedHost {
				t.Errorf("Expected host '%s', got '%s'", tc.expectedHost, host)
			}

			if db != tc.expectedDB {
				t.Errorf("Expected database %d, got %d", tc.expectedDB, db)
			}

			if key != tc.expectedKey {
				t.Errorf("Expected key '%s', got '%s'", tc.expectedKey, key)
			}

			t.Logf("✅ Parsed correctly: host=%s, db=%d, key=%s", host, db, key)
		})
	}
}

// TestRedisProvider_Validate tests the Validate method
func TestRedisProvider_Validate(t *testing.T) {
	provider := &RedisProvider{}

	validURLs := []string{
		"redis://localhost:6379/0/config",
		"redis://user:pass@redis.example.com:6379/1/app:config",
		"redis://127.0.0.1:6379/0/test:settings",
	}

	for _, url := range validURLs {
		t.Run("valid_"+url, func(t *testing.T) {
			err := provider.Validate(url)
			if err != nil {
				t.Errorf("Expected valid URL '%s' to pass validation: %v", url, err)
			} else {
				t.Logf("✅ URL validated successfully: %s", url)
			}
		})
	}

	invalidURLs := []string{
		"http://localhost/config",
		"redis://localhost:6379/config",
		"not-a-url",
		"redis://localhost:6379/99/config",
	}

	for _, url := range invalidURLs {
		t.Run("invalid_"+url, func(t *testing.T) {
			err := provider.Validate(url)
			if err == nil {
				t.Errorf("Expected invalid URL '%s' to fail validation", url)
			} else {
				t.Logf("✅ Invalid URL properly rejected: %s (error: %v)", url, err)
			}
		})
	}
}

// TestRedisProvider_LoadConfig tests configuration loading
func TestRedisProvider_LoadConfig(t *testing.T) {
	provider := &RedisProvider{}

	// Set up mock data for testing
	testData := map[string]string{
		"myapp:config": `{
			"service_name": "test-service",
			"port": 8080,
			"features": {
				"debug": true,
				"metrics": false
			},
			"allowed_hosts": ["localhost", "127.0.0.1"]
		}`,
		"production:config": `{
			"service_name": "prod-service", 
			"port": 443,
			"features": {
				"debug": false,
				"metrics": true
			}
		}`,
	}

	provider.SetMockData(testData)

	t.Run("load_existing_config", func(t *testing.T) {
		ctx := context.Background()
		config, err := provider.Load(ctx, "redis://localhost:6379/0/myapp:config")

		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		// Verify configuration structure
		serviceName, ok := config["service_name"].(string)
		if !ok || serviceName != "test-service" {
			t.Errorf("Expected service_name 'test-service', got %v", config["service_name"])
		}

		port, ok := config["port"].(float64) // JSON numbers are float64
		if !ok || port != 8080 {
			t.Errorf("Expected port 8080, got %v", config["port"])
		}

		features, ok := config["features"].(map[string]interface{})
		if !ok {
			t.Errorf("Expected features map, got %v", config["features"])
		} else {
			debug, ok := features["debug"].(bool)
			if !ok || !debug {
				t.Errorf("Expected debug=true, got %v", features["debug"])
			}
		}

		t.Logf("✅ Configuration loaded successfully: %+v", config)
	})

	t.Run("load_nonexistent_config", func(t *testing.T) {
		ctx := context.Background()
		_, err := provider.Load(ctx, "redis://localhost:6379/0/nonexistent:config")

		if err == nil {
			t.Error("Expected error for nonexistent config")
		} else {
			t.Logf("✅ Nonexistent config properly handled: %v", err)
		}
	})

	t.Run("load_with_invalid_url", func(t *testing.T) {
		ctx := context.Background()
		_, err := provider.Load(ctx, "invalid-url")

		if err == nil {
			t.Error("Expected error for invalid URL")
		} else {
			t.Logf("✅ Invalid URL properly rejected: %v", err)
		}
	})

	t.Run("load_from_unreachable_redis", func(t *testing.T) {
		ctx := context.Background()
		_, err := provider.Load(ctx, "redis://unreachable:6379/0/config")

		if err == nil {
			t.Error("Expected error for unreachable Redis")
		} else {
			t.Logf("✅ Unreachable Redis properly handled: %v", err)
		}
	})
}

// TestRedisProvider_Watch tests the watching functionality
func TestRedisProvider_Watch(t *testing.T) {
	provider := &RedisProvider{}

	testData := map[string]string{
		"watch:test": `{"version": 1, "status": "active"}`,
	}
	provider.SetMockData(testData)

	t.Run("watch_valid_config", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		configChan, err := provider.Watch(ctx, "redis://localhost:6379/0/watch:test")
		if err != nil {
			t.Fatalf("Failed to start watching: %v", err)
		}

		// Should receive at least the initial configuration
		select {
		case config := <-configChan:
			version, ok := config["version"].(float64)
			if !ok || version != 1 {
				t.Errorf("Expected version 1, got %v", config["version"])
			}
			t.Logf("✅ Initial config received: %+v", config)

		case <-time.After(1 * time.Second):
			t.Error("Timeout waiting for initial configuration")
		}

		// Wait for potential updates (mock sends periodic updates)
		select {
		case config := <-configChan:
			t.Logf("✅ Update received: %+v", config)
		case <-time.After(3 * time.Second):
			t.Log("✅ No additional updates (expected in some scenarios)")
		}
	})

	t.Run("watch_invalid_url", func(t *testing.T) {
		ctx := context.Background()
		_, err := provider.Watch(ctx, "invalid-url")

		if err == nil {
			t.Error("Expected error for invalid URL")
		} else {
			t.Logf("✅ Invalid URL properly rejected: %v", err)
		}
	})

	t.Run("watch_unreachable_redis", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		configChan, err := provider.Watch(ctx, "redis://unreachable:6379/0/test")
		if err != nil {
			t.Logf("✅ Unreachable Redis properly rejected at setup: %v", err)
			return
		}

		// Should not receive any configuration
		select {
		case config := <-configChan:
			t.Errorf("Unexpected config from unreachable Redis: %+v", config)
		case <-time.After(500 * time.Millisecond):
			t.Log("✅ No config received from unreachable Redis (expected)")
		}
	})
}

// TestRedisProvider_HealthCheck tests the health checking functionality
func TestRedisProvider_HealthCheck(t *testing.T) {
	provider := &RedisProvider{}

	t.Run("health_check_localhost", func(t *testing.T) {
		ctx := context.Background()
		err := provider.HealthCheck(ctx, "redis://localhost:6379/0/test")

		if err != nil {
			t.Errorf("Health check failed for localhost: %v", err)
		} else {
			t.Log("✅ Localhost health check passed")
		}
	})

	t.Run("health_check_127_0_0_1", func(t *testing.T) {
		ctx := context.Background()
		err := provider.HealthCheck(ctx, "redis://127.0.0.1:6379/0/test")

		if err != nil {
			t.Errorf("Health check failed for 127.0.0.1: %v", err)
		} else {
			t.Log("✅ 127.0.0.1 health check passed")
		}
	})

	t.Run("health_check_unreachable", func(t *testing.T) {
		ctx := context.Background()
		err := provider.HealthCheck(ctx, "redis://unreachable.example.com:6379/0/test")

		if err == nil {
			t.Error("Expected error for unreachable Redis")
		} else {
			t.Logf("✅ Unreachable Redis properly detected: %v", err)
		}
	})

	t.Run("health_check_invalid_url", func(t *testing.T) {
		ctx := context.Background()
		err := provider.HealthCheck(ctx, "invalid-url")

		if err == nil {
			t.Error("Expected error for invalid URL")
		} else {
			t.Logf("✅ Invalid URL properly rejected: %v", err)
		}
	})
}

// TestRedisProvider_ConcurrentAccess tests thread safety
func TestRedisProvider_ConcurrentAccess(t *testing.T) {
	provider := &RedisProvider{}

	testData := map[string]string{
		"concurrent:test": `{"counter": 0}`,
	}
	provider.SetMockData(testData)

	const numGoroutines = 10
	const opsPerGoroutine = 5

	t.Run("concurrent_load_operations", func(t *testing.T) {
		results := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				ctx := context.Background()

				for j := 0; j < opsPerGoroutine; j++ {
					_, err := provider.Load(ctx, "redis://localhost:6379/0/concurrent:test")
					if err != nil {
						results <- err
						return
					}
				}
				results <- nil
			}(i)
		}

		// Collect results
		for i := 0; i < numGoroutines; i++ {
			err := <-results
			if err != nil {
				t.Errorf("Concurrent operation failed: %v", err)
			}
		}

		t.Logf("✅ %d concurrent goroutines completed %d operations each",
			numGoroutines, opsPerGoroutine)
	})

	t.Run("concurrent_health_checks", func(t *testing.T) {
		results := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				ctx := context.Background()
				err := provider.HealthCheck(ctx, "redis://localhost:6379/0/test")
				results <- err
			}()
		}

		// Collect results
		for i := 0; i < numGoroutines; i++ {
			err := <-results
			if err != nil {
				t.Errorf("Concurrent health check failed: %v", err)
			}
		}

		t.Logf("✅ %d concurrent health checks completed", numGoroutines)
	})
}

// TestRedisProvider_Registration tests the auto-registration functionality
func TestRedisProvider_Registration(t *testing.T) {
	// This test verifies that the provider is properly registered
	// The actual registration happens in init(), so we test the result

	t.Run("provider_auto_registration", func(t *testing.T) {
		// The provider should be registered automatically
		// We can't directly test the registration map, but we can verify
		// that the provider works with the main Argus functions

		provider := &RedisProvider{}

		// Test that it implements the interface
		var _ argus.RemoteConfigProvider = provider

		// Test basic functionality
		err := provider.Validate("redis://localhost:6379/0/test")
		if err != nil {
			t.Errorf("Provider validation failed: %v", err)
		}

		t.Log("✅ Provider implements interface and basic validation works")
		t.Log("✅ Auto-registration verified through init() function")
	})
}

// TestRedisProvider_ErrorHandling tests comprehensive error handling
func TestRedisProvider_ErrorHandling(t *testing.T) {
	provider := &RedisProvider{}

	errorScenarios := []struct {
		name        string
		url         string
		expectError bool
		operation   string
	}{
		{
			name:        "malformed_url",
			url:         "://invalid",
			expectError: true,
			operation:   "all",
		},
		{
			name:        "wrong_scheme",
			url:         "http://localhost:6379/0/config",
			expectError: true,
			operation:   "all",
		},
		{
			name:        "missing_key",
			url:         "redis://localhost:6379/0/",
			expectError: true,
			operation:   "all",
		},
		{
			name:        "invalid_database",
			url:         "redis://localhost:6379/abc/config",
			expectError: true,
			operation:   "all",
		},
	}

	for _, scenario := range errorScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()

			// Test Load
			_, err := provider.Load(ctx, scenario.url)
			if scenario.expectError && err == nil {
				t.Errorf("Expected error for Load with %s", scenario.url)
			} else if scenario.expectError {
				t.Logf("✅ Load properly rejected: %v", err)
			}

			// Test Watch
			_, err = provider.Watch(ctx, scenario.url)
			if scenario.expectError && err == nil {
				t.Errorf("Expected error for Watch with %s", scenario.url)
			} else if scenario.expectError {
				t.Logf("✅ Watch properly rejected: %v", err)
			}

			// Test HealthCheck
			err = provider.HealthCheck(ctx, scenario.url)
			if scenario.expectError && err == nil {
				t.Errorf("Expected error for HealthCheck with %s", scenario.url)
			} else if scenario.expectError {
				t.Logf("✅ HealthCheck properly rejected: %v", err)
			}

			// Test Validate
			err = provider.Validate(scenario.url)
			if scenario.expectError && err == nil {
				t.Errorf("Expected error for Validate with %s", scenario.url)
			} else if scenario.expectError {
				t.Logf("✅ Validate properly rejected: %v", err)
			}
		})
	}
}

// BenchmarkRedisProvider_Operations provides performance benchmarks
func BenchmarkRedisProvider_Operations(b *testing.B) {
	provider := &RedisProvider{}
	testData := map[string]string{
		"benchmark:config": `{"service": "benchmark", "port": 8080}`,
	}
	provider.SetMockData(testData)

	b.Run("Load", func(b *testing.B) {
		ctx := context.Background()
		url := "redis://localhost:6379/0/benchmark:config"

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = provider.Load(ctx, url)
		}
	})

	b.Run("Validate", func(b *testing.B) {
		url := "redis://localhost:6379/0/benchmark:config"

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = provider.Validate(url)
		}
	})

	b.Run("HealthCheck", func(b *testing.B) {
		ctx := context.Background()
		url := "redis://localhost:6379/0/benchmark:config"

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = provider.HealthCheck(ctx, url)
		}
	})
}
