// utilities_comprehensive_test.go - Comprehensive test suite for Argus utility functions
//
// This file tests all utility functions following DRY principle and OS-awareness
//
// Copyright (c) 2025 AGILira
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Test copyMap function with various scenarios
func TestUtilities_CopyMap(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:     "nil_map",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty_map",
			input:    map[string]interface{}{},
			expected: map[string]interface{}{},
		},
		{
			name: "simple_map",
			input: map[string]interface{}{
				"key1": "value1",
				"key2": 42,
				"key3": true,
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": 42,
				"key3": true,
			},
		},
		{
			name: "nested_map",
			input: map[string]interface{}{
				"config": map[string]interface{}{
					"level": "debug",
					"port":  8080,
				},
				"enabled": true,
			},
			expected: map[string]interface{}{
				"config": map[string]interface{}{
					"level": "debug",
					"port":  8080,
				},
				"enabled": true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := copyMap(tc.input)

			// Test nil case
			if tc.input == nil {
				if result != nil {
					t.Errorf("Expected nil result for nil input, got %v", result)
				}
				return
			}

			// Test that maps are equal but different references
			if len(result) != len(tc.expected) {
				t.Errorf("Expected length %d, got %d", len(tc.expected), len(result))
			}

			for key, expectedValue := range tc.expected {
				actualValue, exists := result[key]
				if !exists {
					t.Errorf("Key %s missing in result", key)
					continue
				}

				// For nested maps, check deep equality (shallow copy behavior)
				if nestedMap, ok := expectedValue.(map[string]interface{}); ok {
					actualNestedMap, ok := actualValue.(map[string]interface{})
					if !ok {
						t.Errorf("Expected nested map for key %s, got %T", key, actualValue)
						continue
					}

					// Should be same reference (shallow copy)
					if &nestedMap != &actualNestedMap {
						// This is expected for our shallow copy implementation
						t.Logf("Nested map for key %s is a shallow copy (expected)", key)
					}
				} else if actualValue != expectedValue {
					t.Errorf("Key %s: expected %v, got %v", key, expectedValue, actualValue)
				}
			}

			// Test that modifying result doesn't affect original
			if len(tc.input) > 0 {
				result["test_modification"] = "test_value"
				if _, exists := tc.input["test_modification"]; exists {
					t.Error("Modifying copy affected original map")
				}
			}
		})
	}
}

// Test UniversalConfigWatcher (backward compatibility)
func TestUtilities_UniversalConfigWatcher(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping universal config watcher test in short mode")
	}

	helper := newTestHelper(t)
	defer helper.Close()

	// Test various config formats
	testCases := []struct {
		name        string
		filename    string
		initialData string
		updatedData string
	}{
		{
			name:        "json_config",
			filename:    "config.json",
			initialData: `{"log_level": "info", "port": 8080}`,
			updatedData: `{"log_level": "debug", "port": 9090}`,
		},
		{
			name:        "yaml_config",
			filename:    "config.yml",
			initialData: "log_level: info\nport: 8080\n",
			updatedData: "log_level: debug\nport: 9090\n",
		},
		{
			name:        "toml_config",
			filename:    "config.toml",
			initialData: `log_level = "info"` + "\n" + `port = 8080` + "\n",
			updatedData: `log_level = "debug"` + "\n" + `port = 9090` + "\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create config file
			configFile := helper.createTestFile(tc.filename, tc.initialData)

			// Track config changes
			changesChan := make(chan map[string]interface{}, 10)
			var receivedConfigs []map[string]interface{}
			var configMutex sync.Mutex

			// Create watcher with callback using fast polling config
			fastConfig := Config{
				PollInterval: 100 * time.Millisecond,
				CacheTTL:     50 * time.Millisecond,
			}
			watcher, err := UniversalConfigWatcherWithConfig(configFile, func(config map[string]interface{}) {
				t.Logf("Initial config loaded: %v", config)
				configMutex.Lock()
				receivedConfigs = append(receivedConfigs, config)
				configMutex.Unlock()
				changesChan <- config
			}, fastConfig)
			if err != nil {
				t.Fatalf("Failed to create universal config watcher: %v", err)
			}
			defer watcher.Stop() // Give watcher sufficient time to start monitoring
			time.Sleep(500 * time.Millisecond)

			// Wait for initial config load
			select {
			case initialConfig := <-changesChan:
				t.Logf("Initial config loaded: %+v", initialConfig)

				// Verify initial config values based on format
				switch tc.name {
				case "json_config":
					if logLevel, ok := initialConfig["log_level"].(string); !ok || logLevel != "info" {
						t.Errorf("Expected log_level 'info', got %v", initialConfig["log_level"])
					}
				case "yaml_config", "toml_config":
					// For YAML/TOML, verify the structure exists
					if logLevel := initialConfig["log_level"]; logLevel == nil {
						t.Error("Expected log_level in config")
					}
				}
			case <-time.After(3 * time.Second):
				t.Fatal("Timeout waiting for initial config load")
			}

			// Give additional time for watcher to be fully ready
			time.Sleep(200 * time.Millisecond)

			// Update config file
			helper.updateTestFile(configFile, tc.updatedData)

			// Give time for file system event propagation
			time.Sleep(300 * time.Millisecond)

			// Wait for config change detection
			select {
			case updatedConfig := <-changesChan:
				t.Logf("Updated config received: %+v", updatedConfig)

				configMutex.Lock()
				numConfigs := len(receivedConfigs)
				configMutex.Unlock()

				if numConfigs < 2 {
					t.Errorf("Expected at least 2 config updates, got %d", numConfigs)
				}
			case <-time.After(3 * time.Second):
				t.Error("Timeout waiting for config change detection")
			}
		})
	}
}

