// audit_query_fuzz_test.go: Fuzz harness for AuditLogger.Query.
//
// Invariants asserted on every corpus entry:
//  1. Query never panics.
//  2. Any error returned is a typed go-errors error (errors.ErrorCoder).
//  3. The audit_events table still has its original row count after the call
//     — i.e., a successful SQL injection cannot delete or corrupt the corpus.
//
// Run:
//
//	go test -fuzz=FuzzQuery_Filter -fuzztime=5m
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/agilira/go-errors"
)

// buildPopulatedLogger creates an AuditLogger backed by a fresh SQLite DB with
// n pre-seeded events. tb may be *testing.T or *testing.F.
// The caller is responsible for defer al.Close().
func buildPopulatedLogger(tb testing.TB, n int) *AuditLogger {
	tb.Helper()
	dbPath := filepath.Join(tb.TempDir(), "fuzz.db")
	config := AuditConfig{
		Enabled:       true,
		OutputFile:    dbPath,
		BufferSize:    n + 10,
		FlushInterval: 0,
	}
	al, err := NewAuditLogger(config)
	if err != nil {
		tb.Fatalf("buildPopulatedLogger: %v", err)
	}

	base := time.Now().UTC()
	events := make([]AuditEvent, n)
	for i := range events {
		ev := AuditEvent{
			Timestamp:   base.Add(time.Duration(i) * time.Millisecond),
			Level:       AuditInfo,
			Event:       fmt.Sprintf("seed.event.%d", i),
			Component:   "fuzz-seed",
			ProcessID:   1,
			ProcessName: "fuzz",
		}
		ev.Checksum = al.generateChecksum(ev)
		events[i] = ev
	}
	if err := al.backend.Write(events); err != nil {
		tb.Fatalf("buildPopulatedLogger write: %v", err)
	}
	return al
}

// FuzzQuery_Filter exercises the Query API with adversarial EventPrefix and
// Component values. Seeds cover SQL metacharacters, control characters, and
// overlong inputs. The corpus is pre-populated so every fuzz call has real data
// to query against.
func FuzzQuery_Filter(f *testing.F) {
	// ── seeds ────────────────────────────────────────────────────────────────
	// SQL injection attempts.
	f.Add("'; DROP TABLE audit_events --", "secrets")
	f.Add("' OR '1'='1", "")
	f.Add("' UNION SELECT * FROM audit_events --", "comp")
	// SQL LIKE metacharacters.
	f.Add("%", "")
	f.Add("_", "")
	f.Add("%_%", "")
	f.Add("\\", "")
	// Control / null characters.
	f.Add("\x00null", "comp")
	f.Add("prefix\x00suffix", "")
	f.Add("\x1f\x7f\xff", "")
	// Overlong inputs (> 4 kB to probe buffer-sizing paths).
	f.Add(string(make([]byte, 4096)), "")
	f.Add("", string(make([]byte, 4096)))
	// Valid prefix matching the seed data.
	f.Add("seed.", "")
	f.Add("seed.event", "fuzz-seed")
	f.Add("", "fuzz-seed")
	f.Add("", "")

	// ── shared corpus DB (created once, reused across all fuzz calls) ────────
	al := buildPopulatedLogger(f, 100)
	defer func() {
		if err := al.Close(); err != nil {
			// f.Log is not available outside f.Fuzz; use fmt.
			_ = err
		}
	}()

	// ── fuzz target ──────────────────────────────────────────────────────────
	f.Fuzz(func(t *testing.T, prefix, component string) {
		// Invariant 1: must never panic.
		events, err := al.Query(AuditEventFilter{
			EventPrefix: prefix,
			Component:   component,
			Limit:       100,
		})

		// Invariant 2: any error must be a typed go-errors error.
		if err != nil {
			if _, ok := err.(errors.ErrorCoder); !ok {
				t.Fatalf("Query returned non-typed error: %T %v", err, err)
			}
			// ErrAuditChainBroken is acceptable (seed data is always valid,
			// but chain errors from concurrent writes are theoretically possible).
		}

		// When no error: result must be non-nil.
		if err == nil && events == nil {
			t.Fatal("Query returned nil slice without error")
		}

		// Invariant 3: the DB must still have its 100 seed rows.
		// A successful SQL injection that deleted rows would violate this.
		stats, statsErr := al.GetStats()
		if statsErr != nil {
			t.Fatalf("GetStats failed after query: %v", statsErr)
		}
		if stats.TotalEvents < 100 {
			t.Fatalf("DB was mutated by fuzz input: %d events remain (expected ≥ 100)",
				stats.TotalEvents)
		}
	})
}
