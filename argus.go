// argus: Ultra-lightweight configuration watcher with BoreasLite ultra-fast ring buffer
//
// Philosophy:
// - Minimal dependencies (AGILira ecosystem only: go-errors, go-timecache)
// - Polling-based approach for maximum OS portability
// - Intelligent caching to minimize os.Stat() syscalls (like go-timecache)
// - Thread-safe atomic operations
// - Zero allocations in hot paths
// - Configurable polling intervals
//
// Example Usage:
//   watcher := argus.New(argus.Config{
//       PollInterval: 5 * time.Second,
//       CacheTTL:     2 * time.Second,
//   })
//
//   watcher.Watch("config.json", func(event argus.ChangeEvent) {
//       // Handle configuration change
//       newConfig, err := LoadConfig(event.Path)
//       if err == nil {
//           atomicLevel.SetLevel(newConfig.Level)
//       }
//   })
//
//   watcher.Start()
//   defer watcher.Stop()
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agilira/go-errors"
	"github.com/agilira/go-timecache"
)

// fileStat caches file metadata to minimize os.Stat() calls
// Using value types instead of pointers to avoid use-after-free in concurrent access

// Error codes for Argus operations
const (
	ErrCodeInvalidConfig          = "ARGUS_INVALID_CONFIG"
	ErrCodeFileNotFound           = "ARGUS_FILE_NOT_FOUND"
	ErrCodeWatcherStopped         = "ARGUS_WATCHER_STOPPED"
	ErrCodeWatcherBusy            = "ARGUS_WATCHER_BUSY"
	ErrCodeRemoteConfigError      = "ARGUS_REMOTE_CONFIG_ERROR"
	ErrCodeConfigNotFound         = "ARGUS_CONFIG_NOT_FOUND"
	ErrCodeInvalidPollInterval    = "ARGUS_INVALID_POLL_INTERVAL"
	ErrCodeInvalidCacheTTL        = "ARGUS_INVALID_CACHE_TTL"
	ErrCodeInvalidMaxWatchedFiles = "ARGUS_INVALID_MAX_WATCHED_FILES"
	ErrCodeInvalidOptimization    = "ARGUS_INVALID_OPTIMIZATION"
	ErrCodeInvalidAuditConfig     = "ARGUS_INVALID_AUDIT_CONFIG"
	ErrCodeInvalidBufferSize      = "ARGUS_INVALID_BUFFER_SIZE"
	ErrCodeInvalidFlushInterval   = "ARGUS_INVALID_FLUSH_INTERVAL"
	ErrCodeInvalidOutputFile      = "ARGUS_INVALID_OUTPUT_FILE"
	ErrCodeUnwritableOutputFile   = "ARGUS_UNWRITABLE_OUTPUT_FILE"
	ErrCodeCacheTTLTooLarge       = "ARGUS_CACHE_TTL_TOO_LARGE"
	ErrCodePollIntervalTooSmall   = "ARGUS_POLL_INTERVAL_TOO_SMALL"
	ErrCodeMaxFilesTooLarge       = "ARGUS_MAX_FILES_TOO_LARGE"
	ErrCodeBoreasCapacityInvalid  = "ARGUS_INVALID_BOREAS_CAPACITY"
	ErrCodeConfigWriterError      = "ARGUS_CONFIG_WRITER_ERROR"
	ErrCodeSerializationError     = "ARGUS_SERIALIZATION_ERROR"
	ErrCodeIOError                = "ARGUS_IO_ERROR"
)

// ChangeEvent represents a file change notification
type ChangeEvent struct {
	Path     string    // File path that changed
	ModTime  time.Time // New modification time
	Size     int64     // New file size
	IsCreate bool      // True if file was created
	IsDelete bool      // True if file was deleted
	IsModify bool      // True if file was modified
}

// UpdateCallback is called when a watched file changes
type UpdateCallback func(event ChangeEvent)

// ErrorHandler is called when errors occur during file watching or parsing
// It receives the error and the file path where the error occurred
type ErrorHandler func(err error, filepath string)

// OptimizationStrategy defines how BoreasLite should optimize performance
type OptimizationStrategy int

const (
	// OptimizationAuto automatically chooses the best strategy based on file count
	// - 1-3 files: SingleEvent strategy (ultra-low latency)
	// - 4-20 files: SmallBatch strategy (balanced)
	// - 21+ files: LargeBatch strategy (high throughput)
	OptimizationAuto OptimizationStrategy = iota

	// OptimizationSingleEvent optimizes for 1-2 files with ultra-low latency
	// - Fast path for single events (24ns)
	// - Minimal batching overhead
	// - Aggressive spinning for immediate processing
	OptimizationSingleEvent

	// OptimizationSmallBatch optimizes for 3-20 files with balanced performance
	// - Small batch sizes (2-8 events)
	// - Moderate spinning with short sleeps
	// - Good balance between latency and throughput
	OptimizationSmallBatch

	// OptimizationLargeBatch optimizes for 20+ files with high throughput
	// - Large batch sizes (16-64 events)
	// - Zephyros-style 4x unrolling
	// - Focus on maximum throughput over latency
	OptimizationLargeBatch
)

// Config configures the Argus watcher behavior
type Config struct {
	// PollInterval is how often to check for file changes
	// Default: 5 seconds (good balance of responsiveness vs overhead)
	PollInterval time.Duration

	// CacheTTL is how long to cache os.Stat() results
	// Should be <= PollInterval for effectiveness
	// Default: PollInterval / 2
	CacheTTL time.Duration

	// MaxWatchedFiles limits the number of files that can be watched
	// Default: 100 (generous for config files)
	MaxWatchedFiles int

	// Audit configuration for security and compliance
	// Default: Enabled with secure defaults
	Audit AuditConfig

	// ErrorHandler is called when errors occur during file watching/parsing
	// If nil, errors are logged to stderr (backward compatible)
	// Example: func(err error, path string) { metrics.Increment("config.errors") }
	ErrorHandler ErrorHandler

	// OptimizationStrategy determines how BoreasLite optimizes performance
	// - OptimizationAuto: Automatically choose based on file count (default)
	// - OptimizationSingleEvent: Ultra-low latency for 1-2 files
	// - OptimizationSmallBatch: Balanced for 3-20 files
	// - OptimizationLargeBatch: High throughput for 20+ files
	OptimizationStrategy OptimizationStrategy

	// BoreasLiteCapacity sets the ring buffer size (must be power of 2)
	// - Auto/SingleEvent: 64 (minimal memory)
	// - SmallBatch: 128 (balanced)
	// - LargeBatch: 256+ (high throughput)
	// Default: 0 (auto-calculated based on strategy)
	BoreasLiteCapacity int64

	// Remote configuration with automatic fallback capabilities
	// When enabled, provides distributed configuration management with local fallback
	// Default: Disabled for backward compatibility
	Remote RemoteConfig
}

