// audit_query_security_test.go: Security tests for AuditLogger.Query.
//
// Covers:
//   - SQL injection via EventPrefix and Component fields (CWE-89).
//   - LIKE metacharacter escaping (%, _).
//   - Null-byte handling in filter fields.
//   - SHA-chain tamper detection framed as an attack scenario.
//   - Error messages must not leak SQL statements or parameter values (CWE-209).
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agilira/go-errors"
)

// ── helpers (shared with audit_query_test.go within the same package) ─────────

// seedDB writes n events and returns the populated logger.
// The caller owns the Close() call.
func seedDB(t *testing.T, n int) *AuditLogger {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "sec_test.db")
	config := AuditConfig{
		Enabled:       true,
		OutputFile:    dbPath,
		BufferSize:    n + 10,
		FlushInterval: 0,
	}
	al, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("seedDB: %v", err)
	}
	writeBatch(t, al, n)
	return al
}

// tableExists returns true if the named table is present in the DB.
func tableExists(t *testing.T, al *AuditLogger, table string) bool {
	t.Helper()
	db := rawDB(t, al)
	var name string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
	).Scan(&name)
	return err == nil && name == table
}

// rowCount returns the number of rows in the named table.
func rowCount(t *testing.T, al *AuditLogger, table string) int64 {
	t.Helper()
	db := rawDB(t, al)
	var count int64
	if err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count); err != nil {
		t.Fatalf("rowCount(%s): %v", table, err)
	}
	return count
}

// ── SQL injection tests ───────────────────────────────────────────────────────

func TestSec_SQLInjection_PrefixField(t *testing.T) {
	t.Parallel()
	al := seedDB(t, 5)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	// Classic SQL injection attempt via EventPrefix.
	_, err := al.Query(AuditEventFilter{
		EventPrefix: "'; DROP TABLE audit_events --",
		Limit:       10,
	})
	// Query may succeed (0 results) or return a typed error.
	if err != nil {
		coder, ok := err.(errors.ErrorCoder)
		if !ok {
			t.Fatalf("non-typed error from Query with injection prefix: %v", err)
		}
		_ = coder // typed error is acceptable
	}

	// The audit_events table must still exist with all rows intact.
	if !tableExists(t, al, "audit_events") {
		t.Fatal("SQL injection deleted the audit_events table via EventPrefix")
	}
	if got := rowCount(t, al, "audit_events"); got < 5 {
		t.Fatalf("SQL injection reduced row count to %d (expected ≥ 5)", got)
	}
}

func TestSec_SQLInjection_ComponentField(t *testing.T) {
	t.Parallel()
	al := seedDB(t, 5)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	_, err := al.Query(AuditEventFilter{
		Component: "'; DROP TABLE audit_events --",
		Limit:     10,
	})
	if err != nil {
		if _, ok := err.(errors.ErrorCoder); !ok {
			t.Fatalf("non-typed error from Query with injection component: %v", err)
		}
	}

	if !tableExists(t, al, "audit_events") {
		t.Fatal("SQL injection deleted the audit_events table via Component")
	}
	if got := rowCount(t, al, "audit_events"); got < 5 {
		t.Fatalf("SQL injection reduced row count to %d (expected ≥ 5)", got)
	}
}

// TestSec_LikeMetacharacters_Escaped verifies that % and _ in EventPrefix are
// treated as literal characters, not SQL LIKE wildcards.
func TestSec_LikeMetacharacters_Escaped(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	// Anchor events in the past so that the default Until=time.Now() bound in
	// each Query call always includes all four events, even on platforms with
	// coarse clock resolution (e.g. macOS CI runners).
	ts := time.Now().UTC().Add(-time.Second)
	// Store one event whose name literally contains %, one with _, and decoys.
	writeEventAt(t, al, ts, "ev%literal", "c", AuditInfo)
	writeEventAt(t, al, ts.Add(time.Millisecond), "ev_literal", "c", AuditInfo)
	writeEventAt(t, al, ts.Add(2*time.Millisecond), "evXliteral", "c", AuditInfo)
	writeEventAt(t, al, ts.Add(3*time.Millisecond), "evYliteral", "c", AuditInfo)

	// "%" as a wildcard would match all four; as a literal it matches only "ev%literal".
	eventsPercent, err := al.Query(AuditEventFilter{EventPrefix: "ev%"})
	if err != nil {
		t.Fatalf("unexpected error for prefix 'ev%%': %v", err)
	}
	if len(eventsPercent) != 1 {
		t.Fatalf("prefix 'ev%%' (literal): expected 1 match, got %d", len(eventsPercent))
	}

	// "_" as a wildcard would match "evXliteral" and "evYliteral"; literal → "ev_literal".
	eventsUnderscore, err := al.Query(AuditEventFilter{EventPrefix: "ev_"})
	if err != nil {
		t.Fatalf("unexpected error for prefix 'ev_': %v", err)
	}
	if len(eventsUnderscore) != 1 {
		t.Fatalf("prefix 'ev_' (literal): expected 1 match, got %d", len(eventsUnderscore))
	}
}

