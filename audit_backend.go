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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	goerrors "github.com/agilira/go-errors"
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
	// The audit destination goes through the same path validation as a watched
	// file. Only the environment variable was validated before, so a path
	// rejected as ARGUS_AUDIT_OUTPUT_FILE was accepted when the same string
	// arrived through AuditConfig — the defence applied to one caller and not
	// the other. It also makes the os.OpenFile calls below provably safe.
	if config.OutputFile != "" {
		if err := ValidateSecurePath(config.OutputFile); err != nil {
			return nil, fmt.Errorf("unsafe audit output file: %w", err)
		}
	}

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
//
// LOCATION: the database lives in a per-user state directory, never in the
// shared temporary directory. An audit trail records which files a process
// watches and every path it rejected; os.TempDir() is world-writable, so the
// first user to create the argus/ subdirectory there owned it for everyone,
// and a pre-created symlink would have redirected another user's trail
// (CWE-377). The resolution order is:
//
//  1. $ARGUS_AUDIT_DIR, for deployments that place it explicitly
//  2. $XDG_STATE_HOME/argus, the freedesktop location for state that should
//     persist between restarts
//  3. os.UserConfigDir()/argus — ~/.local/state equivalents on macOS and
//     %AppData% on Windows, both per-user
//  4. os.TempDir()/argus-<uid>, a last resort that is at least not shared
func getUnifiedAuditPath() string {
	return filepath.Join(unifiedAuditDir(), "system-audit.db")
}

// unifiedAuditDir resolves the directory holding the unified audit database.
func unifiedAuditDir() string {
	if dir := os.Getenv("ARGUS_AUDIT_DIR"); dir != "" {
		return dir
	}

	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		return filepath.Join(stateHome, "argus")
	}

	// ~/.local/state is the freedesktop convention, which macOS and Windows do
	// not follow; those fall through to os.UserConfigDir below.
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, ".local", "state", "argus")
		}
	}

	if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
		return filepath.Join(configDir, "argus")
	}

	// Last resort: still in the temporary directory, but scoped to this user so
	// it cannot be pre-created or read by another account.
	return filepath.Join(os.TempDir(), fmt.Sprintf("argus-%d", os.Getuid()))
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

// chainTailQuery reads the checksum the next record must chain onto.
//
// It runs inside the insert transaction rather than from a cached field: the
// unified database is shared between processes, so a tail cached at open time
// goes stale the moment another process appends and the two would fork the
// chain. SQLite serialises writers, so the read and the inserts that follow it
// see a consistent tail.
const chainTailQuery = `SELECT checksum FROM audit_events ORDER BY id DESC LIMIT 1`