// RemoteConfig defines distributed configuration management with automatic fallback.
// This struct enables enterprise-grade remote configuration loading with resilient
// fallback capabilities for production deployments where configuration comes from
// distributed systems (Consul, etcd, Redis) but local fallback is required.
//
// The RemoteConfig system implements the following fallback sequence:
// 1. Attempt to load from PrimaryURL (e.g., consul://prod-consul/myapp/config)
// 2. On failure, attempt FallbackURL if configured (e.g., consul://backup-consul/myapp/config)
// 3. On complete remote failure, load from FallbackPath (e.g., /etc/myapp/fallback-config.json)
// 4. Continue with SyncInterval for automatic recovery when remote systems recover
//
// Zero-allocation design: All URLs and paths are pre-parsed and cached during
// initialization to avoid allocations during runtime operations.
//
// Production deployment patterns:
//
//	// Consul with local fallback (recommended)
//	Remote: RemoteConfig{
//	    Enabled:      true,
//	    PrimaryURL:   "consul://prod-consul:8500/config/myapp",
//	    FallbackPath: "/etc/myapp/config.json",
//	    SyncInterval: 30 * time.Second,
//	    Timeout:      10 * time.Second,
//	}
//
//	// Multi-datacenter setup with remote fallback
//	Remote: RemoteConfig{
//	    Enabled:      true,
//	    PrimaryURL:   "consul://dc1-consul:8500/config/myapp",
//	    FallbackURL:  "consul://dc2-consul:8500/config/myapp",
//	    FallbackPath: "/etc/myapp/emergency-config.json",
//	    SyncInterval: 60 * time.Second,
//	}
//
//	// Redis with backup Redis
//	Remote: RemoteConfig{
//	    Enabled:     true,
//	    PrimaryURL:  "redis://prod-redis:6379/0/myapp:config",
//	    FallbackURL: "redis://backup-redis:6379/0/myapp:config",
//	    SyncInterval: 15 * time.Second,
//	}
//
// Thread safety: RemoteConfig operations are thread-safe and can be called
// concurrently from multiple goroutines without external synchronization.
//
// Error handling: Failed remote loads automatically trigger fallback sequence.
// Applications receive the most recent successful configuration and error notifications
// through the standard ErrorHandler mechanism.
//
// Monitoring integration: All remote configuration operations generate audit events
// for monitoring, alerting, and compliance tracking in production environments.
type RemoteConfig struct {
	// Enabled controls whether remote configuration loading is active
	// Default: false (for backward compatibility)
	// When false, all other RemoteConfig fields are ignored
	Enabled bool `json:"enabled" yaml:"enabled" toml:"enabled"`

	// PrimaryURL is the main remote configuration source
	// Supports all registered remote providers (consul://, redis://, etcd://, http://, https://)
	// Examples:
	//   - "consul://prod-consul:8500/config/myapp?datacenter=dc1"
	//   - "redis://prod-redis:6379/0/myapp:config"
	//   - "etcd://prod-etcd:2379/config/myapp"
	//   - "https://config-api.company.com/api/v1/config/myapp"
	// Required when Enabled=true
	PrimaryURL string `json:"primary_url" yaml:"primary_url" toml:"primary_url"`

	// FallbackURL is an optional secondary remote configuration source
	// Used when PrimaryURL fails but before falling back to local file
	// Should typically be a different instance/datacenter of the same system
	// Examples:
	//   - "consul://backup-consul:8500/config/myapp"
	//   - "redis://backup-redis:6379/0/myapp:config"
	// Optional: can be empty to skip remote fallback
	FallbackURL string `json:"fallback_url,omitempty" yaml:"fallback_url,omitempty" toml:"fallback_url,omitempty"`

	// FallbackPath is a local file path used when all remote sources fail
	// This provides the ultimate fallback for high-availability deployments
	// The file should contain a valid configuration in JSON, YAML, or TOML format
	// Examples:
	//   - "/etc/myapp/emergency-config.json"
	//   - "/opt/myapp/fallback-config.yaml"
	//   - "./config/local-fallback.toml"
	// Recommended: Always configure for production deployments
	FallbackPath string `json:"fallback_path,omitempty" yaml:"fallback_path,omitempty" toml:"fallback_path,omitempty"`

	// SyncInterval controls how often to check for remote configuration updates
	// This applies to all remote sources (primary and fallback)
	// Shorter intervals provide faster updates but increase system load
	// Default: 30 seconds (good balance for most applications)
	// Production considerations:
	//   - High-frequency apps: 10-15 seconds
	//   - Standard apps: 30-60 seconds
	//   - Batch jobs: 5+ minutes
	SyncInterval time.Duration `json:"sync_interval" yaml:"sync_interval" toml:"sync_interval"`

	// Timeout controls the maximum time to wait for each remote configuration request
	// Applied to both primary and fallback URL requests
	// Should be shorter than SyncInterval to allow for fallback attempts
	// Default: 10 seconds (allows for network latency and processing)
	// Production recommendations:
	//   - Local network: 5-10 seconds
	//   - Cross-datacenter: 10-20 seconds
	//   - Internet-based: 20-30 seconds
	Timeout time.Duration `json:"timeout" yaml:"timeout" toml:"timeout"`

	// MaxRetries controls retry attempts for failed remote requests
	// Applied per URL (primary/fallback) before moving to next fallback level
	// Default: 2 (total of 3 attempts: initial + 2 retries)
	// Higher values increase reliability but also increase latency during failures
	MaxRetries int `json:"max_retries" yaml:"max_retries" toml:"max_retries"`

	// RetryDelay is the base delay between retry attempts
	// Uses exponential backoff: attempt N waits RetryDelay * 2^N
	// Default: 1 second (results in 1s, 2s, 4s... delays)
	// Should be balanced with Timeout to ensure retries fit within timeout window
	RetryDelay time.Duration `json:"retry_delay" yaml:"retry_delay" toml:"retry_delay"`
}

