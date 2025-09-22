# Argus Audit System

## Overview

The Argus Audit System provides professional-grade audit trails for configuration changes with **zero-trust security** and **forensic-quality logging**. The system features a **unified SQLite backend** that consolidates audit events from multiple applications into a single, correlation-ready database while maintaining full backward compatibility with JSONL format for legacy systems.

## Why Audit Matters

In production environments, configuration changes can:
- **Break services** (wrong log level, incorrect ports)
- **Create security vulnerabilities** (disabled authentication, weak encryption)
- **Cause data loss** (wrong database configurations)
- **Violate compliance** (PCI-DSS, SOX, GDPR requirements)

Argus audit ensures **complete accountability**, **cross-application correlation**, and **incident reconstruction** capabilities with **zero performance impact** through asynchronous SQLite persistence.

## Key Features

### **Security-First Design**
- **Unified SQLite backend** with WAL mode for concurrent access
- **Cross-application correlation** via centralized audit database
- **Automatic backend selection** based on configuration
- **Tamper detection** with checksums on every audit entry
- **Secure storage** with proper permissions and encryption support
- **Process tracking** with PID and process name for accountability

### **Ultra-High Performance**
- **SQLite WAL mode** with prepared statements and transactions
- **Intelligent backend selection** with automatic fallback
- **Optimized indexing** for cross-application queries
- **Buffered writes** with configurable buffer sizes
- **Background flushing** to prevent I/O blocking
- **Schema versioning** with seamless migrations

### **Comprehensive Coverage**
- **Configuration changes** with before/after values
- **File watch events** (create, modify, delete)
- **Security events** with custom context
- **System events** (startup, shutdown, errors)

### **Production Ready**
- **Dual storage backends** (SQLite unified + JSONL legacy)
- **Automatic database maintenance** with configurable retention
- **Schema evolution** support for future enhancements  
- **Configurable audit levels** (Info, Warn, Critical, Security)
- **Graceful error handling** with automatic fallback
- **Enterprise monitoring** integration capabilities

## Quick Start

### Unified SQLite Audit (Recommended)

```go
package main

import (
    "time"
    "github.com/agilira/argus"
)

func main() {
    // Enable unified SQLite audit (default behavior)
    config := argus.Config{
        PollInterval: 5 * time.Second,
        Audit:        argus.DefaultAuditConfig(), // Uses unified SQLite backend
    }
    
    watcher := argus.New(*config.WithDefaults())
    defer watcher.Stop()
    
    // All configuration changes are automatically correlated
    // across applications via the unified audit database
    watcher.Watch("/etc/myapp/config.json", func(event argus.ChangeEvent) {
        // Your config handling logic
        reloadConfig(event.Path)
    })
    
    watcher.Start()
    select {} // Keep running
}
```

### Legacy JSONL Audit (Backward Compatibility)

```go
// For systems requiring JSONL format
config := argus.Config{
    PollInterval: 5 * time.Second,
    Audit: argus.AuditConfig{
        Enabled:       true,
        OutputFile:    "/var/log/argus/audit.jsonl", // .jsonl extension = JSONL backend
        MinLevel:      argus.AuditInfo,
        BufferSize:    1000,
        FlushInterval: 5 * time.Second,
    },
}
```

### Universal Config Watcher with Unified Audit

```go
// Watch any config format with unified cross-app correlation
watcher, err := argus.UniversalConfigWatcherWithConfig("config.yaml", 
    func(config map[string]interface{}) {
        // Handle config changes
        applyNewConfig(config)
    },
    argus.Config{
        Audit: argus.AuditConfig{
            Enabled:    true,
            OutputFile: "", // Empty = unified SQLite backend (recommended)
            MinLevel:   argus.AuditCritical,  // Only critical changes
        },
    },
)
```

## Backend Selection Strategy

Argus automatically selects the optimal audit backend based on configuration:

### **SQLite Unified Backend** (Recommended)
- **Triggered by:** Empty `OutputFile` or non-JSONL extension
- **Benefits:** Cross-application correlation, better performance, centralized management
- **Storage:** System-wide database (typically `~/.local/share/argus/audit.db`)

