// directory_watcher_silent_failure_test.go: failures the watcher used to swallow
//
// Two discards of a returned value hid real conditions from the caller: a
// malformed glob pattern matched nothing instead of being rejected, and a
// watched directory that became unreadable was skipped on every poll without
// reaching the configured ErrorHandler.
//
// Copyright (c) 2025 AGILira
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestWatchDirectory_RejectsMalformedPattern pins that a glob the standard
// library cannot parse is refused when the watcher is built, not silently
// treated as "matches nothing".
//
// filepath.Match("[a-", name) returns (false, ErrBadPattern). matchesPatterns
// discarded that error, so a typo in Patterns produced a watcher that ran
// forever and reported no file — indistinguishable from an empty directory.
func TestWatchDirectory_RejectsMalformedPattern(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.json"), []byte(`{"a":1}`), 0600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	dw, err := WatchDirectory(dir,
		DirectoryWatchOptions{Patterns: []string{"*.json", "[a-"}},
		func(DirectoryConfigUpdate) {})
	if err == nil {
		_ = dw.Close()
		t.Fatal("a malformed pattern must be rejected, not silently matched against")
	}
}

// TestWatchDirectory_AcceptsValidPatterns is the companion guard: the
// validation must not reject the patterns people actually write.
func TestWatchDirectory_AcceptsValidPatterns(t *testing.T) {
	dir := t.TempDir()
	dw, err := WatchDirectory(dir,
		DirectoryWatchOptions{Patterns: []string{"*.yaml", "*.yml", "app-?.json", "[0-9]*.toml"}},
		func(DirectoryConfigUpdate) {})
	if err != nil {
		t.Fatalf("valid patterns must be accepted, got %v", err)
	}
	t.Cleanup(func() { _ = dw.Close() })
}

// TestDirectoryWatcher_UnreadableDirectoryReachesErrorHandler pins that losing
// the watched directory is reported.
//
// scan()'s walk function returned nil for every error, including the one for
// the root itself, so scan() always returned nil and the poll loop's
// `_ = dw.scan()` had nothing to drop. A directory deleted under a running
// watcher left it polling in silence.
func TestDirectoryWatcher_UnreadableDirectoryReachesErrorHandler(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "configs")
	if err := os.Mkdir(dir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.json"), []byte(`{"a":1}`), 0600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	var mu sync.Mutex
	var errs []error

	dw, err := WatchDirectory(dir, DirectoryWatchOptions{
		PollInterval: 20 * time.Millisecond,
		ErrorHandler: func(err error, _ string) {
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
		},
	}, func(DirectoryConfigUpdate) {})
	if err != nil {
		t.Fatalf("WatchDirectory: %v", err)
	}
	t.Cleanup(func() { _ = dw.Close() })

	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove watched directory: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(errs)
		mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the watched directory disappeared and the ErrorHandler was never called")
}