// fileStat represents cached file statistics for efficient os.Stat() caching.
// Uses value types and timecache for zero-allocation performance optimization.
//
// ═══════════════════════════════════════════════════════════════════════════════
// ENGINEERING NOTE: Why timecache Instead of time.Now()?
// ═══════════════════════════════════════════════════════════════════════════════
// time.Now() is deceptively expensive:
// - Makes a system call (clock_gettime on Linux)
// - Allocates a time.Time struct (24 bytes)
// - ~50ns per call on modern hardware
//
// In hot paths that run millions of times per second, this adds up.
// go-timecache provides a cached timestamp updated every millisecond,
// returning the same int64 nanosecond value for ~1000 calls.
//
// For TTL checking, millisecond precision is MORE than sufficient.
// Config files don't change at microsecond granularity. This optimization
// reduces time-related overhead by 99% in the polling loop.
//
// The cachedAt field stores raw nanoseconds (int64) instead of time.Time
// to avoid allocation and enable direct integer comparison.
// ═══════════════════════════════════════════════════════════════════════════════
type fileStat struct {
	modTime  time.Time // Last modification time from os.Stat()
	size     int64     // File size in bytes
	exists   bool      // Whether the file exists
	cachedAt int64     // Use timecache nano timestamp for zero-allocation timing
}

// isExpired checks if the cached stat is expired using timecache for zero-allocation timing
func (fs *fileStat) isExpired(ttl time.Duration) bool {
	now := timecache.CachedTimeNano()
	return (now - fs.cachedAt) > int64(ttl)
}

// watchedFile represents a file under observation with its callback and cached state.
// Optimized for minimal memory footprint and fast access during polling.
type watchedFile struct {
	path     string         // Absolute file path being watched
	callback UpdateCallback // User-provided callback for file changes
	lastStat fileStat       // Cached file statistics for change detection
}

// Watcher monitors configuration files for changes
// ULTRA-OPTIMIZED: Uses BoreasLite MPSC ring buffer + lock-free cache for maximum performance
//
// ═══════════════════════════════════════════════════════════════════════════════
// ENGINEERING NOTE: Why Polling Instead of inotify/FSEvents/kqueue?
// ═══════════════════════════════════════════════════════════════════════════════
// This was a deliberate architectural decision, not a limitation:
//
//  1. CROSS-PLATFORM CONSISTENCY: inotify (Linux), FSEvents (macOS), and
//     ReadDirectoryChangesW (Windows) have subtle behavioral differences.
//     Network filesystems (NFS, CIFS, FUSE) often don't support them at all.
//     Polling works identically everywhere.
//
//  2. CONTAINER/K8S RELIABILITY: In Kubernetes, ConfigMaps are mounted via
//     symlink atomics. inotify often misses these updates because the inode
//     changes. Polling via os.Stat() catches 100% of changes.
//
//  3. PREDICTABLE RESOURCE USAGE: OS notification systems can exhaust file
//     descriptors under load. Polling uses constant resources regardless
//     of the number of watched files.
//
//  4. PERFORMANCE IS EXCELLENT: With timecache reducing os.Stat() calls by
//     ~90% and BoreasLite providing 40M+ ops/sec event processing, polling
//     overhead is negligible (<0.001% CPU for typical config watching).
//
// The lock-free cache (atomic.Pointer) ensures polling threads don't block
// on cache reads, achieving true zero-contention read access.
// ═══════════════════════════════════════════════════════════════════════════════
type Watcher struct {
	config  Config
	files   map[string]*watchedFile
	filesMu sync.RWMutex

	// LOCK-FREE CACHE: Uses atomic.Pointer for zero-contention reads
	// ───────────────────────────────────────────────────────────────────
	// This implements a Copy-on-Write (COW) pattern for the stat cache.
	// Readers load the pointer atomically (zero locks), while writers
	// create a new map and swap the pointer. This trades memory for speed
	// - perfect for read-heavy workloads like config watching.
	// ───────────────────────────────────────────────────────────────────
	statCache atomic.Pointer[map[string]fileStat]

	// ZERO-ALLOCATION POLLING: Reusable slice to avoid allocations in pollFiles
	filesBuffer []*watchedFile

	// BOREAS LITE: Ultra-fast MPSC ring buffer for file events (DEFAULT)
	eventRing *BoreasLite

	// AUDIT SYSTEM: Comprehensive security and compliance logging
	auditLogger *AuditLogger

	running   atomic.Bool
	stopped   atomic.Bool // Tracks if explicitly stopped vs just not started
	stopCh    chan struct{}
	stoppedCh chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
}

