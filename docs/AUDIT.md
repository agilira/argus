# Argus Audit System

## Overview

The Argus Audit System provides professional-grade audit trails for configuration changes with **zero-trust security** and **forensic-quality logging**. Every configuration change is immutably logged with tamper detection, making it perfect for compliance, security auditing, and operational forensics.

## Why Audit Matters

In production environments, configuration changes can:
- **Break services** (wrong log level, incorrect ports)
- **Create security vulnerabilities** (disabled authentication, weak encryption)
- **Cause data loss** (wrong database configurations)
- **Violate compliance** (PCI-DSS, SOX, GDPR requirements)

Argus audit ensures **complete accountability** and **incident reconstruction** capabilities.

## Key Features

### **Security-First Design**
- **Tamper detection** with checksums on every audit entry
- **Immutable logs** with append-only JSON Lines format
- **Secure file permissions** (0640) and directory creation (0750)
- **Process tracking** with PID and process name
- **Structured context** for advanced correlation

### **Ultra-High Performance**
- **Sub-microsecond impact** using `go-timecache` (121x faster than `time.Now()`)
- **Buffered writes** with configurable buffer sizes
- **Background flushing** to prevent I/O blocking
- **Zero allocations** in hot paths

### **Comprehensive Coverage**
- **Configuration changes** with before/after values
- **File watch events** (create, modify, delete)
- **Security events** with custom context
- **System events** (startup, shutdown, errors)

### **Production Ready**
- **Configurable audit levels** (Info, Warn, Critical, Security)
- **Automatic log rotation** compatibility
- **Graceful error handling** with fallback options
- **Integration with monitoring systems**

## Quick Start

### Basic Audit Configuration

```go
package main

import (
    "time"
    "github.com/agilira/argus"
)

func main() {
    // Enable audit with secure defaults
    config := argus.Config{
        PollInterval: 5 * time.Second,
        Audit: argus.AuditConfig{
            Enabled:       true,
            OutputFile:    "/var/log/argus/audit.jsonl",
            MinLevel:      argus.AuditInfo,
            BufferSize:    1000,
            FlushInterval: 5 * time.Second,
        },
    }
    
    watcher := argus.New(*config.WithDefaults())
    defer watcher.Stop()
    
    // All configuration changes are now audited automatically
    watcher.Watch("/etc/myapp/config.json", func(event argus.ChangeEvent) {
        // Your config handling logic
        reloadConfig(event.Path)
    })
    
    watcher.Start()
    select {} // Keep running
}
```

### Universal Config Watcher with Audit

```go
// Watch any config format with full audit trail
watcher, err := argus.UniversalConfigWatcherWithConfig("config.yaml", 
    func(config map[string]interface{}) {
        // Handle config changes
        applyNewConfig(config)
    },
    argus.Config{
        Audit: argus.AuditConfig{
            Enabled:    true,
            OutputFile: "/var/log/audit/config-changes.jsonl",
            MinLevel:   argus.AuditCritical,  // Only critical changes
        },
    },
)
```

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
// Secure enterprise defaults
defaultConfig := argus.AuditConfig{
    Enabled:       true,
    OutputFile:    "/var/log/argus/audit.jsonl",
    MinLevel:      argus.AuditInfo,
    BufferSize:    1000,
    FlushInterval: 5 * time.Second,
    IncludeStack:  false,  // Performance optimization
}
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

## Audit Log Analysis

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

Based on comprehensive testing, Argus audit has **minimal performance impact**:

- **Audit overhead:** <0.5µs per event (cached timestamps)
- **Memory overhead:** 8KB fixed + (200 bytes × buffer size)
- **I/O overhead:** Buffered writes reduce syscalls by 1000x
- **Application impact:** <0.001% for typical config change frequency

### Performance Configuration

```go
// High-throughput configuration
highThroughputAudit := argus.AuditConfig{
    Enabled:       true,
    OutputFile:    "/var/log/argus/audit.jsonl",
    MinLevel:      argus.AuditCritical,  // Reduce noise
    BufferSize:    5000,                 // Larger buffer
    FlushInterval: 10 * time.Second,     // Less frequent flushes
    IncludeStack:  false,                // Skip expensive stack traces
}

// Real-time configuration (security-focused)
realTimeAudit := argus.AuditConfig{
    Enabled:       true,
    OutputFile:    "/var/log/security/real-time-audit.jsonl",
    MinLevel:      argus.AuditSecurity,  // Security events only
    BufferSize:    100,                  // Small buffer
    FlushInterval: 100 * time.Millisecond, // Immediate flushing
    IncludeStack:  true,                 // Full forensic detail
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

## Best Practices

### 1. **Audit Level Strategy**
- Use `AuditInfo` for development and testing
- Use `AuditCritical` for production configuration changes
- Use `AuditSecurity` for security-sensitive environments

### 2. **Performance Optimization**
- Set appropriate buffer sizes (1000-5000 for high throughput)
- Adjust flush intervals based on audit requirements
- Use faster storage for audit logs (SSD, dedicated volumes)

### 3. **Security Hardening**
- Store audit logs on separate filesystem
- Use log shipping to central SIEM systems
- Implement log integrity monitoring
- Regular audit log review procedures

### 4. **Operational Excellence**
- Monitor audit log growth and implement rotation
- Test audit log recovery procedures
- Document audit log analysis procedures
- Train operations team on audit log investigation

The Argus Audit System provides professional-grade audit capabilities that meet the most stringent compliance and security requirements while maintaining ultra-high performance and operational simplicity.

---

Argus • an AGILira fragment