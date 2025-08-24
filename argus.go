// argus: Ultra-lightweight configuration
//
// Core Philosophy:
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
// Copyright (c) 2025 AGILira
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"context"
	"os"
	"path/filepath"
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
	ErrCodeInvalidConfig  = "ARGUS_INVALID_CONFIG"
	ErrCodeFileNotFound   = "ARGUS_FILE_NOT_FOUND"
	ErrCodeWatcherStopped = "ARGUS_WATCHER_STOPPED"
	ErrCodeWatcherBusy    = "ARGUS_WATCHER_BUSY"
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
}

// fileStat represents cached file statistics
type fileStat struct {
	modTime  time.Time
	size     int64
	exists   bool
	cachedAt int64 // Use timecache nano timestamp for zero-allocation timing
}

// isExpired checks if the cached stat is expired using timecache for zero-allocation timing
func (fs *fileStat) isExpired(ttl time.Duration) bool {
	now := timecache.CachedTimeNano()
	return (now - fs.cachedAt) > int64(ttl)
}

// watchedFile represents a file under observation
type watchedFile struct {
	path     string
	callback UpdateCallback
	lastStat fileStat
}

// Watcher monitors configuration files for changes
// ULTRA-OPTIMIZED: Uses BoreasLite MPSC ring buffer + lock-free cache for maximum performance
type Watcher struct {
	config  Config
	files   map[string]*watchedFile
	filesMu sync.RWMutex

	// LOCK-FREE CACHE: Uses atomic.Pointer for zero-contention reads
	statCache atomic.Pointer[map[string]fileStat]

	// ZERO-ALLOCATION POLLING: Reusable slice to avoid allocations in pollFiles
	filesBuffer []*watchedFile

	// BOREAS LITE: Ultra-fast MPSC ring buffer for file events (DEFAULT)
	eventRing *BoreasLite

	// AUDIT SYSTEM: Comprehensive security and compliance logging
	auditLogger *AuditLogger

	running   atomic.Bool
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

	absPath, err := filepath.Abs(path)
	if err != nil {
		return errors.Wrap(err, ErrCodeInvalidConfig, "invalid file path").
			WithContext("path", path)
	}

	// AUDIT: Log file watch start
	w.auditLogger.LogFileWatch("watch_start", absPath)

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

// CacheStats returns statistics about the internal cache
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
