// settings_reload_regression_test.go: regression tests for the reload cycle
//
// Copyright (c) 2026 AGILira - A. Giordano
// Series: an AGILira library
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// remoteRuns makes every run's scheme its own: the provider registry is global
// and cannot replace an entry, so a test run with -count above one would
// otherwise collide with itself.
var remoteRuns atomic.Int64

// freshRemote registers provider under a scheme no earlier run has used.
func freshRemote(t *testing.T, provider *settingsRemoteProvider) string {
	t.Helper()
	provider.scheme += strconv.FormatInt(remoteRuns.Add(1), 10)
	return useRemoteProvider(t, provider)
}

// startByHand starts a builder on a backend driven by the test.
func startByHand(t *testing.T, b *SetupBuilder) (*Settings, *handBackend) {
	t.Helper()
	hb := &handBackend{}
	b.backend = hb
	s, err := b.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, hb
}

// TestReload_FileEditWinsOverARemoteKey: a key named by both the file and the
// remote is the file's. An edit to it must be published, not hidden behind the
// remote's unchanged value.
func TestReload_FileEditWinsOverARemoteKey(t *testing.T) {
	url := freshRemote(t, &settingsRemoteProvider{
		scheme: "regressionshadow",
		values: map[string]interface{}{"port": 9999},
	})
	config := writeDoc(t, filepath.Join(t.TempDir(), "config.json"), `{"port": 8080}`)

	s, hb := startByHand(t, Setup("svc").File(config).
		Remote(url, &RemoteConfigOptions{RetryAttempts: 1}).RemoteInterval(time.Hour))

	writeDoc(t, config, `{"port": 18081}`)
	hb.tick()

	if got := s.GetInt("port"); got != 18081 {
		t.Errorf("port = %d after editing the file, want 18081 (revision %d)", got, s.Revision())
	}
}

// TestReload_NoCycleAfterClose: a backend may still hold a tick when it is
// told to stop. When Close has returned, that tick must do nothing.
func TestReload_NoCycleAfterClose(t *testing.T) {
	config := writeDoc(t, filepath.Join(t.TempDir(), "config.json"), `{"port": 8080}`)

	var reloads atomic.Int64
	s, hb := startByHand(t, Setup("svc").File(config).
		OnReload(func(*Settings, Change) { reloads.Add(1) }))

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	writeDoc(t, config, `{"port": 18081}`)
	hb.tick() // the tick the backend had already handed out

	if s.Revision() != 1 || reloads.Load() != 0 {
		t.Errorf("a cycle ran after Close: revision %d, %d reload(s)", s.Revision(), reloads.Load())
	}
}

// TestReload_FailingSoleRemoteWaitsForItsInterval: a remote that is down is
// tried again when its interval falls due, not at every poll.
func TestReload_FailingSoleRemoteWaitsForItsInterval(t *testing.T) {
	provider := &settingsRemoteProvider{
		scheme: "regressionsoleretry",
		values: map[string]interface{}{"k": 1},
	}
	url := freshRemote(t, provider)

	s, hb := startByHand(t, Setup("svc").
		Remote(url, &RemoteConfigOptions{RetryAttempts: 1}).RemoteInterval(time.Hour))

	provider.fail.Store(true)
	skipAhead(s, 2*time.Hour)
	hb.tick() // due: one failed fetch
	base := provider.loads.Load()
	for i := 0; i < 5; i++ {
		hb.tick()
	}

	if got := provider.loads.Load() - base; got != 0 {
		t.Errorf("a failing remote was called %d more times within its interval, want 0", got)
	}
}

// TestReload_RemoteOutageSpendsNoRevision: a remote that goes down and comes
// back with the same values changes no value, so it spends no revision — and
// Explain still says what happened to it.
func TestReload_RemoteOutageSpendsNoRevision(t *testing.T) {
	provider := &settingsRemoteProvider{
		scheme: "regressionoutage",
		values: map[string]interface{}{"remote_key": "v"},
	}
	url := freshRemote(t, provider)
	config := writeDoc(t, filepath.Join(t.TempDir(), "config.json"), `{"local": 1}`)

	var reloads atomic.Int64
	s, hb := startByHand(t, Setup("svc").File(config).
		Remote(url, &RemoteConfigOptions{RetryAttempts: 1}).RemoteInterval(time.Hour).
		OnReload(func(*Settings, Change) { reloads.Add(1) }))

	provider.fail.Store(true)
	skipAhead(s, 2*time.Hour)
	hb.tick()

	if got := s.Revision(); got != 1 {
		t.Errorf("revision = %d after an outage that changed no value, want 1", got)
	}
	if remoteState(s) != "stale" || len(s.Explain().Issues) == 0 {
		t.Errorf("the outage is not reported: state %q, issues %v", remoteState(s), s.Explain().Issues)
	}

	provider.fail.Store(false)
	skipAhead(s, 4*time.Hour)
	hb.tick()

	if got := s.Revision(); got != 1 {
		t.Errorf("revision = %d after the remote came back unchanged, want 1", got)
	}
	if remoteState(s) != "loaded" || len(s.Explain().Issues) != 0 {
		t.Errorf("the recovery is not reported: state %q, issues %v", remoteState(s), s.Explain().Issues)
	}
	if reloads.Load() != 0 {
		t.Errorf("%d reload(s) for revisions that were never spent", reloads.Load())
	}
}

// remoteState is the state Explain gives the remote source.
func remoteState(s *Settings) string {
	for _, state := range s.Explain().Sources {
		if state.Source == SourceRemote {
			return state.State
		}
	}
	return ""
}

// TestReload_RefusalCountsEveryFailingCycle: a file that stays broken has
// been failing at every cycle, not once.
func TestReload_RefusalCountsEveryFailingCycle(t *testing.T) {
	config := writeDoc(t, filepath.Join(t.TempDir(), "config.json"), `{"port": 8080}`)
	s, hb := startByHand(t, Setup("svc").File(config))

	writeDoc(t, config, `{"port":`)
	hb.tick()
	hb.tick()
	hb.tick()

	refusal := s.Explain().LastRefusal
	if refusal == nil || refusal.Cycles != 3 {
		t.Fatalf("LastRefusal = %+v after three failing cycles, want Cycles 3", refusal)
	}
	if refusal.Revision != 1 {
		t.Errorf("refusal revision = %d, want 1", refusal.Revision)
	}
}

// TestExplain_OptionalFileStateFollowsTheRevision: an optional file that was
// absent at the revision in force, and then appears broken, is refused. The
// revision serves none of it, so Explain must not call it loaded.
func TestExplain_OptionalFileStateFollowsTheRevision(t *testing.T) {
	dir := t.TempDir()
	base := writeDoc(t, filepath.Join(dir, "base.json"), `{"port": 8080}`)
	optional := filepath.Join(dir, "local.json")

	s, hb := startByHand(t, Setup("svc").File(base).FileIfPresent(optional))

	writeDoc(t, optional, `{"port":`)
	hb.tick()

	if s.Explain().LastRefusal == nil {
		t.Fatal("the broken optional file was not refused")
	}
	for _, state := range s.Explain().Sources {
		if state.Detail == optional && state.State != "absent" {
			t.Errorf("optional file state = %q, want absent: the revision in force does not have it", state.State)
		}
	}
}
