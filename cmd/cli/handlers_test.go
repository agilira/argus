// handlers_test.go: CLI handlers testing with engineering-grade quality
//
// These tests serve as both safety net and problem-finding tools.
// Each test is designed to catch real-world issues that could occur in production.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agilira/argus"
)

// Test utilities - DRY and reusable across tests

// createTestConfig creates a temporary config file for testing.
// Returns file path and cleanup function.
func createTestConfig(t *testing.T, format argus.ConfigFormat, content string) (string, func()) {
	t.Helper()

	var ext string
	switch format {
	case argus.FormatJSON:
		ext = ".json"
	case argus.FormatYAML:
		ext = ".yaml"
	case argus.FormatTOML:
		ext = ".toml"
	default:
		ext = ".json"
	}

	tmpFile, err := os.CreateTemp("", "argus_cli_test_*"+ext)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		t.Fatalf("Failed to write test config: %v", err)
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpFile.Name())
		t.Fatalf("Failed to close temp file: %v", err)
	}

	return tmpFile.Name(), func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Logf("Failed to cleanup temp file: %v", err)
		}
	}
}

// Test Suite: handleConfigGet - Critical read operations

func TestHandleConfigGet_Success(t *testing.T) {
	// Test the happy path: valid config file with existing key
	testConfig := `{
		"database": {
			"host": "localhost",
			"port": 5432
		},
		"debug": true
	}`

	configPath, cleanup := createTestConfig(t, argus.FormatJSON, testConfig)
	defer cleanup()

	manager := NewManager()

	// Test 1: Get nested key (most common use case)
	t.Run("nested_key_access", func(t *testing.T) {
		// We need to test this differently since orpheus.Context is complex
		// Let's test the core logic by calling the manager methods directly

		format := manager.detectFormat(configPath, "")
		if format != argus.FormatJSON {
			t.Errorf("Expected JSON format, got %v", format)
		}

		config, err := manager.loadConfig(configPath, format)
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		// Verify config loaded correctly
		if config == nil {
			t.Fatal("Config is nil")
		}

		// Test ConfigWriter creation (what handleConfigGet uses internally)
		writer, err := argus.NewConfigWriter(configPath, format, config)
		if err != nil {
			t.Fatalf("Failed to create config writer: %v", err)
		}

		// Test value retrieval
		value := writer.GetValue("database.host")
		if value != "localhost" {
			t.Errorf("Expected 'localhost', got %v", value)
		}

		value = writer.GetValue("database.port")
		if value != float64(5432) { // JSON numbers are float64
			t.Errorf("Expected 5432, got %v", value)
		}
	})

	t.Run("root_key_access", func(t *testing.T) {
		format := manager.detectFormat(configPath, "")
		config, err := manager.loadConfig(configPath, format)
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		writer, err := argus.NewConfigWriter(configPath, format, config)
		if err != nil {
			t.Fatalf("Failed to create config writer: %v", err)
		}

		// Test root-level boolean access
		value := writer.GetValue("debug")
		if value != true {
			t.Errorf("Expected true, got %v", value)
		}
	})
}

