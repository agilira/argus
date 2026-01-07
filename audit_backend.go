// audit_backend.go: Backend interface and implementations for Argus audit system
//
// This file defines the pluggable backend architecture for audit logging,
// supporting multiple storage backends (JSONL, SQLite) with transparent
// migration and unified API.
//
// Features:
// - Backend interface for pluggable audit storage
// - Automatic backend selection based on configuration
// - Comprehensive error handling and recovery
// - Thread-safe operations with proper resource management
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver registration
)

// auditBackend defines the interface for audit storage backends.
//
// This interface abstracts the storage mechanism, allowing transparent
// switching between JSONL files, SQLite databases, or future backends
// without changing the public API.
//
// ═══════════════════════════════════════════════════════════════════════════════
// ENGINEERING NOTE: Why a Backend Interface Instead of Direct SQLite?
// ═══════════════════════════════════════════════════════════════════════════════
// Enterprise audit requirements vary wildly:
//
//  1. SMALL DEPLOYMENTS: JSONL files are perfect - human-readable, grep-able,
//     easily shipped to log aggregators (ELK, Splunk).
//
//  2. MEDIUM DEPLOYMENTS: SQLite provides queryable audit trails without
//     external dependencies. Perfect for single-node apps.
//
//  3. LARGE DEPLOYMENTS: May need PostgreSQL, Elasticsearch, or cloud-native
//     solutions. The interface makes adding these trivial.
//
// The createAuditBackend() function implements graceful degradation:
// SQLite → JSONL → Error. This ensures audit logging NEVER prevents app
// startup, while still capturing data via the fallback mechanism.
//
// The interface is minimal by design: Write, Flush, Close, Maintenance.
// Backends can implement complex logic internally while keeping the contract
// simple. This follows the Interface Segregation Principle.
// ═══════════════════════════════════════════════════════════════════════════════
type auditBackend interface {
	// Write persists a batch of audit events to the backend.
	// Implementations must handle concurrent writes safely.
	Write(events []AuditEvent) error

	// Flush ensures all pending writes are committed to storage.
	// This is called during graceful shutdown and periodic flushes.
	Flush() error

	// Close releases all resources and performs final cleanup.
	// After calling Close, the backend must not be used again.
	Close() error

	// Maintenance performs backend-specific maintenance operations.
	// For SQLite: cleans old entries, optimizes database, updates statistics.
	// For JSONL: archives old files, compresses historical data.
	Maintenance() error

	// GetStats returns statistics about the audit backend.
	// For SQLite: detailed database statistics with event counts and performance metrics.
	// For JSONL: basic file statistics (implementation may return limited data).
	GetStats() (*AuditDatabaseStats, error)
}

// createAuditBackend creates the appropriate audit backend based on configuration.
//
// Backend selection strategy:
//  1. Always attempt SQLite unified backend first (for consolidation)
//  2. Fall back to JSONL if SQLite is unavailable or fails
//  3. Return error only if both backends fail initialization
//
// This ensures maximum compatibility while providing unified audit trails
// when possible.
func createAuditBackend(config AuditConfig) (auditBackend, error) {
	// Check if user explicitly requested JSONL format via .jsonl extension
	if config.OutputFile != "" && filepath.Ext(config.OutputFile) == ".jsonl" {
		return newJSONLBackend(config)
	}

	// For all other cases, try SQLite unified backend first for consolidation
	backend, err := newSQLiteBackend(config)
	if err == nil {
		return backend, nil
	}

	// Fall back to JSONL backend if SQLite fails
	jsonlBackend, jsonlErr := newJSONLBackend(config)
	if jsonlErr != nil {
		return nil, fmt.Errorf("all audit backends failed - SQLite: %w, JSONL: %v", err, jsonlErr)
	}

	return jsonlBackend, nil
}

// getUnifiedAuditPath returns the standard path for the unified SQLite audit database.
//
// The unified database consolidates all Argus audit events from the system
// into a single queryable database, regardless of the original OutputFile
// configuration. This enables cross-component correlation and simplified
// audit management.
func getUnifiedAuditPath() string {
	return filepath.Join(os.TempDir(), "argus", "system-audit.db")
}

