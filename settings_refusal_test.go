// settings_refusal_test.go - tests for what a refused revision leaves behind
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRefusal_IsVisibleWithoutAnErrorHandler: an application that declares no
// OnError still has to be able to find out that what it is serving is not what
// is on disk. A refused candidate never becomes a revision, so it has to be
// somewhere else.
func TestRefusal_IsVisibleWithoutAnErrorHandler(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"port": 8080}`)

	backend := &handBackend{}
	settings, err := Setup("service").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()
	settings.core.reloader.backend = backend
	_ = backend.start(settings.core.reloader.cycle)

	if settings.Explain().LastRefusal != nil {
		t.Fatal("a refusal before anything was refused")
	}

	writeDoc(t, config, `{"port": `)
	backend.tick()

	refusal := settings.Explain().LastRefusal
	if refusal == nil {
		t.Fatal("a refused candidate left no trace, and nobody was told")
	}
	if !strings.Contains(refusal.Reason, "parse") {
		t.Errorf("Reason = %q", refusal.Reason)
	}
	if refusal.Revision != 1 {
		t.Errorf("Revision = %d, want the one still in force", refusal.Revision)
	}
	if refusal.Cycles != 1 {
		t.Errorf("Cycles = %d, want 1", refusal.Cycles)
	}
	if time.Since(refusal.At) > time.Minute {
		t.Errorf("At = %v", refusal.At)
	}

	// It reaches a debug endpoint like the rest of Explain.
	rendered, err := json.Marshal(settings.Explain())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(rendered), "last_refusal") {
		t.Errorf("Explain JSON has no refusal: %s", rendered)
	}
}

// TestRefusal_CountsConsecutiveFailures: a remote that has been failing for an
// hour and one that failed once are different situations.
func TestRefusal_CountsConsecutiveFailures(t *testing.T) {
	provider := failingProvider("refusalcounts", nil)
	url := useRemoteProvider(t, provider)

	backend := &handBackend{}
	settings, err := Setup("service").
		Remote(url, &RemoteConfigOptions{RetryAttempts: 1, RetryDelay: time.Millisecond}).
		RemoteInterval(time.Nanosecond).
		Start()
	if err == nil {
		defer func() { _ = settings.Close() }()
		t.Fatal("Start succeeded with nothing but an unreachable remote")
	}

	// The same shape, with a local file so that Start comes up.
	dir := t.TempDir()
	settings, err = Setup("service").
		File(writeDoc(t, filepath.Join(dir, "config.json"), `{"port": 8080}`)).
		Remote(url, &RemoteConfigOptions{RetryAttempts: 1, RetryDelay: time.Millisecond}).
		RemoteInterval(time.Nanosecond).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()
	settings.core.reloader.backend = backend
	_ = backend.start(settings.core.reloader.cycle)

	// A remote that is down is an issue, not a refusal: the local file still
	// resolves. Make the candidate itself impossible instead.
	config := filepath.Join(dir, "config.json")
	writeDoc(t, config, `{"port": `)
	backend.tick()
	if got := settings.Explain().LastRefusal; got == nil || got.Cycles != 1 {
		t.Fatalf("first refusal = %+v", got)
	}

	// Each further failed cycle counts, and the reason stays the same.
	for i := 0; i < 2; i++ {
		writeDoc(t, config, `{"port": `+strings.Repeat(" ", i+1))
		backend.tick()
	}
	refusal := settings.Explain().LastRefusal
	if refusal == nil || refusal.Cycles != 3 {
		t.Errorf("refusal = %+v, want three consecutive", refusal)
	}
}

// TestRefusal_ClearedByAGoodRevision: the state says what is true now.
func TestRefusal_ClearedByAGoodRevision(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"port": 8080}`)

	backend := &handBackend{}
	settings, err := Setup("service").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()
	settings.core.reloader.backend = backend
	_ = backend.start(settings.core.reloader.cycle)

	writeDoc(t, config, `{"port": `)
	backend.tick()
	if settings.Explain().LastRefusal == nil {
		t.Fatal("no refusal recorded")
	}

	writeDoc(t, config, `{"port": 9090}`)
	backend.tick()

	if got := settings.Explain().LastRefusal; got != nil {
		t.Errorf("LastRefusal = %+v after a good revision", got)
	}
	if got := settings.GetInt("port"); got != 9090 {
		t.Errorf("port = %d", got)
	}
}