func TestHandleConfigGet_ErrorCases(t *testing.T) {
	manager := NewManager()

	// Test 2: Nonexistent file (common user error)
	t.Run("nonexistent_file", func(t *testing.T) {
		nonexistentPath := "/tmp/definitely_does_not_exist_12345.json"

		_, err := manager.loadConfig(nonexistentPath, argus.FormatJSON)
		if err == nil {
			t.Error("Expected error for nonexistent file, got nil")
		}

		// Verify error type is meaningful - updated after bug fix
		if !strings.Contains(err.Error(), "no such file") &&
			!strings.Contains(err.Error(), "cannot find") &&
			!strings.Contains(err.Error(), "does not exist") {
			t.Errorf("Error message should indicate file not found, got: %v", err)
		}
	})

	// Test 3: Invalid JSON (corruption safety net)
	t.Run("corrupted_json", func(t *testing.T) {
		corruptedJSON := `{"database": {"host": "localhost", "port":` // Intentionally broken

		configPath, cleanup := createTestConfig(t, argus.FormatJSON, corruptedJSON)
		defer cleanup()

		_, err := manager.loadConfig(configPath, argus.FormatJSON)
		if err == nil {
			t.Error("Expected error for corrupted JSON, got nil")
		}

		// Verify we get a parsing error
		if !strings.Contains(strings.ToLower(err.Error()), "json") &&
			!strings.Contains(strings.ToLower(err.Error()), "parse") {
			t.Errorf("Error should indicate JSON parsing issue, got: %v", err)
		}
	})

	// Test 4: Key not found (common user error)
	t.Run("key_not_found", func(t *testing.T) {
		validConfig := `{"existing_key": "value"}`

		configPath, cleanup := createTestConfig(t, argus.FormatJSON, validConfig)
		defer cleanup()

		format := manager.detectFormat(configPath, "")
		config, err := manager.loadConfig(configPath, format)
		if err != nil {
			t.Fatalf("Failed to load valid config: %v", err)
		}

		writer, err := argus.NewConfigWriter(configPath, format, config)
		if err != nil {
			t.Fatalf("Failed to create config writer: %v", err)
		}

		// Test nonexistent key
		value := writer.GetValue("nonexistent.key")
		if value != nil {
			t.Errorf("Expected nil for nonexistent key, got %v", value)
		}
	})
}

func TestHandleConfigGet_FormatDetection(t *testing.T) {
	manager := NewManager()

	// Test 5: Format auto-detection (critical for user experience)
	testCases := []struct {
		name     string
		filename string
		format   argus.ConfigFormat
		content  string
	}{
		{
			name:     "json_extension",
			filename: "config.json",
			format:   argus.FormatJSON,
			content:  `{"key": "value"}`,
		},
		{
			name:     "yaml_extension",
			filename: "config.yaml",
			format:   argus.FormatYAML,
			content:  "key: value\n",
		},
		{
			name:     "yml_extension",
			filename: "config.yml",
			format:   argus.FormatYAML,
			content:  "key: value\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create temp file with specific extension
			tmpDir, err := os.MkdirTemp("", "argus_format_test_")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer func() {
				if err := os.RemoveAll(tmpDir); err != nil {
					t.Logf("Failed to cleanup temp dir: %v", err)
				}
			}()

			configPath := filepath.Join(tmpDir, tc.filename)
			if err := os.WriteFile(configPath, []byte(tc.content), 0644); err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			// Test format detection
			detectedFormat := manager.detectFormat(configPath, "")
			if detectedFormat != tc.format {
				t.Errorf("Expected format %v, got %v", tc.format, detectedFormat)
			}

			// Verify it actually loads correctly
			_, err = manager.loadConfig(configPath, detectedFormat)
			if err != nil {
				t.Errorf("Failed to load config with detected format: %v", err)
			}
		})
	}
}

// Test Suite: handleConfigSet - Critical write operations

func TestHandleConfigSet_NewFile(t *testing.T) {
	// Test creating new config file from scratch (common use case)
	manager := NewManager()

	tmpDir, err := os.MkdirTemp("", "argus_set_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("Failed to cleanup temp dir: %v", err)
		}
	}()

	newConfigPath := filepath.Join(tmpDir, "new_config.json")

	t.Run("create_new_json_file", func(t *testing.T) {
		// Simulate the core logic of handleConfigSet for new file
		format := manager.detectFormat(newConfigPath, "")

		var config map[string]interface{}
		_, err := manager.loadConfig(newConfigPath, format)
		if err != nil {
			// File doesn't exist, create empty config (handleConfigSet behavior)
			config = make(map[string]interface{})
		}

		writer, err := argus.NewConfigWriter(newConfigPath, format, config)
		if err != nil {
			t.Fatalf("Failed to create config writer: %v", err)
		}

		// Test setting string value
		if err := writer.SetValue("app.name", "test-app"); err != nil {
			t.Fatalf("Failed to set string value: %v", err)
		}

		// Test setting nested values
		if err := writer.SetValue("database.host", "localhost"); err != nil {
			t.Fatalf("Failed to set nested value: %v", err)
		}

		// Atomic write
		if err := writer.WriteConfig(); err != nil {
			t.Fatalf("Failed to write config: %v", err)
		}

		// Verify file was created and values are correct
		if _, err := os.Stat(newConfigPath); os.IsNotExist(err) {
			t.Error("Config file was not created")
		}

		// Load and verify content
		verifyConfig, err := manager.loadConfig(newConfigPath, format)
		if err != nil {
			t.Fatalf("Failed to load created config: %v", err)
		}

		verifyWriter, err := argus.NewConfigWriter(newConfigPath, format, verifyConfig)
		if err != nil {
			t.Fatalf("Failed to create verify writer: %v", err)
		}

		if value := verifyWriter.GetValue("app.name"); value != "test-app" {
			t.Errorf("Expected 'test-app', got %v", value)
		}

		if value := verifyWriter.GetValue("database.host"); value != "localhost" {
			t.Errorf("Expected 'localhost', got %v", value)
		}
	})
}

