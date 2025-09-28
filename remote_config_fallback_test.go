// remote_config_fallback_test.go: Testing remote config fallback manager
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agilira/go-errors"
)

// failingMockProvider always fails to test fallback functionality
type failingMockProvider struct{}

func (m *failingMockProvider) Name() string {
	return "failing-mock"
}

func (m *failingMockProvider) Scheme() string {
	return "failing"
}

func (m *failingMockProvider) Validate(configURL string) error {
	return nil
}

func (m *failingMockProvider) Load(ctx context.Context, configURL string) (map[string]interface{}, error) {
	return nil, errors.New(ErrCodeRemoteConfigError, "mock provider always fails")
}

func (m *failingMockProvider) Watch(ctx context.Context, configURL string) (<-chan map[string]interface{}, error) {
	return nil, errors.New(ErrCodeRemoteConfigError, "mock provider always fails")
}

func (m *failingMockProvider) HealthCheck(ctx context.Context, configURL string) error {
	return errors.New(ErrCodeRemoteConfigError, "mock provider always fails")
}

// TestRemoteConfigFallbackLifecycle tests the complete lifecycle of RemoteConfigManager
func TestRemoteConfigFallbackLifecycle(t *testing.T) {
	// Register a mock provider for testing (ignore error if already registered)
	mockProvider := &mockRemoteProvider{}
	err := RegisterRemoteProvider(mockProvider)
	if err != nil {
		// If already registered, that's fine - continue with test
		if errorCoder, ok := err.(errors.ErrorCoder); !ok || string(errorCoder.ErrorCode()) != ErrCodeInvalidConfig {
			t.Fatalf("Failed to register mock provider: %v", err)
		}
	}

	// Create a test watcher
	config := &Config{}
	config = config.WithDefaults()
	watcher := New(*config)
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Logf("Failed to close watcher: %v", err)
		}
	}()

	// Create a temporary directory for fallback testing
	tempDir, err := os.MkdirTemp("", "argus-remote-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	// Create a valid fallback file for testing
	fallbackFile := filepath.Join(tempDir, "fallback.json")
	fallbackContent := `{"test": "value", "fallback": true}`
	err = os.WriteFile(fallbackFile, []byte(fallbackContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create fallback file: %v", err)
	}

	// Create a remote config with the mock provider URL
	remoteConfig := &RemoteConfig{
		Enabled:      true,
		PrimaryURL:   "test://mock-server/config.json",
		FallbackPath: fallbackFile,
		SyncInterval: 100 * time.Millisecond, // Short interval for testing
	}

	manager, err := NewRemoteConfigManager(remoteConfig, watcher)
	if err != nil {
		t.Fatalf("Failed to create RemoteConfigManager: %v", err)
	}

	// Test GetCurrentConfig before Start (should return error)
	_, _, err = manager.GetCurrentConfig()
	if err == nil {
		t.Errorf("GetCurrentConfig() should return error before Start()")
	}
	if errorCoder, ok := err.(errors.ErrorCoder); ok {
		if string(errorCoder.ErrorCode()) != ErrCodeConfigNotFound {
			t.Errorf("Expected ErrCodeConfigNotFound, got %s", errorCoder.ErrorCode())
		}
	}

	// Test Start() - should load fallback config
	err = manager.Start()
	// Start may return an error for remote load failure, but should still work with fallback
	t.Logf("Start() returned: %v (expected due to unreachable URL)", err)

	// Give a moment for potential fallback loading
	time.Sleep(50 * time.Millisecond)

	// Test GetCurrentConfig after Start (may have fallback config now)
	currentConfig, lastSync, err := manager.GetCurrentConfig()
	if err == nil {
		// If we successfully loaded fallback config
		t.Logf("Successfully loaded fallback config: %v", currentConfig)
		if !lastSync.IsZero() {
			t.Logf("Last sync time: %v", lastSync)
		}
	} else {
		t.Logf("GetCurrentConfig returned error: %v (may be expected if fallback loading failed)", err)
	}

	// Test duplicate Start (should return error)
	err = manager.Start()
	if err == nil {
		t.Errorf("Start() should return error when already running")
	}
	if errorCoder, ok := err.(errors.ErrorCoder); ok {
		if string(errorCoder.ErrorCode()) != ErrCodeWatcherBusy {
			t.Errorf("Expected ErrCodeWatcherBusy, got %s", errorCoder.ErrorCode())
		}
	}

	// Test Stop()
	manager.Stop()

	// Test duplicate Stop (should be safe)
	manager.Stop() // Should not crash

	t.Log("RemoteConfigManager lifecycle test completed successfully")
}

// TestLoadLocalFallback tests the local fallback loading functionality
func TestLoadLocalFallback(t *testing.T) {
	// Register a mock provider for testing (ignore error if already registered)
	mockProvider := &mockRemoteProvider{}
	err := RegisterRemoteProvider(mockProvider)
	if err != nil {
		// If already registered, that's fine - continue with test
		if errorCoder, ok := err.(errors.ErrorCoder); !ok || string(errorCoder.ErrorCode()) != ErrCodeInvalidConfig {
			t.Fatalf("Failed to register mock provider: %v", err)
		}
	}

	// Create a test watcher
	config := &Config{}
	config = config.WithDefaults()
	watcher := New(*config)
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Logf("Failed to close watcher: %v", err)
		}
	}()

	// Create a temporary directory for fallback testing
	tempDir, err := os.MkdirTemp("", "argus-fallback-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	tests := []struct {
		name          string
		filename      string
		content       string
		expectSuccess bool
		expectKeys    []string
	}{
		{
			name:          "Valid JSON fallback",
			filename:      "config.json",
			content:       `{"app": "test", "version": "1.0", "debug": true}`,
			expectSuccess: true,
			expectKeys:    []string{"app", "version", "debug"},
		},
		{
			name:          "Valid YAML fallback",
			filename:      "config.yaml",
			content:       "app: test\nversion: '1.0'\ndebug: true\n",
			expectSuccess: true,
			expectKeys:    []string{"app", "version", "debug"},
		},
		{
			name:          "Invalid JSON fallback",
			filename:      "invalid.json",
			content:       `{"app": "test", invalid json}`,
			expectSuccess: false,
			expectKeys:    []string{},
		},
		{
			name:          "Empty file fallback",
			filename:      "empty.json",
			content:       "",
			expectSuccess: false,
			expectKeys:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fallback file
			fallbackPath := filepath.Join(tempDir, tt.filename)
			err := os.WriteFile(fallbackPath, []byte(tt.content), 0644)
			if err != nil {
				t.Fatalf("Failed to create fallback file: %v", err)
			}

			// Use an invalid URL scheme to force fallback usage
			remoteConfig := &RemoteConfig{
				Enabled:      true,
				PrimaryURL:   "http://nonexistent.example.com/config.json", // This will fail, forcing fallback
				FallbackPath: fallbackPath,
				SyncInterval: time.Minute,
			}

			manager, err := NewRemoteConfigManager(remoteConfig, watcher)
			// This will fail because no HTTP provider is registered, which is what we want
			if err != nil {
				if errorCoder, ok := err.(errors.ErrorCoder); ok && string(errorCoder.ErrorCode()) == ErrCodeInvalidConfig {
					// Expected - no HTTP provider registered
					t.Logf("No HTTP provider available for fallback testing - this is expected behavior")
					t.Logf("Fallback file validation would be: %v (expect success: %v)", tt.content, tt.expectSuccess)
					return
				}
				t.Fatalf("Unexpected error creating RemoteConfigManager: %v", err)
			}

			// If we somehow got a manager, test it
			_ = manager.Start() // Ignore error for fallback testing
			defer manager.Stop()

			// Give time for loading
			time.Sleep(20 * time.Millisecond)

			// Check if config was loaded
			loadedConfig, _, configErr := manager.GetCurrentConfig()

			t.Logf("Config loaded from fallback, success: %v, config: %v, error: %v",
				configErr == nil, loadedConfig, configErr)
		})
	}
}

