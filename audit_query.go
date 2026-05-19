// audit_query.go: Event retrieval API for the Argus audit trail.
//
// Provides operator-facing read access to the SQLite audit trail written by
// AuditLogger, with SHA-chain integrity verification on every call.
//
// Usage:
//
//	events, err := logger.Query(argus.AuditEventFilter{
//	    Since:       time.Now().Add(-24 * time.Hour),
//	    EventPrefix: "secret.",
//	    Limit:       500,
//	})
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agilira/go-errors"
)

// Error codes for audit query operations.
const (
	// ErrCodeAuditChainBroken indicates SHA-chain integrity failed: a stored
	// checksum does not match the recomputed hash of the event data.
	// Caused by direct DB tampering or silent bit-rot (CWE-345).
	// The error carries the index of the first broken event via
	// WithContext("index", n).
	ErrCodeAuditChainBroken = "ARGUS_AUDIT_CHAIN_BROKEN"

	// ErrCodeAuditBackendUnsupported is returned when Query is called on a
	// backend that does not support event retrieval (e.g. the JSONL backend).
	// Switch to a SQLite-backed AuditLogger to enable Query.
	ErrCodeAuditBackendUnsupported = "ARGUS_AUDIT_BACKEND_UNSUPPORTED"

	// ErrCodeAuditQueryError wraps unexpected database execution failures so
	// callers can distinguish them from business-logic errors.
	ErrCodeAuditQueryError = "ARGUS_AUDIT_QUERY_ERROR"
)

// DefaultQueryLimit is the safety cap applied when AuditEventFilter.Limit is 0.
// 10,000 is generous for interactive inspection and protects against
// memory exhaustion from a poorly-bounded operator query (CWE-770).
const DefaultQueryLimit = 10_000

// AuditEventFilter restricts the events returned by Query.
// The zero value matches every event, subject to DefaultQueryLimit.
type AuditEventFilter struct {
	// Since restricts to events at or after this timestamp (inclusive).
	// Zero value means no lower bound (beginning of the trail).
	Since time.Time

	// Until restricts to events at or before this timestamp (inclusive).
	// Zero value means the current time at query execution.
	Until time.Time

	// EventPrefix matches events whose Event field starts with this string.
	// Empty string matches every event.
	// SQL LIKE metacharacters (%, _) in the prefix are matched literally —
	// they are escaped before binding, never treated as wildcards.
	EventPrefix string

	// Component restricts to events whose Component field equals this string
	// exactly (case-sensitive). Empty string matches every component.
	Component string

	// Level is the minimum severity to include.
	// Zero value defaults to AuditInfo (all severities).
	Level AuditLevel

	// Limit caps the number of events returned.
	// 0 or negative → DefaultQueryLimit.
	Limit int
}

// queryableBackend is satisfied only by backends that implement event retrieval.
// The JSONL backend intentionally does not implement this interface.
type queryableBackend interface {
	queryEvents(filter AuditEventFilter) ([]AuditEvent, error)
}

// GetStats returns aggregate statistics about the active audit backend.
// Useful for health checks and fuzz-test invariant verification.
func (al *AuditLogger) GetStats() (*AuditDatabaseStats, error) {
	if al == nil || al.backend == nil {
		return nil, errors.New(ErrCodeInvalidConfig, "GetStats called on nil AuditLogger")
	}
	return al.backend.GetStats()
}