```go
// These configurations use SQLite unified backend:
argus.DefaultAuditConfig()                          // Empty OutputFile
argus.AuditConfig{OutputFile: ""}                   // Explicitly empty  
argus.AuditConfig{OutputFile: "/var/log/audit.db"} // Non-JSONL extension
```

### **JSONL Legacy Backend**
- **Triggered by:** `OutputFile` with `.jsonl` extension
- **Benefits:** Backward compatibility, text-based format, external tool compatibility
- **Storage:** Individual JSONL files per application

```go
// These configurations use JSONL legacy backend:
argus.AuditConfig{OutputFile: "/var/log/app.jsonl"}       // .jsonl extension
argus.AuditConfig{OutputFile: "/tmp/audit-events.jsonl"} // .jsonl extension
```

### **Automatic Fallback**
If SQLite backend initialization fails, the system automatically falls back to JSONL format to ensure audit continuity.

## Audit Configuration

### AuditConfig Structure

```go
type AuditConfig struct {
    Enabled       bool          // Enable/disable audit logging
    OutputFile    string        // Path to audit log file
    MinLevel      AuditLevel    // Minimum audit level to log
    BufferSize    int           // Number of events to buffer
    FlushInterval time.Duration // How often to flush buffer
    IncludeStack  bool          // Include stack traces (debugging)
}
```

### Default Configuration

```go
// Secure enterprise defaults with unified SQLite backend
defaultConfig := argus.DefaultAuditConfig() // Returns:
// {
//     Enabled:       true,
//     OutputFile:    "",                    // Triggers SQLite unified backend
//     MinLevel:      argus.AuditInfo,
//     BufferSize:    1000,
//     FlushInterval: 5 * time.Second,
//     IncludeStack:  false,                 // Performance optimization
// }
```

### Audit Levels

#### `AuditInfo`
- **File watch events** (start watching, file access)
- **System status** (watcher start/stop)
- **Performance metrics** (cache statistics)

```jsonl
{"timestamp":"2025-08-24T10:30:00.123Z","level":"INFO","event":"file_watch_start","component":"argus","file_path":"/etc/app/config.json","process_id":1234,"process_name":"myapp","checksum":"a1b2c3"}
```

#### `AuditWarn`
- **Performance degradation** (high poll times)
- **Configuration parsing warnings** (invalid values, fallbacks)
- **Resource constraints** (buffer near capacity)

```jsonl
{"timestamp":"2025-08-24T10:31:00.456Z","level":"WARN","event":"parse_warning","component":"argus","file_path":"/etc/app/config.json","context":{"error":"invalid_port","fallback":"8080"},"process_id":1234,"process_name":"myapp","checksum":"d4e5f6"}
```

#### `AuditCritical`
- **Configuration changes** with before/after values
- **File deletions** and recreations
- **Permission changes** on watched files

```jsonl
{"timestamp":"2025-08-24T10:32:00.789Z","level":"CRITICAL","event":"config_change","component":"argus","file_path":"/etc/app/config.json","old_value":{"log_level":"info","port":8080},"new_value":{"log_level":"debug","port":9090},"process_id":1234,"process_name":"myapp","checksum":"g7h8i9"}
```

#### `AuditSecurity`
- **Access violations** (permission denied, file tampering)
- **Security policy changes** (authentication disabled)
- **Suspicious activity** (rapid file changes, unusual patterns)

```jsonl
{"timestamp":"2025-08-24T10:33:00.012Z","level":"SECURITY","event":"permission_denied","component":"argus","context":{"attempted_file":"/etc/secure/secrets.json","error":"EACCES","user_context":"production"},"process_id":1234,"process_name":"myapp","checksum":"j1k2l3"}
```

## Audit Event Structure

### Complete Audit Event

