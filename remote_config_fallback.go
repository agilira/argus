// remote_config_fallback.go: Remote configuration with automatic fallback capabilities
//
// This file implements enterprise-grade remote configuration loading with resilient
// fallback mechanisms for production deployments. The implementation follows the
// zero-allocation design principles of Argus while providing robust error handling
// and automatic recovery capabilities.
//
// Fallback sequence implementation:
// 1. Primary remote source (e.g., consul://prod-consul/config)
// 2. Fallback remote source (e.g., consul://backup-consul/config)
// 3. Local fallback file (e.g., /etc/app/emergency-config.json)
// 4. Continuous sync with automatic recovery
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agilira/go-errors"
)

// RemoteConfigManager manages remote configuration loading with automatic fallback.
// This struct encapsulates all remote configuration state and provides thread-safe
// operations for loading, watching, and fallback management.
//
// Zero-allocation design: Pre-allocates all necessary structures and reuses
// contexts, channels, and error objects to minimize heap pressure during operations.
//
// Thread safety: All methods are safe for concurrent use and employ atomic operations
// for state management to avoid lock contention in hot paths.
type RemoteConfigManager struct {
	config  *RemoteConfig
	watcher *Watcher // Back-reference for error handling and audit logging

	// Atomic state management (zero-allocation)
	running  atomic.Bool
	lastSync atomic.Int64 // Unix nano timestamp of last successful sync

	// Current configuration cache (atomic pointer for lock-free reads)
	currentConfig atomic.Pointer[map[string]interface{}]

	// Cancellation and synchronization
	ctx       context.Context
	cancel    context.CancelFunc
	syncMutex sync.Mutex // Protects sync operations (not hot path)

	// lifecycleMu guards ctx, cancel and done, which Start replaces on every
	// run so a manager can be stopped and started again.
	lifecycleMu sync.Mutex

	// done is closed by syncLoop on exit so Stop can wait for it, which is
	// what Stop's documentation has always promised.
	done chan struct{}
}

// NewRemoteConfigManager creates a new remote configuration manager.
// This constructor validates the RemoteConfig settings and initializes all
// necessary state for zero-allocation operation.
//
// Parameters:
//   - config: RemoteConfig settings with URLs, timeouts, and fallback paths
//   - watcher: Parent Watcher for error handling and audit integration
//
// Returns:
//   - *RemoteConfigManager: Configured manager ready for Start()
//   - error: Configuration validation errors
func NewRemoteConfigManager(config *RemoteConfig, watcher *Watcher) (*RemoteConfigManager, error) {
	if config == nil {
		return nil, errors.New(ErrCodeInvalidConfig, "RemoteConfig cannot be nil")
	}

	if !config.Enabled {
		return nil, errors.New(ErrCodeInvalidConfig, "RemoteConfig is not enabled")
	}

	// The manager logs every sync through watcher.auditLogger, so a nil watcher
	// is a nil dereference waiting for the first Start rather than a usable
	// configuration.
	if watcher == nil {
		return nil, errors.New(ErrCodeInvalidConfig, "RemoteConfigManager requires a watcher")
	}

	if config.PrimaryURL == "" {
		return nil, errors.New(ErrCodeInvalidConfig, "RemoteConfig PrimaryURL is required when enabled")
	}

	// Validate URLs by attempting to parse them
	if err := validateRemoteURL(config.PrimaryURL); err != nil {
		return nil, errors.Wrap(err, ErrCodeInvalidConfig, "invalid PrimaryURL")
	}

	if config.FallbackURL != "" {
		if err := validateRemoteURL(config.FallbackURL); err != nil {
			return nil, errors.Wrap(err, ErrCodeInvalidConfig, "invalid FallbackURL")
		}
	}

	// Validate fallback path if provided
	if config.FallbackPath != "" {
		if !filepath.IsAbs(config.FallbackPath) && !isRelativePathSafe(config.FallbackPath) {
			return nil, errors.New(ErrCodeInvalidConfig, "FallbackPath must be absolute or safe relative path")
		}
	}

	// Defaults are applied to a COPY. The caller keeps ownership of the
	// RemoteConfig it passed in: writing defaults straight into it changed a
	// struct the application may still be reading, and rewrote the very
	// Config.Remote a Watcher was built from.
	settings := *config

	// Validate and set SyncInterval with safe default
	if settings.SyncInterval <= 0 {
		settings.SyncInterval = 30 * time.Second // Safe default to prevent NewTicker panic
	}

	// Validate Timeout with safe default
	if settings.Timeout <= 0 {
		settings.Timeout = 10 * time.Second // Safe default
	}

	// Ensure Timeout is not longer than SyncInterval
	if settings.Timeout >= settings.SyncInterval {
		settings.Timeout = settings.SyncInterval / 2
	}

	ctx, cancel := context.WithCancel(context.Background())

	manager := &RemoteConfigManager{
		config:  &settings,
		watcher: watcher,
		ctx:     ctx,
		cancel:  cancel,
		done:    make(chan struct{}),
	}

	return manager, nil
}

