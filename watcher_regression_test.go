// watcher_regression_test.go: regression tests for the file watcher, the BoreasLite ring and the directory watcher
//
// Copyright (c) 2026 AGILira - A. Giordano
// Series: an AGILira library
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// regressionWatcher returns an audit-free watcher polling every 20ms.
func regressionWatcher(t *testing.T, cfg Config) *Watcher {
	t.Helper()
	cfg.DisableAudit = true
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 20 * time.Millisecond
	}
	w := New(cfg)
	t.Cleanup(func() { _ = w.Close() })
	return w
}

// waitFor polls cond until it holds or the timeout expires.
func waitRegression(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

func writeRegressionFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A write refused because the ring is full must not consume a sequence
// number: the consumer would wait on that unpublished slot forever.
func TestRingOverflowDoesNotWedgeConsumer(t *testing.T) {
	var processed atomic.Int64
	b := NewBoreasLite(64, OptimizationAuto, func(*FileChangeEvent) { processed.Add(1) })

	for i := 0; i < 65; i++ { // one more than fits, consumer not running yet
		b.WriteFileChange("/f", time.Now(), int64(i), false, false, true)
	}
	if got := b.Stats()["items_dropped"]; got != 1 {
		t.Fatalf("items_dropped = %d, want 1", got)
	}

	go b.RunProcessor()
	defer b.Stop()
	if !waitRegression(2*time.Second, func() bool { return processed.Load() == 64 }) {
		t.Fatalf("processed %d of the 64 buffered events", processed.Load())
	}

	b.WriteFileChange("/f", time.Now(), 99, false, false, true)
	if !waitRegression(2*time.Second, func() bool { return processed.Load() == 65 }) {
		t.Fatalf("event written after an overflow never delivered; stats=%v", b.Stats())
	}
}

// AdaptStrategy (called by Watch/Unwatch) must not race with the consumer
// reading the batch size. Meaningful under -race.
func TestAdaptStrategyConcurrentWithConsumer(t *testing.T) {
	b := NewBoreasLite(64, OptimizationAuto, func(*FileChangeEvent) { time.Sleep(50 * time.Microsecond) })
	go b.RunProcessor()
	defer b.Stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			for j := 0; j < 10; j++ {
				b.WriteFileChange("/x", time.Now(), 1, false, false, true)
			}
			time.Sleep(200 * time.Microsecond)
		}
	}()
	for i := 0; i < 1000; i++ {
		b.AdaptStrategy(i % 100)
		time.Sleep(10 * time.Microsecond)
	}
	<-done
}

