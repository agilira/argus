// argus_cache_test.go: the stat cache must not make a poll cycle superlinear.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// watcherWithFiles builds a watcher over n real files, not started: the tests
// below drive pollFiles by hand so a cycle is a cycle, not a race with a timer.
func watcherWithFiles(tb testing.TB, n int) (*Watcher, []string) {
	tb.Helper()

	dir := tb.TempDir()
	paths := make([]string, n)
	for i := range paths {
		p := filepath.Join(dir, fmt.Sprintf("config_%05d.json", i))
		if err := os.WriteFile(p, []byte(`{"a":1}`), 0o600); err != nil {
			tb.Fatalf("WriteFile: %v", err)
		}
		paths[i] = p
	}

	w := New(Config{
		PollInterval: time.Hour, // nothing polls unless the test says so

		// Polling stats the filesystem and ignores the cache, so this only
		// matters if someone puts a cache read back into the poll path: the
		// measurement below stays the cold path either way. It is also what
		// production sees, since WithDefaults clamps CacheTTL to
		// PollInterval and an entry is expired by the following cycle.
		CacheTTL: time.Nanosecond,

		MaxWatchedFiles: n + 10,
		DisableAudit:    true,
	})
	tb.Cleanup(func() { _ = w.Close() })

	for _, p := range paths {
		if err := w.Watch(p, func(ChangeEvent) {}); err != nil {
			tb.Fatalf("Watch: %v", err)
		}
	}
	return w, paths
}

// pollCost is what one poll cycle costs per file polled. Bytes, not
// allocation count: copying a map of N entries is a handful of large
// allocations, so a count hides the quadratic growth that bytes expose.
func pollCost(tb testing.TB, n int) (bytesPerFile, allocsPerFile float64) {
	tb.Helper()

	w, _ := watcherWithFiles(tb, n)
	w.pollFiles() // warm up whatever wants warming

	const cycles = 3
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < cycles; i++ {
		w.pollFiles()
	}
	runtime.ReadMemStats(&after)

	polled := float64(cycles * n)
	return float64(after.TotalAlloc-before.TotalAlloc) / polled,
		float64(after.Mallocs-before.Mallocs) / polled
}

// TestPollCycle_CostPerFileIsFlat is the regression test for the O(N²) stat
// cache: copy-on-write per entry copies the whole map for every single file,
// so the cost of polling one file grows with the number of files watched.
//
// Whatever the cache is made of, polling N files must cost N times polling one.
func TestPollCycle_CostPerFileIsFlat(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates a lot on purpose")
	}

	smallBytes, smallAllocs := pollCost(t, 100)
	largeBytes, largeAllocs := pollCost(t, 1000)

	t.Logf("per file polled: 100 files = %.0f B / %.1f allocs, 1000 files = %.0f B / %.1f allocs",
		smallBytes, smallAllocs, largeBytes, largeAllocs)

	// A tenfold increase in files must not raise the per-file cost. The slack
	// covers map growth and scheduling noise, not a different complexity class.
	if largeBytes > smallBytes*3 {
		t.Errorf("per-file cost grew from %.0f B (100 files) to %.0f B (1000 files): the poll cycle is superlinear",
			smallBytes, largeBytes)
	}

	// And the absolute cost should be an os.Stat and little else.
	if largeBytes > 1024 {
		t.Errorf("polling one file allocates %.0f B; expected an os.Stat and not much more", largeBytes)
	}
	if largeAllocs > 8 {
		t.Errorf("polling one file takes %.1f allocations; expected an os.Stat and not much more", largeAllocs)
	}
}

// TestPollCycle_ReadsTheFilesystemNotTheCache: a poll cycle exists to find out
// what changed on disk. Serving it a cached stat means the change is reported
// late, or not at all.
//
// CacheTTL == PollInterval is legal (WithDefaults only clamps above that), and
// under it every poll read used to hit a still-fresh entry.
func TestPollCycle_ReadsTheFilesystemNotTheCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w := New(Config{
		PollInterval: time.Hour, // this test drives the cycle itself
		CacheTTL:     time.Hour, // every cached stat stays fresh throughout
		DisableAudit: true,
	})
	defer func() { _ = w.Close() }()

	seen := make(chan ChangeEvent, 8)
	if err := w.Watch(path, func(e ChangeEvent) { seen <- e }); err != nil {
		t.Fatalf("Watch: %v", err) // also caches the initial stat
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Change the file, and make the change unmistakable on disk.
	if err := os.WriteFile(path, []byte(`{"a":2,"b":3}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	w.pollFiles()

	select {
	case e := <-seen:
		if e.Path != path {
			t.Errorf("event for %q, want %q", e.Path, path)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the poll cycle missed a change on disk: it read the stat cache instead of the file")
	}
}