// sqliteAuditBackend implements auditBackend using SQLite for unified audit storage.
//
// This backend consolidates all Argus audit events into a single SQLite database
// regardless of the original OutputFile configuration. It tracks the original
// source configuration for backward compatibility and debugging.
type sqliteAuditBackend struct {
	db         *sql.DB
	dbPath     string
	sourceFile string // Original OutputFile for source tracking
	insertStmt *sql.Stmt
	mu         sync.RWMutex
	closed     bool
}

// newSQLiteBackend creates a new SQLite audit backend with unified storage.
//
// This function initializes the SQLite database, creates the schema if needed,
// and prepares statements for efficient batch inserts. The database uses
// WAL mode for concurrent access and optimal performance.
//
// Parameters:
//   - config: AuditConfig containing the original configuration
//
// Returns:
//   - Configured SQLite backend ready for use
//   - Error if database initialization fails
func newSQLiteBackend(config AuditConfig) (*sqliteAuditBackend, error) {
	// Determine and setup database path
	dbPath, err := setupDatabasePath(config)
	if err != nil {
		return nil, err
	}

	// Open and test database connection
	db, err := openSQLiteDatabase(dbPath)
	if err != nil {
		return nil, err
	}

	// Create backend instance
	backend := &sqliteAuditBackend{
		db:         db,
		dbPath:     dbPath,
		sourceFile: config.OutputFile,
	}

	// Initialize backend components
	if err := initializeBackendComponents(backend); err != nil {
		return nil, err
	}

	return backend, nil
}

// setupDatabasePath determines and creates database path
func setupDatabasePath(config AuditConfig) (string, error) {
	// Determine database path - respect OutputFile if specified with .db extension
	var dbPath string
	if config.OutputFile != "" && filepath.Ext(config.OutputFile) == ".db" {
		// Use specified path for database files (useful for tests and custom setups)
		dbPath = config.OutputFile
	} else {
		// Use unified path for consolidation (default behavior)
		dbPath = getUnifiedAuditPath()
	}

	// Ensure directory exists with appropriate permissions
	if err := os.MkdirAll(filepath.Dir(dbPath), 0750); err != nil {
		return "", fmt.Errorf("failed to create audit database directory: %w", err)
	}

	return dbPath, nil
}

// openSQLiteDatabase opens and tests SQLite database connection
//
// ═══════════════════════════════════════════════════════════════════════════════
// ENGINEERING NOTE: SQLite Pragmas for Maximum Performance
// ═══════════════════════════════════════════════════════════════════════════════
// These SQLite pragmas are carefully chosen for audit logging workloads:
//
// 1. _journal_mode=WAL (Write-Ahead Logging):
//   - Readers NEVER block writers, writers NEVER block readers
//   - Critical for audit logging where we write frequently but read rarely
//   - Provides ~10x better write performance than default rollback journal
//   - Crash recovery: WAL is replayed on next open, zero data loss
//
// 2. _busy_timeout=5000:
//   - Wait up to 5 seconds if database is locked by another process
//   - Prevents "database is locked" errors in multi-process deployments
//   - Essential for Kubernetes pods sharing audit storage
//
// 3. _synchronous=NORMAL:
//   - Balance between performance and durability
//   - Syncs at critical moments, not every write
//   - Acceptable for audit logs (we can afford to lose last ~1 second)
//   - FULL would be 3x slower for negligible benefit
//
// 4. _cache_size=1000:
//   - Keep 1000 pages (4MB) in memory
//   - Reduces disk I/O for repeated queries (e.g., audit searches)
//   - Modest memory footprint suitable for containers
//
// ═══════════════════════════════════════════════════════════════════════════════
func openSQLiteDatabase(dbPath string) (*sql.DB, error) {
	// Open SQLite database with optimized settings
	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_cache_size=1000", dbPath))
	if err != nil {
		return nil, fmt.Errorf("failed to open audit database: %w", err)
	}

	// Test database connection
	if err := db.Ping(); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed to ping database (close error: %v): %w", closeErr, err)
		}
		return nil, fmt.Errorf("failed to ping audit database: %w", err)
	}

	return db, nil
}

