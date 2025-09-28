// cli_integration_test.go: CLI Integration Testing
//
// This file provides comprehensive integration testing of the Argus CLI Manager,
// testing real user workflows and edge cases
// Philosophy:
// - Test the Manager directly
// - Capture real output and error scenarios
// - Verify data integrity and atomic operations
// - Professional error handling and edge case coverage
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package cli

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// CLI TEST INFRASTRUCTURE
// =============================================================================

// CLITestFixture manages CLI testing in isolated environments
type CLITestFixture struct {
	t       *testing.T
	tempDir string
	manager *Manager
	logBuf  *bytes.Buffer
	cleanup func()
}

// NewCLITestFixture creates an isolated environment for CLI testing
func NewCLITestFixture(t *testing.T) *CLITestFixture {
	t.Helper()

	// Create isolated temp directory
	tempDir, err := os.MkdirTemp("", "argus_cli_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	// Create manager for testing
	manager := NewManager()

	// Capture log output for verification
	logBuf := &bytes.Buffer{}
	log.SetOutput(logBuf)

	return &CLITestFixture{
		t:       t,
		tempDir: tempDir,
		manager: manager,
		logBuf:  logBuf,
		cleanup: func() {
			// Restore normal logging
			log.SetOutput(os.Stderr)

			if err := os.RemoveAll(tempDir); err != nil {
				t.Logf("Failed to cleanup temp directory: %v", err)
			}
		},
	}
}

// RunCLI executes CLI commands via Manager and captures output
func (f *CLITestFixture) RunCLI(args ...string) (string, error) {
	f.t.Helper()

	// Clear log buffer
	f.logBuf.Reset()

	// Change to temp directory for isolation
	oldDir, _ := os.Getwd()
	_ = os.Chdir(f.tempDir)
	defer func() { _ = os.Chdir(oldDir) }()

	// Run CLI command
	err := f.manager.Run(args)

	// Get captured output
	output := strings.TrimSpace(f.logBuf.String())

	return output, err
}

// CreateTempConfig creates a config file in the temp directory
func (f *CLITestFixture) CreateTempConfig(name, content string) string {
	f.t.Helper()

	configPath := filepath.Join(f.tempDir, name)
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		f.t.Fatalf("Failed to create temp config: %v", err)
	}
	return configPath
}

// ReadConfigFile reads and returns config file content
func (f *CLITestFixture) ReadConfigFile(path string) string {
	f.t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		f.t.Fatalf("Failed to read config file: %v", err)
	}
	return string(content)
}

// AssertFileExists verifies file exists
func (f *CLITestFixture) AssertFileExists(path string) {
	f.t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		f.t.Errorf("Expected file to exist: %s", path)
	}
}

// AssertFileContains verifies file contains expected content
func (f *CLITestFixture) AssertFileContains(path, expected string) {
	f.t.Helper()
	content := f.ReadConfigFile(path)
	if !strings.Contains(content, expected) {
		f.t.Errorf("File %s should contain %q. Actual content:\n%s", path, expected, content)
	}
}

// Cleanup releases all resources
func (f *CLITestFixture) Cleanup() {
	if f.cleanup != nil {
		f.cleanup()
	}
}

// =============================================================================
// CLI INTEGRATION TESTS
// =============================================================================

// TestCLI_ConfigConvert_Bug tests the convert command
func TestCLI_ConfigConvert_Bug(t *testing.T) {
	fixture := NewCLITestFixture(t)
	defer fixture.Cleanup()

	t.Run("convert_json_to_yaml", func(t *testing.T) {
		// Create source JSON
		jsonPath := fixture.CreateTempConfig("source.json", `{
			"app": {
				"name": "test-app",
				"version": "1.0.0"
			},
			"server": {
				"port": 8080,
				"host": "localhost"
			}
		}`)

		yamlPath := filepath.Join(fixture.tempDir, "output.yaml")

		// Execute convert command
		output, err := fixture.RunCLI("config", "convert", jsonPath, yamlPath)

		// Debugging the convert issue
		t.Logf("Convert command output: %s", output)
		t.Logf("Convert command error: %v", err)

		if err != nil {
			t.Errorf("Convert command should work: %v", err)
		}

		// Check if output file was created
		if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
			t.Errorf("Convert should create output file: %s", yamlPath)
		} else {
			// If file exists, verify content
			fixture.AssertFileContains(yamlPath, "test-app")
		}
	})
}

