// audit.go: Comprehensive audit trail system for Argus
//
// This provides security audit logging for all configuration changes,
// ensuring full accountability and traceability in production environments.
//
// Features:
// - Immutable audit logs with tamper detection
// - Structured logging with context
// - Performance optimized (sub-microsecond impact)
// - Configurable audit levels and outputs
//
// Copyright (c) 2025 AGILira
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/agilira/go-timecache"
)

// AuditLevel represents the severity of audit events
type AuditLevel int

const (
	AuditInfo AuditLevel = iota
	AuditWarn
	AuditCritical
	AuditSecurity
)

func (al AuditLevel) String() string {
	switch al {
	case AuditInfo:
		return "INFO"
	case AuditWarn:
		return "WARN"
	case AuditCritical:
		return "CRITICAL"
	case AuditSecurity:
		return "SECURITY"
	default:
		return "UNKNOWN"
	}
}

// AuditEvent represents a single auditable event
type AuditEvent struct {
	Timestamp   time.Time              `json:"timestamp"`
	Level       AuditLevel             `json:"level"`
	Event       string                 `json:"event"`
	Component   string                 `json:"component"`
	FilePath    string                 `json:"file_path,omitempty"`
	OldValue    interface{}            `json:"old_value,omitempty"`
	NewValue    interface{}            `json:"new_value,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	ProcessID   int                    `json:"process_id"`
	ProcessName string                 `json:"process_name"`
	Context     map[string]interface{} `json:"context,omitempty"`

	// PrevChecksum is the Checksum of the record that precedes this one in the
	// trail, empty for the first record of a chain. Storage backends that keep
	// no ordering (JSONL) leave it empty on every record.
	PrevChecksum string `json:"prev_checksum,omitempty"`

	// Checksum links this record to the one before it:
	// SHA-256 over PrevChecksum and the digest of this record's fields.
	// Verifying a record proves its fields are intact; verifying that each
	// record's PrevChecksum equals its predecessor's Checksum proves no record
	// was removed or moved (see AuditLogger.VerifyAuditChain).
	Checksum string `json:"checksum"`
}

// AuditConfig configures the audit system
type AuditConfig struct {
	Enabled       bool          `json:"enabled"`
	OutputFile    string        `json:"output_file"`
	MinLevel      AuditLevel    `json:"min_level"`
	BufferSize    int           `json:"buffer_size"`
	FlushInterval time.Duration `json:"flush_interval"`
	IncludeStack  bool          `json:"include_stack"`
}

// DefaultAuditConfig returns secure default audit configuration with unified SQLite storage.
//
// The default configuration uses the unified SQLite audit system, which consolidates
// all Argus audit events into a single system-wide database. This provides:
//   - Cross-component event correlation
//   - Efficient storage and querying
//   - Automatic schema management
//   - WAL mode for concurrent access
//
// For applications requiring JSONL format, specify OutputFile with .jsonl extension.
func DefaultAuditConfig() AuditConfig {
	// Use empty OutputFile to trigger unified SQLite backend selection
	// The backend will automatically use the system audit database path
	return AuditConfig{
		Enabled:       true,
		OutputFile:    "", // Empty triggers unified SQLite backend
		MinLevel:      AuditInfo,
		BufferSize:    1000,
		FlushInterval: 5 * time.Second,
		IncludeStack:  false,
	}
}

// AuditLogger provides high-performance audit logging with pluggable backends.
//
// This logger implements a unified audit system that automatically selects
// the optimal storage backend (SQLite for unified system audit, JSONL for
// backward compatibility) while maintaining the same public API.
//
// The logger uses buffering and background flushing for optimal performance
// in high-throughput scenarios while ensuring audit integrity.
type AuditLogger struct {
	config      AuditConfig
	backend     auditBackend // Pluggable storage backend (SQLite or JSONL)
	buffer      []AuditEvent
	bufferMu    sync.Mutex
	flushTicker *time.Ticker
	stopCh      chan struct{}
	closeOnce   sync.Once
	processID   int
	processName string
}

// NewAuditLogger creates a new audit logger with automatic backend selection.
//
// The logger automatically selects the optimal audit backend based on system
// capabilities and configuration:
//   - SQLite unified backend for consolidation (preferred)
//   - JSONL fallback for compatibility
//
// This approach ensures seamless migration to unified audit trails while
// maintaining backward compatibility with existing configurations.
//
// Parameters:
//   - config: Audit configuration specifying behavior and output preferences
//
// Returns:
//   - Configured audit logger ready for use
//   - Error if both backend initialization attempts fail
func NewAuditLogger(config AuditConfig) (*AuditLogger, error) {
	// Initialize backend using automatic selection
	backend, err := createAuditBackend(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize audit backend: %w", err)
	}

	logger := &AuditLogger{
		config:      config,
		backend:     backend,
		buffer:      make([]AuditEvent, 0, config.BufferSize),
		stopCh:      make(chan struct{}),
		processID:   os.Getpid(),
		processName: getProcessName(),
	}

	// Start background flusher
	if config.FlushInterval > 0 {
		logger.flushTicker = time.NewTicker(config.FlushInterval)
		go logger.flushLoop()
	}

	return logger, nil
}

// newDisabledAuditLogger returns an inert AuditLogger: no storage backend and
// no flush goroutine. Every method stays safe on it — Log returns early on the
// nil backend (buffering nothing), Flush sees an empty buffer, GetStats/Query
// return a typed error, and Close is a no-op beyond closing stopCh.
//
// WHY a dedicated constructor instead of NewAuditLogger(AuditConfig{Enabled:false}):
// NewAuditLogger always calls createAuditBackend, which opens the unified SQLite
// system database regardless of Enabled. That is exactly the resource we want
// to avoid when a host opts out via Config.DisableAudit. This constructor opens
// nothing.
func newDisabledAuditLogger() *AuditLogger {
	return &AuditLogger{
		config:      AuditConfig{Enabled: false},
		backend:     nil,
		stopCh:      make(chan struct{}),
		processID:   os.Getpid(),
		processName: getProcessName(),
	}
}

// Log records an audit event with ultra-high performance
func (al *AuditLogger) Log(level AuditLevel, event, component, filePath string, oldVal, newVal interface{}, context map[string]interface{}) {
	if al == nil || al.backend == nil || !al.config.Enabled || level < al.config.MinLevel {
		return
	}

	// Use cached timestamp for performance (121x faster than time.Now())
	timestamp := timecache.CachedTime()

	auditEvent := AuditEvent{
		Timestamp:   timestamp,
		Level:       level,
		Event:       event,
		Component:   component,
		FilePath:    filePath,
		OldValue:    oldVal,
		NewValue:    newVal,
		ProcessID:   al.processID,
		ProcessName: al.processName,
		Context:     context,
	}

	// Link the record to nothing yet: a storage backend that keeps an ordering
	// re-links it to the real tail inside the same transaction as the insert,
	// which is the only place the position is known and cannot race.
	auditEvent.Checksum = chainChecksum("", recordDigest(auditEvent))

	// Buffer the event
	al.bufferMu.Lock()
	al.buffer = append(al.buffer, auditEvent)
	if len(al.buffer) >= al.config.BufferSize {
		_ = al.flushBufferUnsafe() // Ignore flush errors during buffering to maintain performance
	}
	al.bufferMu.Unlock()
}

// LogConfigChange logs configuration file changes (most common use case)
func (al *AuditLogger) LogConfigChange(filePath string, oldConfig, newConfig map[string]interface{}) {
	al.Log(AuditCritical, "config_change", "argus", filePath, oldConfig, newConfig, nil)
}

// LogFileWatch logs file watch events
func (al *AuditLogger) LogFileWatch(event, filePath string) {
	al.Log(AuditInfo, event, "argus", filePath, nil, nil, nil)
}

// LogSecurityEvent logs security-related events.
//
// details is the human-readable description of what was rejected and why. It
// is recorded under the "details" context key: previously the argument was
// accepted and then dropped on the floor, so every security record in the
// trail — "Rejected malicious file path", "Symlink points to dangerous
// target" — reached storage without its explanation.
//
// The caller's context map is never modified.
func (al *AuditLogger) LogSecurityEvent(event, details string, context map[string]interface{}) {
	enriched := make(map[string]interface{}, len(context)+1)
	for k, v := range context {
		enriched[k] = v
	}
	if details != "" {
		enriched["details"] = details
	}
	al.Log(AuditSecurity, event, "argus", "", nil, nil, enriched)
}

// Flush immediately writes all buffered events.
// Like Log, GetStats and Query, Flush tolerates a nil logger so callers do not
// have to guard every call site.
func (al *AuditLogger) Flush() error {
	if al == nil {
		return nil
	}
	al.bufferMu.Lock()
	defer al.bufferMu.Unlock()
	return al.flushBufferUnsafe()
}

// Close gracefully shuts down the audit logger.
//
// Close is idempotent: the shutdown runs once and later calls return nil.
// Watcher.Close and Watcher.Stop both reach this method, and a caller that
// also closes the logger it passed in must not be punished with a panic on
// "close of closed channel".
func (al *AuditLogger) Close() error {
	if al == nil {
		return nil
	}

	var err error
	al.closeOnce.Do(func() {
		close(al.stopCh)
		if al.flushTicker != nil {
			al.flushTicker.Stop()
		}

		// Final flush to ensure all events are persisted
		if flushErr := al.Flush(); flushErr != nil {
			err = fmt.Errorf("failed to flush audit logger during close: %w", flushErr)
			return
		}

		// Close backend and release resources
		if al.backend != nil {
			if closeErr := al.backend.Close(); closeErr != nil {
				err = fmt.Errorf("failed to close audit backend: %w", closeErr)
			}
		}
	})

	return err
}

// flushLoop runs the background flush process
func (al *AuditLogger) flushLoop() {
	for {
		select {
		case <-al.flushTicker.C:
			_ = al.Flush() // Ignore flush errors in background process to maintain performance
		case <-al.stopCh:
			return
		}
	}
}

// flushBufferUnsafe writes buffer to backend storage (caller must hold bufferMu).
//
// This method delegates to the configured backend (SQLite or JSONL) for
// actual persistence. It handles batch writing for optimal performance
// and proper error handling with buffer management.
func (al *AuditLogger) flushBufferUnsafe() error {
	if len(al.buffer) == 0 {
		return nil
	}

	// Write batch to backend
	if err := al.backend.Write(al.buffer); err != nil {
		return fmt.Errorf("failed to write audit events to backend: %w", err)
	}

	// Clear buffer after successful write
	al.buffer = al.buffer[:0]
	return nil
}

// recordDigest hashes the content of one audit record.
//
// COVERAGE: every field a reader acts on is hashed. The earlier formula
// covered only timestamp, event, component and the old/new values, which left
// the three fields that carry the security meaning of a record unprotected:
// FilePath (which file was touched), Level (how serious it was) and Context
// (why it was rejected, and what path was rejected). A record could be
// rewritten from /etc/shadow to /tmp/harmless.json, or downgraded from
// SECURITY to INFO, and still verify (CWE-345).
//
// FIELD SEPARATION: each field is hashed as a length prefix followed by its
// bytes, so no combination of values can be rearranged into the same digest.
// A plain ":"-joined string lets "a:b" and "a" + ":b" collide.
//
// CANONICAL FORM: interface{} fields are hashed as their JSON encoding, which
// is exactly what the backend stores and what the reader parses back.
// encoding/json sorts map keys, so the form is stable, and a value that
// survives the round trip hashes identically on both sides. Formatting them
// with %v instead would have made the hash depend on Go's rendering of types
// that JSON does not preserve.
//
// UTC NORMALISATION: the timestamp is hashed in UTC. It pairs with the
// matching .UTC() at the SQL write site (audit_backend.go) so the hash
// computed on read (any timezone string parsed → UTC) matches the hash
// computed at write. Lexical SQL bounds comparison stays correct because both
// sides are UTC RFC3339Nano.
//
// PrevChecksum is deliberately NOT part of the digest: the digest describes
// the record, chainChecksum describes its place in the trail.
func recordDigest(event AuditEvent) string {
	h := sha256.New()

	hashField(h, event.Timestamp.UTC().Format(time.RFC3339Nano))
	hashField(h, event.Level.String())
	hashField(h, event.Event)
	hashField(h, event.Component)
	hashField(h, event.FilePath)
	hashField(h, event.ProcessName)
	hashField(h, strconv.Itoa(event.ProcessID))
	hashField(h, canonicalJSON(event.OldValue))
	hashField(h, canonicalJSON(event.NewValue))
	hashField(h, canonicalJSON(event.Context))

	return hex.EncodeToString(h.Sum(nil))
}

// chainChecksum links a record digest to the checksum of the record before it.
//
// This is what makes the trail a chain rather than a bag of independently
// hashed rows. With per-record hashes alone, an attacker could DELETE the row
// recording their access, or reorder rows to put an event outside the window
// an investigator is looking at, and every surviving row would still verify —
// the documentation called it a "SHA-chain" while nothing was chained.
//
// prev is empty for the first record of a chain.
func chainChecksum(prev, digest string) string {
	h := sha256.New()
	hashField(h, prev)
	hashField(h, digest)
	return hex.EncodeToString(h.Sum(nil))
}

// generateChecksum recomputes the stored checksum of an event, i.e. the link
// its fields and its recorded position imply. A record whose fields were
// altered, or whose PrevChecksum was rewritten, no longer matches.
func (al *AuditLogger) generateChecksum(event AuditEvent) string {
	return chainChecksum(event.PrevChecksum, recordDigest(event))
}

// hashField feeds one field to the digest, length-prefixed so field boundaries
// cannot be shifted.
func hashField(h hash.Hash, value string) {
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(value)))
	_, _ = h.Write(lenBuf[:])
	_, _ = h.Write([]byte(value))
}

// canonicalJSON renders a value the same way the backend stores it, so the
// checksum computed at write time and the one recomputed after a read agree.
//
// A value that cannot be marshalled also cannot be stored; falling back to %v
// keeps such a record hashable rather than silently hashing it as empty.
func canonicalJSON(value interface{}) string {
	if value == nil {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

// Helper functions
func getProcessName() string {
	return "argus" // Could read from /proc/self/comm
}