// New creates a new Argus file watcher with BoreasLite integration
func New(config Config) *Watcher {
	cfg := config.WithDefaults()
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize audit logger
	auditLogger, err := NewAuditLogger(cfg.Audit)
	if err != nil {
		// Fallback to disabled audit if setup fails
		auditLogger, _ = NewAuditLogger(AuditConfig{Enabled: false})
	}

	watcher := &Watcher{
		config:      *cfg,
		files:       make(map[string]*watchedFile),
		auditLogger: auditLogger,
		stopCh:      make(chan struct{}),
		stoppedCh:   make(chan struct{}),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Initialize lock-free cache
	initialCache := make(map[string]fileStat)
	watcher.statCache.Store(&initialCache)

	// Initialize BoreasLite MPSC ring buffer with configured strategy
	watcher.eventRing = NewBoreasLite(
		watcher.config.BoreasLiteCapacity,
		watcher.config.OptimizationStrategy,
		watcher.processFileEvent,
	)

	return watcher
}

// processFileEvent processes events from the BoreasLite ring buffer
// This method is called by BoreasLite for each file change event
func (w *Watcher) processFileEvent(fileEvent *FileChangeEvent) {
	// CRITICAL: Panic recovery to prevent callback panics from crashing the watcher
	defer func() {
		if r := recover(); r != nil {
			w.auditLogger.LogFileWatch("callback_panic", string(fileEvent.Path[:]))
		}
	}()

	// Convert BoreasLite event back to standard ChangeEvent
	event := ConvertFileEventToChangeEvent(*fileEvent)

	// Find the corresponding watched file and call its callback
	w.filesMu.RLock()
	if wf, exists := w.files[event.Path]; exists {
		// Call the user's callback function
		wf.callback(event)

		// Log basic file change to audit system
		w.auditLogger.LogFileWatch("file_changed", event.Path)
	}
	w.filesMu.RUnlock()
}

// Watch adds a file to the watch list
func (w *Watcher) Watch(path string, callback UpdateCallback) error {
	if callback == nil {
		return errors.New(ErrCodeInvalidConfig, "callback cannot be nil")
	}

	// Check if watcher was explicitly stopped (not just not started)
	if w.stopped.Load() {
		return errors.New(ErrCodeWatcherStopped, "cannot add watch to stopped watcher")
	}

	// Validate and secure the path
	absPath, err := w.validateAndSecurePath(path)
	if err != nil {
		return err
	}

	// AUDIT: Log file watch start
	w.auditLogger.LogFileWatch("watch_start", absPath)

	return w.addWatchedFile(absPath, callback)
}

// validateAndSecurePath validates path security and returns absolute path
func (w *Watcher) validateAndSecurePath(path string) (string, error) {
	// SECURITY FIX: Validate path before processing to prevent path traversal attacks
	if err := ValidateSecurePath(path); err != nil {
		// AUDIT: Log security event for path traversal attempt
		w.auditLogger.LogSecurityEvent("path_traversal_attempt", "Rejected malicious file path",
			map[string]interface{}{
				"rejected_path": path,
				"reason":        err.Error(),
			})
		return "", errors.Wrap(err, ErrCodeInvalidConfig, "invalid or unsafe file path").
			WithContext("path", path)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", errors.Wrap(err, ErrCodeInvalidConfig, "invalid file path").
			WithContext("path", path)
	}

	// SECURITY: Double-check absolute path after resolution
	if err := ValidateSecurePath(absPath); err != nil {
		w.auditLogger.LogSecurityEvent("path_traversal_attempt", "Rejected malicious absolute path",
			map[string]interface{}{
				"rejected_path": absPath,
				"original_path": path,
				"reason":        err.Error(),
			})
		return "", errors.Wrap(err, ErrCodeInvalidConfig, "resolved path is unsafe").
			WithContext("absolute_path", absPath).
			WithContext("original_path", path)
	}

	// SECURITY: Check for symlink traversal attacks
	// If the path is a symlink, verify that its target is also safe
	if info, err := os.Lstat(absPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			// If we can't resolve the symlink target, reject it for security
			w.auditLogger.LogSecurityEvent("symlink_traversal_attempt", "Symlink target resolution failed",
				map[string]interface{}{
					"symlink_path": absPath,
					"reason":       err.Error(),
				})
			return "", errors.Wrap(err, ErrCodeInvalidConfig, "cannot resolve symlink target").
				WithContext("symlink_path", absPath)
		}

		// Validate the symlink target
		if err := ValidateSecurePath(target); err != nil {
			w.auditLogger.LogSecurityEvent("symlink_traversal_attempt", "Symlink points to dangerous target",
				map[string]interface{}{
					"symlink_path": absPath,
					"target_path":  target,
					"reason":       err.Error(),
				})
			return "", errors.Wrap(err, ErrCodeInvalidConfig, "symlink target is unsafe").
				WithContext("symlink_path", absPath).
				WithContext("target_path", target)
		}

		// Update absPath to the resolved target for consistency
		absPath = target
	}

	// Validate symlinks
	if err := w.validateSymlinks(absPath, path); err != nil {
		return "", err
	}

	return absPath, nil
}

// validateSymlinks checks symlink security
func (w *Watcher) validateSymlinks(absPath, originalPath string) error {
	// SECURITY: Symlink resolution check
	// Resolve any symlinks and validate the final target path
	realPath, err := filepath.EvalSymlinks(absPath)
	if err == nil && realPath != absPath {
		// Path contains symlinks - validate the resolved target
		if err := ValidateSecurePath(realPath); err != nil {
			w.auditLogger.LogSecurityEvent("symlink_traversal_attempt", "Symlink points to unsafe location",
				map[string]interface{}{
					"symlink_path":  absPath,
					"resolved_path": realPath,
					"original_path": originalPath,
					"reason":        err.Error(),
				})
			return errors.Wrap(err, ErrCodeInvalidConfig, "symlink target is unsafe").
				WithContext("symlink_path", absPath).
				WithContext("resolved_path", realPath).
				WithContext("original_path", originalPath)
		}

		// Additional check: ensure symlink doesn't escape to system directories
		if w.isSystemDirectory(realPath) {
			w.auditLogger.LogSecurityEvent("symlink_system_access", "Symlink attempts to access system directory",
				map[string]interface{}{
					"symlink_path":  absPath,
					"resolved_path": realPath,
					"original_path": originalPath,
				})
			return errors.New(ErrCodeInvalidConfig, "symlink target accesses restricted system directory").
				WithContext("symlink_path", absPath).
				WithContext("resolved_path", realPath)
		}
	}
	return nil
}

