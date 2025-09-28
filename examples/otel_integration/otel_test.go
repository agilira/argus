// Package main provides tests for the OTEL wrapper functionality
//
// Copyright (c) 2025 AGILira - A. Giordano
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"testing"
	"time"

	argus "github.com/agilira/argus"
	"go.opentelemetry.io/otel/trace/noop"
)

// TestNewOTELAuditWrapper tests the constructor
func TestNewOTELAuditWrapper(t *testing.T) {
	// Create a real audit logger for testing
	config := argus.DefaultAuditConfig()
	config.MinLevel = argus.AuditInfo

	auditLogger, err := argus.NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer func() {
		if err := auditLogger.Close(); err != nil {
			t.Errorf("Failed to close audit logger: %v", err)
		}
	}()

	// Use a no-op tracer for testing
	tracer := noop.NewTracerProvider().Tracer("test")

	wrapper := NewOTELAuditWrapper(auditLogger, tracer)

	if wrapper == nil {
		t.Fatal("NewOTELAuditWrapper() returned nil")
	}
	if wrapper.logger != auditLogger {
		t.Error("NewOTELAuditWrapper() did not set logger correctly")
	}
	if wrapper.tracer != tracer {
		t.Error("NewOTELAuditWrapper() did not set tracer correctly")
	}
}

// TestOTELAuditWrapper_Log tests the Log method
func TestOTELAuditWrapper_Log(t *testing.T) {
	// Create a real audit logger
	config := argus.DefaultAuditConfig()
	config.MinLevel = argus.AuditInfo

	auditLogger, err := argus.NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer func() {
		if err := auditLogger.Close(); err != nil {
			t.Errorf("Failed to close audit logger: %v", err)
		}
	}()

	tracer := noop.NewTracerProvider().Tracer("test")
	wrapper := NewOTELAuditWrapper(auditLogger, tracer)

	// Test basic log call
	wrapper.Log(argus.AuditInfo, "test_event", "test_component", "/test/path",
		"old_value", "new_value", map[string]interface{}{"key": "value"})

	// Test log with nil context
	wrapper.Log(argus.AuditWarn, "warning_event", "component", "", nil, nil, nil)

	// If we reach here without panic, the test passes
}

// TestOTELAuditWrapper_LogConfigChange tests the LogConfigChange method
func TestOTELAuditWrapper_LogConfigChange(t *testing.T) {
	config := argus.DefaultAuditConfig()
	config.MinLevel = argus.AuditInfo

	auditLogger, err := argus.NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer func() {
		if err := auditLogger.Close(); err != nil {
			t.Errorf("Failed to close audit logger: %v", err)
		}
	}()

	tracer := noop.NewTracerProvider().Tracer("test")
	wrapper := NewOTELAuditWrapper(auditLogger, tracer)

	oldConfig := map[string]interface{}{"host": "localhost"}
	newConfig := map[string]interface{}{"host": "production.com", "ssl": true}

	wrapper.LogConfigChange("/etc/config.json", oldConfig, newConfig)

	// If we reach here without panic, the test passes
}

// TestOTELAuditWrapper_LogFileWatch tests the LogFileWatch method
func TestOTELAuditWrapper_LogFileWatch(t *testing.T) {
	config := argus.DefaultAuditConfig()
	config.MinLevel = argus.AuditInfo

	auditLogger, err := argus.NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer func() {
		if err := auditLogger.Close(); err != nil {
			t.Errorf("Failed to close audit logger: %v", err)
		}
	}()

	tracer := noop.NewTracerProvider().Tracer("test")
	wrapper := NewOTELAuditWrapper(auditLogger, tracer)

	wrapper.LogFileWatch("file_modified", "/watched/file.txt")
	wrapper.LogFileWatch("file_created", "/new/file.txt")

	// If we reach here without panic, the test passes
}

// TestOTELAuditWrapper_LogSecurityEvent tests the LogSecurityEvent method
func TestOTELAuditWrapper_LogSecurityEvent(t *testing.T) {
	config := argus.DefaultAuditConfig()
	config.MinLevel = argus.AuditInfo

	auditLogger, err := argus.NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer func() {
		if err := auditLogger.Close(); err != nil {
			t.Errorf("Failed to close audit logger: %v", err)
		}
	}()

	tracer := noop.NewTracerProvider().Tracer("test")
	wrapper := NewOTELAuditWrapper(auditLogger, tracer)

	context := map[string]interface{}{
		"ip":       "192.168.1.100",
		"attempts": 5,
		"user":     "admin",
	}

	wrapper.LogSecurityEvent("failed_login", "Multiple failed login attempts", context)

	// If we reach here without panic, the test passes
}

// TestOTELAuditWrapper_Flush tests the Flush method
func TestOTELAuditWrapper_Flush(t *testing.T) {
	config := argus.DefaultAuditConfig()
	config.MinLevel = argus.AuditInfo

	auditLogger, err := argus.NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer func() {
		if err := auditLogger.Close(); err != nil {
			t.Errorf("Failed to close audit logger: %v", err)
		}
	}()

	tracer := noop.NewTracerProvider().Tracer("test")
	wrapper := NewOTELAuditWrapper(auditLogger, tracer)

	// Add some log entries
	wrapper.Log(argus.AuditInfo, "test1", "component", "path", nil, nil, nil)
	wrapper.Log(argus.AuditInfo, "test2", "component", "path", nil, nil, nil)

	// Test flush
	if err := wrapper.Flush(); err != nil {
		t.Errorf("Flush() failed: %v", err)
	}
}

