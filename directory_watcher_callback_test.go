package argus

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatchDirectory_DeleteCallbackCanCloseWatcher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("key: value\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var watcher atomic.Pointer[DirectoryWatcher]
	closeReturned := make(chan bool, 1)
	dw, err := WatchDirectory(dir, DirectoryWatchOptions{
		Patterns:     []string{"*.yaml"},
		PollInterval: 10 * time.Millisecond,
	}, func(update DirectoryConfigUpdate) {
		if !update.IsDelete {
			return
		}
		w := watcher.Load()
		if w == nil {
			closeReturned <- false
			return
		}

		// A callback must be able to stop its own watcher. Close is called
		// on the callback's goroutine: Close waits for in-flight callbacks
		// (none may run after it returns), so a callback that blocked on a
		// Close running elsewhere would be waiting on itself.
		_ = w.Close()
		closeReturned <- true
	})
	if err != nil {
		t.Fatalf("WatchDirectory: %v", err)
	}
	watcher.Store(dw)
	t.Cleanup(func() { _ = dw.Close() })

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	select {
	case ok := <-closeReturned:
		if !ok {
			t.Fatal("Close from the delete callback did not complete")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("delete callback was not delivered")
	}
}
