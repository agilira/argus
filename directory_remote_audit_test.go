// directory_remote_audit_test.go: regression tests for directory watching and
// remote configuration defects found in audit
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"context"
	goerrors "errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestDirectoryWatcher_CloseFromCallback: notifyMerged holds dw.mu.RLock while
// it runs the user callback, so a callback that closes the watcher — the
// obvious "stop once I have what I need" shape — blocks forever on dw.mu.Lock.
func TestDirectoryWatcher_CloseFromCallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var holder struct {
		mu sync.Mutex
		dw *DirectoryWatcher
	}
	done := make(chan struct{})
	var once sync.Once

	dw, err := WatchDirectoryMerged(dir, DirectoryWatchOptions{
		PollInterval: 30 * time.Millisecond,
	}, func(merged map[string]interface{}, files []string) {
		once.Do(func() {
			holder.mu.Lock()
			w := holder.dw
			holder.mu.Unlock()
			if w != nil {
				_ = w.Close()
			}
			close(done)
		})
	})
	if err != nil {
		t.Fatalf("WatchDirectoryMerged: %v", err)
	}
	holder.mu.Lock()
	holder.dw = dw
	holder.mu.Unlock()
	defer func() { _ = dw.Close() }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("a merged callback that calls Close never returned: deadlock on dw.mu")
	}
}

// TestDirectoryWatcher_MergedRunsOneScanLoop: WatchDirectoryMerged starts
// mergeLoop on top of the pollLoop that watchDirectoryInternal already
// started, so the directory is walked twice per interval by two goroutines
// racing on the same state.
func TestDirectoryWatcher_MergedRunsOneScanLoop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dw, err := WatchDirectoryMerged(dir, DirectoryWatchOptions{
		PollInterval: 25 * time.Millisecond,
	}, func(map[string]interface{}, []string) {})
	if err != nil {
		t.Fatalf("WatchDirectoryMerged: %v", err)
	}
	defer func() { _ = dw.Close() }()

	time.Sleep(80 * time.Millisecond)

	buf := make([]byte, 1<<18)
	stacks := string(buf[:runtime.Stack(buf, true)])
	poll := strings.Contains(stacks, "DirectoryWatcher).pollLoop")
	merge := strings.Contains(stacks, "DirectoryWatcher).mergeLoop")
	if poll && merge {
		t.Error("merged mode runs both pollLoop and mergeLoop, scanning the same directory twice per interval")
	}
}

