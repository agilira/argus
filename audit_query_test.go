// audit_query_test.go: Tests for AuditLogger.Query — happy path and chain integrity.
//
// Strategy:
//   - Table-driven where multiple cases share the same shape.
//   - t.Parallel() on every leaf test that is safe to parallelise.
//   - Direct backend writes (bypassing the buffer) for timestamp-sensitive tests
//     so we control the exact values stored in the DB.
//   - Black-box error assertions via errors.ErrorCoder interface.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/agilira/go-errors"
	_ "github.com/mattn/go-sqlite3"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// newQueryTestLogger creates an AuditLogger backed by a fresh temp SQLite DB.
// FlushInterval is 0 so no background goroutine runs during tests.
// The caller is responsible for defer al.Close().
func newQueryTestLogger(t *testing.T) *AuditLogger {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "query_test.db")
	config := AuditConfig{
		Enabled:       true,
		OutputFile:    dbPath,
		BufferSize:    10000,
		FlushInterval: 0,
	}
	al, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("newQueryTestLogger: %v", err)
	}
	return al
}

// writeEventAt writes a single AuditEvent with the given timestamp directly to
// the backend, bypassing the buffer. This lets tests control exact timestamps.
func writeEventAt(t *testing.T, al *AuditLogger, ts time.Time, event, component string, level AuditLevel) {
	t.Helper()
	ev := AuditEvent{
		Timestamp:   ts,
		Level:       level,
		Event:       event,
		Component:   component,
		ProcessID:   1,
		ProcessName: "test",
	}
	ev.Checksum = al.generateChecksum(ev)
	if err := al.backend.Write([]AuditEvent{ev}); err != nil {
		t.Fatalf("writeEventAt: %v", err)
	}
}

// writeBatch writes n events with sequential timestamps to the backend.
// Events are anchored well in the past so the default Until=time.Now() filter
// includes all of them without timing-dependent failures.
func writeBatch(t *testing.T, al *AuditLogger, n int) {
	t.Helper()
	base := time.Now().UTC().Add(-time.Duration(n+1) * time.Millisecond)
	events := make([]AuditEvent, n)
	for i := range events {
		ev := AuditEvent{
			Timestamp:   base.Add(time.Duration(i) * time.Millisecond),
			Level:       AuditInfo,
			Event:       fmt.Sprintf("batch.event.%d", i),
			Component:   "test",
			ProcessID:   1,
			ProcessName: "test",
		}
		ev.Checksum = al.generateChecksum(ev)
		events[i] = ev
	}
	if err := al.backend.Write(events); err != nil {
		t.Fatalf("writeBatch(%d): %v", n, err)
	}
}

// dbPath extracts the SQLite file path from an AuditLogger's backend.
func dbPath(t *testing.T, al *AuditLogger) string {
	t.Helper()
	sb, ok := al.backend.(*sqliteAuditBackend)
	if !ok {
		t.Fatal("dbPath: backend is not sqliteAuditBackend")
	}
	return sb.dbPath
}

// rawDB opens a direct connection to the test DB (for tamper operations).
func rawDB(t *testing.T, al *AuditLogger) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", dbPath(t, al))
	if err != nil {
		t.Fatalf("rawDB open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("rawDB close: %v", err)
		}
	})
	return db
}

// assertErrorCode checks that err is a typed go-errors error with the given code.
func assertErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %q, got nil", want)
	}
	coder, ok := err.(errors.ErrorCoder)
	if !ok {
		t.Fatalf("expected typed error (errors.ErrorCoder), got %T: %v", err, err)
	}
	if got := string(coder.ErrorCode()); got != want {
		t.Fatalf("expected error code %q, got %q (err: %v)", want, got, err)
	}
}

// ── happy-path tests ──────────────────────────────────────────────────────────