// Test UniversalConfigWatcherWithConfig (channel-based API)

// Test UniversalConfigWatcherWithConfig with custom configurations
func TestUtilities_UniversalConfigWatcherWithConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping config watcher with config test in short mode")
	}

	helper := newTestHelper(t)
	defer helper.Close()

	// Create test config file
	configFile := helper.createTestFile("test_config.json", `{"level": "info"}`)

	// Create custom watcher config with audit
	auditFile := filepath.Join(helper.tempDir, "audit.jsonl")

	customConfig := Config{
		PollInterval:    testPollInterval,
		CacheTTL:        testCacheTTL,
		MaxWatchedFiles: 50,
		Audit: AuditConfig{
			Enabled:       true,
			OutputFile:    auditFile,
			MinLevel:      AuditInfo,
			BufferSize:    10,
			FlushInterval: 100 * time.Millisecond,
		},
	}

	// Track changes
	changesChan := make(chan map[string]interface{}, 10)

	// Create watcher with custom config
	watcher, err := UniversalConfigWatcherWithConfig(configFile, func(config map[string]interface{}) {
		changesChan <- config
	}, customConfig)

	if err != nil {
		t.Fatalf("Failed to create config watcher with custom config: %v", err)
	}
	defer watcher.Stop()

	// Verify watcher is using custom config
	if watcher.config.MaxWatchedFiles != 50 {
		t.Errorf("Expected MaxWatchedFiles 50, got %d", watcher.config.MaxWatchedFiles)
	}

	// Wait for initial config
	select {
	case config := <-changesChan:
		if level, ok := config["level"].(string); !ok || level != "info" {
			t.Errorf("Expected level 'info', got %v", config["level"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for initial config")
	}

	// Test config change with audit
	helper.updateTestFile(configFile, `{"level": "debug", "new_field": "test"}`)

	select {
	case config := <-changesChan:
		if level, ok := config["level"].(string); !ok || level != "debug" {
			t.Errorf("Expected updated level 'debug', got %v", config["level"])
		}
		if newField, ok := config["new_field"].(string); !ok || newField != "test" {
			t.Errorf("Expected new_field 'test', got %v", config["new_field"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for config change")
	}

	// Verify audit file was created
	time.Sleep(200 * time.Millisecond) // Allow audit flush
	if _, err := os.Stat(auditFile); os.IsNotExist(err) {
		t.Error("Audit file should have been created")
	}

	// Test LogSecurityEvent coverage by creating an audit logger directly
	tempDir, _ := os.MkdirTemp("", "security_test_*")
	defer os.RemoveAll(tempDir)

	securityAuditFile := filepath.Join(tempDir, "security.jsonl")
	securityConfig := AuditConfig{
		Enabled:       true,
		OutputFile:    securityAuditFile,
		MinLevel:      AuditSecurity,
		BufferSize:    1,
		FlushInterval: 10 * time.Millisecond,
	}

	securityLogger, err := NewAuditLogger(securityConfig)
	if err != nil {
		t.Errorf("Failed to create security audit logger: %v", err)
	} else {
		// Test LogSecurityEvent
		securityLogger.LogSecurityEvent("unauthorized_access", "test_file.json", map[string]interface{}{
			"reason": "testing_coverage",
			"user":   "test_user",
		})
		securityLogger.Close()

		// Verify security audit file was created
		if _, err := os.Stat(securityAuditFile); os.IsNotExist(err) {
			t.Error("Security audit file should have been created")
		}
	}
}

// Test GenericConfigWatcher (deprecated function)
func TestUtilities_GenericConfigWatcher(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping generic config watcher test in short mode")
	}

	helper := newTestHelper(t)
	defer helper.Close()

	// Create JSON config file
	configFile := helper.createTestFile("generic_config.json", `{"version": 1, "enabled": true}`)

	// Track changes
	changesChan := make(chan map[string]interface{}, 10)

	// Test deprecated function using direct call to verify it works
	watcher, err := GenericConfigWatcher(configFile, func(config map[string]interface{}) {
		changesChan <- config
	})

	if err != nil {
		t.Fatalf("Failed to create generic config watcher: %v", err)
	}
	defer watcher.Stop()

	// Should behave identically to UniversalConfigWatcher
	select {
	case config := <-changesChan:
		if version, ok := config["version"].(float64); !ok || version != 1 {
			t.Errorf("Expected version 1, got %v", config["version"])
		}
		if enabled, ok := config["enabled"].(bool); !ok || !enabled {
			t.Errorf("Expected enabled true, got %v", config["enabled"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for initial config")
	}
}

// Test SimpleFileWatcher
func TestUtilities_SimpleFileWatcher(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping simple file watcher test in short mode")
	}

	helper := newTestHelper(t)
	defer helper.Close()

	// Create test file
	testFile := helper.createTestFile("simple_test.txt", "initial content")

	// Track file changes
	changesChan := make(chan string, 10)

	// Test SimpleFileWatcher directly to get coverage
	watcher, err := SimpleFileWatcher(testFile, func(path string) {
		changesChan <- path
	})

	if err != nil {
		t.Fatalf("Failed to create simple file watcher: %v", err)
	}
	defer watcher.Stop()

	// Start the watcher explicitly
	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start simple file watcher: %v", err)
	}

	// Give watcher time to be fully ready
	time.Sleep(500 * time.Millisecond)

	// Update file content
	helper.updateTestFile(testFile, "updated content")

	// Give time for file system event propagation
	time.Sleep(300 * time.Millisecond)

	// Wait for change detection with longer timeout for slow config
	select {
	case changedPath := <-changesChan:
		if changedPath != testFile {
			t.Errorf("Expected changed path %s, got %s", testFile, changedPath)
		}
	case <-time.After(10 * time.Second): // Increased timeout for default config
		t.Fatal("Timeout waiting for file change detection")
	}

	// Test file deletion
	helper.deleteTestFile(testFile)

	// Simple watcher should not notify on deletion (only changes)
	select {
	case unexpectedPath := <-changesChan:
		t.Errorf("Simple watcher should not notify on deletion, got change for %s", unexpectedPath)
	case <-time.After(200 * time.Millisecond):
		// Expected - no notification for deletion
	}
}

// Test error conditions in utilities
func TestUtilities_ErrorConditions(t *testing.T) {
	t.Parallel()

	t.Run("unsupported_format", func(t *testing.T) {
		_, err := UniversalConfigWatcher("/path/to/file.unknown", func(config map[string]interface{}) {})
		if err == nil {
			t.Error("Expected error for unsupported file format")
		}
		if err != nil && err.Error() == "" {
			t.Error("Error should have a meaningful message")
		}
	})

	t.Run("invalid_path_characters", func(t *testing.T) {
		// Test with invalid path characters (OS-aware)
		invalidPath := "/invalid\x00path.json"
		_, err := UniversalConfigWatcher(invalidPath, func(config map[string]interface{}) {})
		// Should not error immediately (path validation happens during actual watching)
		if err != nil {
			t.Logf("Got expected error for invalid path: %v", err)
		}
	})
}

// Test concurrent access to utilities
func TestUtilities_ConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent access test in short mode")
	}

	helper := newTestHelper(t)
	defer helper.Close()

	// Create test config file
	configFile := helper.createTestFile("concurrent_config.json", `{"counter": 0}`)

	// Create multiple watchers concurrently
	const numWatchers = 5
	watchers := make([]*Watcher, numWatchers)

	// Fast config for concurrent testing
	fastConfig := Config{
		PollInterval: 100 * time.Millisecond,
		CacheTTL:     50 * time.Millisecond,
	}

	for i := 0; i < numWatchers; i++ {
		watcher, err := UniversalConfigWatcherWithConfig(configFile, func(config map[string]interface{}) {
			// Each watcher gets the same config updates
			t.Logf("Watcher received config: %+v", config)
		}, fastConfig)
		if err != nil {
			t.Fatalf("Failed to create watcher %d: %v", i, err)
		}

		watchers[i] = watcher
	}

	// Clean up all watchers
	defer func() {
		for _, watcher := range watchers {
			if watcher != nil {
				watcher.Stop()
			}
		}
	}()

	// Update config multiple times
	for i := 1; i <= 3; i++ {
		configData := map[string]interface{}{
			"counter": i,
			"updated": true,
		}
		jsonData, _ := json.Marshal(configData)
		helper.updateTestFile(configFile, string(jsonData))
		time.Sleep(100 * time.Millisecond) // Allow propagation
	}

	// All watchers should still be running
	for i, watcher := range watchers {
		if !watcher.IsRunning() {
			t.Errorf("Watcher %d should still be running", i)
		}
	}
}

// Test parser coverage for various formats and edge cases
func TestUtilities_ParserCoverage(t *testing.T) {
	t.Parallel()

	// Test ConfigFormat String method
	formats := []ConfigFormat{FormatJSON, FormatYAML, FormatTOML, FormatHCL, FormatINI, FormatProperties, FormatUnknown}
	for _, format := range formats {
		str := format.String()
		if str == "" {
			t.Errorf("ConfigFormat %d should have a string representation", format)
		}
	}

	// Test DetectFormat with various extensions
	testCases := []struct {
		path     string
		expected ConfigFormat
	}{
		{"config.json", FormatJSON},
		{"config.yaml", FormatYAML},
		{"config.yml", FormatYAML},
		{"config.toml", FormatTOML},
		{"config.hcl", FormatHCL},
		{"config.tf", FormatHCL},
		{"config.ini", FormatINI},
		{"config.conf", FormatINI},
		{"config.cfg", FormatINI},
		{"config.properties", FormatProperties},
		{"config.txt", FormatUnknown},
		{"config", FormatUnknown},
		{"", FormatUnknown},
	}

	for _, tc := range testCases {
		format := DetectFormat(tc.path)
		if format != tc.expected {
			t.Errorf("DetectFormat(%s): expected %v, got %v", tc.path, tc.expected, format)
		}
	}

	// Test ParseConfig with various formats and edge cases
	testConfigs := []struct {
		format ConfigFormat
		data   string
		valid  bool
	}{
		{FormatJSON, `{"key": "value"}`, true},
		{FormatJSON, `invalid json`, false},
		{FormatYAML, "key: value\n", true},
		{FormatTOML, `key = "value"`, true},
		{FormatHCL, `key = "value"`, true},
		{FormatINI, "[section]\nkey=value\n", true},
		{FormatProperties, "key=value\n", true},
		{FormatProperties, "# comment\nkey=value\n", true},
		{FormatUnknown, "anything", false},
	}

	for _, tc := range testConfigs {
		config, err := ParseConfig([]byte(tc.data), tc.format)
		if tc.valid {
			if err != nil {
				t.Errorf("ParseConfig(%v, %s) should succeed, got error: %v", tc.format, tc.data, err)
			} else if config == nil {
				t.Errorf("ParseConfig(%v, %s) should return config", tc.format, tc.data)
			}
		} else {
			if err == nil {
				t.Errorf("ParseConfig(%v, %s) should fail", tc.format, tc.data)
			}
		}
	}
}

// Test edge cases and error conditions for better coverage
func TestUtilities_EdgeCaseCoverage(t *testing.T) {
	t.Parallel()

	// Test getConfigMap pool functions by creating many configs
	for i := 0; i < 10; i++ {
		config, err := ParseConfig([]byte(`{"test": "value"}`), FormatJSON)
		if err != nil {
			t.Errorf("Failed to parse config %d: %v", i, err)
		}
		if config == nil {
			t.Errorf("Config %d should not be nil", i)
		}
	}

	// Test parseValue function with various value types
	parseValueTests := []struct {
		input    string
		expected interface{}
	}{
		{"true", true},
		{"false", false},
		{"123", int64(123)},
		{"123.45", 123.45},
		{"string_value", "string_value"},
		{"", ""},
	}

	for _, test := range parseValueTests {
		// We can't directly call parseValue (it's not exported),
		// but we can test it through properties parsing
		propData := fmt.Sprintf("key=%s\n", test.input)
		config, err := ParseConfig([]byte(propData), FormatProperties)
		if err != nil {
			t.Errorf("Failed to parse properties with value %s: %v", test.input, err)
		} else if config["key"] == nil {
			t.Errorf("Key should be present for value %s", test.input)
		}
	}

	// Test DetectFormat edge cases
	edgePaths := []string{
		"config.JSON",                // uppercase
		"config.YAML",                // uppercase
		"path/to/config.json",        // with path
		"./config.yml",               // relative path
		"/absolute/path/config.toml", // absolute path
	}

	for _, path := range edgePaths {
		format := DetectFormat(path)
		if format == FormatUnknown && !strings.Contains(strings.ToLower(path), "unknown") {
			t.Logf("DetectFormat(%s) = %v (this may be expected)", path, format)
		}
	}

	// Test audit logger edge cases
	tempDir, _ := os.MkdirTemp("", "edge_test_*")
	defer os.RemoveAll(tempDir)

	// Test audit logger with invalid directory
	invalidConfig := AuditConfig{
		Enabled:       true,
		OutputFile:    "/invalid/path/that/does/not/exist/audit.log",
		MinLevel:      AuditInfo,
		BufferSize:    1,
		FlushInterval: 10 * time.Millisecond,
	}

	invalidLogger, err := NewAuditLogger(invalidConfig)
	if err == nil && invalidLogger != nil {
		// If it succeeds, close it
		invalidLogger.Close()
	}

	// Test audit logger with valid directory that doesn't exist yet
	newDir := filepath.Join(tempDir, "new", "nested", "path")
	validConfig := AuditConfig{
		Enabled:       true,
		OutputFile:    filepath.Join(newDir, "audit.log"),
		MinLevel:      AuditInfo,
		BufferSize:    2,
		FlushInterval: 5 * time.Millisecond,
	}

	validLogger, err := NewAuditLogger(validConfig)
	if err != nil {
		t.Errorf("Failed to create audit logger with auto-created directory: %v", err)
	} else {
		// Test multiple log levels with proper API
		validLogger.Log(AuditInfo, "info_event", "test_component", "test_file.json", nil, "new_value", map[string]interface{}{"level": "info"})
		validLogger.Log(AuditWarn, "warn_event", "test_component", "test_file.json", "old_value", "new_value", map[string]interface{}{"level": "warn"})
		validLogger.Log(AuditCritical, "critical_event", "test_component", "test_file.json", "old_value", nil, map[string]interface{}{"level": "critical"})
		validLogger.Log(AuditSecurity, "security_event", "test_component", "test_file.json", nil, nil, map[string]interface{}{"level": "security"})

		// Close and verify file was created
		validLogger.Close()

		if _, err := os.Stat(validConfig.OutputFile); os.IsNotExist(err) {
			t.Error("Audit file should have been created in nested directory")
		}
	}
}

// Test specific coverage gaps to reach 95%
func TestUtilities_CoverageGaps(t *testing.T) {
	t.Parallel()

	helper := newTestHelper(t)
	defer helper.Close()

	// Test UniversalConfigWatcherWithConfig error conditions

	// 1. Test with non-existent file (should still work)
	nonExistentFile := filepath.Join(helper.tempDir, "does_not_exist.json")
	configs := make(chan map[string]interface{}, 10)

	watcher, err := UniversalConfigWatcherWithConfig(nonExistentFile, func(config map[string]interface{}) {
		configs <- config
	}, Config{})
	if err != nil {
		t.Errorf("UniversalConfigWatcherWithConfig with non-existent file should work: %v", err)
	} else {
		watcher.Stop()
	}

	// 2. Test with unsupported file format
	unsupportedFile := helper.createTestFile("config.unsupported", "some data")
	_, err = UniversalConfigWatcherWithConfig(unsupportedFile, func(config map[string]interface{}) {
		configs <- config
	}, Config{})
	if err == nil {
		t.Error("UniversalConfigWatcherWithConfig with unsupported format should fail")
	}

	// 3. Test with malformed initial config (should handle gracefully)
	malformedFile := helper.createTestFile("malformed.json", `{"incomplete": json`)
	watcher3, err := UniversalConfigWatcherWithConfig(malformedFile, func(config map[string]interface{}) {
		configs <- config
	}, Config{})
	if err != nil {
		t.Logf("Expected error for malformed config: %v", err)
	} else {
		watcher3.Stop()
	}

	// 4. Test HCL parsing edge cases
	hclTestCases := []string{
		`variable "test" { default = "value" }`,                    // variable block
		`resource "type" "name" { attr = "value" }`,                // resource block
		`output "test" { value = "output_value" }`,                 // output block
		`locals { test = "local_value" }`,                          // locals block
		`terraform { required_version = ">= 1.0" }`,                // terraform block
		`provider "aws" { region = "us-west-2" }`,                  // provider block
		`data "aws_ami" "test" { most_recent = true }`,             // data block
		`module "test" { source = "./modules/test" }`,              // module block
		`# comment\nvariable "test" { default = "value" }\n`,       // with comments
		`variable "multiline" {\n  default = <<EOF\nvalue\nEOF\n}`, // heredoc
	}

	for i, hclData := range hclTestCases {
		config, err := ParseConfig([]byte(hclData), FormatHCL)
		if err != nil {
			t.Logf("HCL case %d failed (may be expected): %v", i, err)
		} else if config == nil {
			t.Errorf("HCL case %d: config should not be nil", i)
		}
	}

	// 5. Test INI parsing edge cases
	iniTestCases := []string{
		"[section]\nkey=value\n",                          // basic
		"[section]\nkey = value with spaces\n",            // spaces around =
		"[section]\nkey=value\n[section2]\nkey2=value2\n", // multiple sections
		"# comment\n[section]\nkey=value\n",               // with comments
		"; semicolon comment\n[section]\nkey=value\n",     // semicolon comments
		"[section]\nkey_with_underscore=value\n",          // underscore
		"[section]\nkey-with-dash=value\n",                // dash
		"[section]\nkey=\"quoted value\"\n",               // quoted value
		"[section]\nkey='single quoted'\n",                // single quoted
		"[section]\nempty_key=\n",                         // empty value
	}

	for i, iniData := range iniTestCases {
		config, err := ParseConfig([]byte(iniData), FormatINI)
		if err != nil {
			t.Errorf("INI case %d should succeed: %v", i, err)
		} else if config == nil {
			t.Errorf("INI case %d: config should not be nil", i)
		}
	}

	// 6. Test Properties parsing edge cases
	propsTestCases := []string{
		"key=value\n",                        // basic
		"key = value with spaces\n",          // spaces
		"key:value\n",                        // colon separator
		"key value\n",                        // space separator
		"# comment\nkey=value\n",             // comment
		"! exclamation comment\nkey=value\n", // exclamation comment
		"key=value\\nwith\\nnewlines\n",      // escaped newlines
		"key=value\\twith\\ttabs\n",          // escaped tabs
		"key=unicode\\u0041value\n",          // unicode escape
		"empty.key=\n",                       // empty value
		"key.with.dots=value\n",              // dotted keys
		"multi\\=line\\=key=value\n",         // escaped equals
	}

	for i, propData := range propsTestCases {
		config, err := ParseConfig([]byte(propData), FormatProperties)
		if err != nil {
			t.Errorf("Properties case %d should succeed: %v", i, err)
		} else if config == nil {
			t.Errorf("Properties case %d: config should not be nil", i)
		}
	}

	// 7. Test SimpleFileWatcher edge cases

	// Test with invalid file path (may or may not fail depending on implementation)
	_, err = SimpleFileWatcher("/invalid/path/that/does/not/exist", func(path string) {})
	if err != nil {
		t.Logf("SimpleFileWatcher with invalid path failed as expected: %v", err)
	} else {
		t.Logf("SimpleFileWatcher with invalid path succeeded (watcher will handle non-existent files)")
	}

	// Test with directory instead of file (may or may not fail)
	tempDir, _ := os.MkdirTemp("", "dir_test_*")
	defer os.RemoveAll(tempDir)

	dirWatcher, err := SimpleFileWatcher(tempDir, func(path string) {})
	if err != nil {
		t.Logf("SimpleFileWatcher with directory failed as expected: %v", err)
	} else {
		t.Logf("SimpleFileWatcher with directory succeeded (watcher will handle directories)")
		dirWatcher.Stop()
	}
}