// createChainHeadSQL anchors where the trail currently ends.
//
// WHY: a hash chain proves that the records you can see were not altered and
// that none was removed FROM THE MIDDLE — but removing records from the END
// leaves a shorter chain that is internally perfect. Recording the head
// separately means a truncated trail no longer matches what the database says
// its own end is.
//
// LIMIT, stated plainly: the head lives in the same file. An attacker with
// write access to the database can delete records and rewrite the head row to
// match. This catches truncation by accident, by partial restore, by a tool
// that pruned the table, and by an attacker who did not know to look here — it
// is not a guarantee against a fully privileged local attacker. That requires
// anchoring the head somewhere Argus does not control: a remote log, a signed
// checkpoint, or append-only storage.
const createChainHeadSQL = `
	CREATE TABLE IF NOT EXISTS chain_head (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		checksum TEXT NOT NULL,
		record_count INTEGER NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

// chainHeadQuery reads the recorded end of the trail.
const chainHeadQuery = `SELECT checksum, record_count FROM chain_head WHERE id = 1`

// chainHeadUpsert records the new end of the trail.
const chainHeadUpsert = `
	INSERT INTO chain_head (id, checksum, record_count, updated_at)
	VALUES (1, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(id) DO UPDATE SET
		checksum = excluded.checksum,
		record_count = excluded.record_count,
		updated_at = CURRENT_TIMESTAMP`

// chainHead reads the anchor. found is false on a trail written before the
// anchor existed, which is verified without it.
func chainHead(tx *sql.Tx) (checksum string, count int64, found bool, err error) {
	err = tx.QueryRow(chainHeadQuery).Scan(&checksum, &count)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", 0, false, nil
	case err != nil:
		return "", 0, false, fmt.Errorf("failed to read audit chain head: %w", err)
	}
	return checksum, count, true, nil
}

// chainScanQuery walks the whole trail in insertion order for verification.
const chainScanQuery = `
SELECT id, timestamp, level, event, component,
       file_path, old_value, new_value,
       process_id, process_name, context, prev_checksum, checksum
  FROM audit_events
 ORDER BY id ASC`

// scanChainRow reads one row of chainScanQuery into an AuditEvent.
//
// It deliberately reuses the same column order and the same JSON handling as
// the query path, so a record verifies identically however it was read.
func scanChainRow(rows *sql.Rows, id *int64) (AuditEvent, error) {
	var (
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
		prevChecksum sql.NullString
		checksum     sql.NullString
	)

	if err := rows.Scan(
		id, &tsStr, &levelStr, &event, &component,
		&filePath, &oldValueJSON, &newValueJSON,
		&processID, &processName, &contextJSON, &prevChecksum, &checksum,
	); err != nil {
		return AuditEvent{}, fmt.Errorf("failed to scan audit record: %w", err)
	}

	ts, err := time.Parse(time.RFC3339Nano, tsStr)
	if err != nil {
		return AuditEvent{}, fmt.Errorf("invalid timestamp %q: %w", tsStr, err)
	}

	ev := AuditEvent{
		Timestamp:    ts,
		Level:        parseStoredAuditLevel(levelStr),
		Event:        event,
		Component:    component,
		FilePath:     filePath.String,
		ProcessID:    processID,
		ProcessName:  processName,
		PrevChecksum: prevChecksum.String,
		Checksum:     checksum.String,
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

	// Create the database file ourselves so it never exists with the driver's
	// default 0644. The JSONL backend has always used 0600; an audit trail in
	// SQLite is no less sensitive.
	if err := ensureSecureDatabaseFile(dbPath); err != nil {
		return "", err
	}

	return dbPath, nil
}

// ensureSecureDatabaseFile guarantees the audit database is owner-only.
//
// The file is created empty with 0600 before SQLite opens it, closing the
// window in which it would exist world-readable. An existing file is tightened
// in place; a file we do not own cannot be tightened, and that is reported
// rather than silently accepted, because it means somebody else controls the
// audit trail.
func ensureSecureDatabaseFile(dbPath string) error {
	// #nosec G304 -- dbPath is either the unified per-user path built by this
	// package or an OutputFile validated by ValidateSecurePath in createAuditBackend.
	file, err := os.OpenFile(dbPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		return file.Close()
	}
	if !os.IsExist(err) {
		return fmt.Errorf("failed to create audit database file: %w", err)
	}

	return secureAuditFileMode(dbPath)
}

// secureAuditFileMode restricts one audit file to owner read/write.
// A file that does not exist is not an error: the WAL sidecars appear only
// once SQLite has written to them.
func secureAuditFileMode(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to inspect audit file %s: %w", path, err)
	}

	if info.Mode().Perm()&0o077 == 0 {
		return nil // already owner-only
	}

	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("audit file %s is accessible to other users and cannot be restricted: %w", path, err)
	}

	return nil
}

// secureAuditSidecars restricts the WAL and shared-memory files SQLite creates
// next to the database. They hold committed audit records that have not been
// checkpointed yet, so they need the same protection as the database itself.
func secureAuditSidecars(dbPath string) error {
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := secureAuditFileMode(dbPath + suffix); err != nil {
			return err
		}
	}
	return nil
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
// 5. _txlock=immediate:
//   - Every transaction takes the write lock up front
//   - The write path reads the chain tail and then inserts. Under the default
//     deferred locking that transaction starts as a reader and must UPGRADE to
//     a writer, which SQLite refuses outright when another writer holds the
//     lock — _busy_timeout does not apply to an upgrade, because waiting there
//     would deadlock. Concurrent writers saw "database is locked" immediately.
//   - Taking the lock up front makes _busy_timeout do its job: writers queue
//     for up to 5 seconds instead of failing.
//
// ═══════════════════════════════════════════════════════════════════════════════
func openSQLiteDatabase(dbPath string) (*sql.DB, error) {
	// Open SQLite database with optimized settings
	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_cache_size=1000&_txlock=immediate", dbPath))
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

// verifyChainIntegrity walks the trail in insertion order and checks both
// halves of the guarantee: each record hashes to its stored checksum, and each
// record's prev_checksum is the checksum of the record before it.
//
// The scan is streamed rather than collected: an audit trail is append-only and
// grows without bound, and holding a whole one in memory to verify it would
// turn a health check into an outage.
func (s *sqliteAuditBackend) verifyChainIntegrity(recompute func(AuditEvent) string) error {
	s.mu.RLock()
	closed := s.closed
	s.mu.RUnlock()
	if closed {
		return fmt.Errorf("cannot verify a closed SQLite audit backend")
	}

	rows, err := s.db.Query(chainScanQuery)
	if err != nil {
		return fmt.Errorf("failed to scan audit chain: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var (
		expectedPrev string
		index        int
	)

	for rows.Next() {
		var id int64
		event, err := scanChainRow(rows, &id)
		if err != nil {
			return err
		}

		if event.PrevChecksum != expectedPrev {
			return chainBreak("audit chain integrity check failed: record does not follow its predecessor", index, id)
		}

		if event.Checksum != recompute(event) {
			return chainBreak("audit chain integrity check failed: checksum mismatch", index, id)
		}

		expectedPrev = event.Checksum
		index++
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to read audit chain: %w", err)
	}

	return s.verifyChainHead(expectedPrev, int64(index))
}

// verifyChainHead compares the end of the trail we just walked with the anchor
// the database records for it, which is what makes a truncated tail visible.
func (s *sqliteAuditBackend) verifyChainHead(tail string, count int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to read audit chain head: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	headChecksum, headCount, found, err := chainHead(tx)
	if err != nil {
		return err
	}
	if !found {
		// Trail written before the anchor existed: the per-record and
		// adjacency checks above are all that can be said about it.
		return nil
	}

	if headCount != count {
		return goerrors.New(ErrCodeAuditChainBroken,
			"audit chain integrity check failed: the trail holds fewer records than its recorded end claims").
			WithContext("records_found", count).
			WithContext("records_expected", headCount)
	}

	if headChecksum != tail {
		return goerrors.New(ErrCodeAuditChainBroken,
			"audit chain integrity check failed: the trail does not end where its recorded end says").
			WithContext("records_found", count)
	}

	return nil
}

// chainBreak builds the typed error reporting where a trail stops being trustworthy.
func chainBreak(message string, index int, id int64) error {
	return goerrors.New(ErrCodeAuditChainBroken, message).
		WithContext("index", index).
		WithContext("id", id)
}

// chainCount returns how many records the trail currently holds.
func chainCount(tx *sql.Tx) (int64, error) {
	var count int64
	if err := tx.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count audit records: %w", err)
	}
	return count, nil
}

// chainTail returns the checksum of the most recent record, or the empty
// string when the trail is empty and the next record starts a new chain.
func chainTail(tx *sql.Tx) (string, error) {
	var tail sql.NullString
	err := tx.QueryRow(chainTailQuery).Scan(&tail)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("failed to read audit chain tail: %w", err)
	}
	return tail.String, nil
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

	// The schema transaction has now created the WAL sidecars; restrict them
	// before any audit record reaches them.
	if err := secureAuditSidecars(backend.dbPath); err != nil {
		if closeErr := backend.Close(); closeErr != nil {
			return fmt.Errorf("failed to secure audit sidecar files (close error: %v): %w", closeErr, err)
		}
		return err
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
	const currentSchemaVersion = 3

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

// migrateToV3 adds the hash-chain column.
//
// A database created by migrateToV1 already has the column, because the table
// definition there carries it; only a database created before the chain
// existed needs the ALTER. The check keeps the migration idempotent either way.
//
// Records written before this migration have no prev_checksum and their
// checksum was computed under the old per-record formula, so VerifyAuditChain
// reports the trail as broken at the point the chain begins. That is the
// honest outcome: nothing links those records to each other.
func (s *sqliteAuditBackend) migrateToV3(tx *sql.Tx) error {
	hasColumn, err := columnExists(tx, "audit_events", "prev_checksum")
	if err != nil {
		return err
	}
	if !hasColumn {
		if _, err := tx.Exec(`ALTER TABLE audit_events ADD COLUMN prev_checksum TEXT`); err != nil {
			return fmt.Errorf("failed to add prev_checksum column: %w", err)
		}
	}

	if _, err := tx.Exec(createChainHeadSQL); err != nil {
		return fmt.Errorf("failed to create chain_head table: %w", err)
	}

	return nil
}

// columnExists reports whether a table already has a column.
func columnExists(tx *sql.Tx, table, column string) (bool, error) {
	// #nosec G202 -- table is a package constant, never operator input.
	rows, err := tx.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, fmt.Errorf("failed to inspect table %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			dflt       sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("failed to read table info for %s: %w", table, err)
		}
		if name == column {
			return true, rows.Err()
		}
	}

	return false, rows.Err()
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
		case 2:
			// Migration from v2 to v3 (hash-chain column)
			if err := s.migrateToV3(tx); err != nil {
				return fmt.Errorf("migration to v3 failed: %w", err)
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

		-- Hash chain: checksum links this record to prev_checksum, which is the
		-- checksum of the record before it. See audit.go chainChecksum.
		prev_checksum TEXT,
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
		file_path, old_value, new_value, context, prev_checksum, checksum
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

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

	// Read the tail INSIDE the transaction, then chain each record onto the
	// one before it. Doing this here — rather than when the event was buffered
	// — is what makes the chain correct: only at insert time is a record's
	// position in the trail known, and SQLite serialises writers so two
	// processes appending to the unified database cannot fork it.
	var prev string
	prev, err = chainTail(tx)
	if err != nil {
		return err
	}

	var appended int64
	appended, err = chainCount(tx)
	if err != nil {
		return err
	}
	appended += int64(len(events))

	// Insert all events in the batch
	for _, event := range events {
		event.PrevChecksum = prev
		event.Checksum = chainChecksum(prev, recordDigest(event))

		err = s.insertEvent(txStmt, event)
		if err != nil {
			return fmt.Errorf("failed to insert audit event: %w", err)
		}

		prev = event.Checksum
	}

	// Move the anchor in the same transaction, so it can never disagree with
	// the records it describes.
	if _, err = tx.Exec(chainHeadUpsert, prev, appended); err != nil {
		return fmt.Errorf("failed to update audit chain head: %w", err)
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

	// Execute insert with proper parameter binding. Timestamp is
	// UTC-normalized so query-time lexical comparison against
	// .UTC() bounds (audit_query.go normalizeFilter) is correct
	// regardless of the writer's local timezone — same RFC3339Nano
	// shape used by the checksum in audit.go.generateChecksum.
	_, err := stmt.Exec(
		event.Timestamp.UTC().Format(time.RFC3339Nano),
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
		event.PrevChecksum,
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