// Query returns audit events matching filter, ordered newest-first
// (descending timestamp). An empty result is always a non-nil empty slice,
// never nil.
//
// SHA-chain integrity: every returned event's checksum is recomputed via
// generateChecksum and compared to the stored value. On the first mismatch,
// Query returns all events retrieved from the database AND an
// ErrAuditChainBroken error annotated with the index of the first corrupted
// event. Callers receive both "here is all the data" and "trust ends here".
//
// Query never modifies state; concurrent calls are safe.
func (al *AuditLogger) Query(filter AuditEventFilter) ([]AuditEvent, error) {
	if al == nil {
		return nil, errors.New(ErrCodeInvalidConfig, "Query called on nil AuditLogger")
	}
	if al.backend == nil {
		return nil, errors.New(ErrCodeInvalidConfig, "Query called on AuditLogger with nil backend")
	}

	qb, ok := al.backend.(queryableBackend)
	if !ok {
		return nil, errors.New(ErrCodeAuditBackendUnsupported,
			"Query is not supported by the active audit backend; configure a SQLite-backed AuditLogger")
	}

	events, err := qb.queryEvents(filter)
	if err != nil {
		return nil, err
	}

	return al.verifyChain(events)
}

// verifyChain recomputes each event's checksum and returns ErrAuditChainBroken
// at the first mismatch. All events retrieved from the DB are always returned so
// the caller can perform forensic inspection beyond the break point.
func (al *AuditLogger) verifyChain(events []AuditEvent) ([]AuditEvent, error) {
	for i, ev := range events {
		if ev.Checksum != al.generateChecksum(ev) {
			return events, errors.New(ErrCodeAuditChainBroken,
				"audit chain integrity check failed: checksum mismatch").
				WithContext("index", i)
		}
	}
	return events, nil
}

// normalizedFilter holds filter values after zero-value substitution.
type normalizedFilter struct {
	sinceStr  string // RFC3339Nano lower bound
	untilStr  string // RFC3339Nano upper bound
	likeExpr  string // escaped prefix + '%'
	component string
	levelInt  int
	limit     int
}

// normalizeFilter applies safe defaults to zero fields.
func normalizeFilter(f AuditEventFilter) normalizedFilter {
	// Zero Since uses time.Time{} (year 1), which is earlier than any real audit timestamp.
	since := f.Since

	until := f.Until
	if until.IsZero() {
		until = time.Now().UTC()
	}

	limit := f.Limit
	if limit <= 0 {
		limit = DefaultQueryLimit
	}

	return normalizedFilter{
		sinceStr:  since.UTC().Format(time.RFC3339Nano),
		untilStr:  until.UTC().Format(time.RFC3339Nano),
		likeExpr:  escapeLikePrefix(f.EventPrefix) + "%",
		component: f.Component,
		levelInt:  int(f.Level),
		limit:     limit,
	}
}

// escapeLikePrefix escapes SQLite LIKE metacharacters so that the
// operator-supplied string is matched literally (CWE-89 defence-in-depth).
// The ESCAPE '\' clause in querySQL activates these escape sequences.
func escapeLikePrefix(prefix string) string {
	// Escape backslash first to prevent double-escaping of % and _.
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(prefix)
}

// querySQL is the fully parameterised SELECT used by queryEvents.
// Every operator-controlled value is a bind parameter (?); no string
// concatenation occurs (CWE-89). The ESCAPE clause activates literal
// matching for %, _ in EventPrefix.
//
// #nosec G201 -- all operator inputs are bound parameters, never concatenated.
const querySQL = `
SELECT id, timestamp, level, event, component,
       file_path, old_value, new_value,
       process_id, process_name, context, checksum
  FROM audit_events
 WHERE CASE level
           WHEN 'INFO'     THEN 0
           WHEN 'WARN'     THEN 1
           WHEN 'CRITICAL' THEN 2
           WHEN 'SECURITY' THEN 3
           ELSE 0
       END >= ?
   AND timestamp >= ?
   AND timestamp <= ?
   AND event LIKE ? ESCAPE '\'
   AND (? = '' OR component = ?)
 ORDER BY timestamp DESC
 LIMIT ?`

