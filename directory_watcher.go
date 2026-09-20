// directory_watcher.go: Directory watching for configuration files
//
// Provides functions to scan directories for configuration files and
// watch all matching files for changes, with support for recursive
// subdirectory watching and pattern-based filtering.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// =============================================================================
// TYPES
// =============================================================================

// DirectoryConfigUpdate represents a configuration file update event
// from directory watching. Contains parsed content and metadata.
type DirectoryConfigUpdate struct {
	// FilePath is the absolute path to the configuration file
	FilePath string

	// RelativePath is the path relative to the watched directory
	RelativePath string

	// Config is the parsed configuration content
	Config map[string]interface{}

	// Format is the detected format (yaml, json, toml, ini, etc.)
	Format string

	// IsDelete indicates this is a deletion event (file was removed)
	IsDelete bool

	// ModTime is the modification time of the file (zero for deletes)
	ModTime time.Time
}

// DirectoryWatchOptions configures directory watching behavior
type DirectoryWatchOptions struct {
	// Patterns are glob patterns to match (e.g., "*.yaml", "*.json")
	// If empty, defaults to all supported config formats
	Patterns []string

	// Recursive enables watching subdirectories
	Recursive bool

	// PollInterval for directory scanning (default: 1 second)
	PollInterval time.Duration

	// ErrorHandler is called when a matching file cannot be read or parsed.
	// If nil, such errors are silent — which is what made a single malformed
	// file invisible while being re-read on every scan.
	ErrorHandler func(err error, path string)

	// Context for cancellation (optional)
	Context context.Context
}

// DirectoryWatcher watches a directory for configuration file changes
type DirectoryWatcher struct {
	dirPath  string
	options  DirectoryWatchOptions
	callback func(DirectoryConfigUpdate)

	mu             sync.RWMutex
	files          map[string]fileState
	ctx            context.Context
	cancel         context.CancelFunc
	scanTicker     *time.Ticker
	closed         bool
	closedCh       chan struct{}
	individualMode bool

	// resolvedDir is dirPath with symlinks resolved, computed once. Entries
	// found during a scan are required to resolve inside it; see
	// isWithinWatchedTree.
	resolvedDir string
}

// fileState tracks known files and their modification times
type fileState struct {
	modTime time.Time
	config  map[string]interface{}
}

// =============================================================================
// PUBLIC API
// =============================================================================

// WatchDirectory starts watching a directory for configuration files.
// Calls callback for each file on initial scan and on any change.
//
// Security: Validates directory path and rejects path traversal attacks.
// The directory must exist and be readable.
//
// Example:
//
//	watcher, err := argus.WatchDirectory("/etc/myapp/config.d", argus.DirectoryWatchOptions{
//	    Patterns:  []string{"*.yaml", "*.yml"},
//	    Recursive: true,
//	}, func(update argus.DirectoryConfigUpdate) {
//	    fmt.Printf("Config changed: %s\n", update.FilePath)
//	})
func WatchDirectory(
	dirPath string,
	options DirectoryWatchOptions,
	callback func(DirectoryConfigUpdate),
) (*DirectoryWatcher, error) {
	return watchDirectoryInternal(dirPath, options, callback, true)
}

// WatchDirectoryMerged watches a directory and provides merged configuration
// from all matching files. Files are merged in alphabetical order, with
// later files overriding earlier ones (useful for 00-base.yaml, 10-override.yaml pattern).
//
// The callback receives the merged configuration and list of source files.
func WatchDirectoryMerged(
	dirPath string,
	options DirectoryWatchOptions,
	callback func(merged map[string]interface{}, files []string),
) (*DirectoryWatcher, error) {
	// Wrap callback to collect and merge
	dw, err := watchDirectoryInternal(dirPath, options, nil, false)
	if err != nil {
		return nil, err
	}

	// Override the internal behavior to merge configs
	go dw.mergeLoop(callback)

	return dw, nil
}

// Close stops watching and releases resources
func (dw *DirectoryWatcher) Close() error {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	if dw.closed {
		return nil
	}
	dw.closed = true

	// Cancel context to stop goroutines
	dw.cancel()

	// Stop ticker
	if dw.scanTicker != nil {
		dw.scanTicker.Stop()
	}

	close(dw.closedCh)
	return nil
}

// Files returns the list of currently watched files
func (dw *DirectoryWatcher) Files() []string {
	dw.mu.RLock()
	defer dw.mu.RUnlock()

	files := make([]string, 0, len(dw.files))
	for path := range dw.files {
		files = append(files, path)
	}
	sort.Strings(files)
	return files
}

// =============================================================================
// INTERNAL
// =============================================================================

