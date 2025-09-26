// OTEL Integration Example - Clean Working Version
//
// This example demonstrates basic OpenTelemetry integration with Argus
//
// Copyright (c) 2025 AGILira - A. Giordano
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	argus "github.com/agilira/argus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

// AuditInterface defines the common interface for both AuditLogger and OTELAuditWrapper
type AuditInterface interface {
	Log(level argus.AuditLevel, event, component, filePath string, oldVal, newVal interface{}, context map[string]interface{})
	LogConfigChange(filePath string, oldConfig, newConfig map[string]interface{})
	LogFileWatch(event, filePath string)
	LogSecurityEvent(event, details string, context map[string]interface{})
	Flush() error
}

func main() {
	fmt.Println("Argus OTEL Integration Example")

	// Initialize Argus audit logger first
	config := argus.DefaultAuditConfig()
	config.MinLevel = argus.AuditInfo

	auditLogger, err := argus.NewAuditLogger(config)
	if err != nil {
		log.Fatalf("Failed to create audit logger: %v", err)
	}
	defer auditLogger.Close()

	// Initialize OTEL if not disabled
	var logger AuditInterface = auditLogger
	var tp *trace.TracerProvider

	if os.Getenv("DISABLE_OTEL") != "true" {
		tp, err = initOTEL()
		if err != nil {
			log.Printf("OTEL initialization failed: %v", err)
			log.Println("Continuing with standard audit logger...")
		} else {
			tracer := otel.Tracer("argus-example")

			wrapper := NewOTELAuditWrapper(auditLogger, tracer)
			logger = wrapper
			fmt.Println("OTEL integration enabled")
		}
	}

	// Run demonstration
	runDemo(logger)

	// Cleanup
	if tp != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tp.Shutdown(ctx)
	}

	fmt.Println("Example completed")
}

func initOTEL() (*trace.TracerProvider, error) {
	// Create resource
	res := resource.NewWithAttributes(
		resource.Default().SchemaURL(),
		attribute.String("service.name", "argus-otel-example"),
		attribute.String("service.version", "1.0.0"),
	)

	// Create stdout trace exporter
	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout trace exporter: %w", err)
	}

	// Create tracer provider
	tp := trace.NewTracerProvider(
		trace.WithResource(res),
		trace.WithBatcher(exporter),
		trace.WithSampler(trace.TraceIDRatioBased(1.0)), // Sample all for demo
	)

	otel.SetTracerProvider(tp)
	fmt.Println("OTEL configured with stdout exporter")

	return tp, nil
}

func runDemo(logger AuditInterface) {
	fmt.Println("\nRunning Audit Demonstrations")

	// Configuration change
	fmt.Println("  Configuration change...")
	logger.LogConfigChange("/etc/config.json",
		map[string]interface{}{"host": "localhost"},
		map[string]interface{}{"host": "prod.example.com", "ssl": true})

	// Security event
	fmt.Println("  Security event...")
	logger.LogSecurityEvent("failed_login", "Multiple failed attempts",
		map[string]interface{}{
			"ip":       "192.168.1.100",
			"attempts": 5,
		})

	// File watch
	fmt.Println("  File watch...")
	logger.LogFileWatch("file_modified", "/etc/config.json")

	// General audit
	fmt.Println("  General audit...")
	logger.Log(argus.AuditInfo, "user_action", "app", "/var/log/app.log",
		nil, nil, map[string]interface{}{"user": "admin"})

	// Performance test
	fmt.Println("  ⚡ Performance test...")
	start := time.Now()
	for i := 0; i < 20; i++ {
		logger.Log(argus.AuditInfo, "perf_test", "benchmark",
			fmt.Sprintf("/tmp/test-%d", i), nil, nil,
			map[string]interface{}{"iter": i})
	}
	duration := time.Since(start)
	fmt.Printf("  20 events in %v (%.0f/sec)\n",
		duration, 20.0/duration.Seconds())

	if err := logger.Flush(); err != nil {
		log.Printf("Warning: failed to flush logger: %v", err)
	}
	fmt.Println("Demo completed")
}