// queryEvents implements queryableBackend for the SQLite backend.
// All operator inputs are bound as parameters; none are interpolated into SQL.
func (s *sqliteAuditBackend) queryEvents(filter AuditEventFilter) ([]AuditEvent, error) {
	s.mu.RLock()
	closed := s.closed
	s.mu.RUnlock()

	if closed {
		return nil, errors.New(ErrCodeAuditQueryError, "cannot query closed audit backend")
	}

	norm := normalizeFilter(filter)

	// #nosec G201 -- querySQL is a package-level constant; no user input is concatenated.
	rows, err := s.db.Query(querySQL,
		norm.levelInt,
		norm.sinceStr,
		norm.untilStr,
		norm.likeExpr,
		norm.component, // used in: ? = ''
		norm.component, // used in: component = ?
		norm.limit,
	)
	if err != nil {
		// Typed error without echoing raw DB internals (CWE-209).
		return nil, errors.New(ErrCodeAuditQueryError, "audit query execution failed")
	}
	defer func() {
		_ = rows.Close()
	}()

	events, scanErr := scanRows(rows)
	if scanErr != nil {
		// Typed error without echoing raw scan internals (CWE-209).
		return nil, errors.New(ErrCodeAuditQueryError, "failed to scan audit events")
	}
	return events, nil
}

// scanRows reads all result rows into a slice of AuditEvent.
func scanRows(rows *sql.Rows) ([]AuditEvent, error) {
	events := make([]AuditEvent, 0)
	for rows.Next() {
		ev, err := scanAuditRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit row: %w", err)
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit query iteration error: %w", err)
	}
	return events, nil
}

// scanAuditRow scans one result row into an AuditEvent, deserialising JSON
// columns using the same marshaller as the write path (insertEvent).
func scanAuditRow(rows *sql.Rows) (AuditEvent, error) {
	var (
		id           int64
		tsStr        string
		levelStr     string
		event        string
		component    string
		filePath     sql.NullString
		oldValueJSON sql.NullString
		newValueJSON sql.NullString
		processID    int
		processName  string
		contextJSON  sql.NullString
		checksum     sql.NullString
	)

	if err := rows.Scan(
		&id, &tsStr, &levelStr, &event, &component,
		&filePath, &oldValueJSON, &newValueJSON,
		&processID, &processName, &contextJSON, &checksum,
	); err != nil {
		return AuditEvent{}, err
	}

	ts, err := time.Parse(time.RFC3339Nano, tsStr)
	if err != nil {
		return AuditEvent{}, fmt.Errorf("invalid timestamp %q: %w", tsStr, err)
	}

	ev := AuditEvent{
		Timestamp:   ts,
		Level:       parseStoredAuditLevel(levelStr),
		Event:       event,
		Component:   component,
		FilePath:    filePath.String,
		ProcessID:   processID,
		ProcessName: processName,
		Checksum:    checksum.String,
	}

	if err := unmarshalNullJSON(oldValueJSON, &ev.OldValue); err != nil {
		return AuditEvent{}, fmt.Errorf("failed to deserialise old_value: %w", err)
	}
	if err := unmarshalNullJSON(newValueJSON, &ev.NewValue); err != nil {
		return AuditEvent{}, fmt.Errorf("failed to deserialise new_value: %w", err)
	}
	if contextJSON.Valid && contextJSON.String != "" {
		var ctx map[string]interface{}
		if err := json.Unmarshal([]byte(contextJSON.String), &ctx); err != nil {
			return AuditEvent{}, fmt.Errorf("failed to deserialise context: %w", err)
		}
		ev.Context = ctx
	}

	return ev, nil
}

// unmarshalNullJSON deserialises a nullable JSON column into dst.
// An absent or empty column leaves dst unchanged (nil).
func unmarshalNullJSON(ns sql.NullString, dst *interface{}) error {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal([]byte(ns.String), &v); err != nil {
		return err
	}
	*dst = v
	return nil
}

// parseStoredAuditLevel converts the TEXT level written by insertEvent back to
// AuditLevel. Unknown values default to AuditInfo (safe floor).
func parseStoredAuditLevel(s string) AuditLevel {
	switch s {
	case "WARN":
		return AuditWarn
	case "CRITICAL":
		return AuditCritical
	case "SECURITY":
		return AuditSecurity
	default:
		return AuditInfo
	}
}