// Stop must not return while a callback is running or still queued.
func TestStopWaitsForCallbacks(t *testing.T) {
	dir := t.TempDir()
	w := regressionWatcher(t, Config{})

	var stopReturned, late atomic.Bool
	entered := make(chan struct{}, 10)
	release := make(chan struct{})
	cb := func(ChangeEvent) {
		entered <- struct{}{}
		<-release
		if stopReturned.Load() {
			late.Store(true)
		}
	}
	paths := []string{filepath.Join(dir, "a.json"), filepath.Join(dir, "b.json")}
	for _, p := range paths {
		writeRegressionFile(t, p, "{}")
		if err := w.Watch(p, cb); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		writeRegressionFile(t, p, `{"x":1}`)
	}
	<-entered

	stopDone := make(chan error, 1)
	go func() {
		err := w.Stop()
		stopReturned.Store(true)
		stopDone <- err
	}()
	select {
	case err := <-stopDone:
		t.Fatalf("Stop returned (err %v) while a callback was still running", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-stopDone; err != nil {
		t.Fatal(err)
	}
	if late.Load() {
		t.Fatal("a callback ran after Stop returned")
	}
}

// A callback may stop its own watcher without deadlocking.
func TestStopFromCallbackDoesNotDeadlock(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.json")
	writeRegressionFile(t, p, "{}")
	w := regressionWatcher(t, Config{})
	stopped := make(chan error, 1)
	var once sync.Once
	if err := w.Watch(p, func(ChangeEvent) {
		once.Do(func() { stopped <- w.Stop() })
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	writeRegressionFile(t, p, `{"x":1}`)
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Stop called from a callback deadlocked")
	}
}

// An atomic rename-over that keeps size and mtime (cp -p, rsync -a) replaces
// the file and must be reported.
func TestRenameOverPreservedMtimeDetected(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.json")
	writeRegressionFile(t, p, `{"v":1}`)
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	w := regressionWatcher(t, Config{})
	var events atomic.Int32
	if err := w.Watch(p, func(ChangeEvent) { events.Add(1) }); err != nil {
		t.Fatal(err)
	}
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}

	tmp := filepath.Join(dir, ".c.json.tmp")
	writeRegressionFile(t, tmp, `{"v":2}`)
	if err := os.Chtimes(tmp, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, p); err != nil {
		t.Fatal(err)
	}
	if !waitRegression(2*time.Second, func() bool { return events.Load() > 0 }) {
		t.Fatal("rename-over with preserved size and mtime was not reported")
	}
}

// The directory watcher must see a content change that keeps the mtime.
func TestDirWatcherDetectsChangeWithPreservedMtime(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.json")
	writeRegressionFile(t, p, `{"v":1}`)
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var last map[string]interface{}
	dw, err := WatchDirectory(dir, DirectoryWatchOptions{PollInterval: 20 * time.Millisecond},
		func(u DirectoryConfigUpdate) { mu.Lock(); last = u.Config; mu.Unlock() })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dw.Close() }()

	writeRegressionFile(t, p, `{"v":1,"extra":"yes"}`)
	if err := os.Chtimes(p, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	if !waitRegression(2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		_, ok := last["extra"]
		return ok
	}) {
		t.Fatal("content change with preserved mtime was not reported")
	}
}

// In merged mode a file that fails to parse keeps its last good config.
func TestMergedKeepsLastGoodConfigOnParseError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.json")
	writeRegressionFile(t, p, `{"db":"prod"}`)

	var mu sync.Mutex
	var snaps []map[string]interface{}
	parseErr := make(chan struct{}, 10)
	dw, err := WatchDirectoryMerged(dir, DirectoryWatchOptions{
		PollInterval: 20 * time.Millisecond,
		ErrorHandler: func(error, string) { parseErr <- struct{}{} },
	}, func(m map[string]interface{}, _ []string) {
		mu.Lock()
		snaps = append(snaps, m)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dw.Close() }()

	writeRegressionFile(t, p, `{"db":`)
	select {
	case <-parseErr:
	case <-time.After(2 * time.Second):
		t.Fatal("the half-written file was never scanned")
	}
	writeRegressionFile(t, p, `{"db":"stage"}`)
	if !waitRegression(2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(snaps) > 0 && snaps[len(snaps)-1]["db"] == "stage"
	}) {
		t.Fatal("the repaired file was never merged")
	}

	mu.Lock()
	defer mu.Unlock()
	for i, s := range snaps {
		if _, ok := s["db"]; !ok {
			t.Fatalf("merged snapshot %d lost key \"db\" after a parse error: %v", i, s)
		}
	}
}

// Close must not return while a scan is still delivering callbacks.
func TestDirCloseWaitsForCallbacks(t *testing.T) {
	dir := t.TempDir()
	names := []string{"a.json", "b.json", "c.json"}
	for _, n := range names {
		writeRegressionFile(t, filepath.Join(dir, n), `{}`)
	}
	var armed, closeReturned, late atomic.Bool
	entered := make(chan struct{}, 10)
	release := make(chan struct{})
	dw, err := WatchDirectory(dir, DirectoryWatchOptions{PollInterval: 20 * time.Millisecond},
		func(DirectoryConfigUpdate) {
			if !armed.Load() {
				return
			}
			entered <- struct{}{}
			<-release
			if closeReturned.Load() {
				late.Store(true)
			}
		})
	if err != nil {
		t.Fatal(err)
	}
	armed.Store(true)
	future := time.Now().Add(time.Hour)
	for _, n := range names {
		if err := os.Chtimes(filepath.Join(dir, n), future, future); err != nil {
			t.Fatal(err)
		}
	}
	<-entered

	closeDone := make(chan struct{})
	go func() {
		_ = dw.Close()
		closeReturned.Store(true)
		close(closeDone)
	}()
	select {
	case <-closeDone:
		t.Fatal("Close returned while a callback was still running")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	<-closeDone
	if late.Load() {
		t.Fatal("a callback ran after Close returned")
	}
}

// With no ErrorHandler a stat error other than "not found" goes to stderr,
// as Config.ErrorHandler documents, instead of vanishing.
func TestStatErrorLoggedWithNilHandler(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0000 does not block os.Stat on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(sub, "c.json")
	writeRegressionFile(t, p, `{}`)
	t.Cleanup(func() { _ = os.Chmod(sub, 0o700) })

	r, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = wr
	defer func() { os.Stderr = orig }()

	got := make(chan struct{})
	go func() {
		var b [1]byte
		if n, _ := r.Read(b[:]); n > 0 {
			close(got)
		}
	}()

	w := New(Config{PollInterval: 20 * time.Millisecond, DisableAudit: true})
	defer func() { _ = w.Close() }()
	if err := w.Watch(p, func(ChangeEvent) {}); err != nil {
		t.Fatal(err)
	}
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sub, 0o000); err != nil {
		t.Fatal(err)
	}
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("permission-denied stat error was neither delivered nor logged")
	}
	_ = w.Stop()
	os.Stderr = orig
	_ = wr.Close()
	_ = r.Close()
}