// Start begins remote configuration synchronization.
// This method starts a background goroutine that periodically loads configuration
// from remote sources according to the SyncInterval setting.
//
// The method performs an immediate initial load to populate the configuration cache
// before starting the periodic sync loop. If the initial load fails across all
// fallback sources, an error is returned but the sync loop continues for recovery.
//
// Zero-allocation sync loop: The background goroutine reuses contexts, timers,
// and error objects to minimize garbage collection pressure.
//
// Returns:
//   - error: Initial configuration load errors (sync continues in background)
func (r *RemoteConfigManager) Start() error {
	r.lifecycleMu.Lock()
	if !r.running.CompareAndSwap(false, true) {
		r.lifecycleMu.Unlock()
		return errors.New(ErrCodeWatcherBusy, "RemoteConfigManager is already running")
	}

	// Each run gets its own context and completion channel. Stop cancelled and
	// closed the previous pair, so reusing them would have started a sync loop
	// that exits immediately and then closes an already-closed channel.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	r.ctx, r.cancel, r.done = ctx, cancel, done
	r.lifecycleMu.Unlock()

	// Perform initial configuration load
	config, err := r.loadWithFallback(ctx)
	if err != nil {
		// Continue with sync loop even if initial load fails for recovery
		r.auditRemote(AuditInfo, "remote_config_initial_load_failed", r.config.PrimaryURL, map[string]interface{}{"error": err.Error()})
	} else {
		r.currentConfig.Store(&config)
		r.lastSync.Store(time.Now().UnixNano())
		r.auditRemote(AuditInfo, "remote_config_initial_load_success", r.config.PrimaryURL, nil)
	}

	// Start background sync loop
	go r.syncLoop(ctx, done)

	return err
}

// auditRemote records a remote-configuration event.
//
// The component is "argus" and the event names the action, matching every
// other audit call site in the library. The call sites here used to pass
// "remote_config" as the EVENT and the action as the COMPONENT, so filtering
// an audit query by Component or by EventPrefix skipped remote records
// entirely.
func (r *RemoteConfigManager) auditRemote(level AuditLevel, event, filePath string, context map[string]interface{}) {
	r.watcher.auditLogger.Log(level, event, "argus", filePath, nil, nil, context)
}

// Stop terminates remote configuration synchronization.
// This method gracefully stops the background sync loop and cleans up resources.
//
// The method blocks until the sync loop has fully terminated to ensure clean
// shutdown and prevent resource leaks.
//
// Thread safety: Safe to call multiple times and from multiple goroutines.
func (r *RemoteConfigManager) Stop() {
	if !r.running.CompareAndSwap(true, false) {
		return // Already stopped
	}

	r.lifecycleMu.Lock()
	cancel, done := r.cancel, r.done
	r.lifecycleMu.Unlock()

	cancel()

	// Wait for syncLoop to exit. Returning straight after cancel() left a sync
	// possibly still in flight, so "Stop returned" did not mean "nothing is
	// touching the remote any more" — which is exactly what the caller needs
	// before tearing down what the sync writes into.
	<-done
}