// initializeBackendComponents initializes schema, statements, and performs maintenance
func initializeBackendComponents(backend *sqliteAuditBackend) error {
	// Initialize database schema
	if err := backend.initializeSchema(); err != nil {
		if closeErr := backend.Close(); closeErr != nil {
			return fmt.Errorf("failed to initialize schema (close error: %v): %w", closeErr, err)
		}
		return fmt.Errorf("failed to initialize audit database schema: %w", err)
	}

	// Prepare insert statement for efficient batch operations
	if err := backend.prepareStatements(); err != nil {
		if closeErr := backend.Close(); closeErr != nil {
			return fmt.Errorf("failed to prepare statements (close error: %v): %w", closeErr, err)
		}
		return fmt.Errorf("failed to prepare audit database statements: %w", err)
	}

	// Perform maintenance on initialization to clean up old entries
	if err := backend.performMaintenance(); err != nil {
		// Log error but don't fail initialization - maintenance is not critical
		// In production, this should be logged to system logger
		_ = err
	}

	return nil
}

// ensureSchemaVersion checks the current schema version and performs migrations if needed.
//
// This function implements forward-compatible schema evolution:
//   - Version 1: Initial schema with basic audit tracking
//   - Version 2: Added indexes and performance optimizations (current)
//   - Future versions: Will add new fields/tables without breaking compatibility
//
// Migration is atomic and safe for concurrent access.
func (s *sqliteAuditBackend) ensureSchemaVersion() error {
	const currentSchemaVersion = 2

	// Create schema_info table if it doesn't exist
	createSchemaInfoSQL := `
	CREATE TABLE IF NOT EXISTS schema_info (
		version INTEGER PRIMARY KEY,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	if _, err := s.db.Exec(createSchemaInfoSQL); err != nil {
		return fmt.Errorf("failed to create schema_info table: %w", err)
	}

	// Check current version
	var version int
	err := s.db.QueryRow("SELECT version FROM schema_info ORDER BY version DESC LIMIT 1").Scan(&version)
	if err != nil {
		if err == sql.ErrNoRows {
			// First time setup
			version = 0
		} else {
			return fmt.Errorf("failed to check schema version: %w", err)
		}
	}

	// Perform migrations if needed
	if version < currentSchemaVersion {
		if err := s.migrateSchema(version, currentSchemaVersion); err != nil {
			return fmt.Errorf("schema migration from v%d to v%d failed: %w", version, currentSchemaVersion, err)
		}

		// Update version info
		_, err := s.db.Exec(`
			INSERT OR REPLACE INTO schema_info (version, updated_at) 
			VALUES (?, CURRENT_TIMESTAMP)
		`, currentSchemaVersion)
		if err != nil {
			return fmt.Errorf("failed to update schema version: %w", err)
		}
	}

	return nil
}

// migrateSchema performs incremental schema migrations from oldVersion to newVersion.
//
// Migrations are designed to be:
//   - Atomic (transaction-based)
//   - Backward compatible (old data preserved)
//   - Safe for concurrent access (minimal locking)
//   - Recoverable (can be rerun safely)
func (s *sqliteAuditBackend) migrateSchema(oldVersion, newVersion int) error {
	// Begin transaction for atomic migration
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin migration transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if rollErr := tx.Rollback(); rollErr != nil {
				// Log rollback error but preserve original error
				// In production, you'd want to log this properly
				_ = rollErr
			}
		}
	}()

	// Apply migrations incrementally
	for version := oldVersion; version < newVersion; version++ {
		switch version {
		case 0:
			// Migration from no schema to v1 (basic audit table)
			if err := s.migrateToV1(tx); err != nil {
				return fmt.Errorf("migration to v1 failed: %w", err)
			}
		case 1:
			// Migration from v1 to v2 (add performance indexes)
			if err := s.migrateToV2(tx); err != nil {
				return fmt.Errorf("migration to v2 failed: %w", err)
			}
		default:
			return fmt.Errorf("unknown migration path from version %d", version)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration transaction: %w", err)
	}

	return nil
}

// migrateToV1 creates the basic audit table schema (version 1).
func (s *sqliteAuditBackend) migrateToV1(tx *sql.Tx) error {
	// Create audit events table with comprehensive schema
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS audit_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp TEXT NOT NULL,
		level TEXT NOT NULL,
		event TEXT NOT NULL,
		component TEXT NOT NULL,
		
		-- Source tracking for backward compatibility
		original_output_file TEXT NOT NULL,
		
		-- File and data information
		file_path TEXT,
		old_value TEXT,
		new_value TEXT,
		
		-- Process and correlation tracking
		process_id INTEGER NOT NULL,
		process_name TEXT NOT NULL,
		
		-- Additional context
		context TEXT, -- JSON blob for flexible metadata
		checksum TEXT,
		
		-- Indexing and performance
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	if _, err := tx.Exec(createTableSQL); err != nil {
		return fmt.Errorf("failed to create audit_events table: %w", err)
	}

	// Create basic indexes for v1
	basicIndexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_events(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_audit_level ON audit_events(level)",
		"CREATE INDEX IF NOT EXISTS idx_audit_component ON audit_events(component)",
		"CREATE INDEX IF NOT EXISTS idx_audit_source ON audit_events(original_output_file)",
		"CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_events(created_at)",
	}

	for _, indexSQL := range basicIndexes {
		if _, err := tx.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create basic index: %w", err)
		}
	}

	return nil
}

// migrateToV2 adds performance indexes and optimization for high-volume audit trails.
func (s *sqliteAuditBackend) migrateToV2(tx *sql.Tx) error {
	// Add composite indexes for common query patterns
	compositeIndexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_audit_component_time ON audit_events(component, timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_audit_level_time ON audit_events(level, created_at)",
		"CREATE INDEX IF NOT EXISTS idx_audit_source_component ON audit_events(original_output_file, component)",
		"CREATE INDEX IF NOT EXISTS idx_audit_event_component ON audit_events(event, component, timestamp)",
	}

	for _, indexSQL := range compositeIndexes {
		if _, err := tx.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create composite index: %w", err)
		}
	}

	return nil
}

// performMaintenance runs database maintenance tasks to keep the audit system performant.
//
// Maintenance tasks include:
//   - Cleaning old audit events (configurable retention)
//   - Optimizing database (VACUUM, ANALYZE)
//   - Verifying database integrity
//   - Updating statistics for query optimization
//
// This should be called periodically in production environments.
func (s *sqliteAuditBackend) performMaintenance() error {
	const defaultRetentionDays = 90 // Keep 3 months of audit data by default

	// Clean old events beyond retention period
	cleanupSQL := `
		DELETE FROM audit_events 
		WHERE created_at < datetime('now', '-' || ? || ' days')
	`

	result, err := s.db.Exec(cleanupSQL, defaultRetentionDays)
	if err != nil {
		return fmt.Errorf("failed to cleanup old audit events: %w", err)
	}

	// Log maintenance activity for transparency
	if rowsAffected, err := result.RowsAffected(); err == nil && rowsAffected > 0 {
		// Note: We could log this to the audit trail itself, but that might create recursion
		// In a real implementation, this could go to a separate maintenance log
	}

	// Optimize database performance
	optimizationTasks := []string{
		"PRAGMA optimize",             // Update query planner statistics
		"PRAGMA wal_checkpoint(FULL)", // Ensure WAL is properly checkpointed
	}

	for _, task := range optimizationTasks {
		if _, err := s.db.Exec(task); err != nil {
			// Log error but don't fail maintenance for non-critical optimizations
			continue
		}
	}

	return nil
}

// initializeSchema creates the unified audit schema with versioning and migration support.
//
// The schema is designed for:
//   - Efficient cross-application audit correlation
//   - Backward compatibility tracking
//   - Performance optimized querying
//   - Automatic maintenance and cleanup
//   - Schema evolution support
//
// Schema versioning ensures safe migrations across Argus updates.
func (s *sqliteAuditBackend) initializeSchema() error {
	// Check and migrate schema version if needed
	// All table and index creation is now handled by the migration system
	if err := s.ensureSchemaVersion(); err != nil {
		return fmt.Errorf("schema version migration failed: %w", err)
	}

	return nil
}

// prepareStatements prepares SQL statements for efficient batch operations.
//
// Prepared statements improve performance for high-frequency audit logging
// by avoiding SQL parsing overhead on each insert operation.
func (s *sqliteAuditBackend) prepareStatements() error {
	insertSQL := `
	INSERT INTO audit_events (
		timestamp, level, event, component,
		original_output_file, process_id, process_name,
		file_path, old_value, new_value, context, checksum
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	stmt, err := s.db.Prepare(insertSQL)
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}

	s.insertStmt = stmt
	return nil
}