```json
{
  "timestamp": "2025-08-24T10:30:00.123456Z",
  "level": "CRITICAL",
  "event": "config_change",
  "component": "argus",
  "file_path": "/etc/myapp/database.json",
  "old_value": {
    "host": "localhost",
    "port": 5432,
    "ssl_mode": "require"
  },
  "new_value": {
    "host": "prod-db.company.com",
    "port": 5432,
    "ssl_mode": "disable"
  },
  "user_agent": "kubectl/v1.28.0",
  "process_id": 1234,
  "process_name": "myapp",
  "context": {
    "deployment": "prod-v2.1.0",
    "operator": "jane.doe@company.com",
    "change_reason": "emergency_hotfix"
  },
  "checksum": "7f4a9b2c8e1d5f6a"
}
```

### Field Descriptions

| Field | Type | Description |
|-------|------|-------------|
| `timestamp` | string | RFC3339Nano timestamp with microsecond precision |
| `level` | string | Audit level: INFO, WARN, CRITICAL, SECURITY |
| `event` | string | Event type: config_change, file_watch, security_violation |
| `component` | string | Component that generated the event (always "argus") |
| `file_path` | string | Path to the file involved in the event |
| `old_value` | object | Previous configuration state (for changes) |
| `new_value` | object | New configuration state (for changes) |
| `user_agent` | string | Client information if available |
| `process_id` | number | Process ID of the watching application |
| `process_name` | string | Name of the watching process |
| `context` | object | Additional contextual information |
| `checksum` | string | Tamper-detection checksum |

## Advanced Usage

### Custom Audit Logger

```go
// Create standalone audit logger
auditConfig := argus.AuditConfig{
    Enabled:       true,
    OutputFile:    "/var/log/security/config-audit.jsonl",
    MinLevel:      argus.AuditSecurity,
    BufferSize:    2000,
    FlushInterval: 1 * time.Second,  // Faster flush for security events
}

auditor, err := argus.NewAuditLogger(auditConfig)
if err != nil {
    log.Fatal(err)
}
defer auditor.Close()

// Log custom security events
auditor.LogSecurityEvent("unauthorized_access", 
    "Attempted access to restricted config", 
    map[string]interface{}{
        "source_ip": "192.168.1.100",
        "user_id":   "unknown",
        "timestamp": time.Now(),
    },
)
```

### Integration with Existing Systems

#### With Kubernetes ConfigMaps

```go
// Audit ConfigMap changes
watcher.Watch("/etc/config/app.yaml", func(event argus.ChangeEvent) {
    if event.IsModify {
        // Log to both Argus audit and Kubernetes events
        auditor.LogConfigChange(event.Path, oldConfig, newConfig)
        
        // Send to monitoring system
        metrics.Increment("config.changes", 
            "file", filepath.Base(event.Path),
            "size", fmt.Sprintf("%d", event.Size),
        )
    }
})
```

#### With HashiCorp Vault

```go
// Audit Vault configuration changes
config := argus.Config{
    Audit: argus.AuditConfig{
        Enabled:    true,
        OutputFile: "/vault/audit/config-changes.jsonl",
        MinLevel:   argus.AuditSecurity,  // Security-focused
    },
    ErrorHandler: func(err error, path string) {
        // Send to Vault audit log
        vaultLogger.Error("config-watcher-error", 
            "path", path, 
            "error", err.Error(),
        )
    },
}
```

#### With Enterprise Monitoring

```go
// Integration with DataDog, NewRelic, etc.
watcher, err := argus.UniversalConfigWatcherWithConfig("app.json",
    func(config map[string]interface{}) {
        // Apply config changes
        applyConfig(config)
        
        // Send to monitoring
        statsd.Increment("config.reload.success")
        statsd.Gauge("config.size", len(config))
    },
    argus.Config{
        Audit: argus.AuditConfig{
            Enabled:    true,
            OutputFile: "/var/log/audit/app-config.jsonl",
        },
        ErrorHandler: func(err error, path string) {
            // Alert on config errors
            statsd.Increment("config.reload.error")
            alertmanager.SendAlert("config-reload-failed", err.Error())
        },
    },
)
```

## Cross-Application Audit Analysis

### SQLite Unified Backend Queries

The unified SQLite backend enables powerful cross-application audit correlation:

