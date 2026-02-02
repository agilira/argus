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
	watchers       map[string]*Watcher
	ctx            context.Context
	cancel         context.CancelFunc
	scanTicker     *time.Ticker
	closed         bool
	closedCh       chan struct{}
	individualMode bool
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

	// Close all individual file watchers
	for path, w := range dw.watchers {
		_ = w.Close()
		delete(dw.watchers, path)
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

	// Verify directory exists and is a directory
	info, err := os.Stat(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("argus: directory not accessible: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("argus: path is not a directory")
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
		watchers:       make(map[string]*Watcher),
		ctx:            ctx,
		cancel:         cancel,
		closedCh:       make(chan struct{}),
		individualMode: individualMode,
	}

	// Initial scan
	if err := dw.scan(); err != nil {
		cancel()
		return nil, fmt.Errorf("argus: initial directory scan failed: %w", err)
	}

	// Start directory polling for new/deleted files
	dw.scanTicker = time.NewTicker(options.PollInterval)
	go dw.pollLoop()

	return dw, nil
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
		if w, ok := dw.watchers[path]; ok {
			_ = w.Close()
			delete(dw.watchers, path)
		}

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

		foundFiles[path] = true

		dw.mu.RLock()
		existing, exists := dw.files[path]
		dw.mu.RUnlock()

		if !exists || !info.ModTime().Equal(existing.modTime) {
			_ = dw.loadAndNotify(path, info)
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
		return err
	}

	format := DetectFormat(path)
	config, err := ParseConfig(data, format)
	if err != nil {
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

	ticker := time.NewTicker(dw.options.PollInterval)
	defer ticker.Stop()

	lastHash := dw.computeHash()

	for {
		select {
		case <-dw.ctx.Done():
			return
		case <-dw.closedCh:
			return
		case <-ticker.C:
			_ = dw.scan()
			newHash := dw.computeHash()
			if newHash != lastHash {
				lastHash = newHash
				dw.notifyMerged(callback)
			}
		}
	}
}

// notifyMerged sends merged config to callback
func (dw *DirectoryWatcher) notifyMerged(callback func(map[string]interface{}, []string)) {
	dw.mu.RLock()
	defer dw.mu.RUnlock()

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