// GetCurrentConfig returns the most recently loaded configuration.
// This method provides lock-free access to the current configuration cache
// using atomic pointer operations for maximum performance.
//
// Returns:
//   - map[string]interface{}: Current configuration (may be nil if not yet loaded)
//   - time.Time: Timestamp of last successful configuration load
//   - error: ErrCodeConfigNotFound if no configuration has been loaded
func (r *RemoteConfigManager) GetCurrentConfig() (map[string]interface{}, time.Time, error) {
	configPtr := r.currentConfig.Load()
	if configPtr == nil {
		return nil, time.Time{}, errors.New(ErrCodeConfigNotFound, "no remote configuration loaded")
	}

	lastSync := time.Unix(0, r.lastSync.Load())
	return *configPtr, lastSync, nil
}

// syncLoop runs the periodic configuration synchronization.
// This method implements the zero-allocation sync loop that periodically loads
// configuration from remote sources and updates the cache.
//
// The loop uses a timer for precise interval control and reuses contexts
// to minimize allocations during steady-state operation.
// syncLoop takes the context and completion channel of the run that started
// it, rather than reading the manager's fields: Start may hand a fresh pair to
// a later run while this one is still unwinding.
func (r *RemoteConfigManager) syncLoop(ctx context.Context, done chan struct{}) {
	defer close(done)

	ticker := time.NewTicker(r.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.performSync(ctx)

		case <-ctx.Done():
			return
		}
	}
}

// performSync executes a single configuration synchronization cycle.
// This method attempts to load configuration using the fallback sequence
// and updates the cache atomically if successful.
func (r *RemoteConfigManager) performSync(ctx context.Context) {
	r.syncMutex.Lock()
	defer r.syncMutex.Unlock()

	config, err := r.loadWithFallback(ctx)
	if err != nil {
		r.auditRemote(AuditWarn, "remote_config_sync_failed", r.config.PrimaryURL, map[string]interface{}{"error": err.Error()})

		// Call error handler if configured
		if r.watcher.config.ErrorHandler != nil {
			r.watcher.config.ErrorHandler(err, r.config.PrimaryURL)
		}
		return
	}

	// Update cache atomically
	r.currentConfig.Store(&config)
	r.lastSync.Store(time.Now().UnixNano())
	r.auditRemote(AuditInfo, "remote_config_sync_success", r.config.PrimaryURL, nil)
}

// loadWithFallback implements the complete fallback sequence for configuration loading.
// This method attempts each configured source in order until one succeeds or all fail.
//
// Fallback sequence:
// 1. PrimaryURL with retries
// 2. FallbackURL with retries (if configured)
// 3. FallbackPath local file (if configured)
//
// Returns:
//   - map[string]interface{}: Loaded configuration
//   - error: Combined errors from all failed attempts
func (r *RemoteConfigManager) loadWithFallback(ctx context.Context) (map[string]interface{}, error) {
	var lastErr error

	// Attempt 1: Primary remote URL
	if config, err := r.loadRemoteWithRetries(ctx, r.config.PrimaryURL); err == nil {
		return config, nil
	} else {
		lastErr = err
	}

	// Attempt 2: Fallback remote URL (if configured)
	if r.config.FallbackURL != "" {
		if config, err := r.loadRemoteWithRetries(ctx, r.config.FallbackURL); err == nil {
			r.auditRemote(AuditWarn, "remote_config_fallback_url_used", r.config.FallbackURL, nil)
			return config, nil
		} else {
			lastErr = err
		}
	}

	// Attempt 3: Local fallback file (if configured)
	if r.config.FallbackPath != "" {
		if config, err := r.loadLocalFallback(); err == nil {
			r.auditRemote(AuditCritical, "remote_config_fallback_file_used", r.config.FallbackPath, nil)
			return config, nil
		} else {
			lastErr = err
		}
	}

	return nil, errors.Wrap(lastErr, ErrCodeRemoteConfigError, "all remote configuration sources failed")
}