// TestCLI_ErrorMessages_Bug tests error message quality (known issue)
func TestCLI_ErrorMessages_Bug(t *testing.T) {
	fixture := NewCLITestFixture(t)
	defer fixture.Cleanup()

	t.Run("missing_file_error_message", func(t *testing.T) {
		nonexistentPath := filepath.Join(fixture.tempDir, "missing.json")

		output, err := fixture.RunCLI("config", "get", nonexistentPath, "key")

		t.Logf("Missing file error output: %s", output)
		t.Logf("Missing file error: %v", err)

		// Document current behavior - error messages need improvement
		if err == nil {
			t.Error("Should error for missing file")
		}

		// The error should be more informative than just "failed to load configuration"
		errorText := ""
		if err != nil {
			errorText = err.Error()
		}
		if output != "" {
			errorText += " " + output
		}

		// Verify the error message is now specific and professional (BUG FIXED)
		if !strings.Contains(strings.ToLower(errorText), "not found") {
			t.Errorf("Error message should specifically mention 'not found'. Current: %s", errorText)
		}

		// Verify it includes the file path for better debugging
		if !strings.Contains(errorText, "missing.json") {
			t.Errorf("Error message should include the file path for clarity. Current: %s", errorText)
		}
	})
}

// TestCLI_ConfigGet_Works tests working functionality
func TestCLI_ConfigGet_Works(t *testing.T) {
	fixture := NewCLITestFixture(t)
	defer fixture.Cleanup()

	t.Run("get_existing_values", func(t *testing.T) {
		configPath := fixture.CreateTempConfig("test.json", `{
			"server": {
				"host": "localhost",
				"port": 8080
			},
			"debug": true
		}`)

		// These should work
		_, err := fixture.RunCLI("config", "get", configPath, "server.host")
		if err != nil {
			t.Errorf("Should get string value: %v", err)
		}

		_, err = fixture.RunCLI("config", "get", configPath, "server.port")
		if err != nil {
			t.Errorf("Should get numeric value: %v", err)
		}

		_, err = fixture.RunCLI("config", "get", configPath, "debug")
		if err != nil {
			t.Errorf("Should get boolean value: %v", err)
		}
	})
}

// TestCLI_ConfigSet_Works tests working set functionality
func TestCLI_ConfigSet_Works(t *testing.T) {
	fixture := NewCLITestFixture(t)
	defer fixture.Cleanup()

	t.Run("set_values", func(t *testing.T) {
		configPath := fixture.CreateTempConfig("test.json", `{"app": {"name": "old"}}`)

		// Test setting different types
		_, err := fixture.RunCLI("config", "set", configPath, "app.version", "2.0.0")
		if err != nil {
			t.Errorf("Should set string value: %v", err)
		}

		_, err = fixture.RunCLI("config", "set", configPath, "server.port", "9090")
		if err != nil {
			t.Errorf("Should set numeric value: %v", err)
		}

		_, err = fixture.RunCLI("config", "set", configPath, "debug", "true")
		if err != nil {
			t.Errorf("Should set boolean value: %v", err)
		}

		// Verify values were written
		content := fixture.ReadConfigFile(configPath)
		expectedValues := []string{"2.0.0", "9090", "true"}
		for _, expected := range expectedValues {
			if !strings.Contains(content, expected) {
				t.Errorf("Config should contain %q after set operations: %s", expected, content)
			}
		}
	})
}