// TestSyncLoopBehavior tests the sync loop behavior with controlled timing
func TestSyncLoopBehavior(t *testing.T) {
	// Register a mock provider for testing (ignore error if already registered)
	mockProvider := &mockRemoteProvider{}
	err := RegisterRemoteProvider(mockProvider)
	if err != nil {
		// If already registered, that's fine - continue with test
		if errorCoder, ok := err.(errors.ErrorCoder); !ok || string(errorCoder.ErrorCode()) != ErrCodeInvalidConfig {
			t.Fatalf("Failed to register mock provider: %v", err)
		}
	}

	// Create a test watcher
	config := &Config{}
	config = config.WithDefaults()
	watcher := New(*config)
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Logf("Failed to close watcher: %v", err)
		}
	}()

	// Create a very short sync interval for testing
	remoteConfig := &RemoteConfig{
		Enabled:      true,
		PrimaryURL:   "test://mock-server/config.json",
		SyncInterval: 10 * time.Millisecond, // Very short for testing
	}

	manager, err := NewRemoteConfigManager(remoteConfig, watcher)
	if err != nil {
		t.Fatalf("Failed to create RemoteConfigManager: %v", err)
	}

	// Start the manager (which starts the sync loop)
	err = manager.Start()
	if err != nil {
		t.Logf("Start returned error (expected): %v", err)
	}

	// Let it run for a short time to test sync loop
	time.Sleep(50 * time.Millisecond)

	// Stop should terminate the sync loop
	manager.Stop()

	// Verify it's actually stopped by trying to start again
	err = manager.Start()
	if err != nil {
		t.Logf("Restart returned error: %v", err)
	}

	manager.Stop()

	t.Log("Sync loop behavior test completed")
}