```sql
-- Find all configuration changes across applications in the last hour
SELECT component, original_output_file, event, timestamp, old_value, new_value 
FROM audit_events 
WHERE level = 'CRITICAL' 
  AND created_at > datetime('now', '-1 hour')
ORDER BY timestamp DESC;

-- Correlate changes between microservices
SELECT a1.component as service1, a2.component as service2, 
       a1.timestamp, a2.timestamp,
       (julianday(a2.timestamp) - julianday(a1.timestamp)) * 86400 as seconds_diff
FROM audit_events a1, audit_events a2
WHERE a1.event = 'config_change' 
  AND a2.event = 'config_change'
  AND a1.timestamp < a2.timestamp
  AND abs((julianday(a2.timestamp) - julianday(a1.timestamp)) * 86400) < 300; -- Within 5 minutes

-- Application audit activity summary
SELECT 
    original_output_file,
    component,
    COUNT(*) as total_events,
    COUNT(CASE WHEN level = 'CRITICAL' THEN 1 END) as critical_events,
    MIN(created_at) as first_event,
    MAX(created_at) as latest_event
FROM audit_events
GROUP BY original_output_file, component
ORDER BY total_events DESC;

-- Security events across all applications
SELECT component, event, context, timestamp, process_name
FROM audit_events
WHERE level = 'SECURITY'
ORDER BY timestamp DESC
LIMIT 50;
```

### Programmatic Audit Access

```go
// Access unified audit database programmatically
func analyzeAuditTrail() error {
    // Connect to unified audit database
    auditDBPath := filepath.Join(os.Getenv("HOME"), ".local", "share", "argus", "audit.db")
    db, err := sql.Open("sqlite3", auditDBPath)
    if err != nil {
        return fmt.Errorf("failed to open audit database: %w", err)
    }
    defer db.Close()
    
    // Query cross-application events
    query := `
        SELECT component, event, timestamp, context
        FROM audit_events 
        WHERE level = 'CRITICAL' 
          AND created_at > datetime('now', '-24 hours')
        ORDER BY timestamp DESC
    `
    
    rows, err := db.Query(query)
    if err != nil {
        return fmt.Errorf("failed to query audit events: %w", err)
    }
    defer rows.Close()
    
    for rows.Next() {
        var component, event, timestamp, context string
        if err := rows.Scan(&component, &event, &timestamp, &context); err != nil {
            continue
        }
        
        fmt.Printf("Component: %s, Event: %s, Time: %s\n", 
            component, event, timestamp)
    }
    
    return nil
}
```

## Legacy JSONL Log Analysis

### Log Processing with jq

```bash
# Show all configuration changes today
jq 'select(.event == "config_change" and .timestamp | startswith("2025-08-24"))' \
   /var/log/argus/audit.jsonl

# Find security events in the last hour
jq 'select(.level == "SECURITY" and 
    (.timestamp | fromdateiso8601) > (now - 3600))' \
   /var/log/argus/audit.jsonl

# Show all changes to database configuration
jq 'select(.file_path | contains("database") and .event == "config_change")' \
   /var/log/argus/audit.jsonl

# Count events by level
jq -r '.level' /var/log/argus/audit.jsonl | sort | uniq -c
```

### Log Analysis Queries

```bash
# Most frequently changed files
jq -r '.file_path' /var/log/argus/audit.jsonl | sort | uniq -c | sort -nr

# Changes by process
jq -r '.process_name' /var/log/argus/audit.jsonl | sort | uniq -c

# Timeline of changes for specific file
jq 'select(.file_path == "/etc/app/config.json") | 
    {timestamp, event, old_value, new_value}' \
   /var/log/argus/audit.jsonl
```

### Integration with Log Aggregation

#### ELK Stack Integration

```yaml
# Filebeat configuration
filebeat.inputs:
- type: log
  enabled: true
  paths:
    - /var/log/argus/audit.jsonl
  json.keys_under_root: true
  json.add_error_key: true
  fields:
    logtype: argus_audit
    environment: production
```

#### Fluentd Configuration

