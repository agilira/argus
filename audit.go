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
	"fmt"
	"os"
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
	Checksum    string                 `json:"checksum"` // For tamper detection
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

	// Generate tamper-detection checksum
	auditEvent.Checksum = al.generateChecksum(auditEvent)

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

// LogSecurityEvent logs security-related events
func (al *AuditLogger) LogSecurityEvent(event, details string, context map[string]interface{}) {
	al.Log(AuditSecurity, event, "argus", "", nil, nil, context)
}

// Flush immediately writes all buffered events
func (al *AuditLogger) Flush() error {
	al.bufferMu.Lock()
	defer al.bufferMu.Unlock()
	return al.flushBufferUnsafe()
}

// Close gracefully shuts down the audit logger
func (al *AuditLogger) Close() error {
	close(al.stopCh)
	if al.flushTicker != nil {
		al.flushTicker.Stop()
	}

	// Final flush to ensure all events are persisted
	if err := al.Flush(); err != nil {
		return fmt.Errorf("failed to flush audit logger during close: %w", err)
	}

	// Close backend and release resources
	if al.backend != nil {
		if err := al.backend.Close(); err != nil {
			return fmt.Errorf("failed to close audit backend: %w", err)
		}
	}

	return nil
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

// generateChecksum creates a tamper-detection checksum using SHA-256
func (al *AuditLogger) generateChecksum(event AuditEvent) string {
	// Cryptographic hash for tamper detection
	data := fmt.Sprintf("%s:%s:%s:%v:%v",
		event.Timestamp.Format(time.RFC3339Nano),
		event.Event, event.Component, event.OldValue, event.NewValue)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}

// Helper functions
func getProcessName() string {
	return "argus" // Could read from /proc/self/comm
}