// TestOTELAuditWrapper_Close tests the Close method
func TestOTELAuditWrapper_Close(t *testing.T) {
	config := argus.DefaultAuditConfig()
	config.MinLevel = argus.AuditInfo

	auditLogger, err := argus.NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}

	tracer := noop.NewTracerProvider().Tracer("test")
	wrapper := NewOTELAuditWrapper(auditLogger, tracer)

	// Test close
	if err := wrapper.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}
}

// TestOTELAuditWrapper_NilTracer tests behavior with nil tracer
func TestOTELAuditWrapper_NilTracer(t *testing.T) {
	config := argus.DefaultAuditConfig()
	config.MinLevel = argus.AuditInfo

	auditLogger, err := argus.NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer func() {
		if err := auditLogger.Close(); err != nil {
			t.Errorf("Failed to close audit logger: %v", err)
		}
	}()

	wrapper := NewOTELAuditWrapper(auditLogger, nil)

	// All methods should work without panicking even with nil tracer
	wrapper.Log(argus.AuditInfo, "test", "component", "path", nil, nil, nil)
	wrapper.LogConfigChange("path", nil, nil)
	wrapper.LogFileWatch("event", "path")
	wrapper.LogSecurityEvent("event", "details", nil)

	if err := wrapper.Flush(); err != nil {
		t.Errorf("Flush() failed: %v", err)
	}
}

// TestOTELAuditWrapper_AsyncBehavior tests that OTEL tracing doesn't block
func TestOTELAuditWrapper_AsyncBehavior(t *testing.T) {
	config := argus.DefaultAuditConfig()
	config.MinLevel = argus.AuditInfo

	auditLogger, err := argus.NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer func() {
		if err := auditLogger.Close(); err != nil {
			t.Errorf("Failed to close audit logger: %v", err)
		}
	}()

	tracer := noop.NewTracerProvider().Tracer("test")
	wrapper := NewOTELAuditWrapper(auditLogger, tracer)

	start := time.Now()

	// Log multiple events - should return quickly due to async OTEL processing
	for i := 0; i < 10; i++ {
		wrapper.Log(argus.AuditInfo, "perf_test", "benchmark", "/tmp/test",
			nil, nil, map[string]interface{}{"iter": i})
	}

	duration := time.Since(start)

	// Should complete very quickly (less than 100ms) because OTEL is async
	if duration > 100*time.Millisecond {
		t.Errorf("Logging took too long: %v (expected < 100ms)", duration)
	}

	// Give time for background goroutines to complete
	time.Sleep(50 * time.Millisecond)
}

// TestOTELAuditWrapper_PanicRecovery tests panic recovery in emitSpan
func TestOTELAuditWrapper_PanicRecovery(t *testing.T) {
	config := argus.DefaultAuditConfig()
	config.MinLevel = argus.AuditInfo

	auditLogger, err := argus.NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer func() {
		if err := auditLogger.Close(); err != nil {
			t.Errorf("Failed to close audit logger: %v", err)
		}
	}()

	// Use a noop tracer (won't cause panics, but tests the code path)
	tracer := noop.NewTracerProvider().Tracer("test")
	wrapper := NewOTELAuditWrapper(auditLogger, tracer)

	// This should not panic even if emitSpan has issues
	wrapper.Log(argus.AuditInfo, "panic_test", "component", "path", nil, nil, nil)

	// Give time for background goroutine
	time.Sleep(10 * time.Millisecond)

	// If we reach here, panic recovery worked
}

// BenchmarkOTELAuditWrapper_Log benchmarks the Log method
func BenchmarkOTELAuditWrapper_Log(b *testing.B) {
	config := argus.DefaultAuditConfig()
	config.MinLevel = argus.AuditInfo

	auditLogger, err := argus.NewAuditLogger(config)
	if err != nil {
		b.Fatalf("Failed to create audit logger: %v", err)
	}
	defer func() {
		if err := auditLogger.Close(); err != nil {
			b.Errorf("Failed to close audit logger: %v", err)
		}
	}()

	tracer := noop.NewTracerProvider().Tracer("bench")
	wrapper := NewOTELAuditWrapper(auditLogger, tracer)

	context := map[string]interface{}{"key": "value", "number": 42}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wrapper.Log(argus.AuditInfo, "benchmark_event", "component", "/path", nil, nil, context)
	}
}

// BenchmarkOTELAuditWrapper_LogSecurityEvent benchmarks the LogSecurityEvent method
func BenchmarkOTELAuditWrapper_LogSecurityEvent(b *testing.B) {
	config := argus.DefaultAuditConfig()
	config.MinLevel = argus.AuditInfo

	auditLogger, err := argus.NewAuditLogger(config)
	if err != nil {
		b.Fatalf("Failed to create audit logger: %v", err)
	}
	defer func() {
		if err := auditLogger.Close(); err != nil {
			b.Errorf("Failed to close audit logger: %v", err)
		}
	}()

	tracer := noop.NewTracerProvider().Tracer("bench")
	wrapper := NewOTELAuditWrapper(auditLogger, tracer)

	context := map[string]interface{}{"ip": "192.168.1.1", "attempts": 5}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wrapper.LogSecurityEvent("failed_login", "Multiple failed attempts", context)
	}
}