```conf
# Fluentd config for Argus audit logs
<source>
  @type tail
  path /var/log/argus/audit.jsonl
  pos_file /var/log/fluentd/argus-audit.log.pos
  tag argus.audit
  format json
  time_key timestamp
  time_format %Y-%m-%dT%H:%M:%S.%LZ
</source>

<match argus.audit>
  @type elasticsearch
  host elasticsearch.company.com
  port 9200
  index_name argus-audit-%Y%m%d
  type_name audit
</match>
```

## Security Considerations

### File Permissions

```bash
# Secure audit log directory
sudo mkdir -p /var/log/argus
sudo chown myapp:myapp /var/log/argus
sudo chmod 750 /var/log/argus

# Audit logs should be readable only by app and security team
sudo chmod 640 /var/log/argus/audit.jsonl
sudo chown myapp:security /var/log/argus/audit.jsonl
```

### Tamper Detection

Each audit entry includes a checksum for tamper detection:

```go
// Verify audit log integrity
func verifyAuditLog(logPath string) error {
    file, err := os.Open(logPath)
    if err != nil {
        return err
    }
    defer file.Close()
    
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        var entry argus.AuditEvent
        if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
            continue
        }
        
        // Verify checksum
        expectedChecksum := generateChecksum(entry)
        if entry.Checksum != expectedChecksum {
            return fmt.Errorf("tampered audit entry detected at %s", 
                entry.Timestamp)
        }
    }
    
    return scanner.Err()
}
```

### Log Rotation

```bash
# Logrotate configuration for Argus audit logs
# /etc/logrotate.d/argus
/var/log/argus/audit.jsonl {
    daily
    rotate 90
    compress
    delaycompress
    missingok
    notifempty
    create 640 myapp security
    postrotate
        /bin/kill -HUP `cat /var/run/myapp.pid 2>/dev/null` 2>/dev/null || true
    endscript
}
```

## Performance Impact

### Benchmark Results

Based on comprehensive testing, the unified SQLite backend provides **superior performance**:

- **SQLite backend:** WAL mode with prepared statements (~0.3µs per event)
- **Cross-app correlation:** Zero additional overhead (shared database)
- **Query performance:** Indexed queries 10-100x faster than JSONL parsing
- **Memory overhead:** 12KB fixed + (250 bytes × buffer size)
- **I/O overhead:** Transaction batching reduces disk writes by 1000x
- **Application impact:** <0.001% for typical config change frequency

### Performance Configuration

```go
// High-throughput unified audit configuration
highThroughputAudit := argus.AuditConfig{
    Enabled:       true,
    OutputFile:    "",                   // Use unified SQLite backend
    MinLevel:      argus.AuditCritical,  // Reduce noise
    BufferSize:    5000,                 // Larger buffer for batching
    FlushInterval: 10 * time.Second,     // Less frequent transaction commits
    IncludeStack:  false,                // Skip expensive stack traces
}

// Real-time security audit configuration
realTimeAudit := argus.AuditConfig{
    Enabled:       true,
    OutputFile:    "",                      // Unified SQLite for correlation
    MinLevel:      argus.AuditSecurity,     // Security events only
    BufferSize:    100,                     // Small buffer for immediate processing
    FlushInterval: 100 * time.Millisecond, // Near real-time commits
    IncludeStack:  true,                    // Full forensic detail
}

// Legacy JSONL for specific compliance requirements
legacyAudit := argus.AuditConfig{
    Enabled:       true,
    OutputFile:    "/var/log/compliance/audit.jsonl", // .jsonl = JSONL backend
    MinLevel:      argus.AuditInfo,
    BufferSize:    1000,
    FlushInterval: 5 * time.Second,
}
```

## Compliance and Standards

### SOX Compliance

Argus audit supports **Sarbanes-Oxley** requirements:

- **Immutable audit trails** with tamper detection
- **Complete configuration change tracking** with before/after values
- **Process accountability** with PID and process name tracking
- **Timestamp precision** to microsecond level
- **Retention policies** via log rotation

### PCI-DSS Compliance

For **Payment Card Industry** environments:

- **Access logging** for all configuration changes
- **Failed access attempts** via security-level events
- **File integrity monitoring** through checksum validation
- **Secure log storage** with proper file permissions
- **Regular log review** capability through structured JSON

### GDPR Compliance

For **data protection** compliance:

- **Data processing activity logging** via context fields
- **Configuration change attribution** with process tracking
- **Audit log retention controls** via rotation policies
- **Right to explanation** through detailed change logs

## Integration Examples

### Kubernetes Operator

```go
// Kubernetes operator with full audit trail
func (r *AppConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Watch ConfigMap changes with audit
    configMapPath := "/etc/config/app.yaml"
    
    watcher, err := argus.UniversalConfigWatcherWithConfig(configMapPath,
        func(config map[string]interface{}) {
            // Apply configuration to pods
            r.applyConfigToPods(ctx, config)
        },
        argus.Config{
            Audit: argus.AuditConfig{
                Enabled:    true,
                OutputFile: "/var/log/k8s-audit/config-changes.jsonl",
                Context: map[string]interface{}{
                    "namespace":   req.Namespace,
                    "resource":    req.Name,
                    "operator":    "config-operator",
                    "cluster":     os.Getenv("CLUSTER_NAME"),
                },
            },
        },
    )
    
    if err != nil {
        return ctrl.Result{}, err
    }
    
    return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}
```

### Microservices Architecture

```go
// Service mesh configuration auditing
type ServiceMeshConfig struct {
    auditor *argus.AuditLogger
}

func (smc *ServiceMeshConfig) WatchServiceConfigs() error {
    services := []string{
        "/etc/istio/gateway.yaml",
        "/etc/istio/virtualservice.yaml", 
        "/etc/consul/service-mesh.json",
    }
    
    for _, servicePath := range services {
        watcher, err := argus.UniversalConfigWatcherWithConfig(servicePath,
            func(config map[string]interface{}) {
                smc.updateServiceMesh(servicePath, config)
            },
            argus.Config{
                Audit: argus.AuditConfig{
                    Enabled:    true,
                    OutputFile: "/var/log/service-mesh/config-audit.jsonl",
                    MinLevel:   argus.AuditCritical,
                },
                ErrorHandler: func(err error, path string) {
                    smc.auditor.LogSecurityEvent("config_error", 
                        fmt.Sprintf("Failed to process %s: %v", path, err),
                        map[string]interface{}{
                            "service": filepath.Base(path),
                            "mesh":    "istio",
                        },
                    )
                },
            },
        )
        
        if err != nil {
            return err
        }
        
        go watcher.Start()
    }
    
    return nil
}
```

## Troubleshooting

### Common Issues

#### Audit Log Permission Denied

```bash
# Check file permissions
ls -la /var/log/argus/audit.jsonl

# Fix permissions
sudo chown myapp:myapp /var/log/argus/audit.jsonl
sudo chmod 640 /var/log/argus/audit.jsonl
```

#### High Memory Usage

```go
// Reduce buffer size for memory-constrained environments
config := argus.AuditConfig{
    BufferSize:    100,  // Smaller buffer
    FlushInterval: 1 * time.Second,  // More frequent flushes
}
```

#### Missing Audit Events

```go
// Enable debug logging to troubleshoot
config := argus.Config{
    Audit: argus.AuditConfig{
        Enabled:      true,
        MinLevel:     argus.AuditInfo,  // Lower threshold
        IncludeStack: true,             // Enable for debugging
    },
    ErrorHandler: func(err error, path string) {
        log.Printf("DEBUG: Audit error for %s: %v", path, err)
    },
}
```

### Monitoring Audit Health

```go
// Monitor audit system health
func monitorAuditHealth(auditor *argus.AuditLogger) {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        // Check if audit file is writable
        if err := auditor.Flush(); err != nil {
            log.Printf("ALERT: Audit system unhealthy: %v", err)
            // Send alert to monitoring system
            alertmanager.SendAlert("audit-system-failure", err.Error())
        }
    }
}
```

## OpenTelemetry Integration

### Overview