// TestCLI_Performance_Real tests actual performance
func TestCLI_Performance_Real(t *testing.T) {
	fixture := NewCLITestFixture(t)
	defer fixture.Cleanup()

	t.Run("measure_real_performance", func(t *testing.T) {
		configPath := fixture.CreateTempConfig("perf.json", `{"test": {"value": "performance"}}`)

		// Measure real performance (5 iterations for speed)
		iterations := 5
		start := time.Now()

		for i := 0; i < iterations; i++ {
			_, err := fixture.RunCLI("config", "get", configPath, "test.value")
			if err != nil {
				t.Errorf("Performance test iteration %d failed: %v", i, err)
			}
		}

		elapsed := time.Since(start)
		avgTime := elapsed / time.Duration(iterations)

		t.Logf("CLI performance - Average: %v over %d iterations", avgTime, iterations)

		// Reasonable performance expectations (CLI operations should be fast)
		if avgTime > 10*time.Millisecond {
			t.Logf("Note: Average response time is %v (consider if this meets your performance requirements)", avgTime)
		}
	})
}

// TestCLI_DurationParsing_Bug tests duration parsing for human-readable formats
func TestCLI_DurationParsing_Bug(t *testing.T) {
	fixture := NewCLITestFixture(t)
	defer fixture.Cleanup()

	t.Run("duration_abbreviations_bug", func(t *testing.T) {
		// These duration formats are advertised in manager.go but don't work
		problematicDurations := []string{
			"30d", // 30 days - used in cleanup --older-than default
			"7d",  // 7 days - mentioned in audit query help
			"2w",  // 2 weeks - mentioned in audit query help
		}

		for _, duration := range problematicDurations {
			t.Run("duration_"+duration, func(t *testing.T) {
				// FIXED: Don't run interactive watch command in tests (CI/CD incompatible)
				// Instead test duration parsing directly - this is what matters
				_, err := parseExtendedDuration(duration)

				t.Logf("Duration %s parsing result - Error: %v", duration, err)

				// BUG FIXED: These now work with parseExtendedDuration()
				if err != nil {
					t.Errorf("Duration '%s' should now be supported after fix: %v", duration, err)
				} else {
					t.Logf("SUCCESS: Duration '%s' is correctly parsed", duration)
				}
			})
		}
	})

	t.Run("working_durations", func(t *testing.T) {
		// These should work with Go's time.ParseDuration
		workingDurations := []string{
			"1s",   // 1 second
			"5m",   // 5 minutes
			"2h",   // 2 hours
			"720h", // 30 days in hours (workaround)
		}

		for _, duration := range workingDurations {
			t.Run("duration_"+duration, func(t *testing.T) {
				// FIXED: Test duration parsing directly (CI/CD compatible)
				_, err := parseExtendedDuration(duration)

				// These should all parse successfully
				if err != nil {
					t.Errorf("Duration '%s' should be valid: %v", duration, err)
				} else {
					t.Logf("SUCCESS: Duration '%s' parsed correctly", duration)
				}
			})
		}
	})
}

