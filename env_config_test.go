// env_config_test.go: Tests for Environment Variables Support
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
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
		if err := os.Setenv(key, value); err != nil {
			t.Logf("Failed to set env var %s: %v", key, err)
		}
	}

	t.Cleanup(func() {
		for key := range envVars {
			if err := os.Unsetenv(key); err != nil {
				t.Logf("Failed to unset env var %s: %v", key, err)
			}
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
		if err := os.Unsetenv(key); err != nil {
			t.Logf("Failed to unset env var %s: %v", key, err)
		}
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
	if err := os.Setenv("ARGUS_POLL_INTERVAL", "8s"); err != nil {
		t.Fatalf("Failed to set ARGUS_POLL_INTERVAL: %v", err)
	}
	if err := os.Setenv("ARGUS_AUDIT_ENABLED", "true"); err != nil {
		t.Fatalf("Failed to set ARGUS_AUDIT_ENABLED: %v", err)
	}

	defer func() {
		if err := os.Unsetenv("ARGUS_POLL_INTERVAL"); err != nil {
			t.Errorf("Failed to unset ARGUS_POLL_INTERVAL: %v", err)
		}
		if err := os.Unsetenv("ARGUS_AUDIT_ENABLED"); err != nil {
			t.Errorf("Failed to unset ARGUS_AUDIT_ENABLED: %v", err)
		}
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
			if err := os.Setenv("ARGUS_OPTIMIZATION_STRATEGY", tc.envValue); err != nil {
				t.Logf("Failed to set ARGUS_OPTIMIZATION_STRATEGY: %v", err)
			}
			defer func() {
				if err := os.Unsetenv("ARGUS_OPTIMIZATION_STRATEGY"); err != nil {
					t.Logf("Failed to unset ARGUS_OPTIMIZATION_STRATEGY: %v", err)
				}
			}()

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
			if err := os.Setenv("ARGUS_AUDIT_MIN_LEVEL", tc.envValue); err != nil {
				t.Logf("Failed to set ARGUS_AUDIT_MIN_LEVEL: %v", err)
			}
			if err := os.Setenv("ARGUS_AUDIT_ENABLED", "true"); err != nil {
				t.Logf("Failed to set ARGUS_AUDIT_ENABLED: %v", err)
			}
			defer func() {
				if err := os.Unsetenv("ARGUS_AUDIT_MIN_LEVEL"); err != nil {
					t.Logf("Failed to unset ARGUS_AUDIT_MIN_LEVEL: %v", err)
				}
				if err := os.Unsetenv("ARGUS_AUDIT_ENABLED"); err != nil {
					t.Logf("Failed to unset ARGUS_AUDIT_ENABLED: %v", err)
				}
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
	if err := os.Setenv("TEST_STRING", "testvalue"); err != nil {
		t.Logf("Failed to set TEST_STRING: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("TEST_STRING"); err != nil {
			t.Logf("Failed to unset TEST_STRING: %v", err)
		}
	}()

	if result := GetEnvWithDefault("TEST_STRING", "default"); result != "testvalue" {
		t.Errorf("Expected 'testvalue', got %q", result)
	}

	if result := GetEnvWithDefault("NONEXISTENT", "default"); result != "default" {
		t.Errorf("Expected 'default', got %q", result)
	}

	// Test GetEnvDurationWithDefault
	if err := os.Setenv("TEST_DURATION", "30s"); err != nil {
		t.Logf("Failed to set TEST_DURATION: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("TEST_DURATION"); err != nil {
			t.Logf("Failed to unset TEST_DURATION: %v", err)
		}
	}()

	if result := GetEnvDurationWithDefault("TEST_DURATION", time.Minute); result != 30*time.Second {
		t.Errorf("Expected 30s, got %v", result)
	}

	if result := GetEnvDurationWithDefault("NONEXISTENT_DURATION", time.Minute); result != time.Minute {
		t.Errorf("Expected 1m, got %v", result)
	}

	// Test GetEnvIntWithDefault
	if err := os.Setenv("TEST_INT", "42"); err != nil {
		t.Logf("Failed to set TEST_INT: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("TEST_INT"); err != nil {
			t.Logf("Failed to unset TEST_INT: %v", err)
		}
	}()

	if result := GetEnvIntWithDefault("TEST_INT", 100); result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}

	if result := GetEnvIntWithDefault("NONEXISTENT_INT", 100); result != 100 {
		t.Errorf("Expected 100, got %d", result)
	}

	// Test GetEnvBoolWithDefault
	if err := os.Setenv("TEST_BOOL", "true"); err != nil {
		t.Logf("Failed to set TEST_BOOL: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("TEST_BOOL"); err != nil {
			t.Logf("Failed to unset TEST_BOOL: %v", err)
		}
	}()

	if result := GetEnvBoolWithDefault("TEST_BOOL", false); !result {
		t.Error("Expected true, got false")
	}

	if result := GetEnvBoolWithDefault("NONEXISTENT_BOOL", false); result {
		t.Error("Expected false, got true")
	}
}

func TestInvalidEnvironmentValues(t *testing.T) {
	// Test invalid duration
	if err := os.Setenv("ARGUS_POLL_INTERVAL", "invalid-duration"); err != nil {
		t.Logf("Failed to set ARGUS_POLL_INTERVAL: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("ARGUS_POLL_INTERVAL"); err != nil {
			t.Logf("Failed to unset ARGUS_POLL_INTERVAL: %v", err)
		}
	}()

	_, err := LoadConfigFromEnv()
	if err == nil {
		t.Error("Expected error for invalid duration")
	}

	// Test invalid integer
	if err := os.Unsetenv("ARGUS_POLL_INTERVAL"); err != nil {
		t.Logf("Failed to unset ARGUS_POLL_INTERVAL: %v", err)
	}
	if err := os.Setenv("ARGUS_MAX_WATCHED_FILES", "not-a-number"); err != nil {
		t.Logf("Failed to set ARGUS_MAX_WATCHED_FILES: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("ARGUS_MAX_WATCHED_FILES"); err != nil {
			t.Logf("Failed to unset ARGUS_MAX_WATCHED_FILES: %v", err)
		}
	}()

	_, err = LoadConfigFromEnv()
	if err == nil {
		t.Error("Expected error for invalid integer")
	}

	// Test invalid optimization strategy
	if err := os.Unsetenv("ARGUS_MAX_WATCHED_FILES"); err != nil {
		t.Logf("Failed to unset ARGUS_MAX_WATCHED_FILES: %v", err)
	}
	if err := os.Setenv("ARGUS_OPTIMIZATION_STRATEGY", "invalid-strategy"); err != nil {
		t.Logf("Failed to set ARGUS_OPTIMIZATION_STRATEGY: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("ARGUS_OPTIMIZATION_STRATEGY"); err != nil {
			t.Logf("Failed to unset ARGUS_OPTIMIZATION_STRATEGY: %v", err)
		}
	}()

	_, err = LoadConfigFromEnv()
	if err == nil {
		t.Error("Expected error for invalid optimization strategy")
	}
}

// TestEnvConfigLoadRemoteConfigEdgeCases tests edge cases in loadRemoteConfig
func TestEnvConfigLoadRemoteConfigEdgeCases(t *testing.T) {
	// Clear all remote-related env vars
	remoteVars := []string{
		"ARGUS_REMOTE_URL",
		"ARGUS_REMOTE_INTERVAL",
		"ARGUS_REMOTE_TIMEOUT",
		"ARGUS_REMOTE_HEADERS",
	}
	for _, v := range remoteVars {
		if err := os.Unsetenv(v); err != nil {
			t.Logf("Failed to unset env var %s: %v", v, err)
		}
	}

	// Test with all remote vars unset (should not fail)
	envConfig := &EnvConfig{}
	err := loadRemoteConfig(envConfig)
	if err != nil {
		t.Errorf("loadRemoteConfig should not fail with no env vars set: %v", err)
	}

	// Test with invalid duration format
	if err := os.Setenv("ARGUS_REMOTE_INTERVAL", "invalid-duration"); err != nil {
		t.Logf("Failed to set ARGUS_REMOTE_INTERVAL: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("ARGUS_REMOTE_INTERVAL"); err != nil {
			t.Logf("Failed to unset ARGUS_REMOTE_INTERVAL: %v", err)
		}
	}()

	envConfig = &EnvConfig{}
	err = loadRemoteConfig(envConfig)
	if err != nil {
		t.Errorf("loadRemoteConfig should handle invalid duration gracefully: %v", err)
	}

	// Test with invalid timeout format
	if err := os.Setenv("ARGUS_REMOTE_TIMEOUT", "not-a-duration"); err != nil {
		t.Logf("Failed to set ARGUS_REMOTE_TIMEOUT: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("ARGUS_REMOTE_TIMEOUT"); err != nil {
			t.Logf("Failed to unset ARGUS_REMOTE_TIMEOUT: %v", err)
		}
	}()

	envConfig = &EnvConfig{}
	err = loadRemoteConfig(envConfig)
	if err != nil {
		t.Errorf("loadRemoteConfig should handle invalid timeout gracefully: %v", err)
	}

	// Test with valid values
	if err := os.Setenv("ARGUS_REMOTE_URL", "http://example.com/config"); err != nil {
		t.Logf("Failed to set ARGUS_REMOTE_URL: %v", err)
	}
	if err := os.Setenv("ARGUS_REMOTE_INTERVAL", "30s"); err != nil {
		t.Logf("Failed to set ARGUS_REMOTE_INTERVAL: %v", err)
	}
	if err := os.Setenv("ARGUS_REMOTE_TIMEOUT", "10s"); err != nil {
		t.Logf("Failed to set ARGUS_REMOTE_TIMEOUT: %v", err)
	}
	if err := os.Setenv("ARGUS_REMOTE_HEADERS", "Authorization: Bearer token"); err != nil {
		t.Logf("Failed to set ARGUS_REMOTE_HEADERS: %v", err)
	}

	envConfig = &EnvConfig{}
	err = loadRemoteConfig(envConfig)
	if err != nil {
		t.Errorf("loadRemoteConfig should succeed with valid values: %v", err)
	}

	if envConfig.RemoteURL != "http://example.com/config" {
		t.Errorf("Expected RemoteURL to be set, got %s", envConfig.RemoteURL)
	}
	if envConfig.RemoteInterval != 30*time.Second {
		t.Errorf("Expected RemoteInterval to be 30s, got %v", envConfig.RemoteInterval)
	}
	if envConfig.RemoteTimeout != 10*time.Second {
		t.Errorf("Expected RemoteTimeout to be 10s, got %v", envConfig.RemoteTimeout)
	}
	if envConfig.RemoteHeaders != "Authorization: Bearer token" {
		t.Errorf("Expected RemoteHeaders to be set, got %s", envConfig.RemoteHeaders)
	}
}

// TestEnvConfigLoadValidationConfigEdgeCases tests edge cases in loadValidationConfig
func TestEnvConfigLoadValidationConfigEdgeCases(t *testing.T) {
	// Clear all validation-related env vars
	validationVars := []string{
		"ARGUS_VALIDATION_ENABLED",
		"ARGUS_VALIDATION_SCHEMA",
		"ARGUS_VALIDATION_STRICT",
	}
	for _, v := range validationVars {
		if err := os.Unsetenv(v); err != nil {
			t.Logf("Failed to unset env var %s: %v", v, err)
		}
	}

	// Test with all validation vars unset
	envConfig := &EnvConfig{}
	err := loadValidationConfig(envConfig)
	if err != nil {
		t.Errorf("loadValidationConfig should not fail with no env vars set: %v", err)
	}

	// Test with invalid boolean values
	if err := os.Setenv("ARGUS_VALIDATION_ENABLED", "maybe"); err != nil {
		t.Logf("Failed to set ARGUS_VALIDATION_ENABLED: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("ARGUS_VALIDATION_ENABLED"); err != nil {
			t.Logf("Failed to unset ARGUS_VALIDATION_ENABLED: %v", err)
		}
	}()

	envConfig = &EnvConfig{}
	err = loadValidationConfig(envConfig)
	if err != nil {
		t.Errorf("loadValidationConfig should handle invalid boolean gracefully: %v", err)
	}

	// Test with valid values
	if err := os.Setenv("ARGUS_VALIDATION_ENABLED", "true"); err != nil {
		t.Logf("Failed to set ARGUS_VALIDATION_ENABLED: %v", err)
	}
	if err := os.Setenv("ARGUS_VALIDATION_SCHEMA", "/path/to/schema.json"); err != nil {
		t.Logf("Failed to set ARGUS_VALIDATION_SCHEMA: %v", err)
	}
	if err := os.Setenv("ARGUS_VALIDATION_STRICT", "false"); err != nil {
		t.Logf("Failed to set ARGUS_VALIDATION_STRICT: %v", err)
	}

	envConfig = &EnvConfig{}
	err = loadValidationConfig(envConfig)
	if err != nil {
		t.Errorf("loadValidationConfig should succeed with valid values: %v", err)
	}

	if !envConfig.ValidationEnabled {
		t.Error("Expected ValidationEnabled to be true")
	}
	if envConfig.ValidationSchema != "/path/to/schema.json" {
		t.Errorf("Expected ValidationSchema to be set, got %s", envConfig.ValidationSchema)
	}
	if envConfig.ValidationStrict {
		t.Error("Expected ValidationStrict to be false")
	}
}

// TestLoadConfigMultiSourceEdgeCases tests edge cases in LoadConfigMultiSource
func TestLoadConfigMultiSourceEdgeCases(t *testing.T) {
	// Test with non-existent file
	config, err := LoadConfigMultiSource("/non/existent/file.json")
	if err != nil {
		t.Errorf("LoadConfigMultiSource should handle non-existent file gracefully: %v", err)
	}
	if config == nil {
		t.Error("LoadConfigMultiSource should return config even with non-existent file")
	}

	// Test with empty file path
	config, err = LoadConfigMultiSource("")
	if err != nil {
		t.Errorf("LoadConfigMultiSource should handle empty file path: %v", err)
	}
	if config == nil {
		t.Error("LoadConfigMultiSource should return config with empty file path")
	}

	// Test with invalid environment variables that cause LoadConfigFromEnv to fail
	// Set an invalid poll interval
	if err := os.Setenv("ARGUS_POLL_INTERVAL", "invalid-duration"); err != nil {
		t.Logf("Failed to set ARGUS_POLL_INTERVAL: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("ARGUS_POLL_INTERVAL"); err != nil {
			t.Logf("Failed to unset ARGUS_POLL_INTERVAL: %v", err)
		}
	}()

	config, err = LoadConfigMultiSource("")
	if err == nil {
		t.Error("LoadConfigMultiSource should return error when LoadConfigFromEnv fails")
	}
	if config == nil {
		t.Error("LoadConfigMultiSource should return partial config even when LoadConfigFromEnv fails")
	}
}

// TestLoadEnvVarsErrorHandling tests error handling in loadEnvVars
func TestLoadEnvVarsErrorHandling(t *testing.T) {
	// Clear all env vars first
	allVars := []string{
		"ARGUS_POLL_INTERVAL", "ARGUS_CACHE_TTL", "ARGUS_MAX_WATCHED_FILES",
		"ARGUS_OPTIMIZATION_STRATEGY", "ARGUS_BOREAS_CAPACITY",
		"ARGUS_AUDIT_ENABLED", "ARGUS_AUDIT_OUTPUT_FILE", "ARGUS_AUDIT_MIN_LEVEL",
		"ARGUS_AUDIT_BUFFER_SIZE", "ARGUS_AUDIT_FLUSH_INTERVAL",
		"ARGUS_REMOTE_URL", "ARGUS_REMOTE_INTERVAL", "ARGUS_REMOTE_TIMEOUT", "ARGUS_REMOTE_HEADERS",
		"ARGUS_VALIDATION_ENABLED", "ARGUS_VALIDATION_SCHEMA", "ARGUS_VALIDATION_STRICT",
	}
	for _, v := range allVars {
		if err := os.Unsetenv(v); err != nil {
			t.Logf("Failed to unset env var %s: %v", v, err)
		}
	}

	// Test with invalid poll interval (should cause loadCoreConfig to fail)
	if err := os.Setenv("ARGUS_POLL_INTERVAL", "not-a-duration"); err != nil {
		t.Logf("Failed to set ARGUS_POLL_INTERVAL: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("ARGUS_POLL_INTERVAL"); err != nil {
			t.Logf("Failed to unset ARGUS_POLL_INTERVAL: %v", err)
		}
	}()

	envConfig := &EnvConfig{}
	err := loadEnvVars(envConfig)
	if err == nil {
		t.Error("loadEnvVars should return error when loadCoreConfig fails")
	}

	// Test with valid configuration
	if err := os.Setenv("ARGUS_POLL_INTERVAL", "10s"); err != nil {
		t.Logf("Failed to set ARGUS_POLL_INTERVAL: %v", err)
	}
	if err := os.Setenv("ARGUS_CACHE_TTL", "5s"); err != nil {
		t.Logf("Failed to set ARGUS_CACHE_TTL: %v", err)
	}
	if err := os.Setenv("ARGUS_MAX_WATCHED_FILES", "50"); err != nil {
		t.Logf("Failed to set ARGUS_MAX_WATCHED_FILES: %v", err)
	}

	envConfig = &EnvConfig{}
	err = loadEnvVars(envConfig)
	if err != nil {
		t.Errorf("loadEnvVars should succeed with valid config: %v", err)
	}

	if envConfig.PollInterval != 10*time.Second {
		t.Errorf("Expected PollInterval to be 10s, got %v", envConfig.PollInterval)
	}
	if envConfig.CacheTTL != 5*time.Second {
		t.Errorf("Expected CacheTTL to be 5s, got %v", envConfig.CacheTTL)
	}
	if envConfig.MaxWatchedFiles != 50 {
		t.Errorf("Expected MaxWatchedFiles to be 50, got %d", envConfig.MaxWatchedFiles)
	}
}

// TestConvertEnvToConfigErrorHandling tests error handling in convertEnvToConfig
func TestConvertEnvToConfigErrorHandling(t *testing.T) {
	// Test with invalid optimization strategy (should cause convertPerformanceConfig to fail)
	envConfig := &EnvConfig{
		OptimizationStrategy: "invalid-strategy",
	}

	config := &Config{}
	err := convertEnvToConfig(envConfig, config)
	if err == nil {
		t.Error("convertEnvToConfig should return error for invalid optimization strategy")
	}

	// Test with valid configuration
	envConfig = &EnvConfig{
		PollInterval:         10 * time.Second,
		CacheTTL:             5 * time.Second,
		MaxWatchedFiles:      50,
		OptimizationStrategy: "auto",
		BoreasLiteCapacity:   256,
	}

	config = &Config{}
	err = convertEnvToConfig(envConfig, config)
	if err != nil {
		t.Errorf("convertEnvToConfig should succeed with valid config: %v", err)
	}

	// Verify conversions
	if config.PollInterval != 10*time.Second {
		t.Errorf("Expected PollInterval to be 10s, got %v", config.PollInterval)
	}
	if config.CacheTTL != 5*time.Second {
		t.Errorf("Expected CacheTTL to be 5s, got %v", config.CacheTTL)
	}
	if config.MaxWatchedFiles != 50 {
		t.Errorf("Expected MaxWatchedFiles to be 50, got %d", config.MaxWatchedFiles)
	}
	if config.OptimizationStrategy != OptimizationAuto {
		t.Errorf("Expected OptimizationStrategy to be OptimizationAuto, got %v", config.OptimizationStrategy)
	}
	if config.BoreasLiteCapacity != 256 {
		t.Errorf("Expected BoreasLiteCapacity to be 256, got %d", config.BoreasLiteCapacity)
	}
}

// TestLoadConfigFromFile tests the new file loading functionality in LoadConfigMultiSource
func TestLoadConfigFromFile(t *testing.T) {
	// Test 1: Valid configuration file
	t.Run("valid_json_file", func(t *testing.T) {
		// Create temporary JSON config file
		tempDir := t.TempDir()
		configFile := filepath.Join(tempDir, "test_config.json")

		configContent := `{
			"poll_interval": "10s",
			"cache_ttl": "5s",
			"max_watched_files": 50,
			"audit": {
				"enabled": true,
				"min_level": "info"
			}
		}`

		if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to create test config file: %v", err)
		}

		// Test LoadConfigMultiSource with real file
		config, err := LoadConfigMultiSource(configFile)
		if err != nil {
			t.Fatalf("LoadConfigMultiSource should succeed with valid file: %v", err)
		}

		if config == nil {
			t.Fatal("LoadConfigMultiSource should return non-nil config")
		}

		// For now, since we haven't implemented full binding,
		// we just verify the file was processed without error
		// TODO: Add specific field verification once Config binding is implemented
	})

	// Test 2: YAML configuration file
	t.Run("valid_yaml_file", func(t *testing.T) {
		tempDir := t.TempDir()
		configFile := filepath.Join(tempDir, "test_config.yaml")

		configContent := `
poll_interval: 15s
cache_ttl: 7s
max_watched_files: 75
audit:
  enabled: true
  min_level: warn
`

		if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to create test YAML config file: %v", err)
		}

		config, err := LoadConfigMultiSource(configFile)
		if err != nil {
			t.Fatalf("LoadConfigMultiSource should succeed with valid YAML file: %v", err)
		}

		if config == nil {
			t.Fatal("LoadConfigMultiSource should return non-nil config")
		}
	})

	// Test 3: Invalid configuration file (malformed JSON)
	t.Run("invalid_json_file", func(t *testing.T) {
		tempDir := t.TempDir()
		configFile := filepath.Join(tempDir, "invalid_config.json")

		// Malformed JSON
		configContent := `{"invalid": json without quotes}`

		if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to create test config file: %v", err)
		}

		// Should gracefully fallback to defaults when file is invalid
		config, err := LoadConfigMultiSource(configFile)
		if err != nil {
			t.Fatalf("LoadConfigMultiSource should gracefully handle invalid files: %v", err)
		}

		if config == nil {
			t.Fatal("LoadConfigMultiSource should return defaults when file is invalid")
		}
	})

	// Test 4: Non-existent file
	t.Run("nonexistent_file", func(t *testing.T) {
		config, err := LoadConfigMultiSource("/path/to/nonexistent/file.json")
		if err != nil {
			t.Fatalf("LoadConfigMultiSource should handle non-existent files gracefully: %v", err)
		}

		if config == nil {
			t.Fatal("LoadConfigMultiSource should return defaults when file doesn't exist")
		}
	})

	// Test 5: Unsupported file format
	t.Run("unsupported_format", func(t *testing.T) {
		tempDir := t.TempDir()
		configFile := filepath.Join(tempDir, "config.unknown")

		if err := os.WriteFile(configFile, []byte("some content"), 0644); err != nil {
			t.Fatalf("Failed to create test config file: %v", err)
		}

		// Should fallback to defaults when format is unsupported
		config, err := LoadConfigMultiSource(configFile)
		if err != nil {
			t.Fatalf("LoadConfigMultiSource should handle unsupported formats gracefully: %v", err)
		}

		if config == nil {
			t.Fatal("LoadConfigMultiSource should return defaults when format is unsupported")
		}
	})

	// Test 6: File loading with environment override
	t.Run("file_with_env_override", func(t *testing.T) {
		tempDir := t.TempDir()
		configFile := filepath.Join(tempDir, "config_with_env.json")

		configContent := `{"poll_interval": "10s"}`
		if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to create test config file: %v", err)
		}

		// Set environment variable to override
		if err := os.Setenv("ARGUS_POLL_INTERVAL", "5s"); err != nil {
			t.Fatalf("Failed to set ARGUS_POLL_INTERVAL: %v", err)
		}
		defer func() {
			if err := os.Unsetenv("ARGUS_POLL_INTERVAL"); err != nil {
				t.Logf("Failed to unset ARGUS_POLL_INTERVAL: %v", err)
			}
		}()

		config, err := LoadConfigMultiSource(configFile)
		if err != nil {
			t.Fatalf("LoadConfigMultiSource should succeed: %v", err)
		}

		// Environment should override file (precedence test)
		if config.PollInterval != 5*time.Second {
			t.Errorf("Expected environment override (5s), got %v", config.PollInterval)
		}
	})
}

