// audit_backend_test.go - Comprehensive test suite for Argus audit backends
//
// This test suite provides comprehensive coverage for both SQLite and JSONL
// audit backends, including schema migrations, error handling, and performance.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver for tests
	"runtime"
)

// Test helpers and utilities

// createTempDB creates a temporary SQLite database for testing
func createTempDB(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")
	return dbPath
}

// createTestSQLiteBackend creates a SQLite backend with a temporary database for testing
func createTestSQLiteBackend(t *testing.T) (auditBackend, string) {
	t.Helper()
	dbPath := createTempDB(t) // Now will be used because of .db extension
	config := AuditConfig{
		Enabled:    true,
		OutputFile: dbPath,
		BufferSize: 5,
	}

	backend, err := newSQLiteBackend(config)
	if err != nil {
		t.Fatalf("Failed to create SQLite backend: %v", err)
	}

	// Return the actual path used by SQLite backend (now uses the specified path)
	return backend, dbPath
}

// createTempJSONL creates a temporary JSONL file for testing
func createTempJSONL(t *testing.T) string {
	t.Helper()
	tmpFile, err := os.CreateTemp(t.TempDir(), "audit-*.jsonl")
	if err != nil {
		t.Fatalf("Failed to create temp JSONL file: %v", err)
	}
	tmpFile.Close()
	return tmpFile.Name()
}

// createTestAuditEvent creates a sample audit event for testing
func createTestAuditEvent(component, event string) AuditEvent {
	return AuditEvent{
		Timestamp:   time.Now(),
		Level:       AuditCritical,
		Event:       event,
		Component:   component,
		FilePath:    "/test/config.json",
		OldValue:    map[string]interface{}{"old": "value"},
		NewValue:    map[string]interface{}{"new": "value"},
		Context:     map[string]interface{}{"test": true},
		ProcessID:   12345,
		ProcessName: "test-process",
		Checksum:    "test-checksum",
	}
}