Argus provides **optional OpenTelemetry tracing integration** through a non-invasive wrapper pattern. This enables seamless integration with distributed tracing systems like **Jaeger**, **Zipkin**, and observability platforms like **Prometheus**, **Grafana**, and **DataDog**.

### Key Features

- **Zero performance impact** on core audit operations
- **Asynchronous tracing** with no blocking I/O
- **Drop-in replacement** with identical API
- **Graceful degradation** when OTEL is unavailable
- **Standard semantic conventions** for audit events

### Quick Start

```go
import (
    "github.com/agilira/argus"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/jaeger"
    "go.opentelemetry.io/otel/sdk/trace"
)

func main() {
    // Initialize OTEL (optional)
    jaegerExporter, err := jaeger.New(jaeger.WithCollectorEndpoint(
        jaeger.WithEndpoint("http://jaeger:14268/api/traces"),
    ))
    if err != nil {
        log.Fatal(err)
    }
    
    tp := trace.NewTracerProvider(trace.WithBatcher(jaegerExporter))
    otel.SetTracerProvider(tp)
    
    // Create core audit logger (unchanged)
    config := argus.DefaultAuditConfig()
    coreLogger, err := argus.NewAuditLogger(config)
    if err != nil {
        log.Fatal(err)
    }
    defer coreLogger.Close()
    
    // Wrap with OTEL integration
    tracer := otel.Tracer("argus-audit")
    auditLogger, err := argus.NewOTELAuditWrapper(coreLogger, tracer, nil)
    if err != nil {
        log.Fatal(err)
    }
    
    // Use exactly like regular audit logger
    auditLogger.LogConfigChange("/etc/app/config.json", oldConfig, newConfig)
    
    // OTEL spans are created automatically and asynchronously
}
```

### Configuration

```go
// Custom OTEL wrapper configuration
config := &argus.OTELWrapperConfig{
    BaseAttributes: []attribute.KeyValue{
        attribute.String("service.name", "my-application"),
        attribute.String("service.version", "1.2.3"),
        attribute.String("deployment.environment", "production"),
    },
    IncludeEventContext: true,  // Include audit context in spans
    IncludeChecksums:    false, // Skip checksums for performance
    MaxContextSize:      1024,  // Limit context data size
}

wrapper, err := argus.NewOTELAuditWrapper(coreLogger, tracer, config)
```

### OTEL Span Structure

Argus creates structured OTEL spans following semantic conventions:

```yaml
# Configuration change span
span_name: "audit.config_change"
attributes:
  service.name: "my-application"
  audit.level: "CRITICAL"
  audit.event: "config_change" 
  audit.component: "argus"
  audit.operation: "log"
  file.path: "/etc/app/config.json"
  audit.timestamp: "2025-09-22T10:30:00.123Z"
  audit.context.deployment: "v2.1.0"
  audit.context.operator: "jane.doe"

# Security event span  
span_name: "audit.security.unauthorized_access"
attributes:
  audit.level: "SECURITY"
  audit.event: "unauthorized_access"
  audit.security_critical: true
  audit.context.source_ip: "192.168.1.100"
  audit.context.user_id: "unknown"
```

### Performance Characteristics

| Configuration | Latency Impact | Memory Overhead |
|---------------|----------------|-----------------|
| **No OTEL** | 0ns (baseline) | 0 bytes |
| **OTEL Disabled** | +71ns (+23%) | 30 bytes |
| **OTEL Enabled** | +598ns (async) | 1.2KB |

The wrapper ensures **zero impact on hot path performance** through asynchronous span creation.

### Integration Examples

#### Jaeger Distributed Tracing

```go
func setupJaegerTracing() trace.TracerProvider {
    exporter, err := jaeger.New(jaeger.WithCollectorEndpoint(
        jaeger.WithEndpoint("http://jaeger-collector:14268/api/traces"),
    ))
    if err != nil {
        log.Fatal(err)
    }
    
    return trace.NewTracerProvider(
        trace.WithBatcher(exporter),
        trace.WithSampler(trace.TraceIDRatioBased(0.1)), // 10% sampling
        trace.WithResource(resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceNameKey.String("argus-audit"),
            semconv.ServiceVersionKey.String("1.0.0"),
        )),
    )
}
```