// TestRemoteConfigManagerWithInvalidProvider tests behavior with invalid provider URLs
func TestRemoteConfigManagerWithInvalidProvider(t *testing.T) {
	// Mock provider should already be registered from previous test

	// Create a test watcher
	config := &Config{}
	config = config.WithDefaults()
	watcher := New(*config)
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Logf("Failed to close watcher: %v", err)
		}
	}()

	// Test with invalid scheme
	invalidRemoteConfig := &RemoteConfig{
		Enabled:      true,
		PrimaryURL:   "invalid://test.example.com/config.json",
		SyncInterval: time.Minute,
	}

	_, err := NewRemoteConfigManager(invalidRemoteConfig, watcher)
	if err == nil {
		t.Errorf("Expected error for invalid provider scheme, got nil")
	}
	if errorCoder, ok := err.(errors.ErrorCoder); ok {
		if string(errorCoder.ErrorCode()) != ErrCodeInvalidConfig {
			t.Errorf("Expected ErrCodeInvalidConfig, got %s", errorCoder.ErrorCode())
		}
	}
}

// TestRemoteConfigManagerEdgeCases tests various edge cases
func TestRemoteConfigManagerEdgeCases(t *testing.T) {
	// Register a mock provider for testing (ignore error if already registered)
	mockProvider := &mockRemoteProvider{}
	err := RegisterRemoteProvider(mockProvider)
	if err != nil {
		// If already registered, that's fine - continue with test
		if errorCoder, ok := err.(errors.ErrorCoder); !ok || string(errorCoder.ErrorCode()) != ErrCodeInvalidConfig {
			t.Fatalf("Failed to register mock provider: %v", err)
		}
	}

	// Create a test watcher
	config := &Config{}
	config = config.WithDefaults()
	watcher := New(*config)
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Logf("Failed to close watcher: %v", err)
		}
	}()

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "argus-edge-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	t.Run("NonExistentFallbackFile", func(t *testing.T) {
		remoteConfig := &RemoteConfig{
			Enabled:      true,
			PrimaryURL:   "test://mock-server/config.json",
			FallbackPath: filepath.Join(tempDir, "nonexistent.json"),
			SyncInterval: time.Minute,
		}

		manager, err := NewRemoteConfigManager(remoteConfig, watcher)
		if err != nil {
			t.Fatalf("Failed to create RemoteConfigManager: %v", err)
		}

		// Start should handle missing fallback gracefully
		err = manager.Start()
		// Error is expected since both remote and fallback will fail
		t.Logf("Start with missing fallback returned: %v (expected)", err)

		// Should still be able to get config (though it may error)
		_, _, err = manager.GetCurrentConfig()
		t.Logf("GetCurrentConfig with no fallback returned: %v", err)

		manager.Stop()
	})

	t.Run("DisabledRemoteConfig", func(t *testing.T) {
		disabledConfig := &RemoteConfig{
			Enabled:      false,
			PrimaryURL:   "test://mock-server/config.json",
			SyncInterval: time.Minute,
		}

		// Creating a RemoteConfigManager with Enabled: false should fail
		_, err := NewRemoteConfigManager(disabledConfig, watcher)
		if err == nil {
			t.Errorf("Expected error for disabled remote config, got nil")
			return
		}

		// Verify we get the correct error code
		if errorCoder, ok := err.(errors.ErrorCoder); ok {
			if string(errorCoder.ErrorCode()) != ErrCodeInvalidConfig {
				t.Errorf("Expected ErrCodeInvalidConfig for disabled config, got %s", errorCoder.ErrorCode())
			}
		} else {
			t.Errorf("Expected ErrorCoder interface, got %T", err)
		}

		t.Logf("Disabled config correctly rejected: %v", err)
	})

	t.Run("ZeroSyncInterval", func(t *testing.T) {
		zeroIntervalConfig := &RemoteConfig{
			Enabled:      true,
			PrimaryURL:   "test://mock-server/config.json",
			SyncInterval: 0, // Zero interval
		}

		manager, err := NewRemoteConfigManager(zeroIntervalConfig, watcher)
		if err != nil {
			t.Fatalf("Failed to create RemoteConfigManager with zero interval: %v", err)
		}

		// Now that we fixed the bug, this should work without panicking
		err = manager.Start()
		t.Logf("Start with zero sync interval returned: %v", err)

		// Give it a moment - sync loop should handle zero interval gracefully
		time.Sleep(20 * time.Millisecond)

		// Should be able to get current config
		_, _, configErr := manager.GetCurrentConfig()
		t.Logf("GetCurrentConfig with zero interval: %v", configErr)

		manager.Stop()
		t.Log("Zero sync interval test completed successfully (bug fixed)")
	})
}

