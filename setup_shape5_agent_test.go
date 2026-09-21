// setup_shape5_agent_test.go - shape 5: an AI application with keys and documents
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agilira/argus"
)

// TestShape5_Agent is the shape that drives the implementation: a
// configuration file for the keys, a prompts/ directory and a skills/
// directory for the documents, hot reload while the process serves traffic,
// and an audit trail of what changed.
//
// Documents are the piece Argus does not have today. A prompt is bytes: it is
// read, never parsed, and it lives in its own namespace so that a prompt named
// "model" and a key named "model" never meet.
func TestShape5_Agent(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "agent.json")
	writeFile(t, config, `{"model": "claude-opus-5", "temperature": 0.2}`)

	systemPrompt := filepath.Join(dir, "prompts", "system.md")
	writeFile(t, systemPrompt, "You are a careful assistant. Cite your sources.")
	writeFile(t, filepath.Join(dir, "skills", "summarize.md"), "# Summarize\nKeep it short.")
	writeFile(t, filepath.Join(dir, "skills", "translate.md"), "# Translate\nPreserve tone.")

	auditFile := filepath.Join(dir, "audit.jsonl")
	auditor, err := argus.NewAuditLogger(argus.AuditConfig{
		Enabled:       true,
		OutputFile:    auditFile,
		MinLevel:      argus.AuditInfo,
		BufferSize:    64,
		FlushInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}

	reloads := make(chan argus.Change, 4)

	// The one line — folded here, but one call.
	settings, err := argus.Setup("agent").
		File(config).
		Documents("prompts", filepath.Join(dir, "prompts", "*.md")).
		Documents("skills", filepath.Join(dir, "skills", "*.md")).
		AuditTo(auditor).
		MaxStaleness(50 * time.Millisecond).
		OnReload(func(_ *argus.Settings, changed argus.Change) { reloads <- changed }).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Keys are keys.
	if got := settings.GetString("model"); got != "claude-opus-5" {
		t.Errorf("model = %q", got)
	}

	// Documents are bytes, addressed as (group, name).
	prompt, ok := settings.Doc("prompts", "system")
	if !ok {
		t.Fatal("Doc(prompts, system) not found")
	}
	if !strings.HasPrefix(prompt.String(), "You are a careful assistant.") {
		t.Errorf("system prompt = %q", prompt.String())
	}
	if len(settings.Docs("skills")) != 2 {
		t.Errorf("Docs(skills) = %d documents, want 2", len(settings.Docs("skills")))
	}

	// The two namespaces do not touch: a document is not readable as a key.
	if got := settings.GetString("system"); got != "" {
		t.Errorf("GetString(system) = %q, want empty: documents are not keys", got)
	}

	// A prompt change and a new skill saved together are one revision, not two.
	before := settings.Revision()
	writeFile(t, systemPrompt, "You are a careful assistant. Always cite sources.")
	writeFile(t, filepath.Join(dir, "skills", "classify.md"), "# Classify\nOne label.")

	select {
	case changed := <-reloads:
		if !changed.HasDocument("prompts", "system") {
			t.Errorf("Change does not mention prompts/system: %+v", changed)
		}
		if !changed.HasDocument("skills", "classify") {
			t.Errorf("Change does not mention skills/classify: %+v", changed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no reload after the prompt and the skill changed")
	}
	if got := settings.Revision(); got != before+1 {
		t.Errorf("Revision = %d, want %d: two files saved together are one swap", got, before+1)
	}

	updated, _ := settings.Doc("prompts", "system")
	if !strings.Contains(updated.String(), "Always cite sources") {
		t.Errorf("prompt not reloaded: %q", updated.String())
	}

	// Close flushes the audit logger it was given; it does not close a logger
	// it did not create.
	if err := settings.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := auditor.Close(); err != nil {
		t.Fatalf("auditor Close: %v", err)
	}

	// The audit records the name and the hash of what changed. The prompt
	// itself must never reach the audit database.
	trail, err := os.ReadFile(auditFile) // #nosec G304 -- test-owned temporary path
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !strings.Contains(string(trail), "system") {
		t.Error("audit does not name the document that changed")
	}
	if strings.Contains(string(trail), "Always cite sources") {
		t.Error("audit contains the prompt text; it must record a hash, not the content")
	}
}