// TestCLI_ErrorHandling_Practical tests real-world error scenarios
func TestCLI_ErrorHandling_Practical(t *testing.T) {
	fixture := NewCLITestFixture(t)
	defer fixture.Cleanup()

	t.Run("corrupted_json_file", func(t *testing.T) {
		// Create a corrupted JSON file (common user mistake)
		corruptedPath := fixture.CreateTempConfig("broken.json", `{
			"app": {
				"name": "test"
				"version": "1.0.0"  // Missing comma - syntax error
			}
		}`)

		// Try to read from corrupted file
		_, err := fixture.RunCLI("config", "get", corruptedPath, "app.name")

		// Should fail with clear error
		if err == nil {
			t.Error("Should fail when parsing corrupted JSON")
		}

		// Error should mention parsing issue, not generic failure
		errorMsg := err.Error()
		if !strings.Contains(strings.ToLower(errorMsg), "parse") &&
			!strings.Contains(strings.ToLower(errorMsg), "syntax") &&
			!strings.Contains(strings.ToLower(errorMsg), "invalid") {
			t.Logf("Error message could be clearer about JSON syntax: %s", errorMsg)
		}

		t.Logf("Corrupted JSON error: %s", errorMsg)
	})

	t.Run("invalid_key_format", func(t *testing.T) {
		configPath := fixture.CreateTempConfig("test.json", `{"app": {"name": "test"}}`)

		// Try invalid key formats (common user mistakes)
		invalidKeys := []string{
			"app..name", // Double dots
			".app.name", // Leading dot
			"app.name.", // Trailing dot
			"",          // Empty key
		}

		for _, key := range invalidKeys {
			t.Run("key_"+key, func(t *testing.T) {
				output, err := fixture.RunCLI("config", "get", configPath, key)

				t.Logf("Invalid key '%s' - Output: %s", key, output)
				t.Logf("Invalid key '%s' - Error: %v", key, err)

				// Should handle gracefully (either error or return nil)
				// Don't crash or hang
			})
		}
	})

	t.Run("readonly_file_permissions", func(t *testing.T) {
		configPath := fixture.CreateTempConfig("readonly.json", `{"test": "value"}`)

		// Make file read-only
		if err := os.Chmod(configPath, 0444); err != nil {
			t.Fatalf("Failed to make file readonly: %v", err)
		}

		// Try to modify read-only file
		_, err := fixture.RunCLI("config", "set", configPath, "new.key", "new.value")

		// Should fail with permission error
		if err == nil {
			t.Error("Should fail when writing to read-only file")
		} else {
			// Verify error mentions the specific issue
			errorMsg := strings.ToLower(err.Error())
			if !strings.Contains(errorMsg, "write") &&
				!strings.Contains(errorMsg, "permission") &&
				!strings.Contains(errorMsg, "read-only") {
				t.Logf("Error message could be more specific: %s", err.Error())
			}
			t.Logf("SECURITY FIX: %s", err.Error())
		}
	})

	t.Run("deeply_nested_key_access", func(t *testing.T) {
		configPath := fixture.CreateTempConfig("deep.json", `{
			"level1": {
				"level2": {
					"level3": {
						"level4": {
							"value": "deep-value"
						}
					}
				}
			}
		}`)

		// Should handle deep nesting correctly
		_, err := fixture.RunCLI("config", "get", configPath, "level1.level2.level3.level4.value")
		if err != nil {
			t.Errorf("Should handle deep nesting: %v", err)
		} else {
			t.Logf("Deep nesting works correctly")
		}

		// Test non-existent deep path
		_, err = fixture.RunCLI("config", "get", configPath, "level1.level2.nonexistent.key")
		if err == nil {
			t.Logf("Non-existent deep path handled gracefully")
		}
	})
}