// loadRemoteWithRetries attempts to load from a remote URL with exponential backoff.
func (r *RemoteConfigManager) loadRemoteWithRetries(parent context.Context, url string) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(parent, r.config.Timeout)
	defer cancel()

	var lastErr error

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: wait RetryDelay * 2^(attempt-1)
			// Safe calculation to prevent integer overflow
			var delay time.Duration
			if attempt > 30 {
				// Cap exponential growth to prevent overflow
				delay = r.config.RetryDelay * time.Duration(1<<30)
			} else {
				delay = r.config.RetryDelay * time.Duration(1<<(attempt-1))
			}

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, errors.Wrap(ctx.Err(), ErrCodeRemoteConfigError, "timeout during retry delay")
			}
		}

		// Retries belong to THIS loop, which implements the documented
		// RetryDelay * 2^N schedule. Calling the loader with its own defaults
		// nested a second retry policy inside each attempt: 3 outer attempts
		// became 12 requests to the remote, with constant one-second delays
		// that appear nowhere in the documented schedule.
		config, err := LoadRemoteConfigWithContext(ctx, url, &RemoteConfigOptions{
			Timeout:       r.config.Timeout,
			RetryAttempts: 0,
			RetryDelay:    r.config.RetryDelay,
		})
		if err == nil {
			return config, nil
		}

		lastErr = err

		// Check if we should stop retrying (e.g., authentication errors)
		if shouldStopRetrying(err) {
			break
		}
	}

	return nil, lastErr
}

// loadLocalFallback loads configuration from the local fallback file.
//
// The format is detected from the extension and parsed with the same universal
// parser as every other configuration file, so JSON, YAML, TOML, HCL, INI and
// Properties all work here exactly as the FallbackPath documentation says.
//
// This used to return a hardcoded {"fallback": true, "source": …, "message": …}
// without ever opening the file, and to report success doing it. That is the
// worst possible moment to be wrong: the fallback file is read only when every
// remote source is already down, so an application asking for its emergency
// database URL received three meaningless keys and no indication that anything
// had failed. A fallback that cannot be read is reported as an error, which
// lets the caller keep serving the last configuration it already had.
func (r *RemoteConfigManager) loadLocalFallback() (map[string]interface{}, error) {
	path := r.config.FallbackPath

	if err := ValidateSecurePath(path); err != nil {
		return nil, errors.Wrap(err, ErrCodeInvalidConfig, "unsafe fallback path").
			WithContext("fallback_path", path)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.Wrap(err, ErrCodeInvalidConfig, "invalid fallback path").
			WithContext("fallback_path", path)
	}

	format := DetectFormat(absPath)
	if format == FormatUnknown {
		return nil, errors.New(ErrCodeInvalidConfig, "unsupported fallback configuration format").
			WithContext("fallback_path", absPath)
	}

	// #nosec G304 -- absPath is validated by ValidateSecurePath above
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, errors.Wrap(err, ErrCodeFileNotFound, "failed to read fallback configuration").
			WithContext("fallback_path", absPath)
	}

	config, err := ParseConfig(data, format)
	if err != nil {
		return nil, errors.Wrap(err, ErrCodeInvalidConfig,
			"failed to parse "+format.String()+" fallback configuration").
			WithContext("fallback_path", absPath)
	}

	return config, nil
}

// validateRemoteURL validates that a URL is parseable and has a supported scheme.
func validateRemoteURL(url string) error {
	_, err := validateAndGetProvider(url)
	return err
}

// isRelativePathSafe checks if a relative path is safe (no traversal attempts).
func isRelativePathSafe(path string) bool {
	if path == "" {
		return false
	}

	// Must not start with / (Unix-style absolute path)
	if len(path) > 0 && path[0] == '/' {
		return false
	}

	clean := filepath.Clean(path)

	// Must be relative (not absolute)
	if filepath.IsAbs(clean) {
		return false
	}

	// Must not start with . (current directory or hidden files)
	if len(clean) > 0 && clean[0] == '.' {
		return false
	}

	// Must not contain path traversal (..)
	if strings.Contains(clean, "..") {
		return false
	}

	// On Windows, filepath.Clean converts forward slashes to backslashes
	// We need to normalize both paths for comparison
	normalizedOriginal := filepath.ToSlash(path)
	normalizedClean := filepath.ToSlash(clean)

	// Clean path should match original (no normalization needed)
	return normalizedClean == normalizedOriginal
}