// isSystemDirectory checks if path points to system directory
func (w *Watcher) isSystemDirectory(path string) bool {
	lowerPath := strings.ToLower(path)
	return strings.HasPrefix(path, "/etc/") ||
		strings.HasPrefix(path, "/proc/") ||
		strings.HasPrefix(path, "/sys/") ||
		strings.HasPrefix(path, "/dev/") ||
		strings.Contains(lowerPath, "windows\\system32") ||
		strings.Contains(lowerPath, "program files")
}

// addWatchedFile adds the file to watch list with proper locking
func (w *Watcher) addWatchedFile(absPath string, callback UpdateCallback) error {
	w.filesMu.Lock()
	defer w.filesMu.Unlock()

	if len(w.files) >= w.config.MaxWatchedFiles {
		// AUDIT: Log security event for limit exceeded
		w.auditLogger.LogSecurityEvent("watch_limit_exceeded", "Maximum watched files exceeded",
			map[string]interface{}{
				"path":          absPath,
				"max_files":     w.config.MaxWatchedFiles,
				"current_files": len(w.files),
			})
		return errors.New(ErrCodeInvalidConfig, "maximum watched files exceeded").
			WithContext("max_files", w.config.MaxWatchedFiles).
			WithContext("current_files", len(w.files))
	}

	// Get initial file stat
	initialStat, err := w.getStat(absPath)
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, ErrCodeFileNotFound, "failed to stat file").
			WithContext("path", absPath)
	}

	w.files[absPath] = &watchedFile{
		path:     absPath,
		callback: callback,
		lastStat: initialStat,
	}

	// Adapt BoreasLite strategy based on file count (if Auto mode)
	if w.eventRing != nil {
		w.eventRing.AdaptStrategy(len(w.files))
	}

	return nil
}

// Unwatch removes a file from the watch list
func (w *Watcher) Unwatch(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return errors.Wrap(err, ErrCodeInvalidConfig, "invalid file path").
			WithContext("path", path)
	}

	w.filesMu.Lock()
	defer w.filesMu.Unlock()

	delete(w.files, absPath)

	// Adapt BoreasLite strategy based on updated file count (if Auto mode)
	if w.eventRing != nil {
		w.eventRing.AdaptStrategy(len(w.files))
	}

	// Clean up cache entry atomically
	w.removeFromCache(absPath)

	return nil
}

// Start begins watching files for changes
func (w *Watcher) Start() error {
	if !w.running.CompareAndSwap(false, true) {
		return errors.New(ErrCodeWatcherBusy, "watcher is already running")
	}

	// Start BoreasLite event processor in background
	go w.eventRing.RunProcessor()

	// Start main polling loop
	go w.watchLoop()
	return nil
}

// Stop stops the watcher and waits for cleanup
func (w *Watcher) Stop() error {
	if !w.running.CompareAndSwap(true, false) {
		return errors.New(ErrCodeWatcherStopped, "watcher is not running")
	}

	w.stopped.Store(true) // Mark as explicitly stopped
	w.cancel()
	close(w.stopCh)
	<-w.stoppedCh

	// Stop BoreasLite event processor
	w.eventRing.Stop()

	// CRITICAL FIX: Close audit logger to prevent resource leaks
	if w.auditLogger != nil {
		_ = w.auditLogger.Close()
	}

	return nil
}

// IsRunning returns true if the watcher is currently running
func (w *Watcher) IsRunning() bool {
	return w.running.Load()
}

// Close is an alias for Stop() for better resource management patterns
// Implements the common Close() interface for easy integration with defer statements
func (w *Watcher) Close() error {
	return w.Stop()
}

// GracefulShutdown performs a graceful shutdown with timeout control.
// This method provides production-grade shutdown capabilities with deterministic timeout handling,
// ensuring all resources are properly cleaned up without hanging indefinitely.
//
// The method performs the following shutdown sequence:
// 1. Signals shutdown intent to all goroutines via context cancellation
// 2. Waits for all file polling operations to complete
// 3. Flushes all pending audit events to persistent storage
// 4. Closes BoreasLite ring buffer and releases memory
// 5. Cleans up file descriptors and other system resources
//
// Zero-allocation design: Uses pre-allocated channels and avoids heap allocations
// during the shutdown process to maintain performance characteristics even during termination.
//
// Example usage:
//
//	watcher := argus.New(config)
//	defer watcher.GracefulShutdown(30 * time.Second) // 30s timeout for Kubernetes
//
//	// Kubernetes deployment
//	watcher := argus.New(config)
//	defer watcher.GracefulShutdown(time.Duration(terminationGracePeriodSeconds) * time.Second)
//
//	// CI/CD pipelines
//	watcher := argus.New(config)
//	defer watcher.GracefulShutdown(10 * time.Second) // Fast shutdown for tests
//
// Parameters:
//   - timeout: Maximum time to wait for graceful shutdown. If exceeded, the method returns
//     an error but resources are still cleaned up in the background.
//
// Returns:
//   - nil if shutdown completed within timeout
//   - ErrCodeWatcherStopped if watcher was already stopped
//   - ErrCodeWatcherBusy if shutdown timeout was exceeded (resources still cleaned up)
//
// Thread-safety: Safe to call from multiple goroutines. First caller wins, subsequent
// calls return immediately with appropriate status.
//
// Production considerations:
//   - Kubernetes: Use terminationGracePeriodSeconds - 5s to allow for signal propagation
//   - Docker: Typically 10-30 seconds is sufficient
//   - CI/CD: Use shorter timeouts (5-10s) for faster test cycles
//   - Load balancers: Ensure timeout exceeds health check intervals
func (w *Watcher) GracefulShutdown(timeout time.Duration) error {
	// Fast path: Check if already stopped without allocations
	if !w.running.Load() {
		return errors.New(ErrCodeWatcherStopped, "watcher is not running")
	}

	// Pre-validate timeout to avoid work if invalid
	if timeout <= 0 {
		return errors.New(ErrCodeInvalidConfig, "graceful shutdown timeout must be positive")
	}

	// Create timeout context - this is the only allocation we make
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Channel for shutdown completion signaling (buffered to avoid blocking)
	// Pre-allocated with capacity 1 to prevent goroutine leaks
	done := make(chan error, 1)

	// Execute shutdown in separate goroutine to respect timeout
	go func() {
		// Use existing Stop() method which handles all cleanup logic
		// This avoids code duplication and maintains consistency
		err := w.Stop()
		select {
		case done <- err:
			// Successfully sent result
		default:
			// Channel full (timeout already occurred), ignore
			// The shutdown still completes in background for resource safety
		}
	}()

	// Wait for completion or timeout
	// Zero additional allocations in this critical path
	select {
	case err := <-done:
		// Shutdown completed within timeout
		if err != nil {
			// Wrap the error to provide context about graceful shutdown
			return errors.Wrap(err, ErrCodeWatcherStopped, "graceful shutdown encountered error")
		}
		return nil

	case <-ctx.Done():
		// Timeout exceeded - return error but allow background cleanup to continue
		// This ensures resources are eventually freed even if timeout is too short
		return errors.New(ErrCodeWatcherBusy,
			fmt.Sprintf("graceful shutdown timeout (%v) exceeded, cleanup continuing in background", timeout))
	}
}

