// audit_disable_test.go: opt-out path for the system audit trail
//
// Pins Config.DisableAudit: a host that owns its own audit trail can tell
// argus.New NOT to stand up the unified system audit (no SQLite backend, no
// flush goroutine), while the secure default (audit ON) is unchanged for every
// other caller.
//
// Copyright (c) 2025 AGILira
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"testing"
)

// TestConfig_DisableAudit_TurnsAuditOff verifies the opt-out flag wins over the
// secure default: WithDefaults must NOT promote Audit to the enabled default
// when DisableAudit is set.
func TestConfig_DisableAudit_TurnsAuditOff(t *testing.T) {
	c := (&Config{DisableAudit: true}).WithDefaults()
	if c.Audit.Enabled {
		t.Fatalf("DisableAudit=true must yield Audit.Enabled=false, got enabled")
	}
}

// TestConfig_AuditDefaultStaysEnabled is the regression guard for the secure
// default: a zero Config (no DisableAudit, no Audit) must still get the enabled
// audit default. Protects existing callers that rely on audit-by-default.
func TestConfig_AuditDefaultStaysEnabled(t *testing.T) {
	c := (&Config{}).WithDefaults()
	if !c.Audit.Enabled {
		t.Fatalf("zero Config must keep audit enabled by default, got disabled")
	}
}

// TestNewDisabledAuditLogger_Inert pins that the inert logger is fully safe:
// no backend, every method a graceful no-op or graceful error, and Close()
// does not panic on the nil backend / absent flush ticker.
func TestNewDisabledAuditLogger_Inert(t *testing.T) {
	al := newDisabledAuditLogger()
	if al == nil {
		t.Fatal("newDisabledAuditLogger returned nil")
	}
	if al.backend != nil {
		t.Fatal("disabled audit logger must have a nil backend (no SQLite opened)")
	}
	if al.config.Enabled {
		t.Fatal("disabled audit logger must report config.Enabled=false")
	}

	// Log must be a no-op: with a nil backend it returns early and buffers
	// nothing, so a subsequent Flush stays empty and error-free.
	al.Log(AuditInfo, "evt", "comp", "", nil, nil, nil)
	al.LogConfigChange("f", nil, nil)
	al.LogFileWatch("watch", "f")
	al.LogSecurityEvent("sec", "details", nil)
	if err := al.Flush(); err != nil {
		t.Fatalf("Flush on inert logger must be nil, got %v", err)
	}

	// GetStats / Query degrade to a typed error rather than dereferencing the
	// nil backend.
	if _, err := al.GetStats(); err == nil {
		t.Fatal("GetStats on inert logger must return an error, got nil")
	}
	if _, err := al.Query(AuditEventFilter{}); err == nil {
		t.Fatal("Query on inert logger must return an error, got nil")
	}

	// Close must not panic and must report no error.
	if err := al.Close(); err != nil {
		t.Fatalf("Close on inert logger must be nil, got %v", err)
	}
}

// TestNew_DisableAudit_NoBackend verifies the end-to-end wiring: argus.New with
// DisableAudit set installs the inert logger (nil backend) instead of opening
// the unified system audit database.
func TestNew_DisableAudit_NoBackend(t *testing.T) {
	w := New(Config{DisableAudit: true})
	if w.auditLogger == nil {
		t.Fatal("watcher has no audit logger")
	}
	if w.auditLogger.backend != nil {
		t.Fatal("DisableAudit must yield a nil audit backend; system audit DB was opened")
	}
}

// TestNew_AuditEnabledByDefault is the wiring-level regression guard: a default
// New still stands up a real (non-nil) audit backend.
func TestNew_AuditEnabledByDefault(t *testing.T) {
	w := New(Config{})
	if w.auditLogger == nil || w.auditLogger.backend == nil {
		t.Fatal("default New must stand up a real audit backend")
	}
	t.Cleanup(func() { _ = w.auditLogger.Close() })
}

// TestNew_AuditBackendFailure_FallsBackToDisabled exercises the error-fallback
// path in New: when audit is enabled but the backend cannot be created, New
// must degrade to a disabled audit logger rather than fail. We force the
// failure by giving an explicit ".jsonl" OutputFile whose parent is a regular
// FILE, not a directory: the explicit .jsonl extension skips the SQLite path,
// and MkdirAll on "<file>/sub" fails with "not a directory", so NewAuditLogger
// returns an error and New must fall back.
func TestNew_AuditBackendFailure_FallsBackToDisabled(t *testing.T) {
	parentFile := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(parentFile, []byte("x"), 0600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	badPath := filepath.Join(parentFile, "sub", "audit.jsonl")
	w := New(Config{
		Audit: AuditConfig{Enabled: true, OutputFile: badPath},
	})
	if w.auditLogger == nil {
		t.Fatal("New must always install an audit logger, even on backend failure")
	}
	if w.auditLogger.config.Enabled {
		t.Fatal("on backend-creation failure New must fall back to a disabled audit logger")
	}
	t.Cleanup(func() { _ = w.auditLogger.Close() })
}