// A stat of a file that does not exist has no modification time and no size.
func TestFileStatOfMissingFile(t *testing.T) {
	var missing fileStat
	if !missing.modTime().IsZero() || missing.size() != 0 {
		t.Errorf("missing file: modTime %v size %d, want zero", missing.modTime(), missing.size())
	}
}

// With an ErrorHandler set, errors go to it and not to stderr.
func TestReportErrorUsesHandler(t *testing.T) {
	var gotPath string
	w := New(Config{ErrorHandler: func(_ error, path string) { gotPath = path }})
	w.reportError(os.ErrPermission, "config.json")
	if gotPath != "config.json" {
		t.Errorf("handler got path %q, want config.json", gotPath)
	}
}

// With no ErrorHandler set, reportError writes the path and error to stderr.
func TestReportErrorLogsToStderrWithNilHandler(t *testing.T) {
	r, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()

	orig := os.Stderr
	os.Stderr = wr
	defer func() { os.Stderr = orig }()

	w := New(Config{DisableAudit: true})
	defer func() { _ = w.Close() }()
	w.reportError(os.ErrPermission, "config.json")

	os.Stderr = orig
	if err := wr.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "config.json") {
		t.Errorf("stderr = %q, want message naming config.json", string(out))
	}
}

// In merged mode a change that keeps the modification time (cp -p, rsync -a)
// is merged and announced: the merged hash follows loads, not mtimes.
func TestMergedAnnouncesChangeWithPreservedMtime(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.json")
	writeRegressionFile(t, p, `{"db":"prod"}`)
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var last map[string]interface{}
	dw, err := WatchDirectoryMerged(dir, DirectoryWatchOptions{PollInterval: 20 * time.Millisecond},
		func(m map[string]interface{}, _ []string) { mu.Lock(); last = m; mu.Unlock() })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dw.Close() }()

	writeRegressionFile(t, p, `{"db":"stage-longer"}`)
	if err := os.Chtimes(p, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	if !waitRegression(2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return last["db"] == "stage-longer"
	}) {
		t.Fatal("a change with preserved mtime was never merged and announced")
	}
}

// A second Close, concurrent with a first one that is waiting for a callback,
// does not return before that callback has finished.
func TestDirConcurrentCloseWaitsForCallbacks(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.json")
	writeRegressionFile(t, p, `{}`)
	var armed, late atomic.Bool
	var returned atomic.Int32
	entered := make(chan struct{}, 10)
	release := make(chan struct{})
	dw, err := WatchDirectory(dir, DirectoryWatchOptions{PollInterval: 20 * time.Millisecond},
		func(DirectoryConfigUpdate) {
			if !armed.Load() {
				return
			}
			entered <- struct{}{}
			<-release
			if returned.Load() > 0 {
				late.Store(true)
			}
		})
	if err != nil {
		t.Fatal(err)
	}
	armed.Store(true)
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
	<-entered

	done := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_ = dw.Close()
			returned.Add(1)
			done <- struct{}{}
		}()
	}
	select {
	case <-done:
		t.Fatal("a Close returned while a callback was still running")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	<-done
	<-done
	if late.Load() {
		t.Fatal("a callback ran after a Close returned")
	}
}

// A callback that calls Close while another goroutine is already shutting the
// watcher down does not deadlock: the shutdown waits for the consumer without
// holding the lifecycle lock the callback needs.
func TestCloseFromCallbackDuringExternalClose(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.json")
	writeRegressionFile(t, p, `{}`)
	w := regressionWatcher(t, Config{})

	entered := make(chan struct{}, 10)
	release := make(chan struct{})
	callbackDone := make(chan struct{})
	var once sync.Once
	if err := w.Watch(p, func(ChangeEvent) {
		once.Do(func() {
			entered <- struct{}{}
			<-release
			_ = w.Close()
			close(callbackDone)
		})
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	writeRegressionFile(t, p, `{"x":1}`)
	<-entered

	external := make(chan struct{})
	go func() { _ = w.Close(); close(external) }()
	time.Sleep(50 * time.Millisecond) // let the external Close reach its wait
	close(release)

	select {
	case <-external:
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: the external Close never returned")
	}
	select {
	case <-callbackDone:
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: the callback's Close never returned")
	}
}