// WatchedFiles returns the number of currently watched files
func (w *Watcher) WatchedFiles() int {
	w.filesMu.RLock()
	defer w.filesMu.RUnlock()
	return len(w.files)
}

// getStat returns cached file statistics or performs os.Stat if cache is expired
// LOCK-FREE: Uses atomic.Pointer for zero-contention cache access with value types
func (w *Watcher) getStat(path string) (fileStat, error) {
	// Fast path: atomic read of cache (ZERO locks!)
	cacheMap := *w.statCache.Load()
	if cached, exists := cacheMap[path]; exists {
		// Check expiration without any locks
		if !cached.isExpired(w.config.CacheTTL) {
			return cached, nil
		}
	}

	// Slow path: cache miss or expired - perform actual os.Stat()
	info, err := os.Stat(path)
	stat := fileStat{
		cachedAt: timecache.CachedTimeNano(), // Use timecache for zero-allocation timestamp
		exists:   err == nil,
	}

	if err == nil {
		stat.modTime = info.ModTime()
		stat.size = info.Size()
	}

	// Update cache atomically (copy-on-write)
	w.updateCache(path, stat)

	// Return by value (no pointer, no use-after-free risk)
	return stat, err
}

// updateCache atomically updates the cache using copy-on-write (no pool, value types)
func (w *Watcher) updateCache(path string, stat fileStat) {
	for {
		oldMapPtr := w.statCache.Load()
		oldMap := *oldMapPtr
		newMap := make(map[string]fileStat, len(oldMap)+1)

		// Copy existing entries
		for k, v := range oldMap {
			newMap[k] = v
		}

		// Add/update new entry
		newMap[path] = stat

		// Atomic compare-and-swap
		if w.statCache.CompareAndSwap(oldMapPtr, &newMap) {
			return // Success! No pool cleanup needed with value types
		}
		// Retry if another goroutine updated the cache concurrently
	}
}

// removeFromCache atomically removes an entry from the cache (no pool, value types)
func (w *Watcher) removeFromCache(path string) {
	for {
		oldMapPtr := w.statCache.Load()
		oldMap := *oldMapPtr
		if _, exists := oldMap[path]; !exists {
			return // Entry doesn't exist, nothing to do
		}

		newMap := make(map[string]fileStat, len(oldMap)-1)

		// Copy all entries except the one to remove
		for k, v := range oldMap {
			if k != path {
				newMap[k] = v
			}
		}

		// Atomic compare-and-swap
		if w.statCache.CompareAndSwap(oldMapPtr, &newMap) {
			return // Success! No pool cleanup needed with value types
		}
		// Retry if another goroutine updated the cache concurrently
	}
}

// checkFile compares current file stat with last known stat and sends events via BoreasLite
func (w *Watcher) checkFile(wf *watchedFile) {
	currentStat, err := w.getStat(wf.path)

	// Handle stat errors
	if err != nil {
		if os.IsNotExist(err) {
			// File was deleted
			if wf.lastStat.exists {
				// Send delete event via BoreasLite ring buffer
				w.eventRing.WriteFileChange(wf.path, time.Time{}, 0, false, true, false)
				wf.lastStat.exists = false
			}
		} else if w.config.ErrorHandler != nil {
			w.config.ErrorHandler(errors.Wrap(err, ErrCodeFileNotFound, "failed to stat file").
				WithContext("path", wf.path), wf.path)
		}
		return
	}

	// File exists now
	if !wf.lastStat.exists {
		// File was created - send via BoreasLite
		w.eventRing.WriteFileChange(wf.path, currentStat.modTime, currentStat.size, true, false, false)
	} else if currentStat.modTime != wf.lastStat.modTime || currentStat.size != wf.lastStat.size {
		// File was modified - send via BoreasLite
		w.eventRing.WriteFileChange(wf.path, currentStat.modTime, currentStat.size, false, false, true)
	}

	wf.lastStat = currentStat
}

// watchLoop is the main polling loop that checks all watched files
func (w *Watcher) watchLoop() {
	defer close(w.stoppedCh)

	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.pollFiles()
		}
	}
}

// pollFiles checks all watched files for changes
// ULTRA-OPTIMIZED: Zero-allocation version using reusable buffer
func (w *Watcher) pollFiles() {
	w.filesMu.RLock()
	// Reuse buffer to avoid allocations
	w.filesBuffer = w.filesBuffer[:0] // Reset slice but keep capacity
	for _, wf := range w.files {
		w.filesBuffer = append(w.filesBuffer, wf)
	}
	files := w.filesBuffer
	w.filesMu.RUnlock()

	// For single file, use direct checking to avoid goroutine overhead
	if len(files) == 1 {
		w.checkFile(files[0])
		return
	}

	// For multiple files, use parallel checking with limited concurrency
	const maxConcurrency = 8 // Prevent goroutine explosion
	if len(files) <= maxConcurrency {
		// Use goroutines for small number of files
		var wg sync.WaitGroup
		for _, wf := range files {
			wg.Add(1)
			go func(wf *watchedFile) {
				defer wg.Done()
				w.checkFile(wf)
			}(wf)
		}
		wg.Wait()
	} else {
		// Use worker pool for many files
		fileCh := make(chan *watchedFile, len(files))
		var wg sync.WaitGroup

		// Start workers
		for i := 0; i < maxConcurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for wf := range fileCh {
					w.checkFile(wf)
				}
			}()
		}

		// Send files to workers
		for _, wf := range files {
			fileCh <- wf
		}
		close(fileCh)
		wg.Wait()
	}
}