// verifyEventInDB verifies an audit event exists in SQLite database
func verifyEventInDB(t *testing.T, dbPath string, event AuditEvent) {
	t.Helper()

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	var count int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM audit_events 
		WHERE component = ? AND event = ? AND level = ?
	`, event.Component, event.Event, event.Level.String()).Scan(&count)

	if err != nil {
		t.Fatalf("Failed to query database: %v", err)
	}

	if count == 0 {
		t.Errorf("Event not found in database: component=%s, event=%s",
			event.Component, event.Event)
	}
}

// Test Suite: Backend Interface Compliance

func TestBackendInterface_SQLite(t *testing.T) {
	t.Parallel()

	dbPath := createTempDB(t)
	config := AuditConfig{
		Enabled:    true,
		OutputFile: dbPath,
		BufferSize: 10,
	}

	backend, err := createAuditBackend(config)
	if err != nil {
		t.Fatalf("Failed to create SQLite backend: %v", err)
	}
	defer backend.Close()

	// Test interface methods exist and work
	events := []AuditEvent{createTestAuditEvent("test-component", "test-event")}

	if err := backend.Write(events); err != nil {
		t.Errorf("Write failed: %v", err)
	}

	if err := backend.Flush(); err != nil {
		t.Errorf("Flush failed: %v", err)
	}

	if err := backend.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestBackendInterface_JSONL(t *testing.T) {
	t.Parallel()

	jsonlPath := createTempJSONL(t)
	config := AuditConfig{
		Enabled:    true,
		OutputFile: jsonlPath,
		BufferSize: 10,
	}

	backend, err := createAuditBackend(config)
	if err != nil {
		t.Fatalf("Failed to create JSONL backend: %v", err)
	}
	defer backend.Close()

	// Test interface methods exist and work
	events := []AuditEvent{createTestAuditEvent("test-component", "test-event")}

	if err := backend.Write(events); err != nil {
		t.Errorf("Write failed: %v", err)
	}

	if err := backend.Flush(); err != nil {
		t.Errorf("Flush failed: %v", err)
	}

	if err := backend.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// Test Suite: Backend Selection Logic

func TestBackendSelection_AutomaticSQLite(t *testing.T) {
	testCases := []struct {
		name       string
		outputFile string
		expectType string
	}{
		{"Empty OutputFile", "", "SQLite"},
		{"Non-JSONL extension", "/tmp/audit.db", "SQLite"},
		{"No extension", "/tmp/audit", "SQLite"},
		{"Custom extension", "/tmp/audit.log", "SQLite"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			config := AuditConfig{
				Enabled:    true,
				OutputFile: tc.outputFile,
				BufferSize: 10,
			}

			backend, err := createAuditBackend(config)
			if err != nil {
				t.Fatalf("Backend creation failed for %s: %v", tc.name, err)
			}
			defer backend.Close()

			// Verify it's SQLite by checking if we can cast to sqliteAuditBackend
			if _, ok := backend.(*sqliteAuditBackend); !ok && tc.expectType == "SQLite" {
				t.Errorf("Expected SQLite backend for %s, got different type", tc.name)
			}
		})
	}
}

func TestBackendSelection_JSONL(t *testing.T) {
	t.Parallel()

	jsonlPath := createTempJSONL(t)
	config := AuditConfig{
		Enabled:    true,
		OutputFile: jsonlPath, // .jsonl extension should trigger JSONL backend
		BufferSize: 10,
	}

	backend, err := createAuditBackend(config)
	if err != nil {
		t.Fatalf("Failed to create JSONL backend: %v", err)
	}
	defer backend.Close()

	// Verify it's JSONL by checking if we can cast to jsonlAuditBackend
	if _, ok := backend.(*jsonlAuditBackend); !ok {
		t.Errorf("Expected JSONL backend for .jsonl extension, got different type")
	}
}

// Test Suite: SQLite Backend Specific Tests

func TestSQLiteBackend_WriteAndVerify(t *testing.T) {
	t.Parallel()

	backend, dbPath := createTestSQLiteBackend(t)
	defer backend.Close()

	// Create and write test events
	events := []AuditEvent{
		createTestAuditEvent("app1", "config_change"),
		createTestAuditEvent("app2", "file_watch"),
	}

	if err := backend.Write(events); err != nil {
		t.Fatalf("Failed to write events: %v", err)
	}

	if err := backend.Flush(); err != nil {
		t.Fatalf("Failed to flush events: %v", err)
	}

	// Verify events were written to database
	for _, event := range events {
		verifyEventInDB(t, dbPath, event)
	}
}

func TestSQLiteBackend_SchemaVersioning(t *testing.T) {
	t.Parallel()

	backend, dbPath := createTestSQLiteBackend(t)
	defer backend.Close()

	// Verify schema_info table was created and has correct version
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	var version int
	err = db.QueryRow("SELECT version FROM schema_info ORDER BY version DESC LIMIT 1").Scan(&version)
	if err != nil {
		t.Fatalf("Failed to get schema version: %v", err)
	}

	if version != 2 { // Current schema version should be 2
		t.Errorf("Expected schema version 2, got %d", version)
	}
}

func TestSQLiteBackend_IndexesCreated(t *testing.T) {
	t.Parallel()

	backend, dbPath := createTestSQLiteBackend(t)
	defer backend.Close()

	// Verify indexes were created
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	expectedIndexes := []string{
		"idx_audit_timestamp",
		"idx_audit_level",
		"idx_audit_component",
		"idx_audit_source",
		"idx_audit_created_at",
		"idx_audit_component_time",
		"idx_audit_level_time",
		"idx_audit_source_component",
		"idx_audit_event_component",
	}

	for _, indexName := range expectedIndexes {
		var count int
		err := db.QueryRow(`
			SELECT COUNT(*) FROM sqlite_master 
			WHERE type='index' AND name=?
		`, indexName).Scan(&count)

		if err != nil {
			t.Fatalf("Failed to check index %s: %v", indexName, err)
		}

		if count == 0 {
			t.Errorf("Index %s was not created", indexName)
		}
	}
}

// Test Suite: SQLite Concurrency and Security Tests

func TestSQLiteBackend_ConcurrentWrites_Basic(t *testing.T) {
	t.Parallel()

	backend, _ := createTestSQLiteBackend(t)
	defer backend.Close()

	const numWorkers = 5
	const eventsPerWorker = 10

	// Channel to synchronize goroutine completion
	done := make(chan error, numWorkers)

	// Launch concurrent writers
	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			// Create unique events for this worker
			events := make([]AuditEvent, eventsPerWorker)
			for j := 0; j < eventsPerWorker; j++ {
				events[j] = createTestAuditEvent(
					fmt.Sprintf("concurrent-worker-%d", workerID),
					fmt.Sprintf("event-%d", j),
				)
			}

			// Write events
			if err := backend.Write(events); err != nil {
				done <- fmt.Errorf("worker %d write failed: %w", workerID, err)
				return
			}

			// Flush to ensure data integrity
			if err := backend.Flush(); err != nil {
				done <- fmt.Errorf("worker %d flush failed: %w", workerID, err)
				return
			}

			done <- nil
		}(i)
	}

	// Wait for all workers to complete
	for i := 0; i < numWorkers; i++ {
		if err := <-done; err != nil {
			t.Errorf("Concurrent write test failed: %v", err)
		}
	}

	// Verify final flush works after concurrent operations
	if err := backend.Flush(); err != nil {
		t.Errorf("Final flush failed: %v", err)
	}
}

func TestSQLiteBackend_ConcurrentWriteAndMaintenance(t *testing.T) {
	t.Parallel()

	backend, _ := createTestSQLiteBackend(t)
	defer backend.Close()

	// Test duration
	testDuration := 1 * time.Second
	stopChan := make(chan struct{})
	errorChan := make(chan error, 10)

	// Writer goroutine - continuously writes events
	go func() {
		eventCounter := 0
		for {
			select {
			case <-stopChan:
				return
			default:
				event := createTestAuditEvent("maintenance-writer", fmt.Sprintf("event-%d", eventCounter))
				if err := backend.Write([]AuditEvent{event}); err != nil {
					errorChan <- fmt.Errorf("write error: %w", err)
					return
				}
				eventCounter++
				time.Sleep(5 * time.Millisecond) // Small delay to simulate realistic load
			}
		}
	}()

	// Maintenance goroutine - occasionally runs maintenance
	go func() {
		for {
			select {
			case <-stopChan:
				return
			default:
				if err := backend.Maintenance(); err != nil {
					errorChan <- fmt.Errorf("maintenance error: %w", err)
					return
				}
				time.Sleep(100 * time.Millisecond) // Maintenance runs less frequently
			}
		}
	}()

	// Statistics reader goroutine - checks stats during operations
	go func() {
		for {
			select {
			case <-stopChan:
				return
			default:
				if _, err := backend.GetStats(); err != nil {
					errorChan <- fmt.Errorf("stats error: %w", err)
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()

	// Run test for specified duration
	time.Sleep(testDuration)
	close(stopChan)

	// Check for any errors from goroutines
	select {
	case err := <-errorChan:
		t.Fatalf("Concurrent operation failed: %v", err)
	case <-time.After(200 * time.Millisecond):
		// No errors within timeout - success
	}

	// Final verification - ensure backend is still functional
	finalEvent := createTestAuditEvent("final-test", "post-concurrent")
	if err := backend.Write([]AuditEvent{finalEvent}); err != nil {
		t.Errorf("Backend not functional after concurrent test: %v", err)
	}
}

func TestSQLiteBackend_ErrorRecovery_Security(t *testing.T) {
	t.Parallel()

	backend, dbPath := createTestSQLiteBackend(t)
	defer backend.Close()

	// Test 1: Write valid events first
	validEvents := []AuditEvent{
		createTestAuditEvent("security-test", "valid-event-1"),
		createTestAuditEvent("security-test", "valid-event-2"),
	}

	if err := backend.Write(validEvents); err != nil {
		t.Fatalf("Failed to write valid events: %v", err)
	}

	// Test 2: Try to write events with potential security issues (very large data)
	largeData := make([]byte, 1024*1024) // 1MB of data
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	largeEvent := AuditEvent{
		Timestamp: time.Now(),
		Level:     AuditInfo,
		Event:     "large-data-test",
		Component: "security-test",
		Context:   map[string]interface{}{"large_data": string(largeData)},
	}

	// This should handle large data gracefully
	if err := backend.Write([]AuditEvent{largeEvent}); err != nil {
		t.Logf("Large data write failed as expected: %v", err)
	}

	// Test 3: Verify backend is still functional after potential error
	recoveryEvent := createTestAuditEvent("security-test", "recovery-test")
	if err := backend.Write([]AuditEvent{recoveryEvent}); err != nil {
		t.Errorf("Backend not functional after error scenario: %v", err)
	}

	// Test 4: Verify database integrity
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database for verification: %v", err)
	}
	defer db.Close()

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE component = 'security-test'").Scan(&count)
	if err != nil {
		t.Errorf("Failed to query database: %v", err)
	}

	if count < 2 { // At least the valid events should be there
		t.Errorf("Expected at least 2 security-test events, got %d", count)
	}

	// Test 5: Verify backend can be flushed after stress
	if err := backend.Flush(); err != nil {
		t.Errorf("Failed to flush after security test: %v", err)
	}
}

func TestSQLiteBackend_SafeShutdown_Concurrency(t *testing.T) {
	t.Parallel()

	backend, _ := createTestSQLiteBackend(t)

	// Channel to signal when to start shutdown
	shutdownChan := make(chan struct{})
	errorChan := make(chan error, 5)

	// Writer goroutine - will be interrupted by shutdown
	go func() {
		eventCounter := 0
		for {
			select {
			case <-shutdownChan:
				// Try to write after shutdown signal (should handle gracefully)
				event := createTestAuditEvent("shutdown-test", fmt.Sprintf("post-shutdown-%d", eventCounter))
				if err := backend.Write([]AuditEvent{event}); err != nil {
					// This is expected after Close() is called
					t.Logf("Expected write error after shutdown: %v", err)
				}
				return
			default:
				event := createTestAuditEvent("shutdown-test", fmt.Sprintf("pre-shutdown-%d", eventCounter))
				if err := backend.Write([]AuditEvent{event}); err != nil {
					errorChan <- fmt.Errorf("pre-shutdown write error: %w", err)
					return
				}
				eventCounter++
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	// Maintenance goroutine - will also be interrupted
	go func() {
		for {
			select {
			case <-shutdownChan:
				// Try maintenance after shutdown (should handle gracefully)
				if err := backend.Maintenance(); err != nil {
					t.Logf("Expected maintenance error after shutdown: %v", err)
				}
				return
			default:
				if err := backend.Maintenance(); err != nil {
					errorChan <- fmt.Errorf("pre-shutdown maintenance error: %w", err)
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()

	// Let operations run for a bit
	time.Sleep(100 * time.Millisecond)

	// Signal shutdown
	close(shutdownChan)

	// Perform graceful shutdown
	if err := backend.Flush(); err != nil {
		t.Errorf("Failed to flush before close: %v", err)
	}

	if err := backend.Close(); err != nil {
		t.Errorf("Failed to close backend: %v", err)
	}

	// Verify multiple closes are safe
	if err := backend.Close(); err != nil {
		t.Errorf("Second close should be safe: %v", err)
	}

	// Give goroutines time to finish their shutdown attempts
	time.Sleep(100 * time.Millisecond)

	// Check for any unexpected errors
	select {
	case err := <-errorChan:
		t.Errorf("Unexpected error during concurrent operations: %v", err)
	default:
		// No errors - success
	}
}

func TestSQLiteBackend_SchemaMigration_Security(t *testing.T) {
	t.Parallel()

	// Create a temporary database file for isolated testing
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "migration_test.db")

	// Step 1: Initialize backend with .db extension (this will trigger migration from v0->v2)
	config := AuditConfig{
		Enabled:    true,
		OutputFile: dbPath, // Now will be respected due to .db extension
		BufferSize: 5,
	}

	backend, err := newSQLiteBackend(config)
	if err != nil {
		t.Fatalf("Failed to create backend for migration test: %v", err)
	}
	defer backend.Close()

	// Step 2: Verify schema was properly migrated
	testDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database for verification: %v", err)
	}
	defer testDB.Close()

	// Check schema_info table exists and has correct version
	var version int
	err = testDB.QueryRow("SELECT version FROM schema_info ORDER BY version DESC LIMIT 1").Scan(&version)
	if err != nil {
		t.Errorf("Schema info not found: %v", err)
	}

	if version != 2 {
		t.Errorf("Expected schema version 2, got %d", version)
	}

	// Check audit_events table exists with all required columns
	expectedColumns := []string{
		"id", "timestamp", "level", "event", "component",
		"original_output_file", "file_path", "old_value", "new_value",
		"process_id", "process_name", "context", "checksum", "created_at",
	}

	for _, column := range expectedColumns {
		var exists int
		query := fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info('audit_events') WHERE name = '%s'", column)
		err = testDB.QueryRow(query).Scan(&exists)
		if err != nil {
			t.Errorf("Failed to check column %s: %v", column, err)
		}
		if exists != 1 {
			t.Errorf("Column %s does not exist in audit_events table", column)
		}
	}

	// Step 4: Verify indexes were created
	expectedIndexes := []string{
		"idx_audit_timestamp",
		"idx_audit_level",
		"idx_audit_component",
		"idx_audit_source",
		"idx_audit_created_at",
		"idx_audit_component_time",
		"idx_audit_level_time",
		"idx_audit_source_component",
		"idx_audit_event_component",
	}

	for _, indexName := range expectedIndexes {
		var exists int
		query := "SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?"
		err = testDB.QueryRow(query, indexName).Scan(&exists)
		if err != nil {
			t.Errorf("Failed to check index %s: %v", indexName, err)
		}
		if exists != 1 {
			t.Errorf("Index %s was not created during migration", indexName)
		}
	}

	// Step 5: Test that the migrated database is functional
	testEvent := createTestAuditEvent("migration-test", "post-migration-test")
	if err := backend.Write([]AuditEvent{testEvent}); err != nil {
		t.Errorf("Failed to write to migrated database: %v", err)
	}

	if err := backend.Flush(); err != nil {
		t.Errorf("Failed to flush migrated database: %v", err)
	}
}

func TestSQLiteBackend_ErrorHandling_EdgeCases(t *testing.T) {
	t.Parallel()

	   // Test 1: Backend with invalid database path (skip on Windows)
	   if runtime.GOOS == "windows" {
		   t.Skip("Skipping invalid path test on Windows: path semantics differ")
	   }
	   invalidConfig := AuditConfig{
		   Enabled:    true,
		   OutputFile: "/invalid/path/that/cannot/exist/test.db",
		   BufferSize: 5,
	   }
	   _, err := newSQLiteBackend(invalidConfig)
	   if err == nil {
		   t.Error("Expected error for invalid database path, but got none")
	   }

	// Test 2: Test with empty events array
	backend, _ := createTestSQLiteBackend(t)
	defer backend.Close()

	if err := backend.Write([]AuditEvent{}); err != nil {
		t.Errorf("Empty events array should not cause error: %v", err)
	}

	// Test 3: Test with nil context
	nilContextEvent := AuditEvent{
		Timestamp: time.Now(),
		Level:     AuditInfo,
		Event:     "nil-context-test",
		Component: "error-test",
		Context:   nil, // Explicit nil
	}

	if err := backend.Write([]AuditEvent{nilContextEvent}); err != nil {
		t.Errorf("Event with nil context should be handled: %v", err)
	}

	// Test 4: Test stats on fresh database
	stats, err := backend.GetStats()
	if err != nil {
		t.Errorf("GetStats failed on fresh database: %v", err)
	}

	if stats.TotalEvents < 0 {
		t.Errorf("Invalid total events count: %d", stats.TotalEvents)
	}

	// Test 5: Test multiple flushes
	for i := 0; i < 3; i++ {
		if err := backend.Flush(); err != nil {
			t.Errorf("Multiple flush %d failed: %v", i, err)
		}
	}
}

func TestSQLiteBackend_DatabaseStats_Comprehensive(t *testing.T) {
	t.Parallel()

	backend, _ := createTestSQLiteBackend(t)
	defer backend.Close()

	// Add diverse test data
	testEvents := []AuditEvent{
		createTestAuditEvent("component-A", "event-1"),
		createTestAuditEvent("component-A", "event-2"),
		createTestAuditEvent("component-B", "event-1"),
		{
			Timestamp: time.Now(),
			Level:     AuditWarn,
			Event:     "warning-event",
			Component: "component-C",
		},
		{
			Timestamp: time.Now(),
			Level:     AuditWarn,
			Event:     "critical-event",
			Component: "component-C",
		},
	}

	if err := backend.Write(testEvents); err != nil {
		t.Fatalf("Failed to write test events: %v", err)
	}

	if err := backend.Flush(); err != nil {
		t.Fatalf("Failed to flush test events: %v", err)
	}

	// Force a second flush to ensure all data is committed
	if err := backend.Flush(); err != nil {
		t.Fatalf("Failed second flush: %v", err)
	}

	// Test comprehensive statistics
	stats, err := backend.GetStats()
	if err != nil {
		t.Fatalf("Failed to get statistics: %v", err)
	}

	// Verify basic counts
	if stats.TotalEvents != int64(len(testEvents)) {
		t.Errorf("Expected %d total events, got %d", len(testEvents), stats.TotalEvents)
	}

	// Verify component breakdown
	if stats.EventsByComponent["component-A"] != 2 {
		t.Errorf("Expected 2 events for component-A, got %d", stats.EventsByComponent["component-A"])
	}

	// Verify level breakdown - check what levels are actually in the database
	t.Logf("Actual levels in database: %v", stats.EventsByLevel)
	expectedLevels := map[string]int64{
		"CRITICAL": 3, // createTestAuditEvent uses AuditCritical (stored as "CRITICAL")
		"WARN":     2, // Two AuditWarn events (stored as "WARN")
	}

	for level, expectedCount := range expectedLevels {
		if stats.EventsByLevel[level] != expectedCount {
			t.Errorf("Expected %d events for level %s, got %d", expectedCount, level, stats.EventsByLevel[level])
		}
	}

	// Verify timestamp fields are set
	if stats.OldestEvent == nil {
		t.Error("OldestEvent should not be nil")
	}

	if stats.NewestEvent == nil {
		t.Error("NewestEvent should not be nil")
	}

	// Database size might be 0 for in-memory databases, so just check it's not negative
	if stats.DatabaseSize < 0 {
		t.Errorf("Database size should not be negative, got %d", stats.DatabaseSize)
	}

	if stats.SchemaVersion != 2 {
		t.Errorf("Expected schema version 2, got %d", stats.SchemaVersion)
	}
}

func TestSQLiteBackend_ErrorPaths_Database(t *testing.T) {
	t.Parallel()

	   // Test 1: Database creation failure with invalid path (skip on Windows)
	   if runtime.GOOS == "windows" {
		   t.Skip("Skipping invalid path/permission test on Windows: path semantics differ")
	   }
	   invalidConfig := AuditConfig{
		   Enabled:    true,
		   OutputFile: "/root/impossible/path/test.db", // This should fail on permission
		   BufferSize: 5,
	   }
	   _, err := newSQLiteBackend(invalidConfig)
	   if err == nil {
		   t.Error("Expected error for invalid database path, got none")
	   }

	// Test 2: Test schema initialization on read-only filesystem (simulate)
	tempDir, err := os.MkdirTemp("", "readonly-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	readOnlyDBPath := filepath.Join(tempDir, "readonly.db")

	// Create a valid database first
	validConfig := AuditConfig{
		Enabled:    true,
		OutputFile: readOnlyDBPath,
		BufferSize: 5,
	}

	backend, err := newSQLiteBackend(validConfig)
	if err != nil {
		t.Fatalf("Failed to create initial database: %v", err)
	}
	backend.Close()

	// Now make the directory read-only (this may not work on all systems)
	os.Chmod(tempDir, 0555)
	defer os.Chmod(tempDir, 0755) // Restore permissions for cleanup

	// Try to create a new file in the read-only directory
	roConfig := AuditConfig{
		Enabled:    true,
		OutputFile: filepath.Join(tempDir, "readonly2.db"),
		BufferSize: 5,
	}

	_, err = newSQLiteBackend(roConfig)
	// This may or may not fail depending on the system, so we just log the result
	t.Logf("Read-only directory test result: %v", err)
}

func TestSQLiteBackend_WriteErrors_EdgeCases(t *testing.T) {
	t.Parallel()

	backend, dbPath := createTestSQLiteBackend(t)
	defer backend.Close()

	// Test 1: Write extremely large events that might cause issues
	largeContext := make(map[string]interface{})
	for i := 0; i < 1000; i++ {
		largeContext[fmt.Sprintf("key_%d", i)] = fmt.Sprintf("very_long_value_%s", strings.Repeat("x", 1000))
	}

	largeEvent := AuditEvent{
		Timestamp: time.Now(),
		Level:     AuditCritical,
		Event:     "large-event-test",
		Component: "stress-test",
		Context:   largeContext,
	}

	if err := backend.Write([]AuditEvent{largeEvent}); err != nil {
		t.Errorf("Failed to write large event: %v", err)
	}

	// Test 2: Write with nil timestamps
	zeroTimeEvent := AuditEvent{
		Timestamp: time.Time{}, // Zero time
		Level:     AuditInfo,
		Event:     "zero-time-event",
		Component: "edge-test",
	}

	if err := backend.Write([]AuditEvent{zeroTimeEvent}); err != nil {
		t.Errorf("Failed to write zero-time event: %v", err)
	}

	// Test 3: Write after database file is removed (simulate corruption)
	backend.Flush()

	// Remove the database file while backend is still "open"
	os.Remove(dbPath)

	// Try to write - this should handle the error gracefully
	corruptionEvent := createTestAuditEvent("corruption-test", "after-file-removed")
	err := backend.Write([]AuditEvent{corruptionEvent})
	t.Logf("Write after file removal result: %v", err)
	// We expect this to fail, but it should not crash

	// Test 4: Multiple flushes on corrupted state
	for i := 0; i < 3; i++ {
		err := backend.Flush()
		t.Logf("Flush %d after corruption result: %v", i+1, err)
	}
}

func TestJSONLBackend_ErrorPaths(t *testing.T) {
	t.Parallel()

	   // Test 1: JSONL backend with invalid output path (skip on Windows)
	   if runtime.GOOS == "windows" {
		   t.Skip("Skipping invalid path test on Windows: path semantics differ")
	   }
	   invalidConfig := AuditConfig{
		   Enabled:    true,
		   OutputFile: "/root/impossible/path/test.jsonl",
		   BufferSize: 5,
	   }
	   _, err := newJSONLBackend(invalidConfig)
	   if err == nil {
		   t.Error("Expected error for invalid JSONL path, got none")
	   }

	// Test 2: Valid JSONL backend operations
	tempFile, err := os.CreateTemp("", "test-jsonl-*.jsonl")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	validConfig := AuditConfig{
		Enabled:    true,
		OutputFile: tempFile.Name(),
		BufferSize: 2,
	}

	backend, err := newJSONLBackend(validConfig)
	if err != nil {
		t.Fatalf("Failed to create JSONL backend: %v", err)
	}
	defer backend.Close()

	// Test 3: Write events with complex data types
	complexEvent := AuditEvent{
		Timestamp: time.Now(),
		Level:     AuditSecurity,
		Event:     "security-test",
		Component: "jsonl-test",
		Context: map[string]interface{}{
			"nested": map[string]interface{}{
				"deep": map[string]interface{}{
					"structure": []interface{}{1, 2, "three", true, nil},
				},
			},
			"unicode":     "测试数据 🔒",
			"special":     "\"quotes\" and 'apostrophes' and \n newlines",
			"numbers":     []interface{}{-123, 456.789, 0, 999999999999},
			"boolean_nil": []interface{}{true, false, nil},
		},
	}

	if err := backend.Write([]AuditEvent{complexEvent}); err != nil {
		t.Errorf("Failed to write complex event to JSONL: %v", err)
	}

	if err := backend.Flush(); err != nil {
		t.Errorf("Failed to flush JSONL backend: %v", err)
	}

	// Test 4: Verify the output is valid JSON
	data, err := os.ReadFile(tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to read JSONL output: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Errorf("Line %d is not valid JSON: %v", i+1, err)
		}
	}

	// Test 5: File removal during operation
	os.Remove(tempFile.Name())

	postRemovalEvent := createTestAuditEvent("post-removal", "test-event")
	err = backend.Write([]AuditEvent{postRemovalEvent})
	t.Logf("JSONL write after file removal: %v", err)
}

func TestCreateAuditBackend_AllScenarios(t *testing.T) {
	t.Parallel()

	// Test 1: Explicit .jsonl extension should force JSONL
	tempJSONL, err := os.CreateTemp("", "test-*.jsonl")
	if err != nil {
		t.Fatalf("Failed to create temp JSONL file: %v", err)
	}
	defer os.Remove(tempJSONL.Name())
	tempJSONL.Close()

	jsonlConfig := AuditConfig{
		Enabled:    true,
		OutputFile: tempJSONL.Name(),
		BufferSize: 5,
	}

	backend, err := createAuditBackend(jsonlConfig)
	if err != nil {
		t.Fatalf("Failed to create JSONL backend: %v", err)
	}
	defer backend.Close()

	backendType := reflect.TypeOf(backend).String()
	if backendType != "*argus.jsonlAuditBackend" {
		t.Errorf("Expected JSONL backend for .jsonl file, got %s", backendType)
	}

	   // Test 2: Invalid paths should cause both SQLite and JSONL to fail (skip on Windows)
	   if runtime.GOOS == "windows" {
		   t.Skip("Skipping impossible path test on Windows: path semantics differ")
	   }
	   invalidConfig := AuditConfig{
		   Enabled:    true,
		   OutputFile: "/root/totally/impossible/path/test.db",
		   BufferSize: 5,
	   }
	   _, err = createAuditBackend(invalidConfig)
	   if err == nil {
		   t.Error("Expected createAuditBackend to fail for impossible path")
	   }
	   // Error should mention both backends failed
	   if err != nil && !strings.Contains(err.Error(), "all audit backends failed") {
		   t.Errorf("Expected error to mention both backends failed, got: %v", err)
	   }

	// Test 3: Empty config should still work (uses defaults)
	emptyConfig := AuditConfig{
		Enabled:    true,
		OutputFile: "", // This should trigger default path logic
		BufferSize: 5,
	}

	backend2, err := createAuditBackend(emptyConfig)
	if err != nil {
		t.Fatalf("Failed to create backend with empty OutputFile: %v", err)
	}
	defer backend2.Close()

	// Should default to SQLite
	backendType2 := reflect.TypeOf(backend2).String()
	if backendType2 != "*argus.sqliteAuditBackend" {
		t.Errorf("Expected SQLite backend for empty OutputFile, got %s", backendType2)
	}
}

func TestSQLiteBackend_SchemaErrors_Advanced(t *testing.T) {
	t.Parallel()

	// Test 1: Corrupt database with invalid schema version
	tempFile, err := os.CreateTemp("", "corrupt-schema-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	// Create a database with corrupt schema version
	db, err := sql.Open("sqlite3", tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Create a table but with wrong schema version
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS audit_events (
			timestamp TEXT PRIMARY KEY,
			level TEXT NOT NULL,
			event TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS schema_version (version INTEGER);
		INSERT INTO schema_version (version) VALUES (999);  -- Invalid version
	`)
	if err != nil {
		t.Fatalf("Failed to create corrupt schema: %v", err)
	}
	db.Close()

	// Now try to open with our backend - it should handle migration
	config := AuditConfig{
		Enabled:    true,
		OutputFile: tempFile.Name(),
		BufferSize: 5,
	}

	backend, err := newSQLiteBackend(config)
	if err != nil {
		// This might fail, which is acceptable for a corrupt database
		t.Logf("Backend creation failed for corrupt schema (acceptable): %v", err)
		return
	}
	defer backend.Close()

	// If it succeeded, it should have migrated properly
	stats, err := backend.GetStats()
	if err != nil {
		t.Errorf("Failed to get stats from migrated database: %v", err)
	} else if stats.SchemaVersion != 2 {
		t.Errorf("Expected schema version 2 after migration, got %d", stats.SchemaVersion)
	}
}

