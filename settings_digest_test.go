// settings_digest_test.go - tests for the content identity of a revision
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDigest_SameContentSameDigest: two instances that will answer the same
// questions the same way carry the same digest, whatever else differs about
// them. This is the whole point: a run can name its inputs in a way that means
// something on another machine.
func TestDigest_SameContentSameDigest(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	body := `{"model": "claude-opus-5", "temperature": 0.2, "server": {"port": 8080}}`
	prompt := "You are a careful assistant."

	build := func(dir, app string) *Settings {
		t.Helper()
		writeDoc(t, filepath.Join(dir, "config.json"), body)
		writeDoc(t, filepath.Join(dir, "prompts", "system.md"), prompt)
		settings, err := Setup(app).
			File(filepath.Join(dir, "config.json")).
			Documents("prompts", filepath.Join(dir, "prompts", "*.md")).
			Start()
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		return settings
	}

	a := build(first, "instance-a")
	defer func() { _ = a.Close() }()
	b := build(second, "instance-b")
	defer func() { _ = b.Close() }()

	if a.Digest() == "" {
		t.Fatal("empty digest")
	}
	if a.Digest() != b.Digest() {
		t.Errorf("two instances of the same configuration disagree:\n  %s\n  %s", a.Digest(), b.Digest())
	}
	// Reading it twice is reading the same thing.
	once := a.Digest()
	if twice := a.Digest(); twice != once {
		t.Errorf("the digest is not stable within one revision: %s then %s", once, twice)
	}
	if got := a.Explain().Digest; got != a.Digest() {
		t.Errorf("Explain reports %q, Digest says %q", got, a.Digest())
	}
}

// TestDigest_IgnoresWhichSourceSuppliedTheValue: the same value from the
// environment and from a file is the same configuration, and a run reproduced
// from either is the same run.
func TestDigest_IgnoresWhichSourceSuppliedTheValue(t *testing.T) {
	fromFile := t.TempDir()
	writeDoc(t, filepath.Join(fromFile, "config.json"), `{"model": "opus", "port": 9090}`)
	a, err := Setup("service").File(filepath.Join(fromFile, "config.json")).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = a.Close() }()

	fromEnv := t.TempDir()
	writeDoc(t, filepath.Join(fromEnv, "config.json"), `{"model": "opus", "port": 1}`)
	t.Setenv("DIGEST_PORT", "9090")
	b, err := Setup("service").File(filepath.Join(fromEnv, "config.json")).Env("DIGEST_").Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = b.Close() }()

	if a.GetInt("port") != b.GetInt("port") {
		t.Fatalf("the two instances do not agree on port: %d and %d", a.GetInt("port"), b.GetInt("port"))
	}
	if a.Digest() != b.Digest() {
		t.Errorf("same values, different digests:\n  file %s\n  env  %s", a.Digest(), b.Digest())
	}
}

// TestDigest_MovesWithTheContent: a value, a prompt, a new document — each
// changes the identity of what the application is running. And undoing a
// change brings the identity back, because the digest names the content and
// not the history.
func TestDigest_MovesWithTheContent(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"model": "opus", "temperature": 0.2}`)
	prompt := writeDoc(t, filepath.Join(dir, "prompts", "system.md"), "Be careful.")

	reloads := make(chan Change, 8)
	settings, err := Setup("agent").
		File(config).
		Documents("prompts", filepath.Join(dir, "prompts", "*.md")).
		MaxStaleness(20 * time.Millisecond).
		OnReload(func(_ *Settings, c Change) { reloads <- c }).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	await := func(what string) string {
		t.Helper()
		select {
		case <-reloads:
		case <-time.After(5 * time.Second):
			t.Fatalf("no reload after %s", what)
		}
		return settings.Digest()
	}

	seen := map[string]string{settings.Digest(): "start"}
	remember := func(what, digest string) {
		t.Helper()
		if previous, repeated := seen[digest]; repeated {
			t.Errorf("%s produced the digest of %q", what, previous)
		}
		seen[digest] = what
	}

	writeDoc(t, config, `{"model": "opus", "temperature": 0.9}`)
	remember("a value changes", await("a value change"))

	writeDoc(t, prompt, "Be brief.")
	withOnePrompt := await("a prompt change")
	remember("a prompt changes", withOnePrompt)

	tone := filepath.Join(dir, "prompts", "tone.md")
	writeDoc(t, tone, "Warm.")
	remember("a document appears", await("a new document"))

	// Undoing it returns to the identity of the state it was undone from.
	if err := os.Remove(tone); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if back := await("a document going away"); back != withOnePrompt {
		t.Errorf("removing the document did not return to the previous identity:\n  %s\n  %s",
			withOnePrompt, back)
	}
}

// TestDigest_SurvivesARebuildWithTheSameContent: rewriting a file with the
// same bytes is not a new configuration.
func TestDigest_SurvivesARebuildWithTheSameContent(t *testing.T) {
	dir := t.TempDir()
	body := `{"model": "opus", "regions": ["eu", "us"]}`
	config := writeDoc(t, filepath.Join(dir, "config.json"), body)

	backend := &handBackend{}
	settings, err := Setup("service").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()
	settings.core.reloader.backend = backend
	_ = backend.start(settings.core.reloader.cycle)

	before := settings.Digest()

	// Same bytes, new timestamp: a rebuild that finds nothing changed.
	writeDoc(t, config, body)
	backend.tick()

	if after := settings.Digest(); after != before {
		t.Errorf("the digest moved without the content:\n  %s\n  %s", before, after)
	}
}

// TestDigest_ReachesTheAuditTrail: "this run used digest abc123" is only worth
// writing down if the trail agrees.
func TestDigest_ReachesTheAuditTrail(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"model": "opus"}`)
	auditFile := filepath.Join(dir, "audit.jsonl")

	auditor, err := NewAuditLogger(AuditConfig{
		Enabled:       true,
		OutputFile:    auditFile,
		MinLevel:      AuditInfo,
		BufferSize:    16,
		FlushInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}

	settings, err := Setup("agent").File(config).AuditTo(auditor).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	digest := settings.Digest()
	if err := settings.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := auditor.Close(); err != nil {
		t.Fatalf("auditor Close: %v", err)
	}

	trail, err := os.ReadFile(auditFile) // #nosec G304 -- test-owned temporary path
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !strings.Contains(string(trail), digest) {
		t.Errorf("the audit trail does not carry the digest %s", digest)
	}
}
