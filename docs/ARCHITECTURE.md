# Argus Architecture

## Overview

Argus is a high-performance, OS-independent dynamic configuration framework for production environments. The system provides intelligent file monitoring with minimal overhead through polling-based optimization, lock-free operations, and comprehensive audit capabilities. The architecture is modular, with focus on performance optimization, universal format support, and deterministic behavior.

## System Architecture

### Design Principles

1. **Polling-Based Optimization**: OS-independent file monitoring with intelligent optimization strategies.
2. **Zero-Allocation Hot Paths**: No allocations during file stat operations; pre-allocated buffers.
3. **Lock-Free Operations**: All file watching coordination via atomic operations and channels.
4. **Universal Format Support**: Auto-detection and parsing of JSON, YAML, TOML, HCL, INI, and Properties.
5. **Audit System**: Tamper-resistant audit trails with sub-microsecond performance impact.
6. **Configurable Optimization**: Four distinct strategies for different workload patterns.

## Module Structure

### Core Types (`argus.go`)

- `ChangeHandler func(ChangeEvent)`: User-defined callback for file change events.
- `Argus`: Main file watcher structure with optimization engine.
- `ChangeEvent`: Immutable event data with file metadata and change type.
- `Config`: Configuration structure with intelligent defaults and validation.

### Universal Configuration System (`utilities.go`)

The universal config system provides format-agnostic configuration parsing with automatic format detection based on file extensions and content analysis.

### Optimization Engine (`boreaslite.go`)

Four distinct optimization strategies adapt to different workload patterns:
- `OptimizationSingleEvent`: Optimized for single file watching
- `OptimizationSmallBatch`: Efficient for 2-10 files
- `OptimizationLargeBatch`: Optimized for 10+ files  
- `OptimizationAuto`: Adaptive strategy that learns from usage patterns

### Audit System (`audit.go`)

Structured audit logging with tamper detection, event buffering, and compliance-ready output. Supports four audit levels with configurable buffering and background flushing.

## Data Flow Architecture

```
Configuration Files (JSON/YAML/TOML/HCL/INI/Properties)
        │
        ▼
Format Auto-Detection & Parsing
        │
        ▼
Polling-Based File Monitoring (12.11ns overhead)
        │
        ▼
Optimization Engine (4 strategies)
        │
        ▼
Change Detection & Event Generation
        │
        ▼
Audit System (optional, <0.5µs impact)
        │
        ▼
User-Defined Change Handlers
        │
        ▼
Application Configuration Update
```

## Concurrency Model

- **Single-Threaded Polling**: One dedicated goroutine per watcher for deterministic behavior.
- **Lock-Free Operations**: File stat operations use atomic counters and immutable data structures.
- **Channel-Based Communication**: Events propagated via buffered channels for backpressure handling.
- **Graceful Shutdown**: Deterministic shutdown with proper resource cleanup and audit flushing.

## Performance Characteristics

- **Adaptive Optimization**: Automatically adjusts strategy based on file count and change frequency
- **Minimal Memory Footprint**: 8KB fixed overhead plus configurable buffers
- **Sub-Microsecond Audit**: Less than 0.5µs audit impact using cached timestamps (121x faster than time.Now())
- **Zero-Allocation Paths**: File stat operations with pre-allocated buffers

## Configuration Architecture

### Optimization Strategies

```go
type OptimizationStrategy int

const (
    OptimizationSingleEvent  // Single file: fastest polling
    OptimizationSmallBatch   // 2-10 files: batched operations
    OptimizationLargeBatch   // 10+ files: efficient batching
    OptimizationAuto         // Adaptive: learns optimal strategy
)
```

### Audit Configuration

```go
type AuditConfig struct {
    Enabled       bool          // Enable audit logging
    OutputFile    string        // Audit log file path
    MinLevel      AuditLevel    // Minimum audit level (Info/Warn/Critical/Security)
    BufferSize    int           // Event buffer size for batching
    FlushInterval time.Duration // Background flush frequency
    IncludeStack  bool          // Include stack traces (debugging)
}
```

### Multi-Source Configuration

The configuration loader supports automatic format detection and parsing, but does not implement multi-source merging or environment variable interpolation. These are features for future development.

## Format Support Architecture

### Universal Parser Engine

Argus automatically detects and parses multiple configuration formats:

| Format | Extension | Parser | Features |
|--------|-----------|--------|----------|
| JSON | `.json` | `encoding/json` | Standard JSON parsing |
| YAML | `.yaml`, `.yml` | Built-in parser | YAML document parsing |
| TOML | `.toml` | Built-in parser | TOML configuration format |
| HCL | `.hcl`, `.tf` | Built-in parser | HashiCorp Configuration Language |
| INI | `.ini` | Built-in parser | INI files with sections |
| Properties | `.properties` | Built-in parser | Java-style properties files |

### Format Detection Algorithm

1. **Extension-Based**: Primary detection via file extension
2. **Content Analysis**: Fallback parsing attempt for ambiguous files
3. **Error Recovery**: Graceful handling of parsing failures with detailed error context

## Error Handling Strategy