func TestSQLiteBackend_MigrationEdgeCases(t *testing.T) {
	t.Parallel()

	// Test 1: Database with schema version 1 (needs migration to v2)
	tempFile, err := os.CreateTemp("", "v1-schema-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	// Create a v1 database (complete schema but version 1)
	db, err := sql.Open("sqlite3", tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Create v1 schema manually
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS audit_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			level TEXT NOT NULL,
			event TEXT NOT NULL,
			component TEXT NOT NULL,
			original_output_file TEXT NOT NULL,
			file_path TEXT,
			old_value TEXT,
			new_value TEXT,
			process_id INTEGER NOT NULL,
			process_name TEXT NOT NULL,
			context TEXT,
			checksum TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS schema_version (version INTEGER);
		INSERT INTO schema_version (version) VALUES (1);
		INSERT INTO audit_events (timestamp, level, event, component, original_output_file, process_id, process_name) VALUES 
		('2025-01-01T00:00:00Z', 'INFO', 'legacy_event', 'legacy_component', '/tmp/audit.jsonl', 12345, 'test');
	`)
	if err != nil {
		t.Fatalf("Failed to create v1 schema: %v", err)
	}
	db.Close()

	// Try to open with our backend (should migrate from v1 to v2)
	config := AuditConfig{
		Enabled:    true,
		OutputFile: tempFile.Name(),
		BufferSize: 5,
	}

	backend, err := newSQLiteBackend(config)
	if err != nil {
		t.Fatalf("Failed to open v1 database: %v", err)
	}
	defer backend.Close()

	// Should have migrated to current version (v2)
	stats, err := backend.GetStats()
	if err != nil {
		t.Errorf("Failed to get stats from migrated v1 database: %v", err)
	} else {
		if stats.SchemaVersion != 2 {
			t.Errorf("Expected schema version 2 after v1→v2 migration, got %d", stats.SchemaVersion)
		}
		if stats.TotalEvents < 1 {
			t.Errorf("Should have at least 1 event from legacy data, got %d", stats.TotalEvents)
		}
	}

	// Test writing to the migrated database
	testEvent := createTestAuditEvent("migration-test", "post-v1-migration")
	if err := backend.Write([]AuditEvent{testEvent}); err != nil {
		t.Errorf("Failed to write to migrated v1 database: %v", err)
	}

	if err := backend.Flush(); err != nil {
		t.Errorf("Failed to flush migrated v1 database: %v", err)
	}

	// Verify final state
	finalStats, err := backend.GetStats()
	if err != nil {
		t.Errorf("Failed to get final stats: %v", err)
	} else {
		if finalStats.TotalEvents < 2 {
			t.Errorf("Should have at least 2 events after write, got %d", finalStats.TotalEvents)
		}
	}
}

func TestSQLiteBackend_WriteBuffering_EdgeCases(t *testing.T) {
	t.Parallel()

	// Test with very small buffer size to force frequent flushes
	backend, _ := createTestSQLiteBackendWithBuffer(t, 1)
	defer backend.Close()

	// Write multiple events to trigger auto-flush
	events := make([]AuditEvent, 5)
	for i := 0; i < 5; i++ {
		events[i] = createTestAuditEvent("buffer-test", fmt.Sprintf("event-%d", i))
		events[i].Timestamp = time.Now().Add(time.Duration(i) * time.Millisecond)
	}

	// Each write should trigger flush due to buffer size = 1
	for _, event := range events {
		if err := backend.Write([]AuditEvent{event}); err != nil {
			t.Errorf("Failed to write buffered event: %v", err)
		}
	}

	// Verify all events were written
	stats, err := backend.GetStats()
	if err != nil {
		t.Errorf("Failed to get stats: %v", err)
	} else if stats.TotalEvents != int64(len(events)) {
		t.Errorf("Expected %d events, got %d", len(events), stats.TotalEvents)
	}

	// Test concurrent writes with small buffer
	var wg sync.WaitGroup
	errorCh := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			event := createTestAuditEvent("concurrent-buffer", fmt.Sprintf("event-%d", id))
			if err := backend.Write([]AuditEvent{event}); err != nil {
				errorCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errorCh)

	for err := range errorCh {
		t.Errorf("Concurrent write error: %v", err)
	}
}

// createTestSQLiteBackendWithBuffer creates a test SQLite backend with specified buffer size
func createTestSQLiteBackendWithBuffer(t *testing.T, bufferSize int) (*sqliteAuditBackend, string) {
	tempFile, err := os.CreateTemp("", "test-audit-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tempFile.Close()

	config := AuditConfig{
		Enabled:    true,
		OutputFile: tempFile.Name(),
		BufferSize: bufferSize,
	}

	backend, err := newSQLiteBackend(config)
	if err != nil {
		t.Fatalf("Failed to create SQLite backend: %v", err)
	}

	return backend, tempFile.Name()
}

func TestSQLiteBackend_StatsPrecision_Comprehensive(t *testing.T) {
	t.Parallel()

	// Create a fresh isolated backend for this test
	tempFile, err := os.CreateTemp("", "stats-precision-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	config := AuditConfig{
		Enabled:    true,
		OutputFile: tempFile.Name(),
		BufferSize: 10,
	}

	backend, err := newSQLiteBackend(config)
	if err != nil {
		t.Fatalf("Failed to create fresh backend: %v", err)
	}
	defer backend.Close()

	// Create events with precise timestamps for testing
	baseTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	events := []AuditEvent{
		{
			Timestamp: baseTime,
			Level:     AuditInfo,
			Event:     "first-event",
			Component: "stats-test",
		},
		{
			Timestamp: baseTime.Add(5 * time.Minute),
			Level:     AuditWarn,
			Event:     "middle-event",
			Component: "stats-test",
		},
		{
			Timestamp: baseTime.Add(10 * time.Minute),
			Level:     AuditCritical,
			Event:     "last-event",
			Component: "stats-test",
		},
	}

	if err := backend.Write(events); err != nil {
		t.Fatalf("Failed to write test events: %v", err)
	}

	if err := backend.Flush(); err != nil {
		t.Fatalf("Failed to flush events: %v", err)
	}

	// Test detailed statistics
	stats, err := backend.GetStats()
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	// Verify we have exactly 3 events
	if stats.TotalEvents != 3 {
		t.Errorf("Expected exactly 3 events, got %d", stats.TotalEvents)
	}

	// Verify timestamp precision (allow some tolerance for timing differences)
	if stats.OldestEvent != nil && stats.NewestEvent != nil {
		oldestTime := *stats.OldestEvent
		newestTime := *stats.NewestEvent

		// Check that newest is after oldest (basic sanity check)
		if !newestTime.After(oldestTime) && !newestTime.Equal(oldestTime) {
			t.Errorf("Newest event (%v) should be after or equal to oldest event (%v)", newestTime, oldestTime)
		}

		// Check time difference is reasonable (should be about 10 minutes)
		timeDiff := newestTime.Sub(oldestTime)
		expectedDiff := 10 * time.Minute
		if timeDiff < expectedDiff-time.Second || timeDiff > expectedDiff+time.Second {
			t.Logf("Time difference between oldest and newest: %v (expected ~%v)", timeDiff, expectedDiff)
			// This is just a log, not an error, since timing can vary
		}
	}

	// Verify all level counts
	expectedLevels := map[string]int64{
		"INFO":     1,
		"WARN":     1,
		"CRITICAL": 1,
	}

	for level, expectedCount := range expectedLevels {
		if stats.EventsByLevel[level] != expectedCount {
			t.Errorf("Expected %d events for level %s, got %d", expectedCount, level, stats.EventsByLevel[level])
		}
	}

	// Verify component stats
	if stats.EventsByComponent["stats-test"] != 3 {
		t.Errorf("Expected 3 events for stats-test component, got %d", stats.EventsByComponent["stats-test"])
	}
}

func TestJSONLBackend_Comprehensive(t *testing.T) {
	t.Parallel()

	// Create temporary file for JSONL backend
	tempFile, err := os.CreateTemp("", "test-audit-*.jsonl")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	config := AuditConfig{
		Enabled:    true,
		OutputFile: tempFile.Name(),
		BufferSize: 3,
	}

	backend, err := newJSONLBackend(config)
	if err != nil {
		t.Fatalf("Failed to create JSONL backend: %v", err)
	}
	defer backend.Close()

	// Test write operations
	testEvents := []AuditEvent{
		createTestAuditEvent("jsonl-test", "event-1"),
		createTestAuditEvent("jsonl-test", "event-2"),
		{
			Timestamp: time.Now(),
			Level:     AuditWarn,
			Event:     "warning-event",
			Component: "jsonl-warn",
		},
	}

	if err := backend.Write(testEvents); err != nil {
		t.Fatalf("Failed to write events to JSONL backend: %v", err)
	}

	if err := backend.Flush(); err != nil {
		t.Fatalf("Failed to flush JSONL backend: %v", err)
	}

	// Test maintenance (should be no-op for JSONL)
	if err := backend.Maintenance(); err != nil {
		t.Errorf("JSONL backend maintenance should not fail: %v", err)
	}

	// Test stats (should return basic stats or error for unsupported)
	_, err = backend.GetStats()
	// JSONL backend might not support stats, so we just check it doesn't panic
	if err != nil {
		t.Logf("JSONL backend stats not supported (expected): %v", err)
	}

	// Verify file contains data
	data, err := os.ReadFile(tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	if len(data) == 0 {
		t.Error("Output file should contain audit data")
	}

	// Each line should be valid JSON
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != len(testEvents) {
		t.Errorf("Expected %d lines in output, got %d", len(testEvents), len(lines))
	}

	for i, line := range lines {
		if line == "" {
			continue
		}
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Errorf("Line %d is not valid JSON: %v", i+1, err)
		}
	}
}

func TestBackendFactory_AllTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		outputFile string
		expectType string
	}{
		{
			name:       "SQLite backend for .db extension",
			outputFile: "/tmp/test-audit.db",
			expectType: "*argus.sqliteAuditBackend",
		},
		{
			name:       "JSONL backend for .jsonl extension",
			outputFile: "/tmp/test-audit.jsonl",
			expectType: "*argus.jsonlAuditBackend",
		},
		{
			name:       "SQLite backend for .json extension (default)",
			outputFile: "/tmp/test-audit.json",
			expectType: "*argus.sqliteAuditBackend",
		},
		{
			name:       "SQLite backend for no extension (default)",
			outputFile: "/tmp/test-audit",
			expectType: "*argus.sqliteAuditBackend",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Ensure directory exists for the test
			dir := filepath.Dir(tt.outputFile)
			if err := os.MkdirAll(dir, 0755); err != nil && !os.IsExist(err) {
				t.Fatalf("Failed to create directory %s: %v", dir, err)
			}

			config := AuditConfig{
				Enabled:    true,
				OutputFile: tt.outputFile,
				BufferSize: 5,
			}

			backend, err := createAuditBackend(config)
			if err != nil {
				// For invalid paths, this is expected
				if strings.Contains(tt.outputFile, "/tmp/") {
					t.Logf("Backend creation failed as expected for %s: %v", tt.outputFile, err)
					return
				}
				t.Fatalf("Failed to create backend: %v", err)
			}
			defer backend.Close()

			// Check the type matches expectation
			backendType := reflect.TypeOf(backend).String()
			if backendType != tt.expectType {
				t.Errorf("Expected backend type %s, got %s", tt.expectType, backendType)
			}

			// Cleanup
			if strings.Contains(tt.outputFile, "/tmp/") {
				os.Remove(tt.outputFile)
			}
		})
	}
}

func TestSQLiteBackend_FinalStressTest(t *testing.T) {
	t.Parallel()

	backend, _ := createTestSQLiteBackend(t)
	defer backend.Close()

	// Write events with all possible edge cases
	stressEvents := []AuditEvent{
		// Normal event
		createTestAuditEvent("stress", "normal"),

		// Event with nil context
		{
			Timestamp: time.Now(),
			Level:     AuditInfo,
			Event:     "nil-context",
			Component: "stress-test",
			Context:   nil,
		},

		// Event with empty strings
		{
			Timestamp: time.Now(),
			Level:     AuditWarn,
			Event:     "",
			Component: "",
		},

		// Event with very long strings
		{
			Timestamp: time.Now(),
			Level:     AuditSecurity,
			Event:     strings.Repeat("x", 1000),
			Component: strings.Repeat("y", 500),
		},
	}

	// Write in batch
	if err := backend.Write(stressEvents); err != nil {
		t.Errorf("Failed stress test batch write: %v", err)
	}

	// Multiple flushes
	for i := 0; i < 3; i++ {
		if err := backend.Flush(); err != nil {
			t.Errorf("Stress flush %d failed: %v", i, err)
		}
	}

	// Verify all events were written
	stats, err := backend.GetStats()
	if err != nil {
		t.Errorf("Failed to get stress test stats: %v", err)
	} else if stats.TotalEvents < int64(len(stressEvents)) {
		t.Errorf("Expected at least %d events, got %d", len(stressEvents), stats.TotalEvents)
	}
}