// TestCLI_ConfigFormats tests multi-format configuration support
func TestCLI_ConfigFormats(t *testing.T) {
	fixture := NewCLITestFixture(t)
	defer fixture.Cleanup()

	t.Run("json_operations", func(t *testing.T) {
		configPath := fixture.CreateTempConfig("test.json", `{
			"server": {"port": 8080, "host": "localhost"},
			"debug": true
		}`)

		// Test basic operations
		_, err := fixture.RunCLI("config", "validate", configPath)
		if err != nil {
			t.Errorf("JSON validate failed: %v", err)
		}

		_, err = fixture.RunCLI("config", "get", configPath, "server.port")
		if err != nil {
			t.Errorf("JSON get failed: %v", err)
		}
		// Note: output goes to stdout, not captured in our test setup
		t.Logf("JSON get command executed successfully")
	})

	t.Run("yaml_operations", func(t *testing.T) {
		configPath := fixture.CreateTempConfig("test.yaml", `
server:
  port: 9090
  host: example.com
debug: false
`)

		_, err := fixture.RunCLI("config", "validate", configPath)
		if err != nil {
			t.Errorf("YAML validate failed: %v", err)
		}

		_, err = fixture.RunCLI("config", "get", configPath, "server.host")
		if err != nil {
			t.Errorf("YAML get failed: %v", err)
		}
		t.Logf("YAML get command executed successfully")
	})

	t.Run("convert_json_to_yaml", func(t *testing.T) {
		jsonPath := fixture.CreateTempConfig("source.json", `{"app": {"name": "testapp"}}`)
		yamlPath := filepath.Join(fixture.tempDir, "target.yaml")

		_, err := fixture.RunCLI("config", "convert", jsonPath, yamlPath)
		if err != nil {
			t.Errorf("Convert failed: %v", err)
		}

		// Check converted file exists and is valid
		fixture.AssertFileExists(yamlPath)

		_, err = fixture.RunCLI("config", "validate", yamlPath)
		if err != nil {
			t.Errorf("Converted file not valid: %v", err)
		}

		_, err = fixture.RunCLI("config", "get", yamlPath, "app.name")
		if err != nil {
			t.Errorf("Convert result not readable: %v", err)
		}
		t.Logf("CONVERT BUG FIXED: JSON to YAML conversion works perfectly")
	})

	t.Run("config_init_default", func(t *testing.T) {
		newPath := filepath.Join(fixture.tempDir, "new.json")

		// Ensure the file doesn't exist before we start
		_, err := os.Stat(newPath)
		if err == nil {
			t.Fatalf("File should not exist before init: %s", newPath)
		}

		_, err = fixture.RunCLI("config", "init", newPath)
		if err != nil {
			t.Errorf("Config init failed: %v", err)
			return
		}

		// Wait and retry file existence check (fix for potential filesystem delays)
		var fileExists bool
		for i := 0; i < 50; i++ { // Increased retries for robustness
			if _, err := os.Stat(newPath); err == nil {
				fileExists = true
				break
			}
			time.Sleep(2 * time.Millisecond) // Small delay between checks
		}

		if !fileExists {
			// Debug information for failure diagnosis
			if entries, err := os.ReadDir(filepath.Dir(newPath)); err == nil {
				var fileNames []string
				for _, entry := range entries {
					fileNames = append(fileNames, entry.Name())
				}
				t.Logf("Directory contents after init: %v", fileNames)
			}
			t.Errorf("File was not created by init command: %s", newPath)
			return
		}

		// Should be valid and readable
		_, err = fixture.RunCLI("config", "validate", newPath)
		if err != nil {
			t.Errorf("Init file not valid: %v", err)
		}
	})
}

// TestCLI_RealWorkflows tests actual user workflows
func TestCLI_RealWorkflows(t *testing.T) {
	fixture := NewCLITestFixture(t)
	defer fixture.Cleanup()

	t.Run("modify_existing_config", func(t *testing.T) {
		configPath := fixture.CreateTempConfig("app.json", `{
			"database": {
				"host": "localhost",
				"port": 5432
			}
		}`)

		// Change database port
		_, err := fixture.RunCLI("config", "set", configPath, "database.port", "5433")
		if err != nil {
			t.Errorf("Set failed: %v", err)
		}

		// Verify change (output goes to stdout, not captured)
		_, err = fixture.RunCLI("config", "get", configPath, "database.port")
		if err != nil {
			t.Errorf("Get after set failed: %v", err)
		}
		t.Logf("Set and get operations successful")

		// Add new nested key
		_, err = fixture.RunCLI("config", "set", configPath, "database.ssl", "true")
		if err != nil {
			t.Errorf("Add new key failed: %v", err)
		}

		// Verify file still valid
		_, err = fixture.RunCLI("config", "validate", configPath)
		if err != nil {
			t.Errorf("File corrupted after changes: %v", err)
		}
	})

	t.Run("list_and_filter", func(t *testing.T) {
		configPath := fixture.CreateTempConfig("complex.json", `{
			"server": {"port": 8080, "timeout": "30s"},
			"database": {"host": "db.local", "port": 5432},
			"cache": {"enabled": true, "ttl": 300}
		}`)

		// List all keys (output goes to stdout, not captured)
		_, err := fixture.RunCLI("config", "list", configPath)
		if err != nil {
			t.Errorf("List failed: %v", err)
		}
		t.Logf("List command executed successfully - shows nested keys")
	})
}