// AuditDatabaseStats represents statistics about the unified audit database.
type AuditDatabaseStats struct {
	TotalEvents       int64            `json:"total_events"`
	EventsByLevel     map[string]int64 `json:"events_by_level"`
	EventsByComponent map[string]int64 `json:"events_by_component"`
	OldestEvent       *time.Time       `json:"oldest_event"`
	NewestEvent       *time.Time       `json:"newest_event"`
	DatabaseSize      int64            `json:"database_size_bytes"`
	SchemaVersion     int              `json:"schema_version"`
}

// getDatabaseStats retrieves comprehensive statistics about the audit database.
//
// These statistics are useful for:
//   - Monitoring audit system health
//   - Planning maintenance and retention policies
//   - Debugging audit correlation issues
//   - Performance optimization
func (s *sqliteAuditBackend) getDatabaseStats() (*AuditDatabaseStats, error) {
	stats := &AuditDatabaseStats{
		EventsByLevel:     make(map[string]int64),
		EventsByComponent: make(map[string]int64),
	}

	// Get total events count
	if err := s.getTotalEventsCount(stats); err != nil {
		return nil, err
	}

	// Get events by level
	if err := s.getEventsByLevel(stats); err != nil {
		return nil, err
	}

	// Get events by component
	if err := s.getEventsByComponent(stats); err != nil {
		return nil, err
	}

	// Get time range
	if err := s.getEventTimeRange(stats); err != nil {
		return nil, err
	}

	// Get schema version
	if err := s.getSchemaVersion(stats); err != nil {
		return nil, err
	}

	return stats, nil
}

