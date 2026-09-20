// audit_integrity_audit_test.go: regression tests for audit-trail defects found in audit
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestAuditLogger(t *testing.T) (*AuditLogger, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "audit.db")
	al, err := NewAuditLogger(AuditConfig{
		Enabled:       true,
		OutputFile:    dbPath,
		MinLevel:      AuditInfo,
		BufferSize:    100,
		FlushInterval: 0, // no background flusher; the test flushes explicitly
	})
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}
	t.Cleanup(func() { _ = al.Close() })
	return al, dbPath
}

// tamper rewrites one column of the single stored audit row, simulating an
// attacker with write access to the audit database.
func tamper(t *testing.T, dbPath, column, value string) {
	t.Helper()
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec("UPDATE audit_events SET "+column+" = ?", value); err != nil {
		t.Fatalf("tamper %s: %v", column, err)
	}
}

// TestAuditChecksum_CoversFilePath: the checksum is sold as tamper detection
// (CWE-345) but generateChecksum hashes only timestamp, event, component,
// old and new value. The file path — the single most security-relevant field
// in a file watcher's audit trail — is not covered.
func TestAuditChecksum_CoversFilePath(t *testing.T) {
	al, dbPath := newTestAuditLogger(t)
	al.LogFileWatch("watch_start", "/etc/shadow")
	if err := al.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	tamper(t, dbPath, "file_path", "/tmp/harmless.json")

	events, err := al.Query(AuditEventFilter{})
	if err == nil {
		t.Errorf("Query accepted a row whose file_path was rewritten to %q; integrity check did not fire",
			events[0].FilePath)
	}
}

// TestAuditChecksum_CoversLevel: the severity is not hashed either, so a
// SECURITY event can be quietly downgraded to INFO and still verify.
func TestAuditChecksum_CoversLevel(t *testing.T) {
	al, dbPath := newTestAuditLogger(t)
	al.LogSecurityEvent("path_traversal_attempt", "rejected ../../etc/passwd", map[string]interface{}{
		"rejected_path": "../../etc/passwd",
	})
	if err := al.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	tamper(t, dbPath, "level", AuditInfo.String())

	if _, err := al.Query(AuditEventFilter{}); err == nil {
		t.Error("Query accepted a SECURITY row downgraded to INFO; integrity check did not fire")
	}
}

// TestAuditChecksum_CoversContext: the context map carries the rejected path
// and the reason for every security event, and is not hashed.
func TestAuditChecksum_CoversContext(t *testing.T) {
	al, dbPath := newTestAuditLogger(t)
	al.LogSecurityEvent("path_traversal_attempt", "rejected", map[string]interface{}{
		"rejected_path": "../../etc/passwd",
	})
	if err := al.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	tamper(t, dbPath, "context", `{"rejected_path":"config.json"}`)

	if _, err := al.Query(AuditEventFilter{}); err == nil {
		t.Error("Query accepted a row whose context was rewritten; integrity check did not fire")
	}
}

// TestLogSecurityEvent_RecordsDetails: LogSecurityEvent takes a `details`
// string and never passes it to Log, so the human-readable description of
// every security event is silently dropped.
func TestLogSecurityEvent_RecordsDetails(t *testing.T) {
	al, _ := newTestAuditLogger(t)
	al.LogSecurityEvent("path_traversal_attempt", "Rejected malicious file path", map[string]interface{}{
		"rejected_path": "../../etc/passwd",
	})
	if err := al.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	events, err := al.Query(AuditEventFilter{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}

	details, ok := events[0].Context["details"]
	if !ok {
		t.Fatal("the details argument of LogSecurityEvent is not recorded anywhere in the event")
	}
	if details != "Rejected malicious file path" {
		t.Errorf("details = %v, want %q", details, "Rejected malicious file path")
	}
}

// TestAuditDatabase_IsNotWorldReadable: the SQLite backend lets the driver
// create the database with the default 0644, while the JSONL backend of the
// same system is careful to use 0600. The audit trail records which files a
// process watches and every rejected path; on the default location
// (os.TempDir()/argus/system-audit.db) that is readable by anyone who can
// reach the file.
func TestAuditDatabase_IsNotWorldReadable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "audit.db")
	al, err := NewAuditLogger(AuditConfig{
		Enabled: true, OutputFile: dbPath, MinLevel: AuditInfo, BufferSize: 10,
	})
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}
	al.LogFileWatch("watch_start", "/srv/app/config.json")
	if err := al.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	_ = al.Close()

	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("audit database mode = %v, want no group/other access (the JSONL backend uses 0600)", perm)
	}
}

// TestAuditLogger_CloseIsIdempotent: Close closes stopCh unconditionally, so a
// second Close panics with "close of closed channel". Close is exported and
// documented as graceful shutdown.
func TestAuditLogger_CloseIsIdempotent(t *testing.T) {
	al, _ := newTestAuditLogger(t)
	if err := al.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := al.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestAuditLogger_FlushOnNilLoggerIsSafe: Log, GetStats and Query all guard a
// nil receiver; Flush does not, so it panics instead.
func TestAuditLogger_FlushOnNilLoggerIsSafe(t *testing.T) {
	var al *AuditLogger
	if err := al.Flush(); err != nil {
		t.Errorf("Flush on nil logger = %v, want nil", err)
	}
}

var _ = time.Second

// TestUnifiedAuditPath_IsPerUser: the unified audit database used to live at
// os.TempDir()/argus/system-audit.db. That directory is world-writable, so the
// first account to create argus/ owned the audit trail of every other account
// on the host, and a pre-created symlink would have redirected it (CWE-377).
func TestUnifiedAuditPath_IsPerUser(t *testing.T) {
	clearArgusEnv(t)

	path := getUnifiedAuditPath()
	tempRoot := os.TempDir()

	if strings.HasPrefix(path, filepath.Join(tempRoot, "argus")+string(filepath.Separator)) {
		t.Errorf("unified audit path %q is inside the shared temporary directory", path)
	}
	if filepath.Base(path) != "system-audit.db" {
		t.Errorf("unified audit path = %q, want it to end in system-audit.db", path)
	}
}

// TestUnifiedAuditPath_RespectsExplicitDir: deployments that place the trail
// themselves must win over every heuristic.
func TestUnifiedAuditPath_RespectsExplicitDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARGUS_AUDIT_DIR", dir)

	if got, want := getUnifiedAuditPath(), filepath.Join(dir, "system-audit.db"); got != want {
		t.Errorf("getUnifiedAuditPath() = %q, want %q", got, want)
	}
}

// TestAuditDatabase_SidecarsAreNotWorldReadable: WAL and shared-memory files
// hold committed records that have not been checkpointed into the database
// yet, so they need the same protection as the database itself.
func TestAuditDatabase_SidecarsAreNotWorldReadable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "audit.db")
	al, err := NewAuditLogger(AuditConfig{
		Enabled: true, OutputFile: dbPath, MinLevel: AuditInfo, BufferSize: 10,
	})
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}
	al.LogFileWatch("watch_start", "/srv/app/config.json")
	if err := al.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	defer func() { _ = al.Close() }()

	for _, suffix := range []string{"-wal", "-shm"} {
		info, err := os.Stat(dbPath + suffix)
		if err != nil {
			continue // SQLite may have checkpointed it away already
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("%s mode = %v, want no group/other access", filepath.Base(dbPath+suffix), perm)
		}
	}
}
