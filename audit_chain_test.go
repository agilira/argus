// audit_chain_test.go: the audit trail is a hash chain, not a bag of records
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// newChainLogger returns a logger over its own database plus that database's path.
func newChainLogger(t *testing.T, dbPath string) *AuditLogger {
	t.Helper()
	al, err := NewAuditLogger(AuditConfig{
		Enabled:    true,
		OutputFile: dbPath,
		MinLevel:   AuditInfo,
		BufferSize: 100,
	})
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}
	return al
}

// writeTrail records n events and flushes them.
func writeTrail(t *testing.T, al *AuditLogger, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		al.LogFileWatch("watch_start", "/srv/app/config.json")
	}
	if err := al.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
}

// execOnTrail runs a statement straight against the audit database, standing in
// for an attacker with write access to the file.
func execOnTrail(t *testing.T, dbPath, statement string) {
	t.Helper()
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(statement); err != nil {
		t.Fatalf("exec %q: %v", statement, err)
	}
}

// TestAuditChain_AcceptsIntactTrail: an untouched trail verifies.
func TestAuditChain_AcceptsIntactTrail(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "audit.db")
	al := newChainLogger(t, dbPath)
	defer func() { _ = al.Close() }()

	writeTrail(t, al, 5)

	if err := al.VerifyAuditChain(); err != nil {
		t.Errorf("VerifyAuditChain on an intact trail = %v, want nil", err)
	}
}

// TestAuditChain_DetectsDeletedRecord: removing a record breaks the link
// between its neighbours. Per-record checksums cannot see this — every
// surviving record still verifies against its own fields.
func TestAuditChain_DetectsDeletedRecord(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "audit.db")
	al := newChainLogger(t, dbPath)
	defer func() { _ = al.Close() }()

	writeTrail(t, al, 5)

	execOnTrail(t, dbPath, "DELETE FROM audit_events WHERE id = 3")

	if err := al.VerifyAuditChain(); err == nil {
		t.Error("VerifyAuditChain accepted a trail with a deleted record")
	}
}

// TestAuditChain_DetectsTruncatedTail: dropping the most recent records is the
// cheapest way to hide what just happened.
func TestAuditChain_DetectsTruncatedTail(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "audit.db")
	al := newChainLogger(t, dbPath)

	writeTrail(t, al, 5)
	if err := al.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	execOnTrail(t, dbPath, "DELETE FROM audit_events WHERE id >= 4")

	// Reopening must notice that the trail no longer ends where it did.
	reopened := newChainLogger(t, dbPath)
	defer func() { _ = reopened.Close() }()

	if err := reopened.VerifyAuditChain(); err != nil {
		t.Logf("truncation reported on reopen: %v", err)
		return
	}
	t.Error("VerifyAuditChain accepted a truncated trail")
}

// TestAuditChain_DetectsReorder: swapping two records' positions must not verify.
func TestAuditChain_DetectsReorder(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "audit.db")
	al := newChainLogger(t, dbPath)
	defer func() { _ = al.Close() }()

	for i := 0; i < 4; i++ {
		al.LogFileWatch("watch_start", "/srv/app/config.json")
		al.LogSecurityEvent("path_traversal_attempt", "rejected", map[string]interface{}{"n": i})
	}
	if err := al.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	execOnTrail(t, dbPath, "UPDATE audit_events SET id = 99 WHERE id = 2")
	execOnTrail(t, dbPath, "UPDATE audit_events SET id = 2 WHERE id = 3")
	execOnTrail(t, dbPath, "UPDATE audit_events SET id = 3 WHERE id = 99")

	if err := al.VerifyAuditChain(); err == nil {
		t.Error("VerifyAuditChain accepted a reordered trail")
	}
}

// TestAuditChain_SurvivesReopen: the chain continues across process restarts,
// which is what makes the unified system database usable at all.
func TestAuditChain_SurvivesReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "audit.db")

	first := newChainLogger(t, dbPath)
	writeTrail(t, first, 3)
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second := newChainLogger(t, dbPath)
	defer func() { _ = second.Close() }()
	writeTrail(t, second, 3)

	if err := second.VerifyAuditChain(); err != nil {
		t.Errorf("VerifyAuditChain across a reopen = %v, want nil", err)
	}

	events, err := second.Query(AuditEventFilter{})
	if err != nil {
		t.Errorf("Query = %v, want nil", err)
	}
	if len(events) != 6 {
		t.Errorf("got %d events, want 6", len(events))
	}
}

// TestAuditChain_FieldTamperStillCaught: the chain must not weaken the
// per-record protection. Query verifies each returned record on its own, so a
// rewritten field is reported even on a filtered subset.
func TestAuditChain_FieldTamperStillCaught(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "audit.db")
	al := newChainLogger(t, dbPath)
	defer func() { _ = al.Close() }()

	al.LogFileWatch("watch_start", "/etc/shadow")
	if err := al.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	execOnTrail(t, dbPath, "UPDATE audit_events SET file_path = '/tmp/harmless.json'")

	if _, err := al.Query(AuditEventFilter{}); err == nil {
		t.Error("Query accepted a record whose file_path was rewritten")
	}
}