// getTotalEventsCount gets the total number of events
func (s *sqliteAuditBackend) getTotalEventsCount(stats *AuditDatabaseStats) error {
	err := s.db.QueryRow("SELECT COUNT(*) FROM audit_events").Scan(&stats.TotalEvents)
	if err != nil {
		return fmt.Errorf("failed to get total events count: %w", err)
	}
	return nil
}

// getEventsByLevel gets events grouped by level
func (s *sqliteAuditBackend) getEventsByLevel(stats *AuditDatabaseStats) error {
	rows, err := s.db.Query("SELECT level, COUNT(*) FROM audit_events GROUP BY level")
	if err != nil {
		return fmt.Errorf("failed to get events by level: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	for rows.Next() {
		var level string
		var count int64
		if err := rows.Scan(&level, &count); err != nil {
			return fmt.Errorf("failed to scan level stats: %w", err)
		}
		stats.EventsByLevel[level] = count
	}
	return nil
}

// getEventsByComponent gets events grouped by component
func (s *sqliteAuditBackend) getEventsByComponent(stats *AuditDatabaseStats) error {
	rows, err := s.db.Query("SELECT component, COUNT(*) FROM audit_events GROUP BY component")
	if err != nil {
		return fmt.Errorf("failed to get events by component: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	for rows.Next() {
		var component string
		var count int64
		if err := rows.Scan(&component, &count); err != nil {
			return fmt.Errorf("failed to scan component stats: %w", err)
		}
		stats.EventsByComponent[component] = count
	}
	return nil
}

// getEventTimeRange gets the oldest and newest event timestamps
func (s *sqliteAuditBackend) getEventTimeRange(stats *AuditDatabaseStats) error {
	var oldestStr, newestStr sql.NullString
	err := s.db.QueryRow(`
		SELECT 
			MIN(created_at) as oldest,
			MAX(created_at) as newest
		FROM audit_events
	`).Scan(&oldestStr, &newestStr)

	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get event time range: %w", err)
	}

	if oldestStr.Valid {
		if oldest, err := time.Parse("2006-01-02 15:04:05", oldestStr.String); err == nil {
			stats.OldestEvent = &oldest
		}
	}

	if newestStr.Valid {
		if newest, err := time.Parse("2006-01-02 15:04:05", newestStr.String); err == nil {
			stats.NewestEvent = &newest
		}
	}

	return nil
}

// getSchemaVersion gets the current database schema version
func (s *sqliteAuditBackend) getSchemaVersion(stats *AuditDatabaseStats) error {
	err := s.db.QueryRow("SELECT version FROM schema_info ORDER BY version DESC LIMIT 1").Scan(&stats.SchemaVersion)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get schema version: %w", err)
	}
	return nil
}

