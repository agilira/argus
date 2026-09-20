// argus_audit_test.go: regression tests for Watcher defects found in audit
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestWatcher_LongPathStillDeliversEvents: BoreasLite truncates the path to 109
// bytes, and processFileEvent looks the watched file up by that truncated path.
// Anything deeper than 109 bytes therefore registers fine and then silently
// never fires — the exact case the existing long-path tests t.Skip() around.
func TestWatcher_LongPathStillDeliversEvents(t *testing.T) {
	dir := t.TempDir()
	// Build a directory chain that pushes the absolute path past 109 bytes.
	deep := dir
	for len(deep) < 120 {
		deep = filepath.Join(deep, "nested_directory_segment")
	}
	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(deep, "config.json")
	if len(path) <= 109 {
		t.Fatalf("test setup: path is only %d bytes", len(path))
	}
	if err := os.WriteFile(path, []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w := New(Config{
		PollInterval: 20 * time.Millisecond,
		CacheTTL:     10 * time.Millisecond,
		DisableAudit: true,
	})
	defer func() { _ = w.Stop() }()

	var mu sync.Mutex
	var got []ChangeEvent
	if err := w.Watch(path, func(e ChangeEvent) {
		mu.Lock()
		got = append(got, e)
		mu.Unlock()
	}); err != nil {
		t.Fatalf("Watch(%d-byte path): %v", len(path), err)
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(80 * time.Millisecond)

	mu.Lock()
	got = nil
	mu.Unlock()

	if err := os.WriteFile(path, []byte(`{"a":2}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) == 0 {
		t.Fatalf("no event delivered for a %d-byte path; the watch silently does nothing", len(path))
	}
	if got[0].Path != path {
		t.Errorf("event path = %q (%d bytes), want %q (%d bytes)",
			got[0].Path, len(got[0].Path), path, len(path))
	}
}

// TestWatcher_CallbackCanUnwatch: processFileEvent invokes the user callback
// while holding filesMu.RLock, so a callback that touches the watch list
// deadlocks the event processor.
func TestWatcher_CallbackCanUnwatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w := New(Config{
		PollInterval: 20 * time.Millisecond,
		CacheTTL:     10 * time.Millisecond,
		DisableAudit: true,
	})
	defer func() { _ = w.Stop() }()

	done := make(chan struct{})
	var once sync.Once
	if err := w.Watch(path, func(e ChangeEvent) {
		// A one-shot watch: unregister ourselves from inside the callback.
		_ = w.Unwatch(e.Path)
		once.Do(func() { close(done) })
	}); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(60 * time.Millisecond)
	if err := os.WriteFile(path, []byte(`{"a":2}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("callback that calls Unwatch never returned: deadlock on filesMu")
	}
}

// TestWatcher_RestartAfterStop: Start used a CAS on `running`, which succeeds
// again after Stop, but stopCh and stoppedCh are already closed — the polling
// goroutine then panicked on "close of closed channel" where no caller could
// recover it. A watcher is single-use, so Start must refuse instead.
func TestWatcher_RestartAfterStop(t *testing.T) {
	w := New(Config{
		PollInterval: 20 * time.Millisecond,
		CacheTTL:     10 * time.Millisecond,
		DisableAudit: true,
	})

	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := w.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	err := w.Start()
	if err == nil {
		t.Fatal("Start on a stopped watcher returned nil; it must refuse rather than panic in the background")
	}
	if !strings.Contains(err.Error(), ErrCodeWatcherStopped) {
		t.Errorf("Start on a stopped watcher = %v, want %s", err, ErrCodeWatcherStopped)
	}

	// Let any goroutine the refused Start might have spawned run and panic.
	time.Sleep(100 * time.Millisecond)
}

// TestWatcher_StopIsNotIdempotentButCloseIs pins the two contracts apart:
// Stop keeps reporting "not running", Close can always be deferred.
func TestWatcher_StopIsNotIdempotentButCloseIs(t *testing.T) {
	w := New(Config{PollInterval: 20 * time.Millisecond, DisableAudit: true})
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := w.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := w.Stop(); err == nil {
		t.Error("second Stop returned nil, want ErrCodeWatcherStopped")
	}
	if err := w.Close(); err != nil {
		t.Errorf("Close after Stop = %v, want nil", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("second Close = %v, want nil", err)
	}
}

// TestWatcher_StopReleasesAuditLoggerWithoutStart: a watcher that is created but
// never started still holds an open audit backend and its flush goroutine.
// Stop refuses to run, so nothing closes them.
func TestWatcher_StopReleasesAuditLoggerWithoutStart(t *testing.T) {
	w := New(Config{
		PollInterval: time.Second,
		CacheTTL:     500 * time.Millisecond,
	})

	if err := w.Close(); err != nil {
		t.Errorf("Close on a never-started watcher = %v, want nil (it must still release the audit logger)", err)
	}
}

// TestValidateSecurePath_SymlinkToSystemDirectory: validateAndSecurePath rewrites
// absPath to the resolved symlink target, so the isSystemDirectory check in
// validateSymlinks compares the target against itself (realPath == absPath) and
// never fires. A symlink into /etc is therefore accepted.
func TestValidateSecurePath_SymlinkToSystemDirectory(t *testing.T) {
	var target string
	for _, candidate := range []string{"/etc/hostname", "/etc/os-release", "/etc/protocols"} {
		if _, err := os.Stat(candidate); err == nil {
			target = candidate
			break
		}
	}
	if target == "" {
		t.Skip("no readable /etc file available on this host")
	}

	link := filepath.Join(t.TempDir(), "innocent.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	w := New(Config{DisableAudit: true})
	defer func() { _ = w.Close() }()

	err := w.Watch(link, func(ChangeEvent) {})
	if err == nil {
		t.Errorf("Watch on a symlink to %s was accepted; the system-directory guard never runs", target)
		return
	}
	if !strings.Contains(err.Error(), "system") {
		t.Errorf("rejected, but not by the system-directory guard: %v", err)
	}
}