// ClearCache forces clearing of the stat cache (no pool cleanup needed)
// Useful for testing or when you want to force fresh stat calls
func (w *Watcher) ClearCache() {
	emptyCache := make(map[string]fileStat)
	w.statCache.Store(&emptyCache)
}

// CacheStats returns statistics about the internal cache for monitoring and debugging.
// Provides insights into cache efficiency and performance characteristics.
type CacheStats struct {
	Entries   int           // Number of cached entries
	OldestAge time.Duration // Age of oldest cache entry
	NewestAge time.Duration // Age of newest cache entry
}

// GetCacheStats returns current cache statistics using timecache for performance
func (w *Watcher) GetCacheStats() CacheStats {
	cacheMap := *w.statCache.Load()

	if len(cacheMap) == 0 {
		return CacheStats{}
	}

	now := timecache.CachedTimeNano()
	var oldest, newest int64
	first := true

	for _, stat := range cacheMap {
		if first {
			oldest = stat.cachedAt
			newest = stat.cachedAt
			first = false
		} else {
			if stat.cachedAt < oldest {
				oldest = stat.cachedAt
			}
			if stat.cachedAt > newest {
				newest = stat.cachedAt
			}
		}
	}

	return CacheStats{
		Entries:   len(cacheMap),
		OldestAge: time.Duration(now - oldest),
		NewestAge: time.Duration(now - newest),
	}
}

// =============================================================================
// SECURITY: PATH VALIDATION AND SANITIZATION FUNCTIONS
// =============================================================================

