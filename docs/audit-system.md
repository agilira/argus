# Argus Audit System

## Overview

Argus Audit System provides professional-grade audit trails for configuration changes with **zero-trust security** and **forensic-quality logging**. The system features a **unified SQLite backend** that consolidates audit events from multiple applications into a single, correlation-ready database while maintaining full backward compatibility with JSONL format for legacy systems.

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
    defer watcher.Close()
    
    // All configuration changes are automatically correlated
    // across applications via the unified audit database
    watcher.Watch("/etc/myapp/config.json", func(event argus.ChangeEvent) {
        // Your config handling logic
        reloadConfig(event.Path)
    })
    
    watcher.Start()
    select {} // Keep running
}

func reloadConfig(path string) { /* your reload logic */ }
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
- **Storage:** Per-user database, resolved in this order:
  1. `$ARGUS_AUDIT_DIR`
  2. `$XDG_STATE_HOME/argus`
  3. `~/.local/state/argus` (Linux and the BSDs)
  4. `os.UserConfigDir()/argus` (macOS, Windows)
  5. `os.TempDir()/argus-<uid>` as a last resort

  The file is `system-audit.db`, created `0600`, with its WAL sidecars
  restricted to match. It is deliberately **not** in the shared temporary
  directory: an audit trail records which files a process watches and every
  path it rejected, and the first account to create a shared `argus/` there
  would have owned it for every other account on the host (CWE-377).

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
| `prev_checksum` | string | The `checksum` of the record before this one; empty for the first record of a chain |
| `checksum` | string | Chain link: SHA-256 over `prev_checksum` and the digest of this record's fields |

### What the checksum covers

The record digest hashes **timestamp, level, event, component, file_path,
process_id, process_name, old_value, new_value and context** — every field a
reader acts on. Each is length-prefixed, so no rearrangement of values produces
the same digest, and the `interface{}` fields are hashed as their canonical JSON,
which is exactly what the backend stores and what the reader parses back.

The chain link then binds the record to its predecessor. Together they give two
distinct guarantees:

| Check | Catches | API |
|-------|---------|-----|
| Per-record | A rewritten field: a `file_path` changed from `/etc/shadow` to something harmless, a `SECURITY` level downgraded to `INFO`, a rewritten `context` | `Query()`, on every record it returns |
| Continuity | A record deleted from the middle, records reordered, a truncated tail | `VerifyAuditChain()` |

`Query()` cannot check continuity: a filter or a limit legitimately returns a
subset, so adjacency means nothing there.

> **Compatibility.** Records written before schema v3 have no `prev_checksum`
> and their checksum used an older formula that did not cover `file_path`,
> `level` or `context`. Both checks report them as broken. That is intended —
> those checksums never protected the fields that carry the security meaning.

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

## Verifying the whole trail

`Query()` proves that the records it hands back were not altered. It cannot
prove that none is **missing**. `VerifyAuditChain()` walks the entire trail in
insertion order and checks that every record follows its recorded predecessor:

```go
if err := auditor.VerifyAuditChain(); err != nil {
    if goerrors.HasCode(err, argus.ErrCodeAuditChainBroken) {
        // The error carries the position of the first break:
        //   WithContext("index", n) and WithContext("id", rowID)
        log.Printf("AUDIT TRAIL COMPROMISED: %v", err)
    } else {
        log.Printf("verification failed: %v", err)
    }
}
```

Run it on a schedule, and after any restore or migration.

### What the chain does not protect against

Truncation is caught by an anchor row (`chain_head`) recording where the trail
ends. **That anchor lives in the same database file.** An attacker with write
access can delete records and rewrite it to match.

This catches truncation by accident, by partial restore, by a tool that pruned
the table, and by an attacker who did not know to look. It is not a guarantee
against a fully privileged local attacker. For that, the head has to be
anchored somewhere Argus does not control — shipped to a remote log, signed
into a checkpoint, or written to append-only storage.

