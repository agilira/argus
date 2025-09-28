// Package main provides tests for the OTEL integration example
//
// Copyright (c) 2025 AGILira - A. Giordano
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	argus "github.com/agilira/argus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

// MockAuditInterface implements AuditInterface for testing
type MockAuditInterface struct {
	logCalls            int
	configChangeCalls   int
	fileWatchCalls      int
	securityEventCalls  int
	flushCalls          int
	flushError          error
	lastLevel           argus.AuditLevel
	lastEvent           string
	lastComponent       string
	lastFilePath        string
	lastContext         map[string]interface{}
	lastOldConfig       map[string]interface{}
	lastNewConfig       map[string]interface{}
	lastSecurityEvent   string
	lastSecurityDetails string
	lastSecurityContext map[string]interface{}
	lastFileEvent       string
	lastFileWatchPath   string
}

// Log implements AuditInterface.Log for testing
func (m *MockAuditInterface) Log(level argus.AuditLevel, event, component, filePath string, oldVal, newVal interface{}, context map[string]interface{}) {
	m.logCalls++
	m.lastLevel = level
	m.lastEvent = event
	m.lastComponent = component
	m.lastFilePath = filePath
	m.lastContext = context
}

// LogConfigChange implements AuditInterface.LogConfigChange for testing
func (m *MockAuditInterface) LogConfigChange(filePath string, oldConfig, newConfig map[string]interface{}) {
	m.configChangeCalls++
	m.lastFilePath = filePath
	m.lastOldConfig = oldConfig
	m.lastNewConfig = newConfig
}

// LogFileWatch implements AuditInterface.LogFileWatch for testing
func (m *MockAuditInterface) LogFileWatch(event, filePath string) {
	m.fileWatchCalls++
	m.lastFileEvent = event
	m.lastFileWatchPath = filePath
}

// LogSecurityEvent implements AuditInterface.LogSecurityEvent for testing
func (m *MockAuditInterface) LogSecurityEvent(event, details string, context map[string]interface{}) {
	m.securityEventCalls++
	m.lastSecurityEvent = event
	m.lastSecurityDetails = details
	m.lastSecurityContext = context
}

// Flush implements AuditInterface.Flush for testing
func (m *MockAuditInterface) Flush() error {
	m.flushCalls++
	return m.flushError
}

// TestInitOTEL tests the OTEL initialization function
func TestInitOTEL(t *testing.T) {
	tests := []struct {
		name      string
		wantError bool
	}{
		{
			name:      "successful initialization",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset any global OTEL state
			otel.SetTracerProvider(noop.NewTracerProvider())

			tp, err := initOTEL()
			if (err != nil) != tt.wantError {
				t.Errorf("initOTEL() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if !tt.wantError {
				if tp == nil {
					t.Error("initOTEL() returned nil TracerProvider but no error")
					return
				}

				// Test that tracer provider is properly set
				tracer := otel.Tracer("test")
				if tracer == nil {
					t.Error("Failed to get tracer from global provider")
				}

				// Cleanup
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if err := tp.Shutdown(ctx); err != nil {
					t.Errorf("Failed to shutdown tracer provider: %v", err)
				}
			}
		})
	}
}

// TestRunDemo tests the demonstration function
func TestRunDemo(t *testing.T) {
	tests := []struct {
		name        string
		logger      *MockAuditInterface
		flushError  error
		expectCalls map[string]int
	}{
		{
			name:       "successful demo execution",
			logger:     &MockAuditInterface{},
			flushError: nil,
			expectCalls: map[string]int{
				"config":    1,
				"security":  1,
				"filewatch": 1,
				"log":       21, // 1 general + 20 performance test
				"flush":     1,
			},
		},
		{
			name:       "demo with flush error",
			logger:     &MockAuditInterface{},
			flushError: fmt.Errorf("flush failed"), // Use a simple error for testing
			expectCalls: map[string]int{
				"config":    1,
				"security":  1,
				"filewatch": 1,
				"log":       21,
				"flush":     1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.logger.flushError = tt.flushError

			// Capture panics
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("runDemo() panicked: %v", r)
				}
			}()

			runDemo(tt.logger)

			// Verify call counts
			if tt.logger.configChangeCalls != tt.expectCalls["config"] {
				t.Errorf("Expected %d config change calls, got %d",
					tt.expectCalls["config"], tt.logger.configChangeCalls)
			}
			if tt.logger.securityEventCalls != tt.expectCalls["security"] {
				t.Errorf("Expected %d security event calls, got %d",
					tt.expectCalls["security"], tt.logger.securityEventCalls)
			}
			if tt.logger.fileWatchCalls != tt.expectCalls["filewatch"] {
				t.Errorf("Expected %d file watch calls, got %d",
					tt.expectCalls["filewatch"], tt.logger.fileWatchCalls)
			}
			if tt.logger.logCalls != tt.expectCalls["log"] {
				t.Errorf("Expected %d log calls, got %d",
					tt.expectCalls["log"], tt.logger.logCalls)
			}
			if tt.logger.flushCalls != tt.expectCalls["flush"] {
				t.Errorf("Expected %d flush calls, got %d",
					tt.expectCalls["flush"], tt.logger.flushCalls)
			}

			// Verify last calls contain expected data
			if tt.logger.lastSecurityEvent != "failed_login" {
				t.Errorf("Expected last security event 'failed_login', got '%s'",
					tt.logger.lastSecurityEvent)
			}
			if tt.logger.lastFileEvent != "file_modified" {
				t.Errorf("Expected last file event 'file_modified', got '%s'",
					tt.logger.lastFileEvent)
			}
			if tt.logger.lastLevel != argus.AuditInfo {
				t.Errorf("Expected last log level AuditInfo, got %v", tt.logger.lastLevel)
			}
		})
	}
}