func TestHandleConfigSet_ModifyExisting(t *testing.T) {
	// Test modifying existing config (most common production use case)
	initialConfig := `{
		"app": {
			"name": "old-app",
			"version": "1.0.0"
		},
		"database": {
			"host": "old-host",
			"port": 5432
		}
	}`

	configPath, cleanup := createTestConfig(t, argus.FormatJSON, initialConfig)
	defer cleanup()

	manager := NewManager()

	t.Run("modify_existing_values", func(t *testing.T) {
		// Load existing config
		format := manager.detectFormat(configPath, "")
		config, err := manager.loadConfig(configPath, format)
		if err != nil {
			t.Fatalf("Failed to load existing config: %v", err)
		}

		writer, err := argus.NewConfigWriter(configPath, format, config)
		if err != nil {
			t.Fatalf("Failed to create config writer: %v", err)
		}

		// Test modifying existing values
		if err := writer.SetValue("app.name", "new-app"); err != nil {
			t.Fatalf("Failed to update app name: %v", err)
		}

		if err := writer.SetValue("database.host", "new-host"); err != nil {
			t.Fatalf("Failed to update database host: %v", err)
		}

		// Add new nested value
		if err := writer.SetValue("app.debug", true); err != nil {
			t.Fatalf("Failed to add new value: %v", err)
		}

		// Write changes
		if err := writer.WriteConfig(); err != nil {
			t.Fatalf("Failed to write config changes: %v", err)
		}

		// Verify changes were applied correctly
		updatedConfig, err := manager.loadConfig(configPath, format)
		if err != nil {
			t.Fatalf("Failed to reload config: %v", err)
		}

		verifyWriter, err := argus.NewConfigWriter(configPath, format, updatedConfig)
		if err != nil {
			t.Fatalf("Failed to create verify writer: %v", err)
		}

		// Check updated values
		if value := verifyWriter.GetValue("app.name"); value != "new-app" {
			t.Errorf("Expected 'new-app', got %v", value)
		}

		if value := verifyWriter.GetValue("database.host"); value != "new-host" {
			t.Errorf("Expected 'new-host', got %v", value)
		}

		if value := verifyWriter.GetValue("app.debug"); value != true {
			t.Errorf("Expected true, got %v", value)
		}

		// Verify unchanged values remain intact
		if value := verifyWriter.GetValue("app.version"); value != "1.0.0" {
			t.Errorf("Expected unchanged version '1.0.0', got %v", value)
		}

		if value := verifyWriter.GetValue("database.port"); value != float64(5432) {
			t.Errorf("Expected unchanged port 5432, got %v", value)
		}
	})
}

func TestHandleConfigSet_ValueParsing(t *testing.T) {
	// Test automatic value parsing (critical for CLI UX)
	t.Run("value_type_parsing", func(t *testing.T) {
		// Test parseValue function directly (what handleConfigSet uses)
		testCases := []struct {
			input    string
			expected interface{}
			name     string
		}{
			{"true", true, "boolean_true"},
			{"false", false, "boolean_false"},
			{"123", int64(123), "integer"},
			{"3.14", 3.14, "float"},
			{"hello", "hello", "string"},
			{"", "", "empty_string"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := parseValue(tc.input)
				if result != tc.expected {
					t.Errorf("Expected %v (%T), got %v (%T)", tc.expected, tc.expected, result, result)
				}
			})
		}
	})
}

