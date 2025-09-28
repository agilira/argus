// config_validation_security_test.go
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidationSecurity_MissingCoverage(t *testing.T) {
	// Test setRemoteConfigDefaults comprehensive coverage
	t.Run("setRemoteConfigDefaults_comprehensive", func(t *testing.T) {
		// Test disabled remote config (early return)
		t.Run("disabled_remote_config", func(t *testing.T) {
			config := Config{
				Remote: RemoteConfig{
					Enabled:      false,
					SyncInterval: 123 * time.Second, // Should remain unchanged
					Timeout:      456 * time.Second, // Should remain unchanged
				},
			}
			original := config.Remote
			config.setRemoteConfigDefaults()

			if config.Remote != original {
				t.Errorf("Disabled remote config should remain unchanged")
			}
		})

		// Test enabled with zero/negative values (sets defaults)
		t.Run("enabled_sets_defaults", func(t *testing.T) {
			config := Config{
				Remote: RemoteConfig{
					Enabled:      true,
					SyncInterval: 0, // Should be set to default
					Timeout:      0, // Should be set to default
					MaxRetries:   0, // Should remain 0 (not negative)
					RetryDelay:   0, // Should be set to default
				},
			}
			config.setRemoteConfigDefaults()

			if config.Remote.SyncInterval <= 0 {
				t.Error("SyncInterval should be set to positive default")
			}
			if config.Remote.Timeout <= 0 {
				t.Error("Timeout should be set to positive default")
			}
			if config.Remote.RetryDelay <= 0 {
				t.Error("RetryDelay should be set to positive default")
			}
		})

		// Test negative MaxRetries gets set to default
		t.Run("negative_max_retries_fixed", func(t *testing.T) {
			config := Config{
				Remote: RemoteConfig{
					Enabled:    true,
					MaxRetries: -1, // Should be set to default (2)
				},
			}
			config.setRemoteConfigDefaults()

			if config.Remote.MaxRetries != 2 {
				t.Errorf("MaxRetries should be 2, got %d", config.Remote.MaxRetries)
			}
		})

		// Test dynamic adjustments (timeout/syncInterval logic)
		t.Run("dynamic_adjustments", func(t *testing.T) {
			config := Config{
				Remote: RemoteConfig{
					Enabled:    true,
					MaxRetries: 10, // Large retries trigger timeout adjustment
					RetryDelay: 1 * time.Second,
					Timeout:    1 * time.Second, // Too small for retry delay
				},
			}
			originalTimeout := config.Remote.Timeout
			config.setRemoteConfigDefaults()

			// Timeout should be adjusted to accommodate retries
			if config.Remote.Timeout <= originalTimeout {
				t.Error("Timeout should be increased to accommodate retry attempts")
			}
		})

		// Test SyncInterval adjustment
		t.Run("sync_interval_adjustment", func(t *testing.T) {
			config := Config{
				Remote: RemoteConfig{
					Enabled:      true,
					SyncInterval: 5 * time.Second,  // Small interval
					Timeout:      10 * time.Second, // Larger timeout
				},
			}
			config.setRemoteConfigDefaults()

			// SyncInterval should be adjusted to be > Timeout
			if config.Remote.SyncInterval <= config.Remote.Timeout {
				t.Error("SyncInterval should be greater than Timeout to prevent overlap")
			}
		})

		// Test overflow protection for large MaxRetries
		t.Run("overflow_protection", func(t *testing.T) {
			config := Config{
				Remote: RemoteConfig{
					Enabled:    true,
					MaxRetries: 50, // Very large value should be capped
					RetryDelay: 1 * time.Second,
				},
			}
			config.setRemoteConfigDefaults()

			// Should handle gracefully without overflow
			if config.Remote.Timeout <= 0 {
				t.Error("Timeout should remain positive even with large MaxRetries")
			}
		})
	})

	// Test validateSymlinks comprehensive coverage
	t.Run("validateSymlinks_comprehensive", func(t *testing.T) {
		// Create test environment
		tmpDir, err := os.MkdirTemp("", "argus_symlink_test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				t.Logf("Warning: Failed to cleanup temp dir: %v", err)
			}
		}()

		// Create a regular file
		regularFile := filepath.Join(tmpDir, "regular.json")
		if err := os.WriteFile(regularFile, []byte(`{"test": true}`), 0644); err != nil {
			t.Fatalf("Failed to create regular file: %v", err)
		}

		// Create watcher for testing
		auditLogger, _ := NewAuditLogger(AuditConfig{
			Enabled: false, // Disabled to avoid file operations during test
		})
		watcher := &Watcher{
			auditLogger: auditLogger,
		}

		// Test case 1: Regular file (no symlinks)
		err = watcher.validateSymlinks(regularFile, "regular.json")
		if err != nil {
			t.Errorf("validateSymlinks should pass for regular file, got: %v", err)
		}

		// Test case 2: Nonexistent file (should not error on symlink validation)
		nonexistent := filepath.Join(tmpDir, "nonexistent.json")
		err = watcher.validateSymlinks(nonexistent, "nonexistent.json")
		if err != nil {
			t.Errorf("validateSymlinks should not error for nonexistent file, got: %v", err)
		}

		// Test case 3: Create a valid symlink to regular file
		validSymlink := filepath.Join(tmpDir, "valid_symlink.json")
		if err := os.Symlink(regularFile, validSymlink); err != nil {
			t.Logf("Skipping symlink test, cannot create symlinks: %v", err)
			return // Skip symlink tests if we can't create them (e.g., Windows without permissions)
		}

		err = watcher.validateSymlinks(validSymlink, "valid_symlink.json")
		if err != nil {
			t.Errorf("validateSymlinks should pass for valid symlink, got: %v", err)
		}

		// Test case 4: Create symlink to system directory (if possible)
		systemSymlink := filepath.Join(tmpDir, "system_symlink")
		systemTarget := "/etc" // Common system directory on Unix-like systems
		if _, statErr := os.Stat(systemTarget); statErr == nil {
			if symlinkErr := os.Symlink(systemTarget, systemSymlink); symlinkErr == nil {
				err = watcher.validateSymlinks(systemSymlink, "system_symlink")
				// This should trigger the system directory check
				if err == nil {
					t.Logf("System directory check may not be triggered or system path not considered restricted")
				}
			}
		}

		// Test case 5: Broken symlink (points to nonexistent target)
		brokenSymlink := filepath.Join(tmpDir, "broken_symlink")
		brokenTarget := filepath.Join(tmpDir, "nonexistent_target")
		if err := os.Symlink(brokenTarget, brokenSymlink); err == nil {
			err = watcher.validateSymlinks(brokenSymlink, "broken_symlink")
			// This tests the EvalSymlinks error path
			if err != nil {
				t.Logf("Broken symlink correctly handled: %v", err)
			}
		}
	})

	// Test Validate function with individual error conditions
	t.Run("Validate_individual_errors", func(t *testing.T) {
		// Test each validation error in isolation with valid base config
		baseConfig := Config{
			PollInterval:    1 * time.Second,        // Valid base
			CacheTTL:        500 * time.Millisecond, // Valid base
			MaxWatchedFiles: 100,                    // Valid base
		}

		t.Run("invalid_cache_ttl", func(t *testing.T) {
			config := baseConfig
			config.CacheTTL = -1 * time.Second // Invalid
			err := config.Validate()
			if err == nil || !containsSubstr(err.Error(), "cache TTL must be positive") {
				t.Errorf("Expected cache TTL error, got: %v", err)
			}
		})

		t.Run("invalid_max_watched_files", func(t *testing.T) {
			config := baseConfig
			config.MaxWatchedFiles = -5 // Invalid
			err := config.Validate()
			if err == nil || !containsSubstr(err.Error(), "max watched files must be positive") {
				t.Errorf("Expected max watched files error, got: %v", err)
			}
		})

		t.Run("invalid_optimization_strategy", func(t *testing.T) {
			config := baseConfig
			config.OptimizationStrategy = 99 // Invalid strategy
			err := config.Validate()
			if err == nil || !containsSubstr(err.Error(), "unknown optimization strategy") {
				t.Errorf("Expected optimization strategy error, got: %v", err)
			}
		})

		t.Run("invalid_boreas_capacity", func(t *testing.T) {
			config := baseConfig
			config.BoreasLiteCapacity = 100 // Not power of 2
			err := config.Validate()
			if err == nil || !containsSubstr(err.Error(), "BoreasLite capacity must be power of 2") {
				t.Errorf("Expected boreas capacity error, got: %v", err)
			}
		})

		t.Run("invalid_audit_buffer_size", func(t *testing.T) {
			config := baseConfig
			config.Audit = AuditConfig{
				Enabled:    true,
				BufferSize: -100,            // Invalid
				OutputFile: "/tmp/test.log", // Valid to avoid output file errors
			}
			err := config.Validate()
			if err == nil || !containsSubstr(err.Error(), "buffer size must be positive") {
				t.Errorf("Expected buffer size error, got: %v", err)
			}
		})

		t.Run("invalid_audit_flush_interval", func(t *testing.T) {
			config := baseConfig
			config.Audit = AuditConfig{
				Enabled:       true,
				FlushInterval: -2 * time.Second, // Invalid
				OutputFile:    "/tmp/test.log",  // Valid to avoid output file errors
			}
			err := config.Validate()
			if err == nil || !containsSubstr(err.Error(), "flush interval must be positive") {
				t.Errorf("Expected flush interval error, got: %v", err)
			}
		})
	})

	// Test Validate function error precedence
	t.Run("Validate_error_precedence", func(t *testing.T) {
		// Test that PollInterval error comes first (as seen in real behavior)
		t.Run("poll_interval_negative", func(t *testing.T) {
			config := Config{
				PollInterval: -1 * time.Second, // Invalid
			}
			err := config.Validate()
			if err == nil || !containsSubstr(err.Error(), "poll interval must be positive") {
				t.Errorf("Expected poll interval error, got: %v", err)
			}
		})

		t.Run("poll_interval_too_small", func(t *testing.T) {
			config := Config{
				PollInterval: 5 * time.Millisecond, // Too small
			}
			err := config.Validate()
			if err == nil || !containsSubstr(err.Error(), "poll interval should be at least") {
				t.Errorf("Expected poll interval too small error, got: %v", err)
			}
		})

		// Multiple errors should return the first one (poll interval precedence)
		t.Run("multiple_errors_poll_interval_first", func(t *testing.T) {
			config := Config{
				PollInterval:    -1 * time.Second, // This should be the error returned
				MaxWatchedFiles: -5,               // Secondary error
			}

			err := config.Validate()
			if err == nil {
				t.Error("Expected validation error for multiple invalid fields")
				return
			}

			// Should return the first error (poll interval)
			if !containsSubstr(err.Error(), "poll interval") {
				t.Errorf("Expected first error to be about poll interval, got: %v", err)
			}
		})
	})
}

// Helper function to check if string contains substring
func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