// TestRunDemoPerformance tests the performance aspects of runDemo
func TestRunDemoPerformance(t *testing.T) {
	logger := &MockAuditInterface{}

	start := time.Now()
	runDemo(logger)
	duration := time.Since(start)

	// Should complete within reasonable time (5 seconds is very generous)
	if duration > 5*time.Second {
		t.Errorf("runDemo() took too long: %v", duration)
	}

	// Verify performance test generated expected number of events
	expectedPerfEvents := 20
	if logger.logCalls != expectedPerfEvents+1 { // +1 for the general audit log
		t.Errorf("Expected %d total log calls, got %d", expectedPerfEvents+1, logger.logCalls)
	}
}

// TestMainIntegration tests main function integration with environment variables
func TestMainIntegration(t *testing.T) {
	// This test verifies the main flow without actually running main()
	// We test the logic that would be in main() by testing its components

	tests := []struct {
		name        string
		disableOTEL string
		expectOTEL  bool
	}{
		{
			name:        "OTEL enabled (default)",
			disableOTEL: "",
			expectOTEL:  true,
		},
		{
			name:        "OTEL disabled via env var",
			disableOTEL: "true",
			expectOTEL:  false,
		},
		{
			name:        "OTEL enabled via env var false",
			disableOTEL: "false",
			expectOTEL:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			oldValue := os.Getenv("DISABLE_OTEL")
			defer func() {
				if oldValue == "" {
					if err := os.Unsetenv("DISABLE_OTEL"); err != nil {
						t.Errorf("Failed to unset DISABLE_OTEL: %v", err)
					}
				} else {
					if err := os.Setenv("DISABLE_OTEL", oldValue); err != nil {
						t.Errorf("Failed to restore DISABLE_OTEL: %v", err)
					}
				}
			}()

			if tt.disableOTEL != "" {
				if err := os.Setenv("DISABLE_OTEL", tt.disableOTEL); err != nil {
					t.Fatalf("Failed to set DISABLE_OTEL: %v", err)
				}
			}

			// Test the environment check logic
			otelEnabled := os.Getenv("DISABLE_OTEL") != "true"
			if otelEnabled != tt.expectOTEL {
				t.Errorf("Expected OTEL enabled = %v, got %v", tt.expectOTEL, otelEnabled)
			}

			if otelEnabled {
				// Test OTEL initialization
				tp, err := initOTEL()
				if err != nil {
					t.Logf("OTEL initialization failed (expected in CI): %v", err)
					return // Skip rest of test if OTEL can't initialize
				}

				if tp != nil {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					if err := tp.Shutdown(ctx); err != nil {
						t.Errorf("Failed to shutdown tracer provider: %v", err)
					}
				}
			}
		})
	}
}

// TestAuditLoggerCreation tests the creation of audit logger with various configs
func TestAuditLoggerCreation(t *testing.T) {
	tests := []struct {
		name          string
		configSetup   func() argus.AuditConfig
		expectError   bool
		errorContains string
	}{
		{
			name: "default config",
			configSetup: func() argus.AuditConfig {
				config := argus.DefaultAuditConfig()
				config.MinLevel = argus.AuditInfo
				return config
			},
			expectError: false,
		},
		{
			name: "empty config works with defaults",
			configSetup: func() argus.AuditConfig {
				return argus.AuditConfig{} // Empty struct works with defaults
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.configSetup()

			auditLogger, err := argus.NewAuditLogger(config)

			if (err != nil) != tt.expectError {
				t.Errorf("NewAuditLogger() error = %v, expectError %v", err, tt.expectError)
				return
			}

			if tt.expectError && tt.errorContains != "" {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got no error", tt.errorContains)
				} else if !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing '%s', got '%v'", tt.errorContains, err)
				}
			}

			if !tt.expectError && auditLogger != nil {
				// Test that we can use the logger
				auditLogger.Log(argus.AuditInfo, "test", "test", "", nil, nil, nil)

				if err := auditLogger.Close(); err != nil {
					t.Errorf("Failed to close audit logger: %v", err)
				}
			}
		})
	}
}

// contains checks if a string contains a substring (helper function)
func contains(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) &&
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}

// BenchmarkRunDemo benchmarks the runDemo function
func BenchmarkRunDemo(b *testing.B) {
	logger := &MockAuditInterface{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runDemo(logger)
	}
}

// BenchmarkInitOTEL benchmarks the OTEL initialization
func BenchmarkInitOTEL(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tp, err := initOTEL()
		if err != nil {
			b.Fatalf("initOTEL() failed: %v", err)
		}

		// Cleanup each iteration
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if err := tp.Shutdown(ctx); err != nil {
			b.Errorf("Failed to shutdown: %v", err)
		}
		cancel()
	}
}
