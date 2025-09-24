// enterprise_validation_test.go: Enterprise Validation Test Suite
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agilira/argus"
)

func TestValidConfiguration(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "argus_validation_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	// Test valid configuration
	validConfig := &argus.Config{
		PollInterval:         2 * time.Second,
		CacheTTL:             1 * time.Second,
		MaxWatchedFiles:      50,
		OptimizationStrategy: argus.OptimizationAuto,
		BoreasLiteCapacity:   256, // Power of 2
		Audit: argus.AuditConfig{
			Enabled:       true,
			BufferSize:    1000,
			FlushInterval: 5 * time.Second,
			OutputFile:    filepath.Join(tempDir, "audit.log"),
		},
	}

	result := validConfig.ValidateDetailed()
	if !result.Valid {
		t.Errorf("Expected valid configuration, but got errors: %v", result.Errors)
	}

	if len(result.Errors) != 0 {
		t.Errorf("Expected 0 errors, got %d: %v", len(result.Errors), result.Errors)
	}

	// Should have minimal warnings for a well-configured system
	if len(result.Warnings) > 2 {
		t.Errorf("Expected minimal warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}
}

func TestInvalidConfiguration(t *testing.T) {
	// Test invalid configuration
	invalidConfig := &argus.Config{
		PollInterval:         -1 * time.Second,                // INVALID: negative
		CacheTTL:             5 * time.Second,                 // WARNING: larger than poll interval
		MaxWatchedFiles:      0,                               // INVALID: zero
		OptimizationStrategy: argus.OptimizationStrategy(999), // INVALID: unknown strategy
		BoreasLiteCapacity:   15,                              // INVALID: not power of 2
		Audit: argus.AuditConfig{
			Enabled:       true,
			BufferSize:    -1,                       // INVALID: negative
			FlushInterval: -2 * time.Second,         // INVALID: negative
			OutputFile:    "invalid/path/audit.log", // INVALID: invalid path
		},
	}

	result := invalidConfig.ValidateDetailed()
	if result.Valid {
		t.Error("Expected invalid configuration, but got valid")
	}

	if len(result.Errors) < 5 {
		t.Errorf("Expected at least 5 errors, got %d: %v", len(result.Errors), result.Errors)
	}

	// Verify specific error types
	errorCodes := make(map[string]bool)
	for _, err := range result.Errors {
		errorCodes[err] = true
	}

	expectedErrors := []string{
		"ARGUS_INVALID_POLL_INTERVAL",
		"ARGUS_INVALID_MAX_WATCHED_FILES",
		"ARGUS_INVALID_OPTIMIZATION",
		"ARGUS_INVALID_BOREAS_CAPACITY",
		"ARGUS_INVALID_BUFFER_SIZE",
		"ARGUS_INVALID_FLUSH_INTERVAL",
		"ARGUS_INVALID_CONFIG",
	}

	for _, expectedError := range expectedErrors {
		found := false
		for err := range errorCodes {
			if contains(err, expectedError) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected error code %s not found in errors: %v", expectedError, result.Errors)
		}
	}
}

func TestPerformanceWarnings(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "argus_performance_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	// Test configuration with performance warnings
	performanceConfig := &argus.Config{
		PollInterval:         50 * time.Millisecond,         // Fast polling
		CacheTTL:             25 * time.Millisecond,         // Half of poll interval
		MaxWatchedFiles:      500,                           // Many files
		OptimizationStrategy: argus.OptimizationSingleEvent, // Suboptimal for many files
		BoreasLiteCapacity:   4096,                          // Large capacity
		Audit: argus.AuditConfig{
			Enabled:       true,
			BufferSize:    20000,                  // Large buffer
			FlushInterval: 100 * time.Millisecond, // Frequent flushing
			OutputFile:    filepath.Join(tempDir, "audit.log"),
		},
	}

	result := performanceConfig.ValidateDetailed()
	if !result.Valid {
		t.Errorf("Expected valid configuration, but got errors: %v", result.Errors)
	}

	if len(result.Warnings) < 4 {
		t.Errorf("Expected at least 4 performance warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}

	// Verify specific warning types
	warningText := ""
	for _, warning := range result.Warnings {
		warningText += warning + " "
	}

	expectedWarnings := []string{
		"BoreasLite capacity",
		"audit buffer size",
		"Fast polling",
		"Frequent audit flushing",
		"optimization",
	}

	for _, expectedWarning := range expectedWarnings {
		if !contains(warningText, expectedWarning) {
			t.Errorf("Expected warning about '%s' not found in warnings: %v", expectedWarning, result.Warnings)
		}
	}
}

func TestEnvironmentValidation(t *testing.T) {
	// Test environment variable validation
	originalEnv := make(map[string]string)
	envVars := []string{
		"ARGUS_POLL_INTERVAL",
		"ARGUS_CACHE_TTL",
		"ARGUS_MAX_WATCHED_FILES",
		"ARGUS_AUDIT_OUTPUT_FILE",
		"ARGUS_AUDIT_ENABLED",
	}

	// Save original environment
	for _, envVar := range envVars {
		originalEnv[envVar] = os.Getenv(envVar)
	}

	// Clean up after test
	defer func() {
		for _, envVar := range envVars {
			if originalEnv[envVar] == "" {
				os.Unsetenv(envVar)
			} else {
				os.Setenv(envVar, originalEnv[envVar])
			}
		}
	}()

	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "argus_env_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	// Set valid environment variables
	os.Setenv("ARGUS_POLL_INTERVAL", "3s")
	os.Setenv("ARGUS_CACHE_TTL", "1s")
	os.Setenv("ARGUS_MAX_WATCHED_FILES", "200")
	os.Setenv("ARGUS_AUDIT_OUTPUT_FILE", filepath.Join(tempDir, "env_audit.log"))
	os.Setenv("ARGUS_AUDIT_ENABLED", "true")

	// Test environment validation
	err = argus.ValidateEnvironmentConfig()
	if err != nil {
		t.Errorf("Environment validation failed: %v", err)
	}
}

func TestQuickValidation(t *testing.T) {
	// Test quick validation (errors only)
	quickConfig := &argus.Config{
		PollInterval:    -5 * time.Second, // Invalid
		CacheTTL:        1 * time.Second,
		MaxWatchedFiles: 100,
		Audit: argus.AuditConfig{
			Enabled: false, // Disable audit to focus on poll interval error
		},
	}

	err := quickConfig.Validate()
	if err == nil {
		t.Error("Expected validation error for invalid poll interval")
	}

	// Verify error message contains expected information
	errorMsg := err.Error()
	if !contains(errorMsg, "ARGUS_INVALID_POLL_INTERVAL") {
		t.Errorf("Expected error message to contain 'ARGUS_INVALID_POLL_INTERVAL', got: %s", errorMsg)
	}
}

func TestValidationEdgeCases(t *testing.T) {
	t.Run("ZeroValues", func(t *testing.T) {
		config := &argus.Config{
			PollInterval:    0,
			CacheTTL:        0,
			MaxWatchedFiles: 0,
			Audit: argus.AuditConfig{
				Enabled: false,
			},
		}

		result := config.ValidateDetailed()
		if result.Valid {
			t.Error("Expected invalid configuration with zero values")
		}

		if len(result.Errors) < 2 {
			t.Errorf("Expected at least 2 errors for zero values, got %d", len(result.Errors))
		}
	})

	t.Run("NegativeValues", func(t *testing.T) {
		config := &argus.Config{
			PollInterval:    -1 * time.Second,
			CacheTTL:        -1 * time.Second,
			MaxWatchedFiles: -1,
			Audit: argus.AuditConfig{
				Enabled:       true,
				BufferSize:    -1,
				FlushInterval: -1 * time.Second,
			},
		}

		result := config.ValidateDetailed()
		if result.Valid {
			t.Error("Expected invalid configuration with negative values")
		}

		if len(result.Errors) < 4 {
			t.Errorf("Expected at least 4 errors for negative values, got %d", len(result.Errors))
		}
	})

	t.Run("InvalidOptimizationStrategy", func(t *testing.T) {
		config := &argus.Config{
			PollInterval:         1 * time.Second,
			CacheTTL:             1 * time.Second,
			MaxWatchedFiles:      10,
			OptimizationStrategy: argus.OptimizationStrategy(999),
			Audit: argus.AuditConfig{
				Enabled: false,
			},
		}

		result := config.ValidateDetailed()
		if result.Valid {
			t.Error("Expected invalid configuration with unknown optimization strategy")
		}

		found := false
		for _, err := range result.Errors {
			if contains(err, "ARGUS_INVALID_OPTIMIZATION") {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected ARGUS_INVALID_OPTIMIZATION error not found")
		}
	})

	t.Run("InvalidBoreasCapacity", func(t *testing.T) {
		config := &argus.Config{
			PollInterval:       1 * time.Second,
			CacheTTL:           1 * time.Second,
			MaxWatchedFiles:    10,
			BoreasLiteCapacity: 15, // Not a power of 2
			Audit: argus.AuditConfig{
				Enabled: false,
			},
		}

		result := config.ValidateDetailed()
		if result.Valid {
			t.Error("Expected invalid configuration with non-power-of-2 BoreasLite capacity")
		}

		found := false
		for _, err := range result.Errors {
			if contains(err, "ARGUS_INVALID_BOREAS_CAPACITY") {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected ARGUS_INVALID_BOREAS_CAPACITY error not found")
		}
	})
}

func TestAuditConfigurationValidation(t *testing.T) {
	t.Run("ValidAuditConfig", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "argus_audit_test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer func() {
			if err := os.RemoveAll(tempDir); err != nil {
				t.Logf("Failed to remove temp dir: %v", err)
			}
		}()

		config := &argus.Config{
			PollInterval:    1 * time.Second,
			CacheTTL:        1 * time.Second,
			MaxWatchedFiles: 10,
			Audit: argus.AuditConfig{
				Enabled:       true,
				BufferSize:    1000,
				FlushInterval: 5 * time.Second,
				OutputFile:    filepath.Join(tempDir, "audit.log"),
			},
		}

		result := config.ValidateDetailed()
		if !result.Valid {
			t.Errorf("Expected valid audit configuration, but got errors: %v", result.Errors)
		}
	})

	t.Run("InvalidAuditConfig", func(t *testing.T) {
		config := &argus.Config{
			PollInterval:    1 * time.Second,
			CacheTTL:        1 * time.Second,
			MaxWatchedFiles: 10,
			Audit: argus.AuditConfig{
				Enabled:       true,
				BufferSize:    -1,
				FlushInterval: -1 * time.Second,
				OutputFile:    "invalid/path/audit.log",
			},
		}

		result := config.ValidateDetailed()
		if result.Valid {
			t.Error("Expected invalid audit configuration")
		}

		if len(result.Errors) < 3 {
			t.Errorf("Expected at least 3 audit errors, got %d", len(result.Errors))
		}
	})

	t.Run("DisabledAuditConfig", func(t *testing.T) {
		config := &argus.Config{
			PollInterval:    1 * time.Second,
			CacheTTL:        1 * time.Second,
			MaxWatchedFiles: 10,
			Audit: argus.AuditConfig{
				Enabled: false,
			},
		}

		result := config.ValidateDetailed()
		if !result.Valid {
			t.Errorf("Expected valid configuration with disabled audit, but got errors: %v", result.Errors)
		}
	})
}

func TestValidationPerformance(t *testing.T) {
	// Test validation performance
	config := &argus.Config{
		PollInterval:         1 * time.Second,
		CacheTTL:             1 * time.Second,
		MaxWatchedFiles:      100,
		OptimizationStrategy: argus.OptimizationAuto,
		BoreasLiteCapacity:   256,
		Audit: argus.AuditConfig{
			Enabled: false,
		},
	}

	const iterations = 1000
	start := time.Now()

	for i := 0; i < iterations; i++ {
		result := config.ValidateDetailed()
		if !result.Valid {
			t.Fatalf("Validation failed at iteration %d: %v", i, result.Errors)
		}
	}

	duration := time.Since(start)
	avgTime := duration / iterations

	t.Logf("Validation performance: %d iterations in %v", iterations, duration)
	t.Logf("Average time per validation: %v", avgTime)
	t.Logf("Validations per second: %.0f", float64(iterations)/duration.Seconds())

	// Validation should be very fast (less than 100µs per operation)
	if avgTime > 100*time.Microsecond {
		t.Errorf("Validation too slow: %v per operation (expected < 100µs)", avgTime)
	}
}

func TestValidationConcurrency(t *testing.T) {
	// Test concurrent validation
	const goroutines = 10
	const iterationsPerGoroutine = 100

	config := &argus.Config{
		PollInterval:    1 * time.Second,
		CacheTTL:        1 * time.Second,
		MaxWatchedFiles: 10,
		Audit: argus.AuditConfig{
			Enabled: false,
		},
	}

	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(goroutineID int) {
			defer func() { done <- true }()

			for j := 0; j < iterationsPerGoroutine; j++ {
				result := config.ValidateDetailed()
				if !result.Valid {
					t.Errorf("Validation failed in goroutine %d, iteration %d: %v", goroutineID, j, result.Errors)
					return
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < goroutines; i++ {
		<-done
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				contains(s[1:], substr))))
}
