// config_validation_test.go - Validation tests in main package
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfig_ValidateDetailed(t *testing.T) {
	// Create temporary directory for audit file tests
	tempDir, err := os.MkdirTemp("", "argus_test_validation")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
       defer func() {
	       if err := os.RemoveAll(tempDir); err != nil {
		       t.Logf("Failed to remove tempDir: %v", err)
	       }
       }()

	tests := []struct {
		name             string
		config           *Config
		expectedValid    bool
		expectedErrors   int
		expectedWarnings int
	}{
		{
			name: "valid default config",
			config: func() *Config {
				c := (&Config{}).WithDefaults()
				// Override audit path to valid temporary directory
				c.Audit.OutputFile = filepath.Join(tempDir, "audit.log")
				return c
			}(),
			expectedValid:    true,
			expectedErrors:   0,
			expectedWarnings: 0,
		},
		{
			name: "invalid poll interval",
			config: &Config{
				PollInterval:    -1 * time.Second,
				CacheTTL:        0, // Zero to avoid cache TTL comparison warnings
				MaxWatchedFiles: 100,
			},
			expectedValid:    false,
			expectedErrors:   1,
			expectedWarnings: 0,
		},
		{
			name: "poll interval too small",
			config: &Config{
				PollInterval:    5 * time.Millisecond,
				CacheTTL:        1 * time.Millisecond, // Less than PollInterval to avoid extra warnings
				MaxWatchedFiles: 100,
			},
			expectedValid:    false,
			expectedErrors:   1,
			expectedWarnings: 0,
		},
		{
			name: "cache TTL larger than poll interval (warning)",
			config: &Config{
				PollInterval:    1 * time.Second,
				CacheTTL:        2 * time.Second,
				MaxWatchedFiles: 100,
			},
			expectedValid:    true,
			expectedErrors:   0,
			expectedWarnings: 1,
		},
		{
			name: "invalid optimization strategy",
			config: &Config{
				PollInterval:         1 * time.Second,
				CacheTTL:             500 * time.Millisecond, // Less than PollInterval
				MaxWatchedFiles:      100,
				OptimizationStrategy: OptimizationStrategy(999), // Invalid value
			},
			expectedValid:    false,
			expectedErrors:   1,
			expectedWarnings: 0,
		},
		{
			name: "invalid boreas capacity",
			config: &Config{
				PollInterval:       1 * time.Second,
				CacheTTL:           500 * time.Millisecond, // Less than PollInterval
				MaxWatchedFiles:    100,
				BoreasLiteCapacity: 15, // Not power of 2
			},
			expectedValid:    false,
			expectedErrors:   1,
			expectedWarnings: 0,
		},
		{
			name: "large boreas capacity (warning)",
			config: &Config{
				PollInterval:       1 * time.Second,
				CacheTTL:           500 * time.Millisecond, // Less than PollInterval
				MaxWatchedFiles:    100,
				BoreasLiteCapacity: 2048, // Large but valid (power of 2)
			},
			expectedValid:    true,
			expectedErrors:   0,
			expectedWarnings: 1,
		},
		{
			name: "performance warning - fast polling with many files",
			config: &Config{
				PollInterval:    50 * time.Millisecond,
				CacheTTL:        10 * time.Millisecond, // Less than PollInterval
				MaxWatchedFiles: 200,
			},
			expectedValid:    true,
			expectedErrors:   0,
			expectedWarnings: 1, // Performance warning
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.ValidateDetailed()

			if result.Valid != tt.expectedValid {
				t.Errorf("ValidateDetailed() valid = %v, want %v", result.Valid, tt.expectedValid)
			}

			if len(result.Errors) != tt.expectedErrors {
				t.Errorf("ValidateDetailed() errors = %d, want %d. Errors: %v",
					len(result.Errors), tt.expectedErrors, result.Errors)
			}

			if len(result.Warnings) != tt.expectedWarnings {
				t.Errorf("ValidateDetailed() warnings = %d, want %d. Warnings: %v",
					len(result.Warnings), tt.expectedWarnings, result.Warnings)
			}
		})
	}
}