// Test Suite: handleConfigValidate - Critical safety net

func TestHandleConfigValidate_ValidFiles(t *testing.T) {
	// Test validation of correct config files
	manager := NewManager()

	validConfigs := []struct {
		name    string
		format  argus.ConfigFormat
		content string
	}{
		{
			name:   "valid_json",
			format: argus.FormatJSON,
			content: `{
				"app": {
					"name": "test-app",
					"version": "1.0.0"
				},
				"database": {
					"host": "localhost",
					"port": 5432,
					"ssl": true
				}
			}`,
		},
		{
			name:   "valid_yaml",
			format: argus.FormatYAML,
			content: `app:
  name: test-app
  version: 1.0.0
database:
  host: localhost
  port: 5432
  ssl: true
`,
		},
		{
			name:    "minimal_json",
			format:  argus.FormatJSON,
			content: `{}`,
		},
	}

	for _, tc := range validConfigs {
		t.Run(tc.name, func(t *testing.T) {
			configPath, cleanup := createTestConfig(t, tc.format, tc.content)
			defer cleanup()

			// Test the core validation logic from handleConfigValidate
			format := manager.detectFormat(configPath, "")
			_, err := manager.loadConfig(configPath, format)

			if err != nil {
				t.Errorf("Expected valid config to pass validation, got error: %v", err)
			}

			// Verify format detection is correct
			if format != tc.format {
				t.Errorf("Expected format %v, got %v", tc.format, format)
			}
		})
	}
}

func TestHandleConfigValidate_InvalidFiles(t *testing.T) {
	// Test validation catches corrupted files (critical safety net)
	manager := NewManager()

	invalidConfigs := []struct {
		name           string
		format         argus.ConfigFormat
		content        string
		expectedErrMsg string
	}{
		{
			name:           "broken_json_syntax",
			format:         argus.FormatJSON,
			content:        `{"app": {"name": "test", "port":}`, // Missing value
			expectedErrMsg: "json",
		},
		{
			name:           "broken_json_quotes",
			format:         argus.FormatJSON,
			content:        `{"app": {name": "test"}}`, // Missing quote
			expectedErrMsg: "json",
		},

		{
			name:           "empty_file",
			format:         argus.FormatJSON,
			content:        "",
			expectedErrMsg: "json",
		},
	}

	for _, tc := range invalidConfigs {
		t.Run(tc.name, func(t *testing.T) {
			configPath, cleanup := createTestConfig(t, tc.format, tc.content)
			defer cleanup()

			// Test validation catches the corruption
			format := manager.detectFormat(configPath, "")
			_, err := manager.loadConfig(configPath, format)

			if err == nil {
				t.Errorf("Expected validation to fail for corrupted %s config, got nil error", tc.name)
				return
			}

			// Verify error message contains format info (helpful for users)
			errMsg := strings.ToLower(err.Error())
			if !strings.Contains(errMsg, tc.expectedErrMsg) {
				t.Errorf("Expected error to mention '%s', got: %v", tc.expectedErrMsg, err)
			}
		})
	}
}

func TestHandleConfigValidate_FileNotFound(t *testing.T) {
	// Test validation handles missing files gracefully
	manager := NewManager()

	t.Run("nonexistent_file", func(t *testing.T) {
		nonexistentPath := "/tmp/definitely_does_not_exist_validate_test.json"

		// Test validation of nonexistent file
		format := manager.detectFormat(nonexistentPath, "")
		_, err := manager.loadConfig(nonexistentPath, format)

		if err == nil {
			t.Error("Expected validation to fail for nonexistent file, got nil error")
		}

		// Verify it's a file-related error, not a parsing error
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "no such file") &&
			!strings.Contains(errMsg, "cannot find") &&
			!strings.Contains(errMsg, "not exist") {
			t.Errorf("Expected file-not-found error, got: %v", err)
		}
	})
}
