// security_critical_test.go - testing security critical functions
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIsSystemDirectory_Coverage targets isSystemDirectory function
func TestIsSystemDirectory_Coverage(t *testing.T) {
	config := &Config{}
	config = config.WithDefaults()
	watcher := New(*config)

	// Test system directories that should return true
	systemPaths := []string{
		"/etc/passwd",
		"/proc/version",
		"/sys/devices",
		"/dev/null",
		"c:\\windows\\system32\\config",
		"C:\\Program Files\\test",
	}

	for _, path := range systemPaths {
		if !watcher.isSystemDirectory(path) {
			t.Errorf("isSystemDirectory(%q) should return true for system path", path)
		}
	}

	// Test safe paths that should return false
	safePaths := []string{
		"/home/user/config.json",
		"/tmp/test.json",
		"config/app.yaml",
		"./local.toml",
		"",
	}

	for _, path := range safePaths {
		if watcher.isSystemDirectory(path) {
			t.Errorf("isSystemDirectory(%q) should return false for safe path", path)
		}
	}
}

// TestGetWriter_Coverage targets GetWriter function
func TestGetWriter_Coverage(t *testing.T) {
	config := &Config{}
	config = config.WithDefaults()
	watcher := New(*config)

	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.json")

	// Test basic GetWriter functionality
	writer, err := watcher.GetWriter(testFile, FormatJSON, map[string]interface{}{
		"test": "data",
	})

	if err != nil {
		t.Errorf("GetWriter failed: %v", err)
	}

	if writer == nil {
		t.Error("GetWriter returned nil writer")
	}

	// Test with different formats
	formats := []ConfigFormat{FormatJSON, FormatYAML, FormatTOML}
	for _, format := range formats {
		testPath := filepath.Join(tempDir, "test_"+format.String()+".conf")
		w, e := watcher.GetWriter(testPath, format, nil)
		if e != nil {
			t.Errorf("GetWriter failed for format %v: %v", format, e)
		}
		if w == nil {
			t.Errorf("GetWriter returned nil for format %v", format)
		}
	}
}

// TestIsRelativePathSafe_Coverage targets isRelativePathSafe function
func TestIsRelativePathSafe_Coverage(t *testing.T) {
	// Safe paths
	safePaths := []string{
		"config.json",
		"app/config.yaml",
		"modules/user/settings.toml",
	}

	for _, path := range safePaths {
		if !isRelativePathSafe(path) {
			t.Errorf("isRelativePathSafe(%q) should return true for safe path", path)
		}
	}

	// Unsafe paths
	unsafePaths := []string{
		"../config.json",   // Parent directory
		"../../etc/passwd", // Path traversal
		"/etc/passwd",      // Absolute path
		".",                // Current directory
		"..",               // Parent directory
		".config",          // Hidden file
	}

	for _, path := range unsafePaths {
		if isRelativePathSafe(path) {
			t.Errorf("isRelativePathSafe(%q) should return false for unsafe path", path)
		}
	}
}

// TestValidateSymlinks_Coverage improves validateSymlinks coverage
func TestValidateSymlinks_Coverage(t *testing.T) {
	config := &Config{}
	config = config.WithDefaults()
	config.Audit.Enabled = false // Disable audit for clean test
	watcher := New(*config)

	tempDir := t.TempDir()

	// Create a regular file
	regularFile := filepath.Join(tempDir, "regular.json")
	err := os.WriteFile(regularFile, []byte(`{"test": true}`), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test regular file (should pass)
	err = watcher.validateSymlinks(regularFile, "regular.json")
	if err != nil {
		t.Errorf("validateSymlinks failed for regular file: %v", err)
	}

	// Test nonexistent file (should pass - symlink check will be skipped)
	nonexistent := filepath.Join(tempDir, "nonexistent.json")
	err = watcher.validateSymlinks(nonexistent, "nonexistent.json")
	if err != nil {
		t.Errorf("validateSymlinks failed for nonexistent file: %v", err)
	}
}

// TestSecurityFunctionsIntegration_Coverage verifies security functions work together
func TestSecurityFunctionsIntegration_Coverage(t *testing.T) {
	config := &Config{}
	config = config.WithDefaults()
	config.Audit.Enabled = false
	watcher := New(*config)

	// Verify system directory detection blocks dangerous paths
	dangerousPaths := []string{"/etc/passwd", "/proc/version", "/sys/kernel"}
	for _, path := range dangerousPaths {
		if !watcher.isSystemDirectory(path) {
			t.Errorf("Security gap: %s not detected as system directory", path)
		}
	}

	// Verify path safety checks work
	traversalAttempts := []string{"../../../etc/passwd", "config/../../../sensitive"}
	for _, path := range traversalAttempts {
		if isRelativePathSafe(path) {
			t.Errorf("Security gap: %s considered safe relative path", path)
		}
	}

	t.Log("Security critical functions coverage tests completed")
}