- **Graceful Degradation**: Continue monitoring other files when one fails
- **Configurable Error Handlers**: User-defined error handling with context
- **Audit Integration**: All errors logged to audit trail for forensic analysis
- **Recovery Mechanisms**: Automatic retry logic for transient failures

## Security Architecture

### Audit System Security

- **Tamper Detection**: Cryptographic checksums on every audit entry
- **Immutable Logs**: Append-only JSON Lines format with proper file permissions
- **Process Tracking**: Full process context (PID, name, user) for accountability
- **Structured Context**: Flexible metadata for correlation and analysis

### File System Security

- **Permission Validation**: Checks file permissions before monitoring
- **Symlink Handling**: Secure resolution of symbolic links
- **Path Sanitization**: Protection against path traversal attacks
- **Atomic Operations**: Race condition prevention in file operations

## Testing Architecture

Argus includes comprehensive testing across multiple dimensions:

- **Unit Tests** (`*_unit_test.go`): Individual component validation and edge cases
- **Integration Tests** (`*_test.go`): End-to-end workflow validation with real files
- **Performance Tests** (`*_bench_test.go`): Latency and throughput measurements
- **Audit Tests** (`audit_test.go`): Security and compliance validation
- **Parser Tests** (`parsers.go` tests): Universal parser validation across formats
- **BoreasLite Tests** (`boreaslite_test.go`): Ring buffer performance and correctness
- **Utility Tests** (`utilities_test.go`): Configuration parsing and format detection

## Extension Points

### Custom Handlers

```go
// Custom change handler with context
func customHandler(event argus.ChangeEvent) {
    // Application-specific logic
    switch event.Type {
    case argus.EventModify:
        reloadConfiguration(event.Path)
    case argus.EventDelete:
        handleConfigurationRemoval(event.Path)
    }
}
```

### Custom Audit Processors

```go
// Security event logging (available method)
auditor.LogSecurityEvent("deployment", "Configuration deployed to production", 
    map[string]interface{}{
        "version":     "v2.1.0",
        "environment": "production",
        "operator":    "jane.doe@company.com",
    },
)
```

### Integration Patterns

- **Kubernetes ConfigMaps**: Automatic detection of mounted ConfigMap changes
- **HashiCorp Vault**: Integration with dynamic secrets and configuration
- **Service Mesh**: Istio/Consul configuration synchronization
- **Monitoring Systems**: DataDog, New Relic, Prometheus integration

## Performance Optimization Strategies

### Polling Optimization

1. **Adaptive Intervals**: Dynamic adjustment based on change frequency
2. **Batch Operations**: Grouping file stat calls for efficiency
3. **Smart Caching**: Intelligent caching of file metadata
4. **Resource Pooling**: Reuse of system resources across polls

### Memory Optimization

1. **Pre-Allocated Buffers**: Fixed-size buffers for common operations
2. **String Interning**: Reuse of common file paths and metadata
3. **Garbage Collection Tuning**: Minimal allocation strategies
4. **Buffer Pooling**: Reuse of parsing and audit buffers

### CPU Optimization

1. **Lock-Free Algorithms**: Atomic operations instead of mutexes
2. **Vectorized Operations**: SIMD-optimized string processing where available
3. **Branch Prediction**: Code layout optimized for common paths
4. **Cache-Line Alignment**: Data structure layout for CPU cache efficiency

## Deployment Architecture

### Production Deployment

```go
// Production-ready configuration
config := argus.Config{
    PollInterval:         5 * time.Second,
    OptimizationStrategy: argus.OptimizationAuto,
    Audit: argus.AuditConfig{
        Enabled:       true,
        OutputFile:    "/var/log/argus/audit.jsonl",
        MinLevel:      argus.AuditCritical,
        BufferSize:    1000,
        FlushInterval: 10 * time.Second,
    },
    ErrorHandler: productionErrorHandler,
}
```

### High-Availability Setup

- **Multiple Watchers**: Distributed watching across service instances
- **Shared Audit Logs**: Centralized audit collection via log shipping
- **Circuit Breakers**: Automatic fallback for failed configurations
- **Health Checks**: Monitoring integration for watcher health

### Scalability Patterns

- **Horizontal Scaling**: Multiple watcher instances with coordination
- **Vertical Scaling**: Single instance handling hundreds of files efficiently
- **Cloud Native**: Container-optimized with minimal resource requirements
- **Edge Deployment**: Lightweight footprint for edge computing scenarios

## Compliance and Standards

### Security Standards

- **SOX Compliance**: Immutable audit trails with tamper detection
- **PCI-DSS**: Access logging and configuration change tracking
- **GDPR**: Data processing activity logging with retention controls
- **ISO 27001**: Information security management integration

### Production Features

- **Audit System**: Structured audit logging with tamper detection
- **Security Events**: Security-focused logging and compliance tracking
- **File Permissions**: Secure file access and permission validation
- **Performance Monitoring**: Built-in performance metrics and optimization

---

Argus is architected for maximum performance, security, and operational simplicity in demanding production environments. The modular design enables easy extension while maintaining backward compatibility and deterministic behavior.

---

Argus • an AGILira fragment