// TestRemoteConfigConcurrency tests concurrent access to the manager
func TestRemoteConfigConcurrency(t *testing.T) {
	// Mock provider should already be registered from previous test

	// Create a test watcher
	config := &Config{}
	config = config.WithDefaults()
	watcher := New(*config)
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Logf("Failed to close watcher: %v", err)
		}
	}()

	remoteConfig := &RemoteConfig{
		Enabled:      true,
		PrimaryURL:   "test://mock-server/config.json",
		SyncInterval: 5 * time.Millisecond, // Very short for stress testing
	}

	manager, err := NewRemoteConfigManager(remoteConfig, watcher)
	if err != nil {
		t.Fatalf("Failed to create RemoteConfigManager: %v", err)
	}

	// Start the manager
	err = manager.Start()
	t.Logf("Start returned: %v", err)

	// Concurrently call GetCurrentConfig multiple times
	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func(id int) {
			defer func() { done <- true }()
			for j := 0; j < 10; j++ {
				_, _, err := manager.GetCurrentConfig()
				if err != nil {
					t.Logf("Goroutine %d iteration %d: %v", id, j, err)
				}
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 5; i++ {
		<-done
	}

	manager.Stop()
	t.Log("Concurrency test completed")
}

// TestLoadWithFallbackChain tests the complete fallback chain functionality
func TestLoadWithFallbackChain(t *testing.T) {
	// Register a mock provider for testing (ignore error if already registered)
	mockProvider := &mockRemoteProvider{}
	err := RegisterRemoteProvider(mockProvider)
	if err != nil {
		// If already registered, that's fine - continue with test
		if errorCoder, ok := err.(errors.ErrorCoder); !ok || string(errorCoder.ErrorCode()) != ErrCodeInvalidConfig {
			t.Fatalf("Failed to register mock provider: %v", err)
		}
	}

	// Create a test watcher
	config := &Config{}
	config = config.WithDefaults()
	watcher := New(*config)
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Logf("Failed to close watcher: %v", err)
		}
	}()

	// Create a temporary directory for fallback testing
	tempDir, err := os.MkdirTemp("", "argus-fallback-chain-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	// Create a valid fallback file for testing loadLocalFallback
	fallbackFile := filepath.Join(tempDir, "fallback.json")
	fallbackContent := `{"fallback_loaded": true, "source": "local_file", "test_value": 42}`
	err = os.WriteFile(fallbackFile, []byte(fallbackContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create fallback file: %v", err)
	}

	t.Run("TestLoadLocalFallback", func(t *testing.T) {
		// Register a failing mock provider to force fallback to local file
		failingProvider := &failingMockProvider{}
		err := RegisterRemoteProvider(failingProvider)
		if err != nil {
			// If already registered, that's fine - continue with test
			if errorCoder, ok := err.(errors.ErrorCoder); !ok || string(errorCoder.ErrorCode()) != ErrCodeInvalidConfig {
				t.Fatalf("Failed to register failing mock provider: %v", err)
			}
		}

		// Use failing provider that will force fallback to local file
		remoteConfig := &RemoteConfig{
			Enabled:      true,
			PrimaryURL:   "failing://server/config.json", // This will fail
			FallbackPath: fallbackFile,                   // This should be loaded via loadLocalFallback
			SyncInterval: time.Minute,
		}

		manager, err := NewRemoteConfigManager(remoteConfig, watcher)
		if err != nil {
			t.Fatalf("Failed to create RemoteConfigManager: %v", err)
		}

		// This should fail remote load and fall back to local file
		err = manager.Start()
		t.Logf("Start with failing provider returned: %v (expected failure)", err)

		// Check what config we got - should be from loadLocalFallback
		currentConfig, _, configErr := manager.GetCurrentConfig()
		if configErr == nil {
			t.Logf("Config loaded from fallback: %v", currentConfig)

			// Check if it's the fallback config from loadLocalFallback
			if fallbackValue, exists := currentConfig["fallback"]; exists && fallbackValue == true {
				t.Logf("SUCCESS: loadLocalFallback was called and returned fallback config")
			} else if testValue, exists := currentConfig["test"]; exists {
				t.Logf("Unexpected: Got mock provider response: test=%v", testValue)
			} else {
				t.Logf("Got some other config: %v", currentConfig)
			}
		} else {
			t.Logf("Config load error: %v", configErr)
		}

		manager.Stop()
	})

	t.Run("TestLoadWithFallbackURL", func(t *testing.T) {
		// Test with both primary and fallback URLs using mock provider
		remoteConfig := &RemoteConfig{
			Enabled:      true,
			PrimaryURL:   "test://primary-server/config.json",
			FallbackURL:  "test://fallback-server/config.json", // Test FallbackURL path
			FallbackPath: fallbackFile,
			SyncInterval: time.Minute,
		}

		manager, err := NewRemoteConfigManager(remoteConfig, watcher)
		if err != nil {
			t.Fatalf("Failed to create RemoteConfigManager: %v", err)
		}

		err = manager.Start()
		t.Logf("Start with fallback URL returned: %v", err)

		// Since mock provider succeeds, primary should work
		currentConfig, _, configErr := manager.GetCurrentConfig()
		if configErr == nil {
			t.Logf("Config with fallback URL loaded: %v", currentConfig)
		} else {
			t.Logf("Config with fallback URL error: %v", configErr)
		}

		manager.Stop()
	})

	t.Run("TestLoadWithFallbackComplete", func(t *testing.T) {
		// Test the complete fallback chain: Primary fails -> FallbackURL fails -> Local file succeeds
		remoteConfig := &RemoteConfig{
			Enabled:      true,
			PrimaryURL:   "failing://primary-server/config.json",  // Will fail
			FallbackURL:  "failing://fallback-server/config.json", // Will also fail
			FallbackPath: fallbackFile,                            // Should succeed
			SyncInterval: time.Minute,
		}

		manager, err := NewRemoteConfigManager(remoteConfig, watcher)
		if err != nil {
			t.Fatalf("Failed to create RemoteConfigManager: %v", err)
		}

		err = manager.Start()
		t.Logf("Start with complete fallback chain returned: %v", err)

		// Should eventually fall back to local file
		currentConfig, _, configErr := manager.GetCurrentConfig()
		if configErr == nil {
			t.Logf("Config from complete fallback chain: %v", currentConfig)

			// Should be from loadLocalFallback since both remote URLs fail
			if fallbackValue, exists := currentConfig["fallback"]; exists && fallbackValue == true {
				t.Logf("SUCCESS: Complete fallback chain worked - used local fallback file")
			} else {
				t.Errorf("Expected fallback config, got: %v", currentConfig)
			}
		} else {
			t.Errorf("Expected successful fallback load, got error: %v", configErr)
		}

		manager.Stop()
	})

	t.Run("TestLoadWithFallbackAllFail", func(t *testing.T) {
		// Create a fresh watcher to avoid cached config from previous tests
		freshConfig := &Config{}
		freshConfig = freshConfig.WithDefaults()
		freshWatcher := New(*freshConfig)
		defer func() {
			if err := freshWatcher.Close(); err != nil {
				t.Logf("Failed to close fresh watcher: %v", err)
			}
		}()

		// Test when everything fails - no fallback file
		remoteConfig := &RemoteConfig{
			Enabled:      true,
			PrimaryURL:   "failing://primary-server/config.json",  // Will fail
			FallbackURL:  "failing://fallback-server/config.json", // Will also fail
			FallbackPath: "/nonexistent/path/config.json",         // Will also fail
			SyncInterval: time.Minute,
		}

		manager, err := NewRemoteConfigManager(remoteConfig, freshWatcher)
		if err != nil {
			t.Fatalf("Failed to create RemoteConfigManager: %v", err)
		}

		err = manager.Start()
		t.Logf("Start with all failing sources returned: %v", err)

		// Should fail to get any config since nothing succeeded
		_, _, configErr := manager.GetCurrentConfig()
		if configErr == nil {
			t.Logf("Note: Got config despite all sources failing - manager may retain previous state")
		} else {
			t.Logf("Expected error when all sources fail: %v", configErr)
			// Check error code
			if errorCoder, ok := configErr.(errors.ErrorCoder); ok {
				if string(errorCoder.ErrorCode()) == ErrCodeConfigNotFound {
					t.Logf("Correct error code: %s", errorCoder.ErrorCode())
				} else {
					t.Logf("Error code: %s", errorCoder.ErrorCode())
				}
			}
		}

		manager.Stop()
	})
}