// ValidateSecurePath validates that a file path is safe from path traversal attacks.
//
// SECURITY PURPOSE: Prevents directory traversal attacks (CWE-22) by rejecting
// paths that contain dangerous patterns or attempt to escape the intended directory.
//
// This function implements multiple layers of protection:
// 1. Pattern-based detection of traversal sequences (case-insensitive)
// 2. URL decoding to catch encoded attacks
// 3. Normalization attacks prevention
// 4. System file protection
// 5. Device name filtering (Windows)
//
// SECURITY NOTICE: All validation is performed case-insensitively to ensure
// consistent protection across different file systems and OS configurations.
//
// CRITICAL: This function must be called on ALL user-provided paths before
// any file operations to prevent security vulnerabilities.
//
// This function is exported to allow external packages to use the same
// security validation logic as the core Argus library.
func ValidateSecurePath(path string) error {
	if path == "" {
		return errors.New(ErrCodeInvalidConfig, "empty path not allowed")
	}

	// Normalize path to lowercase for consistent security validation
	// This prevents case-based bypass attempts on case-insensitive file systems
	lowerPath := strings.ToLower(path)

	// SECURITY CHECK 1: Detect common path traversal patterns (case-insensitive)
	// These patterns are dangerous regardless of OS
	dangerousPatterns := []string{
		"..",   // Parent directory reference
		"../",  // Unix path traversal
		"..\\", // Windows path traversal
		"/..",  // Unix parent dir
		"\\..", // Windows parent dir
		// Note: "./" removed as it can be legitimate in temp paths
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerPath, pattern) {
			return errors.New(ErrCodeInvalidConfig, "path contains dangerous traversal pattern: "+pattern)
		}
	}

	// SECURITY CHECK 2: URL decoding to catch encoded attacks
	// Attackers often URL-encode traversal sequences to bypass filters

	// Check for URL-encoded dangerous patterns using normalized path
	urlPatterns := []string{
		"%2e%2e",      // ".." encoded
		"%252e%252e",  // ".." double encoded
		"%2f",         // "/" encoded
		"%252f",       // "/" double encoded
		"%5c",         // "\" encoded
		"%255c",       // "\" double encoded
		"%00",         // null byte
		"%2500",       // null byte double encoded
		"..%2f",       // Mixed encoding patterns
		"..%252f",     // Mixed double encoding
		"%2e%2e/",     // Mixed patterns
		"%252e%252e/", // Mixed double encoding
	}

	for _, pattern := range urlPatterns {
		if strings.Contains(lowerPath, pattern) {
			return errors.New(ErrCodeInvalidConfig, "path contains URL-encoded traversal pattern: "+pattern)
		}
	}

	// Additional check for any percent-encoded sequences that decode to dangerous patterns
	// This catches creative encoding attempts
	for i := 0; i < len(path)-2; i++ {
		if path[i] == '%' {
			// Look for sequences like %XX that might decode to dangerous characters
			if i+5 < len(path) {
				sixChar := strings.ToLower(path[i : i+6])
				// Check for double-encoded dots and slashes
				if strings.HasPrefix(sixChar, "%252e") || strings.HasPrefix(sixChar, "%252f") || strings.HasPrefix(sixChar, "%255c") {
					return errors.New(ErrCodeInvalidConfig, "path contains double-encoded traversal sequence: "+sixChar)
				}
			}
		}
	}

	// SECURITY CHECK 3: System file protection
	// Prevent access to known sensitive system files and directories
	// Using already normalized lowerPath for consistency
	sensitiveFiles := []string{
		"/etc/passwd",
		"/etc/shadow",
		"/etc/hosts",
		"/proc/",
		"/sys/",
		"/dev/",
		"windows/system32",
		"windows\\system32",   // Windows backslash variant
		"\\windows\\system32", // Absolute Windows path
		"program files",
		"system volume information",
		".ssh/",
		".aws/",
		".docker/",
	}

	for _, sensitive := range sensitiveFiles {
		if strings.Contains(lowerPath, strings.ToLower(sensitive)) {
			return errors.New(ErrCodeInvalidConfig, "access to system file/directory not allowed: "+sensitive)
		}
	}

	// SECURITY CHECK 4: Windows-specific security threats
	// Multiple Windows-specific attack vectors need protection

	// 4A: Windows device name protection
	windowsDevices := []string{
		"CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
	}

	// SECURITY FIX: Check for UNC paths that access Windows devices
	// UNC paths like //Con, ///Con, \\Con, /\Con are equivalent to device access and must be blocked
	if (strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\")) && len(path) > 1 {
		// Normalize the path: remove all leading slashes and backslashes
		normalizedPath := path
		for len(normalizedPath) > 0 && (normalizedPath[0] == '/' || normalizedPath[0] == '\\') {
			normalizedPath = normalizedPath[1:]
		}

		if len(normalizedPath) > 0 {
			// Split by both types of separators to get path components
			// Replace backslashes with forward slashes for consistent splitting
			normalizedForSplit := strings.ReplaceAll(normalizedPath, "\\", "/")
			components := strings.Split(normalizedForSplit, "/")

			if len(components) > 0 && components[0] != "" {
				// Check if the first component is a device name (after normalizing case)
				firstComponent := strings.ToUpper(components[0])

				// Remove ALL extensions if present (handle multiple extensions)
				for {
					if dotIndex := strings.Index(firstComponent, "."); dotIndex != -1 {
						firstComponent = firstComponent[:dotIndex]
					} else {
						break
					}
				}

				// Special case: If we have exactly 2 components and the first is short (likely server),
				// and second is a device name, this might be legitimate UNC (//server/device)
				// But if first component is also a device name (//Con/anything), block it
				isLikelyDevice := false
				for _, device := range windowsDevices {
					if firstComponent == device {
						isLikelyDevice = true
						break
					}
				}

				if isLikelyDevice {
					// Always block if first component is a device name
					return errors.New(ErrCodeInvalidConfig, "windows device name not allowed via UNC path: "+firstComponent)
				}

				// Also check if this could be a mixed separator attack trying to access device
				// in second position (like /\server\Con)
				if len(components) >= 2 {
					secondComponent := strings.ToUpper(components[1])
					// Remove ALL extensions if present (handle multiple extensions)
					for {
						if dotIndex := strings.Index(secondComponent, "."); dotIndex != -1 {
							secondComponent = secondComponent[:dotIndex]
						} else {
							break
						}
					}

					// If second component is device AND first component looks suspicious
					// (single char, digit, etc.), block it
					for _, device := range windowsDevices {
						if secondComponent == device && len(components[0]) <= 2 {
							return errors.New(ErrCodeInvalidConfig, "windows device name not allowed via UNC path: "+secondComponent)
						}
					}
				}
			}
		}
	}

	baseName := strings.ToUpper(filepath.Base(path))
	// Remove ALL extensions for device name check (handle multiple extensions like PRN.0., COM1.txt.bak)
	// Keep removing extensions until no more dots are found
	for {
		if dotIndex := strings.LastIndex(baseName, "."); dotIndex != -1 {
			baseName = baseName[:dotIndex]
		} else {
			break
		}
	}

	for _, device := range windowsDevices {
		if baseName == device {
			return errors.New(ErrCodeInvalidConfig, "windows device name not allowed: "+device)
		}
	}

	// 4B: Windows Alternate Data Streams (ADS) protection
	// ADS can hide malicious content: filename.txt:hidden_stream
	if strings.Contains(path, ":") {
		// Check if this is a Windows ADS (not a URL scheme or Windows drive letter)
		colonIndex := strings.Index(path, ":")
		if colonIndex > 1 && colonIndex < len(path)-1 {
			// Check if it looks like ADS (no // after colon like in URLs)
			afterColon := path[colonIndex+1:]
			// Allow URLs (://) and network paths (:\\)
			if !strings.HasPrefix(afterColon, "//") && !strings.HasPrefix(afterColon, "\\\\") {
				// Allow drive letters (C:)
				if colonIndex == 1 {
					// This is likely a drive letter, allow it
				} else {
					// Check if this looks like a real ADS attack
					// Real ADS: filename.ext:streamname (streamname typically doesn't start with .)
					// But "test:.json" has colon followed by .json which is not typical ADS
					if !strings.HasPrefix(afterColon, ".") {
						return errors.New(ErrCodeInvalidConfig, "windows alternate data streams not allowed")
					}
				}
			}
		}
	}

	// SECURITY CHECK 5: Path length and complexity limits
	// Prevent extremely long paths that could cause buffer overflows or DoS
	if len(path) > 4096 {
		return errors.New(ErrCodeInvalidConfig, fmt.Sprintf("path too long (max 4096 characters): %d", len(path)))
	}

	// Count directory levels to prevent deeply nested traversal attempts
	separatorCount := strings.Count(path, "/") + strings.Count(path, "\\")
	if separatorCount > 50 {
		return errors.New(ErrCodeInvalidConfig, fmt.Sprintf("path too complex (max 50 directory levels): %d", separatorCount))
	}

	// SECURITY CHECK 6: Null byte injection prevention
	// Null bytes can truncate strings in some languages/systems
	if strings.Contains(path, "\x00") {
		return errors.New(ErrCodeInvalidConfig, "null byte in path not allowed")
	}

	// SECURITY CHECK 7: Control character prevention
	// Control characters can cause unexpected behavior
	for _, char := range path {
		if char < 32 && char != 9 && char != 10 && char != 13 { // Allow tab, LF, CR
			return errors.New(ErrCodeInvalidConfig, fmt.Sprintf("control character in path not allowed: %d", char))
		}
	}

	return nil
}

// GetWriter creates a ConfigWriter for the specified file.
// The writer enables programmatic configuration modifications with atomic operations.
//
// Performance: ~500 ns/op, zero allocations for writer creation
func (w *Watcher) GetWriter(filePath string, format ConfigFormat, initialConfig map[string]interface{}) (*ConfigWriter, error) {
	return NewConfigWriter(filePath, format, initialConfig)
}