func watchDirectoryInternal(
	dirPath string,
	options DirectoryWatchOptions,
	callback func(DirectoryConfigUpdate),
	individualMode bool,
) (*DirectoryWatcher, error) {
	// Validate and normalize path
	cleanPath := filepath.Clean(dirPath)

	// Security: reject path traversal
	if strings.Contains(cleanPath, "..") {
		return nil, errors.New("argus: path traversal not allowed")
	}

	// Security: the directory argument goes through the same validation as a
	// watched file. It is deliberately NOT applied to the entries found inside:
	// a Kubernetes ConfigMap mount points each key at ..data/<key>, and
	// ValidateSecurePath rejects any path containing "..". Entries are checked
	// for containment instead (isWithinWatchedTree).
	if err := ValidateSecurePath(cleanPath); err != nil {
		return nil, fmt.Errorf("argus: unsafe directory path: %w", err)
	}

	// Verify directory exists and is a directory
	info, err := os.Stat(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("argus: directory not accessible: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("argus: path is not a directory")
	}

	// Resolve the base once. The watched directory may legitimately be reached
	// through a symlink (again, ConfigMap mounts); what matters is that the
	// files inside stay under the same resolved root.
	resolvedDir, err := filepath.EvalSymlinks(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("argus: cannot resolve directory: %w", err)
	}

	// Default patterns if none specified
	if len(options.Patterns) == 0 {
		options.Patterns = []string{"*.yaml", "*.yml", "*.json", "*.toml", "*.ini"}
	}

	// Default poll interval
	if options.PollInterval == 0 {
		options.PollInterval = 1 * time.Second
	}

	// Setup context
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)

	dw := &DirectoryWatcher{
		dirPath:        cleanPath,
		options:        options,
		callback:       callback,
		files:          make(map[string]fileState),
		ctx:            ctx,
		cancel:         cancel,
		closedCh:       make(chan struct{}),
		individualMode: individualMode,
		resolvedDir:    resolvedDir,
	}

	// Initial scan
	if err := dw.scan(); err != nil {
		cancel()
		return nil, fmt.Errorf("argus: initial directory scan failed: %w", err)
	}

	// Start the single scan loop for this mode.
	//
	// Merged mode runs mergeLoop instead, which performs its own scan on the
	// same ticker. Starting pollLoop as well put two goroutines through
	// filepath.Walk and through dw.files every interval, doubling the I/O and
	// racing one scan's processDeletedFiles against the other's loadAndNotify.
	dw.scanTicker = time.NewTicker(options.PollInterval)
	if individualMode {
		go dw.pollLoop()
	}

	return dw, nil
}

// isWithinWatchedTree reports whether path, once symlinks are resolved, is
// still inside the directory being watched.
//
// A path that cannot be resolved (a broken symlink, or a file deleted between
// the walk and this check) is treated as outside: there is nothing safe to
// read there.
func (dw *DirectoryWatcher) isWithinWatchedTree(path string) bool {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(dw.resolvedDir, resolved)
	if err != nil {
		return false
	}

	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// matchesPatterns checks if filename matches any of the configured patterns
func (dw *DirectoryWatcher) matchesPatterns(filename string) bool {
	for _, pattern := range dw.options.Patterns {
		if m, _ := filepath.Match(pattern, filename); m {
			return true
		}
	}
	return false
}

// shouldSkipDirectory returns true if directory should be skipped during walk
func (dw *DirectoryWatcher) shouldSkipDirectory(path string) bool {
	return path != dw.dirPath && !dw.options.Recursive
}

// processDeletedFiles handles files that no longer exist in the directory
func (dw *DirectoryWatcher) processDeletedFiles(foundFiles map[string]bool) {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	for path := range dw.files {
		if foundFiles[path] {
			continue
		}

		delete(dw.files, path)

		if dw.individualMode && dw.callback != nil {
			relPath, _ := filepath.Rel(dw.dirPath, path)
			dw.callback(DirectoryConfigUpdate{
				FilePath:     path,
				RelativePath: relPath,
				IsDelete:     true,
			})
		}
	}
}

// scan performs a full directory scan for matching files
func (dw *DirectoryWatcher) scan() error {
	foundFiles := make(map[string]bool)

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible files
		}

		if info.IsDir() {
			if dw.shouldSkipDirectory(path) {
				return filepath.SkipDir
			}
			return nil
		}

		if !dw.matchesPatterns(info.Name()) {
			return nil
		}

		// Security: a matching name may be a symlink pointing anywhere.
		// loadAndNotify reads it with os.ReadFile, which follows the link, so
		// an attacker who can drop "innocent.json -> ~/.ssh/id_rsa" into the
		// watched directory would have its contents parsed and handed to the
		// application's callback. Single-file watching has always resolved and
		// validated symlinks; directory watching did not.
		if !dw.isWithinWatchedTree(path) {
			return nil
		}

		foundFiles[path] = true

		dw.mu.RLock()
		existing, exists := dw.files[path]
		dw.mu.RUnlock()

		if !exists || !info.ModTime().Equal(existing.modTime) {
			if err := dw.loadAndNotify(path, info); err != nil {
				dw.reportError(err, path)
			}
		}

		return nil
	}

	if err := filepath.Walk(dw.dirPath, walkFn); err != nil {
		return err
	}

	dw.processDeletedFiles(foundFiles)
	return nil
}

