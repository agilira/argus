// env_config_test.go: Tests for Environment Variables Support
//
// Copyright (c) 2025 AGILira
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfigFromEnv(t *testing.T) {
	// Setup test environment
	setupTestEnv(t)

	// Load and verify configuration
	config, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("Failed to load config from env: %v", err)
	}

	// Verify all configuration sections
	verifyCoreConfig(t, config)
	verifyOptimizationConfig(t, config)
	verifyAuditConfig(t, config)
}

// setupTestEnv sets up test environment variables
func setupTestEnv(t *testing.T) {
	envVars := map[string]string{
		"ARGUS_POLL_INTERVAL":         "10s",
		"ARGUS_CACHE_TTL":             "5s",
		"ARGUS_MAX_WATCHED_FILES":     "50",
		"ARGUS_OPTIMIZATION_STRATEGY": "smallbatch",
		"ARGUS_BOREAS_CAPACITY":       "256",
		"ARGUS_AUDIT_ENABLED":         "true",
		"ARGUS_AUDIT_OUTPUT_FILE":     "/tmp/argus-test.jsonl",
		"ARGUS_AUDIT_MIN_LEVEL":       "warn",
		"ARGUS_AUDIT_BUFFER_SIZE":     "500",
		"ARGUS_AUDIT_FLUSH_INTERVAL":  "3s",
	}

	for key, value := range envVars {
		os.Setenv(key, value)
	}

	t.Cleanup(func() {
		for key := range envVars {
			os.Unsetenv(key)
		}
	})
}

// verifyCoreConfig verifies core configuration settings
func verifyCoreConfig(t *testing.T, config *Config) {
	if config.PollInterval != 10*time.Second {
		t.Errorf("Expected PollInterval 10s, got %v", config.PollInterval)
	}
	if config.CacheTTL != 5*time.Second {
		t.Errorf("Expected CacheTTL 5s, got %v", config.CacheTTL)
	}
	if config.MaxWatchedFiles != 50 {
		t.Errorf("Expected MaxWatchedFiles 50, got %d", config.MaxWatchedFiles)
	}
}

// verifyOptimizationConfig verifies optimization configuration settings
func verifyOptimizationConfig(t *testing.T, config *Config) {
	if config.OptimizationStrategy != OptimizationSmallBatch {
		t.Errorf("Expected OptimizationSmallBatch, got %v", config.OptimizationStrategy)
	}
	if config.BoreasLiteCapacity != 256 {
		t.Errorf("Expected BoreasLiteCapacity 256, got %d", config.BoreasLiteCapacity)
	}
}

// verifyAuditConfig verifies audit configuration settings
func verifyAuditConfig(t *testing.T, config *Config) {
	if !config.Audit.Enabled {
		t.Error("Expected audit to be enabled")
	}
	if config.Audit.OutputFile != "/tmp/argus-test.jsonl" {
		t.Errorf("Expected audit output file '/tmp/argus-test.jsonl', got %s", config.Audit.OutputFile)
	}
	if config.Audit.MinLevel != AuditWarn {
		t.Errorf("Expected AuditWarn, got %v", config.Audit.MinLevel)
	}
	if config.Audit.BufferSize != 500 {
		t.Errorf("Expected audit buffer size 500, got %d", config.Audit.BufferSize)
	}
	if config.Audit.FlushInterval != 3*time.Second {
		t.Errorf("Expected audit flush interval 3s, got %v", config.Audit.FlushInterval)
	}
}

func TestLoadConfigFromEnvWithDefaults(t *testing.T) {
	// Clear any existing environment variables
	envVars := []string{
		"ARGUS_POLL_INTERVAL",
		"ARGUS_CACHE_TTL",
		"ARGUS_MAX_WATCHED_FILES",
		"ARGUS_OPTIMIZATION_STRATEGY",
		"ARGUS_BOREAS_CAPACITY",
		"ARGUS_AUDIT_ENABLED",
	}

	for _, key := range envVars {
		os.Unsetenv(key)
	}

	// Load configuration with defaults
	config, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("Failed to load config from env: %v", err)
	}

	// Should have default values after WithDefaults()
	if config.PollInterval != 5*time.Second {
		t.Errorf("Expected default PollInterval 5s, got %v", config.PollInterval)
	}

	if config.MaxWatchedFiles != 100 {
		t.Errorf("Expected default MaxWatchedFiles 100, got %d", config.MaxWatchedFiles)
	}

	if config.OptimizationStrategy != OptimizationAuto {
		t.Errorf("Expected default OptimizationAuto, got %v", config.OptimizationStrategy)
	}
}

func TestLoadConfigMultiSource(t *testing.T) {
	// Set some environment variables
	os.Setenv("ARGUS_POLL_INTERVAL", "8s")
	os.Setenv("ARGUS_AUDIT_ENABLED", "true")

	defer func() {
		os.Unsetenv("ARGUS_POLL_INTERVAL")
		os.Unsetenv("ARGUS_AUDIT_ENABLED")
	}()

	// Load multi-source configuration (no file exists)
	config, err := LoadConfigMultiSource("")
	if err != nil {
		t.Fatalf("Failed to load multi-source config: %v", err)
	}

	// Environment should override defaults
	if config.PollInterval != 8*time.Second {
		t.Errorf("Expected PollInterval 8s from env, got %v", config.PollInterval)
	}

	// Should have audit enabled from environment
	if !config.Audit.Enabled {
		t.Error("Expected audit enabled from environment")
	}

	// Should have defaults for unset values
	if config.MaxWatchedFiles != 100 {
		t.Errorf("Expected default MaxWatchedFiles 100, got %d", config.MaxWatchedFiles)
	}
}