// Write persists a batch of audit events to the SQLite database.
//
// This method handles concurrent access safely and performs batch inserts
// within a transaction for optimal performance and consistency.
func (s *sqliteAuditBackend) Write(events []AuditEvent) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return fmt.Errorf("cannot write to closed SQLite audit backend")
	}
	s.mu.RUnlock()

	if len(events) == 0 {
		return nil
	}

	// Begin transaction for batch insert
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin audit transaction: %w", err)
	}

	// Ensure transaction is handled properly
	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				// Log rollback error but don't override original error
				fmt.Fprintf(os.Stderr, "Failed to rollback audit transaction: %v\n", rollbackErr)
			}
		}
	}()

	// Prepare transaction-scoped statement
	txStmt := tx.Stmt(s.insertStmt)
	defer func() {
		if closeErr := txStmt.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Failed to close transaction statement: %v\n", closeErr)
		}
	}()

	// Insert all events in the batch
	for _, event := range events {
		err = s.insertEvent(txStmt, event)
		if err != nil {
			return fmt.Errorf("failed to insert audit event: %w", err)
		}
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit audit transaction: %w", err)
	}

	return nil
}

// insertEvent inserts a single audit event using the provided statement.
//
// This helper method handles JSON serialization and proper parameter binding
// for the audit event data.
func (s *sqliteAuditBackend) insertEvent(stmt *sql.Stmt, event AuditEvent) error {
	// Serialize JSON fields
	oldValueJSON := ""
	if event.OldValue != nil {
		data, err := json.Marshal(event.OldValue)
		if err != nil {
			return fmt.Errorf("failed to serialize old_value: %w", err)
		}
		oldValueJSON = string(data)
	}

	newValueJSON := ""
	if event.NewValue != nil {
		data, err := json.Marshal(event.NewValue)
		if err != nil {
			return fmt.Errorf("failed to serialize new_value: %w", err)
		}
		newValueJSON = string(data)
	}

	contextJSON := ""
	if event.Context != nil {
		data, err := json.Marshal(event.Context)
		if err != nil {
			return fmt.Errorf("failed to serialize context: %w", err)
		}
		contextJSON = string(data)
	}

	// Execute insert with proper parameter binding
	_, err := stmt.Exec(
		event.Timestamp.Format(time.RFC3339Nano),
		event.Level.String(),
		event.Event,
		event.Component,
		s.sourceFile, // Track original output file configuration
		event.ProcessID,
		event.ProcessName,
		event.FilePath,
		oldValueJSON,
		newValueJSON,
		contextJSON,
		event.Checksum,
	)

	return err
}

// Flush ensures all pending writes are committed to storage.
//
// For SQLite with WAL mode, this forces a checkpoint to ensure
// durability of recent transactions.
func (s *sqliteAuditBackend) Flush() error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil // No-op for closed backend
	}
	s.mu.RUnlock()

	// Force WAL checkpoint for durability
	_, err := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	if err != nil {
		return fmt.Errorf("failed to flush SQLite audit backend: %w", err)
	}

	return nil
}

// Maintenance performs database maintenance operations.
// This method is safe to call concurrently and implements the auditBackend interface.
func (s *sqliteAuditBackend) Maintenance() error {
	return s.performMaintenance()
}

// GetStats returns comprehensive database statistics.
// This method is safe to call concurrently and implements the auditBackend interface.
func (s *sqliteAuditBackend) GetStats() (*AuditDatabaseStats, error) {
	return s.getDatabaseStats()
}

// Close releases all resources and performs final cleanup.
//
// This method ensures proper cleanup of prepared statements and database
// connections. It is safe to call multiple times.
// Close releases all resources and performs final cleanup.
//
// CRITICAL: This method automatically performs a final Flush() to ensure all
// pending data in WAL (Write-Ahead Logging) mode is committed to the database
// before closing the connection. This guarantees data integrity even when
// the backend is used directly without going through AuditLogger.Close().
//
// The method is safe to call multiple times and is thread-safe.
func (s *sqliteAuditBackend) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil // Already closed
	}

	var errors []error

	// CRITICAL: Perform final flush to ensure data integrity
	// This ensures all WAL data is committed before closing the connection
	// We temporarily unlock to allow Flush() to acquire read lock
	s.mu.Unlock()
	if err := s.Flush(); err != nil {
		errors = append(errors, fmt.Errorf("failed to flush audit backend during close: %w", err))
	}
	s.mu.Lock()

	// Close prepared statement
	if s.insertStmt != nil {
		if err := s.insertStmt.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close insert statement: %w", err))
		}
	}

	// Close database connection
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close database: %w", err))
		}
	}

	s.closed = true

	// Return combined errors if any occurred
	if len(errors) > 0 {
		return fmt.Errorf("errors closing SQLite audit backend: %v", errors)
	}

	return nil
}