// TestDirectoryWatcher_SymlinkEscapesDirectory: single-file watching runs an
// elaborate symlink guard (validateAndSecurePath). Directory watching only
// checks the directory string for "..", then filepath.Walk hands loadAndNotify
// every matching name and os.ReadFile follows the symlink out of the tree.
func TestDirectoryWatcher_SymlinkEscapesDirectory(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.json")
	if err := os.WriteFile(secret, []byte(`{"token":"s3cr3t"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dir := t.TempDir()
	if err := os.Symlink(secret, filepath.Join(dir, "innocent.json")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	var mu sync.Mutex
	var seen []map[string]interface{}
	dw, err := WatchDirectory(dir, DirectoryWatchOptions{
		PollInterval: 30 * time.Millisecond,
	}, func(u DirectoryConfigUpdate) {
		mu.Lock()
		seen = append(seen, u.Config)
		mu.Unlock()
	})
	if err != nil {
		return // refusing the symlink outright is a correct outcome
	}
	defer func() { _ = dw.Close() }()

	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	for _, c := range seen {
		if c["token"] == "s3cr3t" {
			t.Error("directory watching read a file through a symlink pointing outside the watched directory")
		}
	}
}

// fakeProvider is a registered remote provider so the RemoteConfig validation
// (which requires a provider for the scheme) can be satisfied in tests.
type fakeProvider struct {
	mu       sync.Mutex
	attempts []time.Time
	fail     bool
}

func (f *fakeProvider) Name() string   { return "fake" }
func (f *fakeProvider) Scheme() string { return "fake" }
func (f *fakeProvider) Load(ctx context.Context, configURL string) (map[string]interface{}, error) {
	f.mu.Lock()
	f.attempts = append(f.attempts, time.Now())
	fail := f.fail
	f.mu.Unlock()
	if fail {
		return nil, goerrors.New("temporary network failure")
	}
	return map[string]interface{}{"ok": true}, nil
}
func (f *fakeProvider) Watch(ctx context.Context, configURL string) (<-chan map[string]interface{}, error) {
	return nil, nil
}
func (f *fakeProvider) Validate(configURL string) error                 { return nil }
func (f *fakeProvider) HealthCheck(ctx context.Context, u string) error { return nil }

var fakeProviderOnce sync.Once
var sharedFake = &fakeProvider{}

func registerFakeProvider(t *testing.T) *fakeProvider {
	t.Helper()
	fakeProviderOnce.Do(func() {
		if err := RegisterRemoteProvider(sharedFake); err != nil {
			t.Fatalf("RegisterRemoteProvider: %v", err)
		}
	})
	sharedFake.mu.Lock()
	sharedFake.attempts = nil
	sharedFake.mu.Unlock()
	return sharedFake
}

// TestRemoteConfig_RetryUsesExponentialBackoff: RemoteConfig.RetryDelay is
// documented as "exponential backoff: attempt N waits RetryDelay * 2^N"
// (1s, 2s, 4s...), and config.go sizes the default Timeout from that formula.
//
// It also pins the retry BUDGET. Each attempt used to call the loader with its
// own default options, nesting a second retry policy inside every attempt: the
// documented three attempts reached the remote twelve times.
func TestRemoteConfig_RetryUsesExponentialBackoff(t *testing.T) {
	f := registerFakeProvider(t)
	f.mu.Lock()
	f.fail = true
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.fail = false; f.mu.Unlock() }()

	w := New(Config{DisableAudit: true})
	defer func() { _ = w.Close() }()

	cfg := &RemoteConfig{
		Enabled:      true,
		PrimaryURL:   "fake://host/config",
		SyncInterval: time.Hour, // no periodic sync during the test
		Timeout:      5 * time.Second,
		MaxRetries:   2,
		RetryDelay:   40 * time.Millisecond,
	}
	rm, err := NewRemoteConfigManager(cfg, w)
	if err != nil {
		t.Fatalf("NewRemoteConfigManager: %v", err)
	}
	_ = rm.Start() // the initial load fails and retries; that is what we measure
	rm.Stop()

	f.mu.Lock()
	attempts := append([]time.Time(nil), f.attempts...)
	f.mu.Unlock()

	if len(attempts) != 3 {
		t.Fatalf("provider was called %d times, want exactly 3 (initial + MaxRetries)", len(attempts))
	}

	// Each gap is measured against the documented schedule rather than
	// against the other one. A busy machine only ever makes a gap longer, so
	// a lower bound survives a loaded CI runner; comparing the two gaps does
	// not, because an inflated first gap breaks a ratio that the second one
	// honoured exactly. Linear backoff still fails this: its second gap would
	// be one RetryDelay, well under two.
	tolerance := cfg.RetryDelay / 4
	gaps := []struct {
		measured time.Duration
		want     time.Duration
	}{
		{attempts[1].Sub(attempts[0]), cfg.RetryDelay},
		{attempts[2].Sub(attempts[1]), 2 * cfg.RetryDelay},
	}
	for i, gap := range gaps {
		if gap.measured+tolerance < gap.want {
			t.Errorf("retry %d waited %v, want at least %v (RetryDelay * 2^%d)",
				i+1, gap.measured, gap.want, i)
		}
	}
}

// TestRemoteConfigManager_AuditEventAndComponent: every remote_config audit
// record passes "remote_config" as the event and the action as the component,
// the opposite of every other call site in the library. Filtering by Component
// or EventPrefix therefore misses these records entirely.
func TestRemoteConfigManager_AuditEventAndComponent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "audit.db")
	al, err := NewAuditLogger(AuditConfig{
		Enabled: true, OutputFile: dbPath, MinLevel: AuditInfo, BufferSize: 10,
	})
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}
	defer func() { _ = al.Close() }()

	w := &Watcher{auditLogger: al}
	cfg := &RemoteConfig{Enabled: true, PrimaryURL: "fake://host/config"}
	rm, err := NewRemoteConfigManager(cfg, w)
	if err != nil {
		t.Fatalf("NewRemoteConfigManager: %v", err)
	}
	_ = rm.Start()
	rm.Stop()
	if err := al.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	events, err := al.Query(AuditEventFilter{Component: "argus"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(events) == 0 {
		t.Error(`no remote_config event recorded with Component="argus"; event and component are swapped at the call site`)
	}
}

// TestNewRemoteConfigManager_DoesNotMutateCallerConfig: the constructor writes
// defaults straight into the RemoteConfig the caller owns.
func TestNewRemoteConfigManager_DoesNotMutateCallerConfig(t *testing.T) {
	cfg := &RemoteConfig{Enabled: true, PrimaryURL: "fake://host/config"}
	before := *cfg

	w := New(Config{DisableAudit: true})
	defer func() { _ = w.Close() }()

	if _, err := NewRemoteConfigManager(cfg, w); err != nil {
		t.Fatalf("NewRemoteConfigManager: %v", err)
	}

	if *cfg != before {
		t.Errorf("constructor mutated the caller's RemoteConfig: SyncInterval %v -> %v, Timeout %v -> %v",
			before.SyncInterval, cfg.SyncInterval, before.Timeout, cfg.Timeout)
	}
}

// TestNewRemoteConfigManager_NilWatcher: a nil watcher is accepted and then
// dereferenced by Start on the first audit call.
func TestNewRemoteConfigManager_NilWatcher(t *testing.T) {
	cfg := &RemoteConfig{Enabled: true, PrimaryURL: "fake://host/config"}
	rm, err := NewRemoteConfigManager(cfg, nil)
	if err != nil {
		return // refusing a nil watcher is a correct outcome
	}
	defer rm.Stop()
	_ = rm.Start() // must not panic
}

// TestWatcherRemote_EnabledConfigIsWired: Config.Remote is documented as
// "when enabled, provides distributed configuration management with local
// fallback", and config.go validates and defaults every one of its fields —
// but argus.New never read it and NewRemoteConfigManager had no caller in the
// library, so setting Remote.Enabled did nothing at all.
func TestWatcherRemote_EnabledConfigIsWired(t *testing.T) {
	f := registerFakeProvider(t)

	w := New(Config{
		PollInterval: time.Second,
		DisableAudit: true,
		Remote: RemoteConfig{
			Enabled:      true,
			PrimaryURL:   "fake://host/config",
			SyncInterval: time.Hour,
			Timeout:      2 * time.Second,
		},
	})
	defer func() { _ = w.Close() }()

	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	config, _, err := w.RemoteConfig()
	if err != nil {
		t.Fatalf("RemoteConfig: %v", err)
	}
	if config["ok"] != true {
		t.Errorf("RemoteConfig() = %v, want the configuration the provider returned", config)
	}

	f.mu.Lock()
	attempts := len(f.attempts)
	f.mu.Unlock()
	if attempts == 0 {
		t.Error("the watcher never reached the remote provider")
	}
}

// TestWatcherRemote_DisabledStaysInert: the zero value must remain free.
func TestWatcherRemote_DisabledStaysInert(t *testing.T) {
	w := New(Config{PollInterval: time.Second, DisableAudit: true})
	defer func() { _ = w.Close() }()

	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, _, err := w.RemoteConfig(); err == nil {
		t.Error("RemoteConfig() succeeded on a watcher with remote configuration disabled")
	}
}

// TestWatcherRemote_InvalidConfigFailsStart: New cannot return an error, so a
// remote configuration that cannot be used must surface at Start rather than
// be dropped in silence.
func TestWatcherRemote_InvalidConfigFailsStart(t *testing.T) {
	w := New(Config{
		PollInterval: time.Second,
		DisableAudit: true,
		Remote: RemoteConfig{
			Enabled:    true,
			PrimaryURL: "no-such-scheme://host/config",
		},
	})
	defer func() { _ = w.Close() }()

	if err := w.Start(); err == nil {
		t.Error("Start accepted a remote configuration with no registered provider")
	}
}

// TestWatcherRemote_CloseStopsSync: closing the watcher must stop the sync loop.
func TestWatcherRemote_CloseStopsSync(t *testing.T) {
	registerFakeProvider(t)

	w := New(Config{
		PollInterval: time.Second,
		DisableAudit: true,
		Remote: RemoteConfig{
			Enabled:      true,
			PrimaryURL:   "fake://host/config",
			SyncInterval: 30 * time.Millisecond,
			Timeout:      10 * time.Millisecond,
		},
	})
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(80 * time.Millisecond)

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	f := sharedFake
	f.mu.Lock()
	before := len(f.attempts)
	f.mu.Unlock()

	time.Sleep(120 * time.Millisecond)

	f.mu.Lock()
	after := len(f.attempts)
	f.mu.Unlock()

	if after != before {
		t.Errorf("the provider was called %d more times after Close", after-before)
	}
}

// TestRemoteFallback_LoadsTheLocalFile: FallbackPath is documented as "the
// ultimate fallback for high-availability deployments" — the file to use when
// every remote source is down. loadLocalFallback never opened it: it returned
// a hardcoded {"fallback": true, "source": ..., "message": ...} and reported
// success, so an application reading its emergency configuration got three
// meaningless keys at the exact moment the fallback was supposed to save it.
func TestRemoteFallback_LoadsTheLocalFile(t *testing.T) {
	f := registerFakeProvider(t)
	f.mu.Lock()
	f.fail = true
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.fail = false; f.mu.Unlock() }()

	fallbackPath := filepath.Join(t.TempDir(), "emergency.json")
	body := `{"database_url": "postgres://emergency/db", "workers": 4}`
	if err := os.WriteFile(fallbackPath, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w := New(Config{DisableAudit: true})
	defer func() { _ = w.Close() }()

	rm, err := NewRemoteConfigManager(&RemoteConfig{
		Enabled:      true,
		PrimaryURL:   "fake://host/config",
		FallbackPath: fallbackPath,
		SyncInterval: time.Hour,
		Timeout:      time.Second,
		MaxRetries:   0,
		RetryDelay:   time.Millisecond,
	}, w)
	if err != nil {
		t.Fatalf("NewRemoteConfigManager: %v", err)
	}
	_ = rm.Start()
	defer rm.Stop()

	config, _, err := rm.GetCurrentConfig()
	if err != nil {
		t.Fatalf("GetCurrentConfig: %v", err)
	}

	if config["database_url"] != "postgres://emergency/db" {
		t.Errorf("database_url = %v, want the value from the fallback file", config["database_url"])
	}
	if config["workers"] != float64(4) {
		t.Errorf("workers = %v (%T), want 4 from the fallback file", config["workers"], config["workers"])
	}
}

// TestRemoteFallback_ReportsAMissingFile: a fallback that is configured but
// absent must be an error, not a success carrying placeholder data.
func TestRemoteFallback_ReportsAMissingFile(t *testing.T) {
	f := registerFakeProvider(t)
	f.mu.Lock()
	f.fail = true
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.fail = false; f.mu.Unlock() }()

	w := New(Config{DisableAudit: true})
	defer func() { _ = w.Close() }()

	rm, err := NewRemoteConfigManager(&RemoteConfig{
		Enabled:      true,
		PrimaryURL:   "fake://host/config",
		FallbackPath: filepath.Join(t.TempDir(), "absent.json"),
		SyncInterval: time.Hour,
		Timeout:      time.Second,
		MaxRetries:   0,
		RetryDelay:   time.Millisecond,
	}, w)
	if err != nil {
		t.Fatalf("NewRemoteConfigManager: %v", err)
	}

	if err := rm.Start(); err == nil {
		t.Error("Start reported success with every remote source down and no fallback file on disk")
	}
	rm.Stop()
}

// TestRemoteFallback_SupportsEveryFormat: the field's documentation offers
// JSON, YAML or TOML. Detection must work the same as everywhere else.
func TestRemoteFallback_SupportsEveryFormat(t *testing.T) {
	f := registerFakeProvider(t)
	f.mu.Lock()
	f.fail = true
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.fail = false; f.mu.Unlock() }()

	dir := t.TempDir()
	cases := []struct {
		name string
		body string
	}{
		{"fallback.json", `{"workers": 7}`},
		{"fallback.yaml", "workers: 7\n"},
		{"fallback.toml", "workers = 7\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name)
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			w := New(Config{DisableAudit: true})
			defer func() { _ = w.Close() }()

			rm, err := NewRemoteConfigManager(&RemoteConfig{
				Enabled:      true,
				PrimaryURL:   "fake://host/config",
				FallbackPath: path,
				SyncInterval: time.Hour,
				Timeout:      time.Second,
				MaxRetries:   0,
				RetryDelay:   time.Millisecond,
			}, w)
			if err != nil {
				t.Fatalf("NewRemoteConfigManager: %v", err)
			}
			_ = rm.Start()
			defer rm.Stop()

			config, _, err := rm.GetCurrentConfig()
			if err != nil {
				t.Fatalf("GetCurrentConfig: %v", err)
			}
			if _, ok := config["workers"]; !ok {
				t.Errorf("%s: the fallback file was not parsed: %v", tc.name, config)
			}
		})
	}
}
