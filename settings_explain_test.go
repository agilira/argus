// settings_explain_test.go - tests for Explain and for the audit trail
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestExplain_NamesTheSourceOfEveryKey is the question an operator actually
// has: this pod is behaving oddly, where is it reading that value from?
func TestExplain_NamesTheSourceOfEveryKey(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"),
		`{"port": 8080, "workers": 4, "region": "eu"}`)
	t.Setenv("EX_WORKERS", "16")

	settings, err := Setup("explained").
		File(config).
		Env("EX_").
		Overrides(map[string]interface{}{"region": "us"}).
		Defaults(map[string]interface{}{"timeout": "30s"}).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	explanation := settings.Explain()

	for key, want := range map[string]Source{
		"port":    SourceFile,
		"workers": SourceEnv,
		"region":  SourceOverride,
		"timeout": SourceDefault,
	} {
		if got := explanation.KeySource(key); got != want {
			t.Errorf("KeySource(%q) = %v, want %v", key, got, want)
		}
	}
	if got := explanation.KeySource("nobody_set_this"); got != SourceNone {
		t.Errorf("KeySource of an absent key = %v, want none", got)
	}

	if explanation.App != "explained" || explanation.Revision != 1 {
		t.Errorf("app = %q, revision = %d", explanation.App, explanation.Revision)
	}
	if explanation.Backend == "" {
		t.Error("Explain does not say what is watching the filesystem")
	}
	if explanation.MaxStaleness <= 0 {
		t.Error("Explain does not say how stale a value may be")
	}

	// It is a struct so that it can be served on a debug endpoint.
	if _, err := json.Marshal(explanation); err != nil {
		t.Errorf("Explain does not marshal: %v", err)
	}
}

func TestExplain_CountsDocumentsWithoutQuotingThem(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"model": "claude-opus-5"}`)
	writeDoc(t, filepath.Join(dir, "prompts", "system.md"), "be careful")
	writeDoc(t, filepath.Join(dir, "skills", "summarize.md"), "keep it short")

	settings, err := Setup("agent").
		File(config).
		Documents("prompts", filepath.Join(dir, "prompts", "*.md")).
		Documents("skills", filepath.Join(dir, "skills", "*.md")).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	explanation := settings.Explain()
	if explanation.Documents.Count != 2 {
		t.Errorf("document count = %d, want 2", explanation.Documents.Count)
	}
	if explanation.Documents.Groups["prompts"] != 1 || explanation.Documents.Groups["skills"] != 1 {
		t.Errorf("groups = %v", explanation.Documents.Groups)
	}
	if explanation.Documents.Bytes == 0 {
		t.Error("document bytes = 0")
	}

	rendered, err := json.Marshal(explanation)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(rendered), "be careful") {
		t.Error("Explain quotes a prompt; it counts documents, it does not copy them")
	}
}

// TestExplain_ReportsASkippedDocument: a file left out of a revision is the
// kind of thing nobody notices until an agent answers oddly.
func TestExplain_ReportsASkippedDocument(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"model": "claude-opus-5"}`)
	prompts := filepath.Join(dir, "prompts")
	writeDoc(t, filepath.Join(prompts, "system.md"), "be careful")
	outside := writeDoc(t, filepath.Join(dir, "outside", "secret.md"), "not yours")
	if err := os.Symlink(outside, filepath.Join(prompts, "leak.md")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	settings, err := Setup("agent").
		File(config).
		Documents("prompts", filepath.Join(prompts, "*.md")).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	issues := settings.Explain().Issues
	if len(issues) != 1 || !strings.Contains(issues[0], "leak") {
		t.Errorf("Issues = %v, want the skipped document named", issues)
	}
}

// TestAudit_RecordsARefusal: a broken file saved over a running application
// leaves a trace, with the reason and without the file's contents.
func TestAudit_RecordsARefusal(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"model": "claude-opus-5"}`)
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

	failures := make(chan error, 1)
	settings, err := Setup("agent").
		File(config).
		AuditTo(auditor).
		MaxStaleness(20 * time.Millisecond).
		OnError(func(err error) {
			select {
			case failures <- err:
			default:
			}
		}).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	writeDoc(t, config, `{"model": "claude-opus-5`)

	select {
	case <-failures:
	case <-time.After(5 * time.Second):
		t.Fatal("a broken configuration file was never reported")
	}

	if got := settings.GetString("model"); got != "claude-opus-5" {
		t.Errorf("model = %q, want the last good value", got)
	}
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
	if !strings.Contains(string(trail), "settings_refused") {
		t.Errorf("the refusal is not in the audit trail: %s", trail)
	}
}

// TestSettings_SubReadsOneSubtree: one agent, or one tenant, without repeating
// the prefix at every call.
func TestSettings_SubReadsOneSubtree(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"),
		`{"agents": {"writer": {"model": "opus", "temperature": 0.7}}}`)

	settings, err := Setup("agent").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	writer := settings.Sub("agents.writer")
	if got := writer.GetString("model"); got != "opus" {
		t.Errorf("model = %q", got)
	}
	if got := writer.GetFloat64("temperature"); got != 0.7 {
		t.Errorf("temperature = %v", got)
	}

	// Closing a subtree does nothing: the handle it came from owns the
	// sources, and a library handing a Sub to a caller must not hand it the
	// power to stop the reload.
	if err := writer.Close(); err != nil {
		t.Errorf("closing a Sub: %v", err)
	}
	if settings.Revision() == 0 {
		t.Error("closing a Sub stopped the parent")
	}
}

// TestSettings_CloseStopsTheGoroutines: a handle that leaks its poller leaks
// it in every test of every application that uses Argus.
func TestSettings_CloseStopsTheGoroutines(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"port": 8080}`)

	before := runtime.NumGoroutine()

	for i := 0; i < 10; i++ {
		settings, err := Setup("daemon").File(config).MaxStaleness(time.Millisecond).Start()
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		if err := settings.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if leaked := runtime.NumGoroutine() - before; leaked > 2 {
		t.Errorf("%d goroutines outlived their settings", leaked)
	}
}