// jsonlAuditBackend implements auditBackend using JSONL files for backward compatibility.
//
// This backend provides compatibility with existing JSONL-based audit logging
// while implementing the same interface as the SQLite backend. It wraps the
// existing file-based audit functionality.
type jsonlAuditBackend struct {
	file       *os.File
	sourceFile string
	mu         sync.Mutex
	closed     bool
}

// newJSONLBackend creates a new JSONL audit backend for backward compatibility.
//
// This function provides a fallback mechanism when SQLite is not available,
// maintaining compatibility with existing JSONL-based audit configurations.
//
// Parameters:
//   - config: AuditConfig containing file path and other settings
//
// Returns:
//   - Configured JSONL backend ready for use
//   - Error if file creation or initialization fails
func newJSONLBackend(config AuditConfig) (*jsonlAuditBackend, error) {
	if config.OutputFile == "" {
		return nil, fmt.Errorf("JSONL backend requires OutputFile to be specified")
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(config.OutputFile), 0750); err != nil {
		return nil, fmt.Errorf("failed to create JSONL audit log directory: %w", err)
	}

	// Open audit file with secure permissions (owner read/write only)
	file, err := os.OpenFile(config.OutputFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open JSONL audit log file: %w", err)
	}

	return &jsonlAuditBackend{
		file:       file,
		sourceFile: config.OutputFile,
	}, nil
}

// Write persists a batch of audit events to the JSONL file.
//
// Each event is serialized as a JSON object on a single line,
// following the JSONL format specification.
func (j *jsonlAuditBackend) Write(events []AuditEvent) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.closed {
		return fmt.Errorf("cannot write to closed JSONL audit backend")
	}

	if len(events) == 0 {
		return nil
	}

	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("failed to serialize audit event: %w", err)
		}

		if _, err := j.file.Write(data); err != nil {
			return fmt.Errorf("failed to write audit event to JSONL: %w", err)
		}

		if _, err := j.file.Write([]byte("\n")); err != nil {
			return fmt.Errorf("failed to write audit event newline: %w", err)
		}
	}

	return nil
}

// Flush ensures all pending writes are committed to storage.
//
// For JSONL files, this forces an fsync to ensure data persistence.
func (j *jsonlAuditBackend) Flush() error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.closed {
		return nil // No-op for closed backend
	}

	if err := j.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync JSONL audit file: %w", err)
	}

	return nil
}

// Maintenance performs file-based maintenance operations for JSONL backend.
// For JSONL files, this could include log rotation, compression, or archiving.
// Currently returns nil as JSONL files are self-maintaining.
func (j *jsonlAuditBackend) Maintenance() error {
	// JSONL files are inherently self-maintaining
	// Future enhancements could include:
	// - Log rotation based on size/age
	// - Compression of old files
	// - Archiving to remote storage
	return nil
}

// GetStats returns basic file statistics for JSONL backend.
// This provides limited statistics compared to SQLite backend.
func (j *jsonlAuditBackend) GetStats() (*AuditDatabaseStats, error) {
	stats := &AuditDatabaseStats{
		EventsByLevel:     make(map[string]int64),
		EventsByComponent: make(map[string]int64),
		SchemaVersion:     1, // JSONL format is version 1
	}

	// Get file size if file exists
	if info, err := os.Stat(j.sourceFile); err == nil {
		stats.DatabaseSize = info.Size()
	}

	// Note: Event counting would require parsing the entire JSONL file
	// which could be expensive for large files. For now, we return
	// basic statistics. Future enhancements could include:
	// - Cached event counts
	// - Sampling-based statistics
	// - Incremental count tracking

	return stats, nil
}

// Close releases all resources and performs final cleanup.
//
// This method ensures proper cleanup of file handles and is safe to call
// multiple times.
func (j *jsonlAuditBackend) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.closed {
		return nil // Already closed
	}

	var err error
	if j.file != nil {
		err = j.file.Close()
	}

	j.closed = true
	return err
}
