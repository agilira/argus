package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agilira/argus"
)

// TestNewManager verifies proper initialization of CLI manager.
// Validates core components and default state without external dependencies.
func TestNewManager(t *testing.T) {
	manager := NewManager()

	// Core validation: manager must not be nil
	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}

	// Core validation: internal app must be initialized
	if manager.app == nil {
		t.Fatal("Manager.app not initialized")
	}

	// Default state: audit logger should be nil until explicitly set
	if manager.auditLogger != nil {
		t.Error("Manager.auditLogger should be nil by default")
	}
}

// TestManagerWithAudit verifies audit logger integration.
// Tests fluent interface and proper state management.
func TestManagerWithAudit(t *testing.T) {
	// Create a unique temp directory for this test's SQLite database
	// This avoids "database is locked" errors on Windows CI
	tempDir, err := os.MkdirTemp("", "argus_manager_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	auditDBPath := filepath.Join(tempDir, "manager_audit.db")

	// Configure audit with test-specific database path
	auditConfig := argus.AuditConfig{
		Enabled:       true,
		OutputFile:    auditDBPath,
		MinLevel:      argus.AuditInfo,
		BufferSize:    100,
		FlushInterval: 1 * time.Second,
		IncludeStack:  false,
	}

	auditLogger, err := argus.NewAuditLogger(auditConfig)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer func() {
		if err := auditLogger.Close(); err != nil {
			t.Logf("Failed to close auditLogger: %v", err)
		}
	}()

	// Test fluent interface - separate manager creation and validation
	baseManager := NewManager()
	if baseManager == nil {
		t.Fatal("NewManager() returned nil")
	}

	manager := baseManager.WithAudit(auditLogger)

	// Validate manager is not nil
	if manager == nil {
		t.Fatal("WithAudit() returned nil manager")
	}

	// Validate audit logger was set (now safe from nil dereference)
	if manager.auditLogger == nil {
		t.Error("WithAudit() did not set audit logger")
	}

	// Validate fluent interface returns same instance
	if manager != baseManager {
		t.Error("WithAudit() should return same manager instance for chaining")
	}
}

// TestDetectFormat verifies format detection logic.
// Tests both explicit format parsing and auto-detection from file paths.
func TestDetectFormat(t *testing.T) {
	manager := NewManager()

	// Test cases covering common scenarios
	tests := []struct {
		name           string
		filePath       string
		explicitFormat string
		expected       argus.ConfigFormat
	}{
		// Explicit format tests - should ignore file extension
		{"explicit json", "config.yaml", "json", argus.FormatJSON},
		{"explicit yaml", "config.json", "yaml", argus.FormatYAML},
		{"explicit toml", "config.ini", "toml", argus.FormatTOML},

		// Auto-detection tests - should use file extension
		{"auto json", "config.json", "auto", argus.FormatJSON},
		{"auto yaml", "config.yaml", "auto", argus.FormatYAML},
		{"auto yml", "config.yml", "auto", argus.FormatYAML},

		// Edge cases
		{"empty explicit", "config.json", "", argus.FormatJSON},
		{"unknown format", "config.xyz", "unknown", argus.FormatUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.detectFormat(tt.filePath, tt.explicitFormat)
			if result != tt.expected {
				t.Errorf("detectFormat(%q, %q) = %v, want %v",
					tt.filePath, tt.explicitFormat, result, tt.expected)
			}
		})
	}
}

// TestParseValue verifies automatic type parsing from string values.
// Tests detection of boolean, integer, float, and string types.
func TestParseValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected interface{}
	}{
		// Boolean parsing (only explicit true/false, case insensitive)
		{"true boolean", "true", true},
		{"false boolean", "false", false},
		{"True boolean", "True", true}, // Case insensitive
		{"FALSE boolean", "FALSE", false},

		// Integer parsing
		{"positive int", "42", int64(42)},
		{"negative int", "-123", int64(-123)},
		{"zero int", "0", int64(0)},

		// Float parsing
		{"positive float", "3.14", 3.14},
		{"negative float", "-2.5", -2.5},
		{"zero float", "0.0", 0.0},

		// String fallback (values that don't parse as other types)
		{"plain string", "hello", "hello"},
		{"mixed string", "123abc", "123abc"},
		{"empty string", "", ""},
		{"space string", " ", " "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseValue(tt.input)
			if result != tt.expected {
				t.Errorf("parseValue(%q) = %v (%T), want %v (%T)",
					tt.input, result, result, tt.expected, tt.expected)
			}
		})
	}
}

// TestLoadConfig verifies configuration file loading and parsing.
// Tests with temporary files to ensure real file I/O works correctly.
func TestLoadConfig(t *testing.T) {
	manager := NewManager()

	// Helper function to create temporary config file
	createTempConfig := func(content, extension string) string {
		tempFile := t.TempDir() + "/config." + extension
		if err := os.WriteFile(tempFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		return tempFile
	}

	tests := []struct {
		name        string
		content     string
		extension   string
		expectError bool
		expectKeys  []string // Keys that should exist in result
	}{
		{
			name:        "valid json",
			content:     `{"app": {"name": "test"}, "debug": true}`,
			extension:   "json",
			expectError: false,
			expectKeys:  []string{"app", "debug"},
		},
		{
			name:        "valid yaml",
			content:     "app:\n  name: test\ndebug: true",
			extension:   "yaml",
			expectError: false,
			expectKeys:  []string{"app", "debug"},
		},
		{
			name:        "invalid json",
			content:     `{"invalid": json}`, // Missing quotes around json
			extension:   "json",
			expectError: true,
			expectKeys:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempFile := createTempConfig(tt.content, tt.extension)
			format := manager.detectFormat(tempFile, "auto")

			config, err := manager.loadConfig(tempFile, format)

			// Check error expectation
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check expected keys exist (only for successful cases)
			if !tt.expectError && config != nil {
				for _, key := range tt.expectKeys {
					if _, exists := config[key]; !exists {
						t.Errorf("Expected key %q not found in config", key)
					}
				}
			}
		})
	}
}