Only a SQLite-backed logger can answer this; the JSONL backend keeps no
ordering to verify and returns `ARGUS_AUDIT_BACKEND_UNSUPPORTED`.

## Programmatic Event Query & Integrity Verification

### Querying Audit Events (v1.4.0+)

Argus v1.4.0 introduces a first-class API for querying audit events directly from Go code, with built-in integrity verification:

```go
import (
    "github.com/agilira/argus"
    goerrors "github.com/agilira/go-errors" // for HasCode
)

// Query events from the audit database
filter := argus.AuditEventFilter{
    Since:       time.Now().Add(-24 * time.Hour), // Only last 24h
    EventPrefix: "config.",                      // Only config events
    Component:   "myapp",                        // Only this app
    Level:       argus.AuditCritical,             // Only critical changes
    Limit:       100,                             // Max 100 results
}
events, err := auditor.Query(filter)
if err != nil {
    // ErrCodeAuditChainBroken is a typed error code, not a sentinel error.
    // Use goerrors.HasCode (or check via errors.ErrorCoder) — never errors.Is.
    if goerrors.HasCode(err, argus.ErrCodeAuditChainBroken) {
        // At least one event failed integrity check (checksum mismatch).
        // The full result slice up to the break is still returned for forensics.
        log.Printf("WARNING: Audit chain broken! Results up to break returned.")
    } else {
        log.Fatalf("Query failed: %v", err)
    }
}
for _, ev := range events {
    fmt.Printf("%s %s %s\n", ev.Timestamp, ev.Event, ev.Component)
}
```

### AuditEventFilter Fields

| Field        | Type        | Description                                      |
| ------------| ----------- | ------------------------------------------------ |
| Since       | time.Time   | Lower bound (inclusive) for event timestamp      |
| Until       | time.Time   | Upper bound (inclusive) for event timestamp      |
| EventPrefix | string      | Prefix match for event type (LIKE-escaped)       |
| Component   | string      | Exact match for component                        |
| Level       | AuditLevel  | Minimum level (INFO, WARN, CRITICAL, SECURITY)   |
| Limit       | int         | Max results (default 10,000, capped)             |

- **LIKE metacharacters** (`%`, `_`, `\`) in `EventPrefix` are always escaped for security; only literal prefix matches are allowed.
- **Component** is always an exact match.

### Integrity Verification

Every event returned by `Query` is verified against its stored SHA-256 checksum. If any event fails verification, the full result slice is still returned, but the error will carry the code `ErrCodeAuditChainBroken` (a typed go-errors error). This allows operators to inspect all events up to the break point.

> **Note:** `ErrCodeAuditChainBroken` is an error *code* (`string` constant), not a sentinel `error` value. Do **not** use `errors.Is`. Use `goerrors.HasCode(err, argus.ErrCodeAuditChainBroken)` or inspect the `errors.ErrorCoder` interface.

### Error Codes

- `ARGUS_AUDIT_CHAIN_BROKEN` — checksum mismatch detected in query result
- `ARGUS_AUDIT_BACKEND_UNSUPPORTED` — Query called on non-SQLite backend (e.g. JSONL)
- `ARGUS_AUDIT_QUERY_ERROR` — internal DB error (message does not leak SQL)

### Database Statistics

You can retrieve audit DB stats programmatically:

```go
stats, err := auditor.GetStats()
if err != nil {
    log.Fatalf("GetStats failed: %v", err)
}
fmt.Printf("Total events: %d, Last event: %s\n", stats.TotalEvents, stats.LastEventTimestamp)
```

### Security Notes

- All query filters are parameterized; SQL injection is not possible.
- LIKE metacharacters in `EventPrefix` are always escaped (`ESCAPE '\'`).
- Error messages never leak raw SQL or user input (CWE-209 safe).
- JSONL backend does not support Query and returns a typed error.

---

Argus • an AGILira fragment