func TestConfig_ValidateAuditConfig(t *testing.T) {
	// Create temporary directory for audit file tests
	tempDir, err := os.MkdirTemp("", "argus_test_audit")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
       defer func() {
	       if err := os.RemoveAll(tempDir); err != nil {
		       t.Logf("Failed to remove tempDir: %v", err)
	       }
       }()

	tests := []struct {
		name             string
		auditConfig      AuditConfig
		expectedErrors   int
		expectedWarnings int
	}{
		{
			name: "valid audit config",
			auditConfig: AuditConfig{
				Enabled:       true,
				BufferSize:    1000,
				FlushInterval: 5 * time.Second,
				OutputFile:    filepath.Join(tempDir, "audit.log"),
			},
			expectedErrors:   0,
			expectedWarnings: 0,
		},
		{
			name: "disabled audit config",
			auditConfig: AuditConfig{
				Enabled: false,
			},
			expectedErrors:   0,
			expectedWarnings: 0,
		},
		{
			name: "invalid buffer size",
			auditConfig: AuditConfig{
				Enabled:       true,
				BufferSize:    -1,
				FlushInterval: 5 * time.Second,
				OutputFile:    filepath.Join(tempDir, "audit.log"),
			},
			expectedErrors:   1,
			expectedWarnings: 0,
		},
		{
			name: "zero buffer size (warning)",
			auditConfig: AuditConfig{
				Enabled:       true,
				BufferSize:    0,
				FlushInterval: 5 * time.Second,
				OutputFile:    filepath.Join(tempDir, "audit.log"),
			},
			expectedErrors:   0,
			expectedWarnings: 1,
		},
		{
			name: "large buffer size (warning)",
			auditConfig: AuditConfig{
				Enabled:       true,
				BufferSize:    15000,
				FlushInterval: 5 * time.Second,
				OutputFile:    filepath.Join(tempDir, "audit.log"),
			},
			expectedErrors:   0,
			expectedWarnings: 1,
		},
		{
			name: "invalid flush interval",
			auditConfig: AuditConfig{
				Enabled:       true,
				BufferSize:    1000,
				FlushInterval: -1 * time.Second,
				OutputFile:    filepath.Join(tempDir, "audit.log"),
			},
			expectedErrors:   1,
			expectedWarnings: 0,
		},
		{
			name: "zero flush interval (warning)",
			auditConfig: AuditConfig{
				Enabled:       true,
				BufferSize:    1000,
				FlushInterval: 0,
				OutputFile:    filepath.Join(tempDir, "audit.log"),
			},
			expectedErrors:   0,
			expectedWarnings: 1,
		},
		{
			name: "empty output file",
			auditConfig: AuditConfig{
				Enabled:       true,
				BufferSize:    1000,
				FlushInterval: 5 * time.Second,
				OutputFile:    "",
			},
			expectedErrors:   1,
			expectedWarnings: 0,
		},
		{
			name: "invalid output file directory",
			auditConfig: AuditConfig{
				Enabled:       true,
				BufferSize:    1000,
				FlushInterval: 5 * time.Second,
				OutputFile:    "/nonexistent/directory/audit.log",
			},
			expectedErrors:   1,
			expectedWarnings: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				PollInterval:    1 * time.Second,
				CacheTTL:        500 * time.Millisecond, // Less than PollInterval to avoid warnings
				MaxWatchedFiles: 10,                     // Small number to avoid performance warnings
				Audit:           tt.auditConfig,
			}

			result := config.ValidateDetailed()

			if len(result.Errors) != tt.expectedErrors {
				t.Errorf("ValidateDetailed() errors = %d, want %d. Errors: %v",
					len(result.Errors), tt.expectedErrors, result.Errors)
			}

			if len(result.Warnings) != tt.expectedWarnings {
				t.Errorf("ValidateDetailed() warnings = %d, want %d. Warnings: %v",
					len(result.Warnings), tt.expectedWarnings, result.Warnings)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	// Create temporary directory for audit file tests
	tempDir, err := os.MkdirTemp("", "argus_test_validate")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
       defer func() {
	       if err := os.RemoveAll(tempDir); err != nil {
		       t.Logf("Failed to remove tempDir: %v", err)
	       }
       }()

	tests := []struct {
		name      string
		config    *Config
		wantError bool
	}{
		{
			name: "valid config",
			config: func() *Config {
				c := (&Config{}).WithDefaults()
				// Override audit path to valid temporary directory
				c.Audit.OutputFile = filepath.Join(tempDir, "audit.log")
				return c
			}(),
			wantError: false,
		},
		{
			name: "invalid config",
			config: &Config{
				PollInterval:    -1 * time.Second,
				CacheTTL:        0,
				MaxWatchedFiles: 100,
			},
			wantError: true,
		},
		{
			name: "config with warnings only",
			config: &Config{
				PollInterval:    1 * time.Second,
				CacheTTL:        2 * time.Second, // Warning: larger than poll interval
				MaxWatchedFiles: 100,
			},
			wantError: false, // Warnings don't cause Validate() to fail
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Config.Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestValidateEnvironmentConfig(t *testing.T) {
	// Create temporary directory for audit file tests
	tempDir, err := os.MkdirTemp("", "argus_test_env")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
       defer func() {
	       if err := os.RemoveAll(tempDir); err != nil {
		       t.Logf("Failed to remove tempDir: %v", err)
	       }
       }()

	// Save current environment
	originalEnv := make(map[string]string)
	for _, env := range []string{
		"ARGUS_POLL_INTERVAL",
		"ARGUS_CACHE_TTL",
		"ARGUS_MAX_WATCHED_FILES",
		"ARGUS_AUDIT_OUTPUT_FILE",
	} {
		if val := os.Getenv(env); val != "" {
			originalEnv[env] = val
		}
	}

	// Clean environment for test
       defer func() {
	       for _, env := range []string{
		       "ARGUS_POLL_INTERVAL",
		       "ARGUS_CACHE_TTL",
		       "ARGUS_MAX_WATCHED_FILES",
		       "ARGUS_AUDIT_OUTPUT_FILE",
	       } {
		       if err := os.Unsetenv(env); err != nil {
			       t.Logf("Failed to unset env %s: %v", env, err)
		       }
	       }
	       for env, val := range originalEnv {
		       if err := os.Setenv(env, val); err != nil {
			       t.Logf("Failed to restore env %s: %v", env, err)
		       }
	       }
       }()

	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
	}{
		{
			name: "valid environment config",
			envVars: map[string]string{
				"ARGUS_POLL_INTERVAL":     "1s",
				"ARGUS_CACHE_TTL":         "2s", // Changed from 500ms to meet security requirement (min 1s)
				"ARGUS_MAX_WATCHED_FILES": "100",
				"ARGUS_AUDIT_OUTPUT_FILE": filepath.Join(tempDir, "audit.log"),
			},
			wantErr: false,
		},
		{
			name: "invalid environment config - poll interval validation",
			envVars: map[string]string{
				"ARGUS_POLL_INTERVAL":     "-1s",   // Invalid negative duration
				"ARGUS_CACHE_TTL":         "500ms", // Invalid: below 1s security limit
				"ARGUS_MAX_WATCHED_FILES": "100",
				"ARGUS_AUDIT_OUTPUT_FILE": filepath.Join(tempDir, "audit.log"),
				"ARGUS_AUDIT_ENABLED":     "false",
			},
			wantErr: true, // Security validation now catches invalid values before WithDefaults()
		},
		{
			name: "manual invalid config test",
			envVars: map[string]string{
				"ARGUS_AUDIT_OUTPUT_FILE": filepath.Join(tempDir, "audit.log"),
				"ARGUS_AUDIT_ENABLED":     "false",
			}, // No env vars for core config, we test validation separately
			wantErr: false, // This test case tests the env loading, validation is tested separately
		},
		{
			name: "no environment variables (uses defaults)",
			envVars: map[string]string{
				"ARGUS_AUDIT_OUTPUT_FILE": filepath.Join(tempDir, "audit.log"),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, value := range tt.envVars {
		       if err := os.Setenv(key, value); err != nil {
			       t.Logf("Failed to set env %s: %v", key, err)
		       }
			}

			err := ValidateEnvironmentConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEnvironmentConfig() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Clean up environment variables
			for key := range tt.envVars {
		       if err := os.Unsetenv(key); err != nil {
			       t.Logf("Failed to unset env %s: %v", key, err)
		       }
			}
		})
	}
}

func TestValidateConfigFile(t *testing.T) {
	// Create temporary test files
	tempDir, err := os.MkdirTemp("", "argus_test_config")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
       defer func() {
	       if err := os.RemoveAll(tempDir); err != nil {
		       t.Logf("Failed to remove tempDir: %v", err)
	       }
       }()

	// Create an audit directory for cross-platform compatibility
	auditDir := filepath.Join(tempDir, "audit")
	if err := os.MkdirAll(auditDir, 0755); err != nil {
		t.Fatalf("Failed to create audit dir: %v", err)
	}

	auditPath := filepath.Join(auditDir, "test.jsonl")
	// Escape backslashes for JSON on Windows
	auditPathJSON := strings.ReplaceAll(auditPath, "\\", "\\\\")

	validConfigFile := filepath.Join(tempDir, "valid_config.json")
	configContent := fmt.Sprintf(`{
		"poll_interval": "1s",
		"audit": {
			"enabled": true,
			"output_file": "%s"
		}
	}`, auditPathJSON)

	if err := os.WriteFile(validConfigFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	tests := []struct {
		name       string
		configPath string
		wantErr    bool
	}{
		{
			name:       "valid config file",
			configPath: validConfigFile,
			wantErr:    false, // Should now pass with proper JSON parsing and valid audit path
		},
		{
			name:       "empty config path",
			configPath: "",
			wantErr:    true,
		},
		{
			name:       "nonexistent config file",
			configPath: "/nonexistent/config.json",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfigFile(tt.configPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfigFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidationResult_Complete(t *testing.T) {
	// Test comprehensive validation with mixed errors and warnings
	tempDir, err := os.MkdirTemp("", "argus_validation_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
       defer func() {
	       if err := os.RemoveAll(tempDir); err != nil {
		       t.Logf("Failed to remove tempDir: %v", err)
	       }
       }()

	config := &Config{
		PollInterval:         5 * time.Millisecond,    // Error: too small
		CacheTTL:             -1 * time.Second,        // Error: invalid
		MaxWatchedFiles:      200,                     // OK, but will cause warnings
		OptimizationStrategy: OptimizationSingleEvent, // Warning: suboptimal for many files
		BoreasLiteCapacity:   2048,                    // Warning: large capacity
		Audit: AuditConfig{
			Enabled:       true,
			BufferSize:    0,                      // Warning: zero buffer
			FlushInterval: 100 * time.Millisecond, // Warning: frequent flushing
			OutputFile:    filepath.Join(tempDir, "audit.log"),
		},
	}

	result := config.ValidateDetailed()

	// Should be invalid due to errors
	if result.Valid {
		t.Error("Expected result to be invalid due to errors")
	}

	// Should have multiple errors
	if len(result.Errors) < 2 {
		t.Errorf("Expected at least 2 errors, got %d: %v", len(result.Errors), result.Errors)
	}

	// Should have multiple warnings
	if len(result.Warnings) < 3 {
		t.Errorf("Expected at least 3 warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}

	// Validate() should return an error
	if err := config.Validate(); err == nil {
		t.Error("Expected Validate() to return an error for invalid config")
	}

	t.Logf("Validation result: Valid=%v, Errors=%d, Warnings=%d",
		result.Valid, len(result.Errors), len(result.Warnings))
	t.Logf("Errors: %v", result.Errors)
	t.Logf("Warnings: %v", result.Warnings)
}

// Benchmark validation performance
func BenchmarkConfig_Validate(b *testing.B) {
	config := (&Config{}).WithDefaults()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = config.Validate()
	}
}

func BenchmarkConfig_ValidateDetailed(b *testing.B) {
	config := (&Config{}).WithDefaults()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = config.ValidateDetailed()
	}
}

// Test that validation errors contain proper error codes
func TestValidationErrorCodes(t *testing.T) {
	config := &Config{
		PollInterval:    -1 * time.Second,
		CacheTTL:        -1 * time.Second,
		MaxWatchedFiles: 0,
	}

	result := config.ValidateDetailed()

	expectedErrorSubstrings := []string{
		"ARGUS_INVALID_POLL_INTERVAL",
		"ARGUS_INVALID_CACHE_TTL",
		"ARGUS_INVALID_MAX_WATCHED_FILES",
	}

	for _, expected := range expectedErrorSubstrings {
		found := false
		for _, error := range result.Errors {
			if strings.Contains(fmt.Sprintf("%v", error), expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected error containing '%s' not found in errors: %v", expected, result.Errors)
		}
	}
}

func TestValidationOfInvalidConfigDirectly(t *testing.T) {
	// Test that validation catches invalid values when they're not overwritten by defaults
	config := &Config{
		PollInterval:    -1 * time.Second, // Invalid
		CacheTTL:        500 * time.Millisecond,
		MaxWatchedFiles: 100,
		Audit: AuditConfig{
			Enabled: false, // Disable audit to avoid path validation
		},
	}

	err := config.Validate()
	if err == nil {
		t.Error("Expected validation error for negative poll interval, got nil")
	}

	result := config.ValidateDetailed()
	if result.Valid {
		t.Error("Expected validation result to be invalid")
	}

	if len(result.Errors) == 0 {
		t.Error("Expected validation errors for invalid configuration")
	}

	t.Logf("Validation correctly caught errors: %v", result.Errors)
}