#### Prometheus Metrics Export

```go
import "go.opentelemetry.io/otel/exporters/prometheus"

func setupPrometheusMetrics() {
    exporter, err := prometheus.New()
    if err != nil {
        log.Fatal(err)
    }
    
    provider := metric.NewMeterProvider(metric.WithReader(exporter))
    otel.SetMeterProvider(provider)
    
    // Audit events will appear as Prometheus metrics
    // argus_audit_events_total{level="CRITICAL",component="argus"}
}
```

#### OTLP Generic Export

```go
func setupOTLPExport() {
    exporter, err := otlptrace.New(context.Background(),
        otlptracegrpc.NewClient(
            otlptracegrpc.WithEndpoint("http://otel-collector:4317"),
            otlptracegrpc.WithInsecure(),
        ),
    )
    if err != nil {
        log.Fatal(err)
    }
    
    return trace.NewTracerProvider(trace.WithBatcher(exporter))
}
```

### Kubernetes Integration

```yaml
# Deployment with OTEL sidecar
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app-with-audit
spec:
  template:
    spec:
      containers:
      - name: app
        image: myapp:latest
        env:
        - name: OTEL_EXPORTER_JAEGER_ENDPOINT
          value: "http://jaeger-collector:14268/api/traces"
        - name: OTEL_SERVICE_NAME
          value: "myapp"
        - name: OTEL_RESOURCE_ATTRIBUTES
          value: "deployment.environment=production"
          
      # Optional: OTEL Collector sidecar  
      - name: otel-collector
        image: otel/opentelemetry-collector-contrib:latest
        ports:
        - containerPort: 4317 # OTLP gRPC
        - containerPort: 4318 # OTLP HTTP
```

### Best Practices

#### 1. **Performance Optimization**
- Use OTEL sampling to reduce overhead in high-frequency scenarios
- Configure appropriate buffer sizes for batch export
- Monitor OTEL exporter health independently from audit system

#### 2. **Observability Strategy**
- Include meaningful service metadata in base attributes
- Use consistent span naming conventions across services
- Leverage OTEL context propagation for distributed traces

#### 3. **Error Handling**
- Monitor wrapper tracing errors via `GetTracingErrors()`
- Implement OTEL exporter fallback strategies
- Ensure audit reliability is never compromised by OTEL issues

#### 4. **Security Considerations**
- Avoid including sensitive data in OTEL attributes
- Use secure transport for OTEL exports (TLS)
- Consider data retention policies for tracing data

## Best Practices

### 1. **Backend Selection Strategy**
- Use **unified SQLite backend** (default) for cross-application correlation
- Use **JSONL backend** only when required by legacy systems or compliance tools
- Leverage automatic fallback for maximum reliability

### 2. **Audit Level Strategy**
- Use `AuditInfo` for development and testing
- Use `AuditCritical` for production configuration changes
- Use `AuditSecurity` for security-sensitive environments

### 3. **Performance Optimization**
- Set appropriate buffer sizes (1000-5000 for high throughput)
- Adjust flush intervals based on audit requirements
- Use faster storage for unified database (SSD, dedicated volumes)
- Leverage SQLite WAL mode benefits for concurrent access

### 4. **Cross-Application Correlation**
- Use consistent component naming across applications
- Include meaningful context in audit events
- Design queries for incident investigation workflows
- Implement automated correlation alerts for critical patterns

### 5. **Security Hardening**
- Store unified audit database on separate filesystem
- Implement database-level access controls
- Use log shipping to central SIEM systems
- Regular audit database integrity checks

### 6. **Operational Excellence**
- Monitor unified database growth and implement retention policies
- Test audit database recovery procedures
- Document cross-application analysis procedures
- Train operations team on SQL-based audit investigation

The Argus Audit System provides professional-grade audit capabilities that meet the most stringent compliance and security requirements while maintaining ultra-high performance and operational simplicity.

---

Argus • an AGILira fragment