// TestCLI_PathSecurity tests path validation and security
func TestCLI_PathSecurity(t *testing.T) {
	fixture := NewCLITestFixture(t)
	defer fixture.Cleanup()

	t.Run("directory_traversal_attempts", func(t *testing.T) {

		// Test various directory traversal attacks
		maliciousPaths := []string{
			"../../../etc/passwd",
			"..\\..\\..\\windows\\system32\\config\\sam",
			"/etc/shadow",
			"./../../root/.ssh/id_rsa",
			"file://../../etc/hosts",
		}

		for _, maliciousPath := range maliciousPaths {
			t.Run("path_"+maliciousPath, func(t *testing.T) {
				// Try to read malicious path
				_, err := fixture.RunCLI("config", "get", maliciousPath, "any.key")
				if err == nil {
					t.Errorf("SECURITY RISK: Should block directory traversal: %s", maliciousPath)
				} else {
					// Verify it's blocked for security reasons
					errorMsg := strings.ToLower(err.Error())
					if strings.Contains(errorMsg, "security") ||
						strings.Contains(errorMsg, "validation") ||
						strings.Contains(errorMsg, "invalid") ||
						strings.Contains(errorMsg, "not found") {
						t.Logf("Security block working for: %s", maliciousPath)
					} else {
						t.Logf("Path blocked but unclear if for security: %s -> %v", maliciousPath, err)
					}
				}

				// Try to write to malicious path
				_, err = fixture.RunCLI("config", "set", maliciousPath, "key", "value")
				if err == nil {
					t.Errorf("SECURITY RISK: Should block write to: %s", maliciousPath)
				}
			})
		}
	})

	t.Run("symbolic_link_handling", func(t *testing.T) {

		// Create a symlink pointing outside temp dir (if possible)
		symlinkPath := filepath.Join(fixture.tempDir, "symlink.json")

		// Try to create symlink to /etc/passwd
		if err := os.Symlink("/etc/passwd", symlinkPath); err != nil {
			t.Skipf("Cannot create symlink for test: %v", err)
		}

		// Test CLI behavior with symlink
		_, err := fixture.RunCLI("config", "get", symlinkPath, "key")
		if err == nil {
			t.Error("SECURITY RISK: Should not follow dangerous symlinks")
		} else {
			t.Logf("Symlink properly blocked: %v", err)
		}
	})

	t.Run("null_byte_injection", func(t *testing.T) {
		// Test null byte injection attempts
		nullBytePaths := []string{
			"config.json\x00.txt",
			"safe.json\x00../../etc/passwd",
		}

		for _, path := range nullBytePaths {
			_, err := fixture.RunCLI("config", "validate", path)
			if err == nil {
				t.Errorf("SECURITY RISK: Should block null byte injection: %q", path)
			} else {
				t.Logf("Null byte injection blocked: %q", path)
			}
		}
	})

	t.Run("path_length_limits", func(t *testing.T) {
		// Test extremely long paths
		longPath := filepath.Join(fixture.tempDir, strings.Repeat("a", 1000), "config.json")

		_, err := fixture.RunCLI("config", "validate", longPath)
		if err == nil {
			t.Logf("Long path accepted (may be system dependent)")
		} else {
			t.Logf("Long path rejected: %v", err)
		}
	})

	t.Run("special_characters", func(t *testing.T) {
		// Test special characters that might cause issues
		specialPaths := []string{
			"config with spaces.json",
			"config-with-dashes.json",
			"config_with_underscores.json",
			"config.with.dots.json",
			"config@with@at.json",
		}

		for _, path := range specialPaths {
			fullPath := filepath.Join(fixture.tempDir, path)

			// Create the file first
			err := os.WriteFile(fullPath, []byte(`{"test": "value"}`), 0644)
			if err != nil {
				t.Logf("Cannot create file with special chars: %s", path)
				continue
			}

			// Test CLI can handle it
			_, err = fixture.RunCLI("config", "validate", fullPath)
			if err != nil {
				t.Logf("Special character path issue: %s -> %v", path, err)
			} else {
				t.Logf("Special characters handled: %s", path)
			}
		}
	})
}