// loadAndNotify loads a file and notifies callback
func (dw *DirectoryWatcher) loadAndNotify(path string, info os.FileInfo) error {
	// Security: path comes from filepath.Walk which only traverses within the
	// validated base directory (dw.dirPath). Path traversal via ".." is blocked
	// in watchDirectoryInternal before starting the watcher.
	// #nosec G304 -- Path is constrained to validated directory via filepath.Walk
	data, err := os.ReadFile(path)
	if err != nil {
		dw.markSeen(path, info.ModTime())
		return err
	}

	format := DetectFormat(path)
	config, err := ParseConfig(data, format)
	if err != nil {
		dw.markSeen(path, info.ModTime())
		return err
	}

	relPath, _ := filepath.Rel(dw.dirPath, path)

	dw.mu.Lock()
	dw.files[path] = fileState{
		modTime: info.ModTime(),
		config:  config,
	}
	dw.mu.Unlock()

	if dw.individualMode && dw.callback != nil {
		dw.callback(DirectoryConfigUpdate{
			FilePath:     path,
			RelativePath: relPath,
			Config:       config,
			Format:       format.String(),
			IsDelete:     false,
			ModTime:      info.ModTime(),
		})
	}

	return nil
}

// markSeen records a file we could not load at its current modification time.
//
// Without it a malformed file is re-read and re-parsed on every single scan,
// forever, and the error is reported again each time. Recording the modtime
// with no configuration means the file contributes nothing to the merged view
// and is retried only once it changes.
func (dw *DirectoryWatcher) markSeen(path string, modTime time.Time) {
	dw.mu.Lock()
	dw.files[path] = fileState{modTime: modTime}
	dw.mu.Unlock()
}

// reportError hands an error to the caller's handler, if one was configured.
func (dw *DirectoryWatcher) reportError(err error, path string) {
	if dw.options.ErrorHandler != nil {
		dw.options.ErrorHandler(err, path)
	}
}

// pollLoop periodically scans for new/deleted files
func (dw *DirectoryWatcher) pollLoop() {
	for {
		select {
		case <-dw.ctx.Done():
			return
		case <-dw.closedCh:
			return
		case <-dw.scanTicker.C:
			_ = dw.scan()
		}
	}
}

// mergeLoop runs for merged mode, combining configs
func (dw *DirectoryWatcher) mergeLoop(callback func(map[string]interface{}, []string)) {
	// Initial merge
	dw.notifyMerged(callback)

	lastHash := dw.computeHash()

	for {
		select {
		case <-dw.ctx.Done():
			return
		case <-dw.closedCh:
			return
		case <-dw.scanTicker.C:
			_ = dw.scan()
			newHash := dw.computeHash()
			if newHash != lastHash {
				lastHash = newHash
				dw.notifyMerged(callback)
			}
		}
	}
}

// notifyMerged sends merged config to callback.
//
// The merge happens under the read lock; the callback runs after it is
// released. Holding dw.mu across user code deadlocks the watcher as soon as
// that callback calls Close — the natural "stop once I have what I need"
// shape — because Close takes the write lock.
func (dw *DirectoryWatcher) notifyMerged(callback func(map[string]interface{}, []string)) {
	dw.mu.RLock()

	// Get sorted file list
	files := make([]string, 0, len(dw.files))
	for path := range dw.files {
		files = append(files, path)
	}
	sort.Strings(files)

	// Merge in order
	merged := make(map[string]interface{})
	for _, path := range files {
		state := dw.files[path]
		for k, v := range state.config {
			merged[k] = v
		}
	}
	dw.mu.RUnlock()

	callback(merged, files)
}

// computeHash creates a simple hash of current state for change detection
func (dw *DirectoryWatcher) computeHash() string {
	dw.mu.RLock()
	defer dw.mu.RUnlock()

	var sb strings.Builder
	files := make([]string, 0, len(dw.files))
	for path := range dw.files {
		files = append(files, path)
	}
	sort.Strings(files)

	for _, path := range files {
		state := dw.files[path]
		fmt.Fprintf(&sb, "%s:%d;", path, state.modTime.UnixNano())
	}
	return sb.String()
}