func TestQuery_EmptyDB_ReturnsEmptySlice(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	events, err := al.Query(AuditEventFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if events == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestQuery_NeverNilSlice(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	// A filter that matches nothing.
	events, err := al.Query(AuditEventFilter{
		Component:   "this-component-does-not-exist",
		EventPrefix: "no.such.event.",
		Limit:       1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if events == nil {
		t.Fatal("Query must never return nil slice")
	}
}

func TestQuery_SinceFilter(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	base := time.Now().UTC()
	// Log 5 events: oldest at base-4s, newest at base.
	for i := 4; i >= 0; i-- {
		writeEventAt(t, al, base.Add(-time.Duration(i)*time.Second), "e", "c", AuditInfo)
	}

	// Since = base-2.5s → should include base-2s, base-1s, base (3 events).
	events, err := al.Query(AuditEventFilter{Since: base.Add(-2500 * time.Millisecond)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
}

func TestQuery_UntilFilter(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	base := time.Now().UTC()
	for i := 4; i >= 0; i-- {
		writeEventAt(t, al, base.Add(-time.Duration(i)*time.Second), "e", "c", AuditInfo)
	}

	// Until = base-1.5s → should include base-4s, base-3s, base-2s (3 events).
	// (base-1s and base are after the cutoff, so excluded.)
	events, err := al.Query(AuditEventFilter{Until: base.Add(-1500 * time.Millisecond)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
}

func TestQuery_EventPrefixFilter(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	ts := time.Now().UTC()
	writeEventAt(t, al, ts, "secret.get", "c", AuditInfo)
	writeEventAt(t, al, ts.Add(time.Millisecond), "secret.set", "c", AuditInfo)
	writeEventAt(t, al, ts.Add(2*time.Millisecond), "config.change", "c", AuditInfo)

	events, err := al.Query(AuditEventFilter{EventPrefix: "secret."})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events with prefix 'secret.', got %d", len(events))
	}
	for _, ev := range events {
		if len(ev.Event) < 7 || ev.Event[:7] != "secret." {
			t.Errorf("event %q does not start with 'secret.'", ev.Event)
		}
	}
}

func TestQuery_ComponentFilter(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	ts := time.Now().UTC()
	writeEventAt(t, al, ts, "ev", "secrets", AuditInfo)
	writeEventAt(t, al, ts.Add(time.Millisecond), "ev", "secrets", AuditInfo)
	writeEventAt(t, al, ts.Add(2*time.Millisecond), "ev", "config", AuditInfo)

	events, err := al.Query(AuditEventFilter{Component: "secrets"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events from 'secrets', got %d", len(events))
	}
	for _, ev := range events {
		if ev.Component != "secrets" {
			t.Errorf("component = %q, want 'secrets'", ev.Component)
		}
	}
}

func TestQuery_LevelFilter(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	ts := time.Now().UTC()
	writeEventAt(t, al, ts, "info.ev", "c", AuditInfo)
	writeEventAt(t, al, ts.Add(time.Millisecond), "warn.ev", "c", AuditWarn)
	writeEventAt(t, al, ts.Add(2*time.Millisecond), "crit.ev", "c", AuditCritical)

	events, err := al.Query(AuditEventFilter{Level: AuditWarn})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events at >= Warn, got %d", len(events))
	}
	for _, ev := range events {
		if ev.Level < AuditWarn {
			t.Errorf("event %q has level %v, below Warn", ev.Event, ev.Level)
		}
	}
}

func TestQuery_LimitHonoured(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	writeBatch(t, al, 50)

	events, err := al.Query(AuditEventFilter{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 10 {
		t.Fatalf("expected 10 events (Limit=10), got %d", len(events))
	}
}

func TestQuery_DefaultLimitApplied(t *testing.T) {
	// Not parallel: writes 11 000 events, keep isolated.
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	writeBatch(t, al, DefaultQueryLimit+1000)

	events, err := al.Query(AuditEventFilter{Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != DefaultQueryLimit {
		t.Fatalf("expected DefaultQueryLimit (%d) events, got %d", DefaultQueryLimit, len(events))
	}
}

func TestQuery_NewestFirst(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	base := time.Now().UTC()
	writeEventAt(t, al, base.Add(-2*time.Second), "e1", "c", AuditInfo)
	writeEventAt(t, al, base.Add(-1*time.Second), "e2", "c", AuditInfo)
	writeEventAt(t, al, base, "e3", "c", AuditInfo)

	events, err := al.Query(AuditEventFilter{Limit: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if !events[0].Timestamp.After(events[1].Timestamp) {
		t.Errorf("result[0].Timestamp %v not after result[1].Timestamp %v", events[0].Timestamp, events[1].Timestamp)
	}
	if !events[1].Timestamp.After(events[2].Timestamp) {
		t.Errorf("result[1].Timestamp %v not after result[2].Timestamp %v", events[1].Timestamp, events[2].Timestamp)
	}
}

// ── chain-integrity tests ─────────────────────────────────────────────────────

func TestQuery_DetectsTamperedChecksum(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	base := time.Now().UTC()
	for i := 0; i < 5; i++ {
		writeEventAt(t, al, base.Add(time.Duration(i)*time.Millisecond),
			fmt.Sprintf("event.%d", i), "c", AuditInfo)
	}

	// Mutate the checksum of event #2 (second-oldest, ID=2).
	// In newest-first order it sits at index 3.
	db := rawDB(t, al)
	_, err := db.Exec(`UPDATE audit_events SET checksum = 'tampered' WHERE id = 2`)
	if err != nil {
		t.Fatalf("raw UPDATE failed: %v", err)
	}

	events, err := al.Query(AuditEventFilter{})
	if len(events) != 5 {
		t.Fatalf("expected 5 events (full view up to break), got %d", len(events))
	}
	assertErrorCode(t, err, ErrCodeAuditChainBroken)
}

func TestQuery_DetectsMutatedPayload(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	writeEventAt(t, al, time.Now().UTC(), "original.event", "c", AuditInfo)

	// Mutate the event field without updating the checksum.
	db := rawDB(t, al)
	_, err := db.Exec(`UPDATE audit_events SET event = 'tampered.event'`)
	if err != nil {
		t.Fatalf("raw UPDATE failed: %v", err)
	}

	events, err := al.Query(AuditEventFilter{})
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	assertErrorCode(t, err, ErrCodeAuditChainBroken)
}

func TestQuery_ConcurrentReads(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	writeBatch(t, al, 100)

	const goroutines = 10
	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			events, err := al.Query(AuditEventFilter{Limit: 20})
			if err != nil {
				errs[idx] = err
				return
			}
			if len(events) != 20 {
				errs[idx] = fmt.Errorf("goroutine %d: expected 20 events, got %d", idx, len(events))
			}
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}

// ── JSONL backend guard ───────────────────────────────────────────────────────

func TestQuery_JSONLBackend_ReturnsUnsupportedError(t *testing.T) {
	t.Parallel()
	tmpFile := filepath.Join(t.TempDir(), "audit.jsonl")
	config := AuditConfig{
		Enabled:       true,
		OutputFile:    tmpFile,
		BufferSize:    10,
		FlushInterval: 0,
	}
	al, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	_, err = al.Query(AuditEventFilter{})
	assertErrorCode(t, err, ErrCodeAuditBackendUnsupported)
}

// ── filter normalisation edge cases ──────────────────────────────────────────

func TestQuery_ZeroFilterMatchesAll(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	writeBatch(t, al, 5)

	events, err := al.Query(AuditEventFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("zero filter should match all 5 events, got %d", len(events))
	}
}

func TestQuery_LikeMetacharactersInPrefixMatchedLiterally(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	ts := time.Now().UTC()
	// A real event whose name contains a literal '%'.
	writeEventAt(t, al, ts, "ev%special", "c", AuditInfo)
	// A regular event that should NOT match.
	writeEventAt(t, al, ts.Add(time.Millisecond), "ev.other", "c", AuditInfo)

	// Query with EventPrefix = "ev%" — the % is a literal character, not a wildcard.
	events, err := al.Query(AuditEventFilter{EventPrefix: "ev%"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only "ev%special" starts with the literal "ev%".
	if len(events) != 1 {
		t.Fatalf("expected 1 event matching literal prefix 'ev%%', got %d", len(events))
	}
	if events[0].Event != "ev%special" {
		t.Errorf("expected event 'ev%%special', got %q", events[0].Event)
	}
}

func TestQuery_UnderscoreInPrefixMatchedLiterally(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	ts := time.Now().UTC()
	writeEventAt(t, al, ts, "ev_special", "c", AuditInfo)
	writeEventAt(t, al, ts.Add(time.Millisecond), "evXspecial", "c", AuditInfo)

	// With literal _ escaping, only "ev_special" must match.
	events, err := al.Query(AuditEventFilter{EventPrefix: "ev_"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event matching literal prefix 'ev_', got %d", len(events))
	}
	if events[0].Event != "ev_special" {
		t.Errorf("expected 'ev_special', got %q", events[0].Event)
	}
}

// TestQuery_RoundtripsContextAndValues exercises the nullable OldValue, NewValue,
// and Context branches in scanAuditRow / unmarshalNullJSON, which are not covered
// by the helper-based tests that write bare events.
func TestQuery_RoundtripsContextAndValues(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	wantOld := map[string]interface{}{"host": "localhost", "port": float64(5432)}
	wantNew := map[string]interface{}{"host": "prod-db.example.com", "port": float64(5432)}
	wantCtx := map[string]interface{}{"operator": "test-user", "reason": "migration"}

	ev := AuditEvent{
		Timestamp:   time.Now().UTC().Add(-time.Second),
		Level:       AuditCritical,
		Event:       "config.db.change",
		Component:   "roundtrip",
		ProcessID:   1,
		ProcessName: "test",
		OldValue:    wantOld,
		NewValue:    wantNew,
		Context:     wantCtx,
	}
	ev.Checksum = al.generateChecksum(ev)
	if err := al.backend.Write([]AuditEvent{ev}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	events, err := al.Query(AuditEventFilter{EventPrefix: "config.db."})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	got := events[0]

	// OldValue
	gotOld, ok := got.OldValue.(map[string]interface{})
	if !ok {
		t.Fatalf("OldValue type = %T, want map[string]interface{}", got.OldValue)
	}
	if gotOld["host"] != wantOld["host"] {
		t.Errorf("OldValue[host] = %v, want %v", gotOld["host"], wantOld["host"])
	}
	if gotOld["port"] != wantOld["port"] {
		t.Errorf("OldValue[port] = %v, want %v", gotOld["port"], wantOld["port"])
	}

	// NewValue
	gotNew, ok := got.NewValue.(map[string]interface{})
	if !ok {
		t.Fatalf("NewValue type = %T, want map[string]interface{}", got.NewValue)
	}
	if gotNew["host"] != wantNew["host"] {
		t.Errorf("NewValue[host] = %v, want %v", gotNew["host"], wantNew["host"])
	}
	if gotNew["port"] != wantNew["port"] {
		t.Errorf("NewValue[port] = %v, want %v", gotNew["port"], wantNew["port"])
	}

	// Context
	if got.Context == nil {
		t.Fatal("Context is nil, want non-nil map")
	}
	if got.Context["operator"] != wantCtx["operator"] {
		t.Errorf("Context[operator] = %v, want %v", got.Context["operator"], wantCtx["operator"])
	}
	if got.Context["reason"] != wantCtx["reason"] {
		t.Errorf("Context[reason] = %v, want %v", got.Context["reason"], wantCtx["reason"])
	}
}

// ── error-path / branch-coverage tests ───────────────────────────────────────

// TestQuery_NilReceiver checks that calling Query on a nil *AuditLogger returns
// a typed error rather than silently succeeding.
func TestQuery_NilReceiver(t *testing.T) {
	t.Parallel()
	var al *AuditLogger
	_, err := al.Query(AuditEventFilter{})
	assertErrorCode(t, err, ErrCodeInvalidConfig)
}

// TestQuery_CorruptOldValueJSON triggers the unmarshalNullJSON error path
// (invalid JSON in the old_value column) and the ErrCodeAuditQueryError wrapping
// in scanRows/queryEvents.
func TestQuery_CorruptOldValueJSON(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	writeEventAt(t, al, time.Now().UTC().Add(-time.Second), "e", "c", AuditInfo)
	db := rawDB(t, al)
	if _, err := db.Exec(`UPDATE audit_events SET old_value = 'not-valid-json'`); err != nil {
		t.Fatalf("raw UPDATE: %v", err)
	}

	_, err := al.Query(AuditEventFilter{})
	assertErrorCode(t, err, ErrCodeAuditQueryError)
}

// TestQuery_CorruptTimestamp triggers the time.Parse error branch in scanAuditRow.
// The corrupt value uses a space separator (SQL datetime format) which passes
// SQLite text comparison with RFC3339 bounds but fails Go's time.Parse.
func TestQuery_CorruptTimestamp(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	writeEventAt(t, al, time.Now().UTC().Add(-time.Second), "e", "c", AuditInfo)
	db := rawDB(t, al)
	// "2020-01-01 00:00:00" passes text comparison with RFC3339 bounds but
	// fails time.Parse(RFC3339Nano, ...) — no T separator, no timezone.
	if _, err := db.Exec(`UPDATE audit_events SET timestamp = '2020-01-01 00:00:00'`); err != nil {
		t.Fatalf("raw UPDATE: %v", err)
	}

	// Set Until far in the future so the corrupt row passes the SQL WHERE filter.
	_, err := al.Query(AuditEventFilter{Until: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)})
	assertErrorCode(t, err, ErrCodeAuditQueryError)
}

// TestQuery_AfterClose verifies that querying a closed SQLite backend returns a
// typed ErrCodeAuditQueryError rather than a raw DB error.
func TestQuery_AfterClose(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	writeBatch(t, al, 3)
	if err := al.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err := al.Query(AuditEventFilter{})
	assertErrorCode(t, err, ErrCodeAuditQueryError)
}

// TestQuery_SecurityLevelRoundtrip ensures AuditSecurity is stored and read back
// correctly, covering the SECURITY branch in parseStoredAuditLevel.
func TestQuery_SecurityLevelRoundtrip(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	writeEventAt(t, al, time.Now().UTC().Add(-time.Second), "sec.event", "c", AuditSecurity)

	events, err := al.Query(AuditEventFilter{Level: AuditSecurity})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 SECURITY event, got %d", len(events))
	}
	if events[0].Level != AuditSecurity {
		t.Errorf("Level = %v, want AuditSecurity", events[0].Level)
	}
}

// TestQuery_GetStats verifies GetStats returns accurate row counts.
func TestQuery_GetStats(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	stats, err := al.GetStats()
	if err != nil {
		t.Fatalf("GetStats on empty DB: %v", err)
	}
	if stats.TotalEvents != 0 {
		t.Errorf("expected 0 events, got %d", stats.TotalEvents)
	}

	writeBatch(t, al, 5)

	stats, err = al.GetStats()
	if err != nil {
		t.Fatalf("GetStats after writes: %v", err)
	}
	if stats.TotalEvents != 5 {
		t.Errorf("expected 5 events, got %d", stats.TotalEvents)
	}
	if stats.NewestEvent == nil {
		t.Error("NewestEvent should be non-nil after writes")
	}
}

// TestQuery_GetStats_NilLogger covers the nil-receiver guard in GetStats.
func TestQuery_GetStats_NilLogger(t *testing.T) {
	t.Parallel()
	var al *AuditLogger
	_, err := al.GetStats()
	assertErrorCode(t, err, ErrCodeInvalidConfig)
}

// TestQuery_CorruptNewValueJSON triggers the unmarshalNullJSON error path for the
// new_value column, covering the NewValue error branch in scanAuditRow.
func TestQuery_CorruptNewValueJSON(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	writeEventAt(t, al, time.Now().UTC().Add(-time.Second), "e", "c", AuditInfo)
	db := rawDB(t, al)
	if _, err := db.Exec(`UPDATE audit_events SET new_value = '{invalid'`); err != nil {
		t.Fatalf("raw UPDATE: %v", err)
	}

	_, err := al.Query(AuditEventFilter{})
	assertErrorCode(t, err, ErrCodeAuditQueryError)
}

// TestQuery_CorruptContextJSON triggers the json.Unmarshal error path for the
// context column in scanAuditRow.
func TestQuery_CorruptContextJSON(t *testing.T) {
	t.Parallel()
	al := newQueryTestLogger(t)
	defer func() {
		if err := al.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	writeEventAt(t, al, time.Now().UTC().Add(-time.Second), "e", "c", AuditInfo)
	db := rawDB(t, al)
	// Set context to invalid JSON so the contextJSON.Valid && non-empty branch
	// proceeds to json.Unmarshal and fails.
	if _, err := db.Exec(`UPDATE audit_events SET context = '{bad'`); err != nil {
		t.Fatalf("raw UPDATE: %v", err)
	}

	_, err := al.Query(AuditEventFilter{})
	assertErrorCode(t, err, ErrCodeAuditQueryError)
}