func TestOptimizationStrategyParsing(t *testing.T) {
	testCases := []struct {
		envValue string
		expected OptimizationStrategy
	}{
		{"auto", OptimizationAuto},
		{"AUTO", OptimizationAuto},
		{"single", OptimizationSingleEvent},
		{"singleevent", OptimizationSingleEvent},
		{"SINGLEEVENT", OptimizationSingleEvent},
		{"small", OptimizationSmallBatch},
		{"smallbatch", OptimizationSmallBatch},
		{"SMALLBATCH", OptimizationSmallBatch},
		{"large", OptimizationLargeBatch},
		{"largebatch", OptimizationLargeBatch},
		{"LARGEBATCH", OptimizationLargeBatch},
	}

	for _, tc := range testCases {
		t.Run(tc.envValue, func(t *testing.T) {
			os.Setenv("ARGUS_OPTIMIZATION_STRATEGY", tc.envValue)
			defer os.Unsetenv("ARGUS_OPTIMIZATION_STRATEGY")

			config, err := LoadConfigFromEnv()
			if err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			if config.OptimizationStrategy != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, config.OptimizationStrategy)
			}
		})
	}
}

func TestAuditLevelParsing(t *testing.T) {
	testCases := []struct {
		envValue string
		expected AuditLevel
	}{
		{"info", AuditInfo},
		{"INFO", AuditInfo},
		{"warn", AuditWarn},
		{"warning", AuditWarn},
		{"WARN", AuditWarn},
		{"critical", AuditCritical},
		{"error", AuditCritical},
		{"CRITICAL", AuditCritical},
		{"security", AuditSecurity},
		{"SECURITY", AuditSecurity},
	}

	for _, tc := range testCases {
		t.Run(tc.envValue, func(t *testing.T) {
			os.Setenv("ARGUS_AUDIT_MIN_LEVEL", tc.envValue)
			os.Setenv("ARGUS_AUDIT_ENABLED", "true")
			defer func() {
				os.Unsetenv("ARGUS_AUDIT_MIN_LEVEL")
				os.Unsetenv("ARGUS_AUDIT_ENABLED")
			}()

			config, err := LoadConfigFromEnv()
			if err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			if config.Audit.MinLevel != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, config.Audit.MinLevel)
			}
		})
	}
}

func TestParseBool(t *testing.T) {
	trueCases := []string{"true", "TRUE", "1", "yes", "YES", "on", "ON", "enabled", "ENABLED"}
	falseCases := []string{"false", "FALSE", "0", "no", "NO", "off", "OFF", "disabled", "DISABLED", "invalid", ""}

	for _, value := range trueCases {
		if !parseBool(value) {
			t.Errorf("Expected true for %q", value)
		}
	}

	for _, value := range falseCases {
		if parseBool(value) {
			t.Errorf("Expected false for %q", value)
		}
	}
}

func TestGetEnvHelpers(t *testing.T) {
	// Test GetEnvWithDefault
	os.Setenv("TEST_STRING", "testvalue")
	defer os.Unsetenv("TEST_STRING")

	if result := GetEnvWithDefault("TEST_STRING", "default"); result != "testvalue" {
		t.Errorf("Expected 'testvalue', got %q", result)
	}

	if result := GetEnvWithDefault("NONEXISTENT", "default"); result != "default" {
		t.Errorf("Expected 'default', got %q", result)
	}

	// Test GetEnvDurationWithDefault
	os.Setenv("TEST_DURATION", "30s")
	defer os.Unsetenv("TEST_DURATION")

	if result := GetEnvDurationWithDefault("TEST_DURATION", time.Minute); result != 30*time.Second {
		t.Errorf("Expected 30s, got %v", result)
	}

	if result := GetEnvDurationWithDefault("NONEXISTENT_DURATION", time.Minute); result != time.Minute {
		t.Errorf("Expected 1m, got %v", result)
	}

	// Test GetEnvIntWithDefault
	os.Setenv("TEST_INT", "42")
	defer os.Unsetenv("TEST_INT")

	if result := GetEnvIntWithDefault("TEST_INT", 100); result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}

	if result := GetEnvIntWithDefault("NONEXISTENT_INT", 100); result != 100 {
		t.Errorf("Expected 100, got %d", result)
	}

	// Test GetEnvBoolWithDefault
	os.Setenv("TEST_BOOL", "true")
	defer os.Unsetenv("TEST_BOOL")

	if result := GetEnvBoolWithDefault("TEST_BOOL", false); !result {
		t.Error("Expected true, got false")
	}

	if result := GetEnvBoolWithDefault("NONEXISTENT_BOOL", false); result {
		t.Error("Expected false, got true")
	}
}

func TestInvalidEnvironmentValues(t *testing.T) {
	// Test invalid duration
	os.Setenv("ARGUS_POLL_INTERVAL", "invalid-duration")
	defer os.Unsetenv("ARGUS_POLL_INTERVAL")

	_, err := LoadConfigFromEnv()
	if err == nil {
		t.Error("Expected error for invalid duration")
	}

	// Test invalid integer
	os.Unsetenv("ARGUS_POLL_INTERVAL")
	os.Setenv("ARGUS_MAX_WATCHED_FILES", "not-a-number")
	defer os.Unsetenv("ARGUS_MAX_WATCHED_FILES")

	_, err = LoadConfigFromEnv()
	if err == nil {
		t.Error("Expected error for invalid integer")
	}

	// Test invalid optimization strategy
	os.Unsetenv("ARGUS_MAX_WATCHED_FILES")
	os.Setenv("ARGUS_OPTIMIZATION_STRATEGY", "invalid-strategy")
	defer os.Unsetenv("ARGUS_OPTIMIZATION_STRATEGY")

	_, err = LoadConfigFromEnv()
	if err == nil {
		t.Error("Expected error for invalid optimization strategy")
	}
}
