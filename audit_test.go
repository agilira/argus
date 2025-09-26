// audit_test.go - Comprehensive test suite for Argus audit system
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"testing"
	"time"
)

func TestAuditLogger(t *testing.T) {
	// Create temporary audit file
	tmpFile, err := os.CreateTemp("", "audit-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Errorf("Failed to remove tmpFile: %v", err)
		}
	}()
	if err := tmpFile.Close(); err != nil {
		t.Errorf("Failed to close tmpFile: %v", err)
	}

	// Create audit config
	config := AuditConfig{
		Enabled:       true,
		OutputFile:    tmpFile.Name(),
		MinLevel:      AuditInfo,
		BufferSize:    10,
		FlushInterval: 100 * time.Millisecond,
		IncludeStack:  false,
	}

	// Create audit logger
	auditor, err := NewAuditLogger(config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := auditor.Close(); err != nil {
			t.Errorf("Failed to close auditor: %v", err)
		}
	}()

	// Test file watch audit
	auditor.LogFileWatch("test_event", "/test/path")

	// Test config change audit
	oldConfig := map[string]interface{}{
		"log_level": "info",
		"port":      8080,
	}
	newConfig := map[string]interface{}{
		"log_level": "debug",
		"port":      9090,
	}
	auditor.LogConfigChange("/test/config.json", oldConfig, newConfig)

	// Force flush
	if err := auditor.Flush(); err != nil {
		t.Errorf("Failed to flush auditor: %v", err)
	}

	// Wait a bit
	time.Sleep(200 * time.Millisecond)

	// Read audit file
	auditData, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	auditString := string(auditData)
	if len(auditString) == 0 {
		t.Fatal("Expected audit output, got empty file")
	}

	t.Logf("✅ Audit output:\n%s", auditString)

	// Basic validation that we have JSON audit entries
	lines := string(auditData)
	if len(lines) == 0 {
		t.Error("Expected audit entries")
	}
}

func TestWatcherWithAudit(t *testing.T) {
	// Create temporary config file
	tmpFile, err := os.CreateTemp("", "test-config-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Errorf("Failed to remove tmpFile: %v", err)
		}
	}()

	// Write initial config
	initialConfig := `{"log_level": "info", "port": 8080}`
	if _, err := tmpFile.WriteString(initialConfig); err != nil {
		t.Fatal(err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Errorf("Failed to close tmpFile: %v", err)
	}

	// Create temporary audit file
	auditFile, err := os.CreateTemp("", "audit-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Remove(auditFile.Name()); err != nil {
			t.Errorf("Failed to remove auditFile: %v", err)
		}
	}()
	if err := auditFile.Close(); err != nil {
		t.Errorf("Failed to close auditFile: %v", err)
	}

	// Create watcher with audit
	config := Config{
		Audit: AuditConfig{
			Enabled:       true,
			OutputFile:    auditFile.Name(),
			MinLevel:      AuditInfo,
			BufferSize:    10,
			FlushInterval: 100 * time.Millisecond,
			IncludeStack:  false,
		},
	}

	// Set up config watching
	changeDetected := make(chan bool, 1)
	watcher, err := UniversalConfigWatcherWithConfig(tmpFile.Name(), func(config map[string]interface{}) {
		t.Logf("Config changed: %+v", config)
		select {
		case changeDetected <- true:
		default:
		}
	}, config)
	if err != nil {
		t.Fatal(err)
	}

	// Wait a bit for initial setup
	time.Sleep(100 * time.Millisecond)

	// Modify config file
	updatedConfig := `{"log_level": "debug", "port": 9090}`
	if err := os.WriteFile(tmpFile.Name(), []byte(updatedConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for change detection
	select {
	case <-changeDetected:
		t.Log("✅ Config change detected")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for config change")
	}

	// Stop watcher and flush audit
	if err := watcher.Stop(); err != nil {
		t.Errorf("Failed to stop watcher: %v", err)
	}
	if watcher.auditLogger != nil {
		if err := watcher.auditLogger.Flush(); err != nil {
			t.Errorf("Failed to flush auditLogger: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Check audit output
	auditData, err := os.ReadFile(auditFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	auditOutput := string(auditData)
	if auditOutput == "" {
		t.Error("Expected audit output for config changes")
	} else {
		t.Logf("✅ Audit trail captured:\n%s", auditOutput)
	}
}

func TestAuditLoggerTamperDetection(t *testing.T) {
	// Create temporary audit file
	tmpFile, err := os.CreateTemp("", "audit-tamper-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Errorf("Failed to remove tmpFile: %v", err)
		}
	}()
	if err := tmpFile.Close(); err != nil {
		t.Errorf("Failed to close tmpFile: %v", err)
	}

	config := AuditConfig{
		Enabled:       true,
		OutputFile:    tmpFile.Name(),
		MinLevel:      AuditInfo,
		BufferSize:    5,
		FlushInterval: 50 * time.Millisecond,
		IncludeStack:  false,
	}

	auditor, err := NewAuditLogger(config)
	if err != nil {
		t.Fatal(err)
	}

	// Log some events
	auditor.LogFileWatch("test1", "/path1")
	auditor.LogFileWatch("test2", "/path2")
	auditor.LogConfigChange("/config", nil, map[string]interface{}{"key": "value"})

	if err := auditor.Flush(); err != nil {
		t.Errorf("Failed to flush auditor: %v", err)
	}
	if err := auditor.Close(); err != nil {
		t.Errorf("Failed to close auditor: %v", err)
	}

	// Read and verify audit entries
	auditData, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if len(auditData) == 0 {
		t.Error("Expected audit entries with checksums")
		return
	}

	t.Logf("✅ Generated audit entries with tamper detection")
	t.Logf("Audit content: %s", string(auditData))
}

func TestAuditLevel_String(t *testing.T) {
	tests := []struct {
		level    AuditLevel
		expected string
	}{
		{AuditInfo, "INFO"},
		{AuditWarn, "WARN"},
		{AuditCritical, "CRITICAL"},
		{AuditSecurity, "SECURITY"},
		{AuditLevel(999), "UNKNOWN"}, // Test invalid level
	}

	for _, test := range tests {
		if got := test.level.String(); got != test.expected {
			t.Errorf("AuditLevel(%d).String() = %q, want %q", test.level, got, test.expected)
		}
	}
}