// TestSec_NullByteInFilter verifies Query never panics and returns a valid
// result or a typed error when filter fields contain null bytes.
func TestSec_NullByteInFilter(t *testing.T) {
	t.Parallel()
	al := seedDB(t, 3)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	prefixWithNull := "prefix\x00rest"
	compWithNull := "comp\x00onent"

	// Must not panic; may return empty results or a typed error.
	events, err := al.Query(AuditEventFilter{
		EventPrefix: prefixWithNull,
		Component:   compWithNull,
		Limit:       10,
	})
	if err != nil {
		if _, ok := err.(errors.ErrorCoder); !ok {
			t.Fatalf("null byte caused non-typed error: %v", err)
		}
		// Typed error is acceptable.
		return
	}
	// If no error: result must still be a non-nil slice.
	if events == nil {
		t.Fatal("nil slice returned for null-byte filter (must be non-nil)")
	}
}

// TestSec_ChainTamper_Detected frames a checksum mutation as an adversarial
// attack: an attacker with SQL access mutates a checksum; Query must detect it.
func TestSec_ChainTamper_Detected(t *testing.T) {
	t.Parallel()
	al := seedDB(t, 5)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	// Attacker silently modifies the stored checksum of the first row.
	db := rawDB(t, al)
	_, err := db.Exec(`UPDATE audit_events SET checksum = 'attacker_controlled' WHERE id = 1`)
	if err != nil {
		t.Fatalf("raw UPDATE: %v", err)
	}

	_, err = al.Query(AuditEventFilter{})
	assertErrorCode(t, err, ErrCodeAuditChainBroken)
}

// TestSec_ErrorsDoNotLeakSQL ensures that error messages returned by Query do
// not expose the internal SQL statement text or raw bound parameter values
// (CWE-209 — Insertion of Sensitive Information into Error Message).
func TestSec_ErrorsDoNotLeakSQL(t *testing.T) {
	t.Parallel()

	sensitiveInputs := []struct {
		name      string
		prefix    string
		component string
	}{
		{"drop_table", "'; DROP TABLE audit_events --", ""},
		{"union_select", "' UNION SELECT * FROM audit_events --", ""},
		{"percent", "%", ""},
		{"underscore", "_", ""},
		{"null_byte", "prefix\x00", ""},
		{"component_injection", "", "'; DROP TABLE audit_events --"},
	}

	for _, tc := range sensitiveInputs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			al := seedDB(t, 3)
			defer func() {
				if err := al.Close(); err != nil {
					t.Errorf("Close: %v", err)
				}
			}()

			_, err := al.Query(AuditEventFilter{
				EventPrefix: tc.prefix,
				Component:   tc.component,
				Limit:       5,
			})
			if err == nil {
				// No error → nothing to leak.
				return
			}

			errStr := err.Error()

			// The raw SQL constant must not appear in the error message.
			sqlFragments := []string{
				"SELECT id, timestamp",
				"FROM audit_events",
				"LIKE ? ESCAPE",
				"ORDER BY timestamp DESC",
			}
			for _, frag := range sqlFragments {
				if strings.Contains(errStr, frag) {
					t.Errorf("error message leaks SQL fragment %q: %q", frag, errStr)
				}
			}

			// The raw user-supplied values must not appear verbatim (CWE-209).
			if tc.prefix != "" && strings.Contains(errStr, tc.prefix) {
				t.Errorf("error message leaks EventPrefix value %q: %q", tc.prefix, errStr)
			}
			if tc.component != "" && strings.Contains(errStr, tc.component) {
				t.Errorf("error message leaks Component value %q: %q", tc.component, errStr)
			}
		})
	}
}