// TestLoadConfigFromFileFunction tests the internal loadConfigFromFile function
func TestLoadConfigFromFileFunction(t *testing.T) {
	// Test security validation
	t.Run("security_validation", func(t *testing.T) {
		// Test path traversal attempt
		_, err := loadConfigFromFile("../../../etc/passwd")
		if err == nil {
			t.Error("loadConfigFromFile should reject path traversal attempts")
		}
	})

	// Test format detection
	t.Run("format_detection", func(t *testing.T) {
		tempDir := t.TempDir()

		// Test various formats
		formats := map[string]string{
			"config.json": `{"test": "value"}`,
			"config.yaml": "test: value\n",
			"config.toml": `test = "value"`,
			"config.ini":  "[section]\ntest=value\n",
		}

		for filename, content := range formats {
			filePath := filepath.Join(tempDir, filename)
			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				t.Fatalf("Failed to create %s: %v", filename, err)
			}

			config, err := loadConfigFromFile(filePath)
			if err != nil {
				t.Errorf("loadConfigFromFile failed for %s: %v", filename, err)
			}

			if config == nil {
				t.Errorf("loadConfigFromFile returned nil config for %s", filename)
			}
		}
	})

	// Test error handling for invalid files
	t.Run("invalid_file_handling", func(t *testing.T) {
		tempDir := t.TempDir()

		// Test invalid JSON
		invalidJSON := filepath.Join(tempDir, "invalid.json")
		if err := os.WriteFile(invalidJSON, []byte(`{invalid json`), 0644); err != nil {
			t.Fatalf("Failed to create invalid JSON file: %v", err)
		}

		_, err := loadConfigFromFile(invalidJSON)
		if err == nil {
			t.Error("loadConfigFromFile should return error for invalid JSON")
		}
	})
}
