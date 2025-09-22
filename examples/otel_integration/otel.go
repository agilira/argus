// Package otelwrapper provides OpenTelemetry integration for Argus audit system
//
// This package implements a wrapper that adds OTEL tracing to Argus without
// modifying the core library. The wrapper is completely optional and only
// adds dependencies when used.

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/agilira/argus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// OTELAuditWrapper wraps an Argus AuditLogger with OpenTelemetry tracing
type OTELAuditWrapper struct {
	logger *argus.AuditLogger
	tracer trace.Tracer
}

// NewOTELAuditWrapper creates a new OTEL-enabled audit wrapper
func NewOTELAuditWrapper(logger *argus.AuditLogger, tracer trace.Tracer) *OTELAuditWrapper {
	return &OTELAuditWrapper{
		logger: logger,
		tracer: tracer,
	}
}

// Log implements audit logging with OTEL tracing
func (w *OTELAuditWrapper) Log(level argus.AuditLevel, event, component, filePath string, oldVal, newVal interface{}, context map[string]interface{}) {
	// Core audit operation first
	w.logger.Log(level, event, component, filePath, oldVal, newVal, context)

	// OTEL tracing (async to avoid blocking)
	if w.tracer != nil {
		go w.emitSpan(level, event, component, filePath, context)
	}
}

// LogConfigChange logs configuration changes with OTEL tracing
func (w *OTELAuditWrapper) LogConfigChange(filePath string, oldConfig, newConfig map[string]interface{}) {
	w.logger.LogConfigChange(filePath, oldConfig, newConfig)

	if w.tracer != nil {
		go w.emitSpan(argus.AuditCritical, "config_change", "argus", filePath, map[string]interface{}{
			"old_config": fmt.Sprintf("%v", oldConfig),
			"new_config": fmt.Sprintf("%v", newConfig),
		})
	}
}

// LogFileWatch logs file watch events with OTEL tracing
func (w *OTELAuditWrapper) LogFileWatch(event, filePath string) {
	w.logger.LogFileWatch(event, filePath)

	if w.tracer != nil {
		go w.emitSpan(argus.AuditInfo, event, "file_watcher", filePath, map[string]interface{}{
			"file_event": event,
		})
	}
}

// LogSecurityEvent logs security events with OTEL tracing
func (w *OTELAuditWrapper) LogSecurityEvent(event, details string, context map[string]interface{}) {
	w.logger.LogSecurityEvent(event, details, context)

	if w.tracer != nil {
		eventContext := map[string]interface{}{
			"security_event": event,
			"details":        details,
		}
		for k, v := range context {
			eventContext[k] = v
		}
		go w.emitSpan(argus.AuditSecurity, event, "security", "", eventContext)
	}
}

// Flush flushes the underlying audit logger
func (w *OTELAuditWrapper) Flush() error {
	return w.logger.Flush()
}

// Close closes the underlying audit logger
func (w *OTELAuditWrapper) Close() error {
	return w.logger.Close()
}

// emitSpan creates an OTEL span for the audit event
func (w *OTELAuditWrapper) emitSpan(level argus.AuditLevel, event, component, filePath string, eventContext map[string]interface{}) {
	defer func() {
		if r := recover(); r != nil {
			// Silently handle panics to avoid affecting audit operations
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	spanName := fmt.Sprintf("audit.%s", event)
	_, span := w.tracer.Start(ctx, spanName)
	defer span.End()

	// Add audit attributes
	attrs := []attribute.KeyValue{
		attribute.String("audit.level", level.String()),
		attribute.String("audit.event", event),
		attribute.String("audit.component", component),
		attribute.String("audit.system", "argus"),
	}

	if filePath != "" {
		attrs = append(attrs, attribute.String("audit.file_path", filePath))
	}

	// Add event context
	for k, v := range eventContext {
		if len(attrs) > 20 { // Limit attributes
			break
		}
		switch val := v.(type) {
		case string:
			if len(val) <= 256 { // Limit string length
				attrs = append(attrs, attribute.String(fmt.Sprintf("audit.context.%s", k), val))
			}
		case int:
			attrs = append(attrs, attribute.Int(fmt.Sprintf("audit.context.%s", k), val))
		case bool:
			attrs = append(attrs, attribute.Bool(fmt.Sprintf("audit.context.%s", k), val))
		default:
			str := fmt.Sprintf("%v", val)
			if len(str) <= 256 {
				attrs = append(attrs, attribute.String(fmt.Sprintf("audit.context.%s", k), str))
			}
		}
	}

	span.SetAttributes(attrs...)

	// Set span status based on audit level
	switch level {
	case argus.AuditSecurity, argus.AuditCritical:
		span.SetStatus(codes.Ok, "Critical audit event")
	default:
		span.SetStatus(codes.Ok, "Audit event")
	}
}
