# Argus: Dynamic Configuration Framework for Go
### an AGILira fragment

High-performance configuration management library for Go applications with zero-allocation performance, universal format support (JSON, YAML, TOML, HCL, INI, Properties), and an ultra-fast CLI powered by [Orpheus](https://github.com/agilira/orpheus) & [Flash-Flags](https://github.com/agilira/flash-flags).

[![CI/CD Pipeline](https://github.com/agilira/argus/actions/workflows/ci.yml/badge.svg)](https://github.com/agilira/argus/actions/workflows/ci.yml)
[![Security](https://img.shields.io/badge/security-gosec-brightgreen.svg)](https://github.com/agilira/argus/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/agilira/argus)](https://goreportcard.com/report/github.com/agilira/argus)
[![Test Coverage](https://codecov.io/gh/agilira/argus/branch/main/graph/badge.svg)](https://codecov.io/gh/agilira/argus)
![Xantos Powered](https://img.shields.io/badge/Xantos%20Powered-8A2BE2?style=flat)

**[Installation](#installation) • [Quick Start](#quick-start) • [Performance](#performance) • [Architecture](#architecture) • [Framework](#core-framework) • [Observability](#observability--integrations) • [Philosophy](#the-philosophy-behind-argus) • [Documentation](#documentation)**


### Features

- **Universal Format Support**: JSON, YAML, TOML, HCL, INI, Properties with auto-detection
- **ConfigWriter System**: Atomic configuration file updates with type-safe operations
- **Ultra-Fast CLI**: [Orpheus](https://github.com/agilira/orpheus)-powered CLI 7x-53x faster 
- **Professional Grade Validation**: With detailed error reporting & performance recommendations
- **Secure by Design**: Red-team tested against path traversal, injection, DoS and resource exhaustion attacks
- **Zero-Allocation Design**: Pre-allocated buffers eliminate GC pressure in hot paths
- **Remote Config**: Distributed configuration with automatic fallback (Consul/etcd → Local)
- **Graceful Shutdown**: Timeout-controlled shutdown for Kubernetes and production deployments
- **OpenTelemetry Ready**: Async tracing and metrics with zero contamination of core library
- **Type-Safe Binding**: Zero-reflection configuration binding with fluent API (1.6M ops/sec)
- **Adaptive Optimization**: Four strategies (SingleEvent, SmallBatch, LargeBatch, Auto) 
- **Unified Audit System**: SQLite-based cross-application correlation with JSONL fallback
- **Scalable Monitoring**: Handle 1-1000+ files simultaneously with linear performance

## Compatibility and Support

Argus is designed for Go 1.24+ environments and follows Long-Term Support guidelines to ensure consistent performance across production deployments.

## Installation

```bash
go get github.com/agilira/argus
```

## Quick Start

### Multi-Source Configuration Loading
```go
import "github.com/agilira/argus"

// Load with automatic precedence: ENV vars > File > Defaults
config, err := argus.LoadConfigMultiSource("config.yaml")
if err != nil {
    log.Fatal(err)
}

watcher := argus.New(*config)
```

### Type-Safe Configuration Binding
```go
// Ultra-fast zero-reflection binding (1.6M ops/sec)
var (
    dbHost     string
    dbPort     int
    enableSSL  bool
    timeout    time.Duration
)

err := argus.BindFromConfig(parsedConfig).
    BindString(&dbHost, "database.host", "localhost").
    BindInt(&dbPort, "database.port", 5432).
    BindBool(&enableSSL, "database.ssl", true).
    BindDuration(&timeout, "database.timeout", 30*time.Second).
    Apply()
```

### Real-Time Configuration Updates
```go
// Watch any configuration format - auto-detected
watcher, err := argus.UniversalConfigWatcher("config.yaml", 
    func(config map[string]interface{}) {
        fmt.Printf("Config updated: %+v\n", config)
    })

watcher.Start()
defer watcher.Stop()
```

### Remote Configuration
```go
// Distributed configuration with automatic fallback
remoteManager := argus.NewRemoteConfigWithFallback(
    "https://consul.internal:8500/v1/kv/app/config",  // Primary
    "https://backup-consul.internal:8500/v1/kv/app/config", // Fallback
    "/etc/myapp/fallback.json", // Local fallback
)

watcher := argus.New(argus.Config{
    Remote: remoteManager.Config(),
})

// Graceful shutdown for Kubernetes deployments
defer watcher.GracefulShutdown(30 * time.Second)
```

### CLI Usage
```bash
# Ultra-fast configuration management CLI
argus config get config.yaml server.port
argus config set config.yaml database.host localhost
argus config convert config.yaml config.json
argus watch config.yaml --interval=1s
```
**[Orpheus CLI Integration →](./docs/ORPHEUS_INTEGRATION.md)** - Complete CLI documentation and examples

## Performance

Engineered for production environments with sustained monitoring and minimal overhead:

### Benchmarks
```
Configuration Monitoring:      12.10 ns/op     (99.999% efficiency)
Format Auto-Detection:         2.79 ns/op      (universal format support)
JSON Parsing (small):          1,712 ns/op     (616 B/op, 16 allocs/op)
JSON Parsing (large):          7,793 ns/op     (3,064 B/op, 86 allocs/op)
Event Processing:              24.91 ns/op     (BoreasLite single event)
CLI Command Parsing:             512 ns/op     (3 allocs/op, Orpheus framework)
```
**Reproduce benchmarks**:
```bash
go test -bench=. -benchmem
```

**Scalability (Setup Performance):**
```
File Count    Setup Time    Strategy Used
   50 files    11.92 μs/file  SmallBatch
  500 files    23.95 μs/file  LargeBatch
 1000 files    38.90 μs/file  LargeBatch
```
*Detection rate: 100% across all scales*

## Architecture

Argus provides intelligent configuration management through polling-based optimization with lock-free stat cache (12.10ns monitoring overhead), ultra-fast format detection (2.79ns per operation).

**[Complete Architecture Guide →](./docs/ARCHITECTURE.md)**


### Parser Support

Built-in parsers optimized for rapid deployment with full specification compliance available via plugins.

> **Advanced Features**: Complex configurations requiring full spec compliance should use plugin parsers via `argus.RegisterParser()`. See [docs/PARSERS.md](docs/PARSERS.md) for details.


## Core Framework

### ConfigWriter System
Atomic configuration file management with type-safe operations across all supported formats:

```go
// Create writer with automatic format detection
writer, err := argus.NewConfigWriter("config.yaml", argus.FormatYAML, config)
if err != nil {
    return err
}

// Type-safe value operations (zero allocations)
writer.SetValue("database.host", "localhost")
writer.SetValue("database.port", 5432)
writer.SetValue("debug", true)

// Atomic write to disk
if err := writer.WriteConfig(); err != nil {
    return err
}

// Query operations
host := writer.GetValue("database.host")      // 30ns, 0 allocs
keys := writer.ListKeys("database")           // Lists all database.* keys
exists := writer.DeleteValue("old.setting")   // Removes key if exists
```

### Configuration Binding

```go
// Ultra-fast configuration binding - zero reflection
var (
    dbHost     string
    dbPort     int
    enableSSL  bool
    timeout    time.Duration
)

err := argus.BindFromConfig(config).
    BindString(&dbHost, "database.host", "localhost").
    BindInt(&dbPort, "database.port", 5432).
    BindBool(&enableSSL, "database.ssl", true).
    BindDuration(&timeout, "database.timeout", 30*time.Second).
    Apply()

// Variables are now populated and ready to use!
```

**Performance**: 1,645,489 operations/second with single allocation per bind

**[Configuration Binding Guide →](./docs/CONFIG_BINDING.md)** | **[Full API Reference →](./docs/API.md)**


## Observability & Integrations

Professional OTEL tracing integration with zero core dependency pollution:

```go
// Clean separation: core Argus has no OTEL dependencies
auditLogger, _ := argus.NewAuditLogger(argus.DefaultAuditConfig())

// Optional OTEL wrapper (only when needed)
tracer := otel.Tracer("my-service")
wrapper := NewOTELAuditWrapper(auditLogger, tracer)

// Use either logger or wrapper seamlessly
wrapper.LogConfigChange("/etc/config.json", oldConfig, newConfig)
```

**[Complete OTEL Integration Example →](./examples/otel_integration/)**

## The Philosophy Behind Argus

Argus Panoptes was no ordinary guardian. While others slept, he watched. While others blinked, his hundred eyes remained ever vigilant. Hera chose him not for his strength, but for something rarer—his ability to see everything without ever growing weary.

The giant understood that true protection came not from reactive force, but from constant, intelligent awareness. His vigilance was not frantic or wasteful—each eye served a purpose, each moment of watching was deliberate.

When Zeus finally overcame the great guardian, Hera honored Argus by placing his hundred eyes upon the peacock's tail, ensuring his watchful spirit would endure forever.

### Unified Audit Configuration
```go
// Unified SQLite audit (recommended for cross-application correlation)
config := argus.DefaultAuditConfig()  // Uses unified SQLite backend

// Legacy JSONL audit (for backward compatibility)
config := argus.AuditConfig{
    Enabled:    true,
    OutputFile: filepath.Join(os.TempDir(), "argus-audit.jsonl"), // .jsonl = JSONL backend
    MinLevel:   argus.AuditInfo,
}

// Explicit unified SQLite configuration
config := argus.AuditConfig{
    Enabled:    true,
    OutputFile: "",  // Empty = unified SQLite backend
    MinLevel:   argus.AuditCritical,
}
```

## Documentation

**Quick Links:**
- **[Quick Start Guide](./docs/QUICK_START.md)** - Get running in 2 minutes
- **[Orpheus CLI Integration](./docs/ORPHEUS_INTEGRATION.md)** - Complete CLI documentation and examples
- **[API Reference](./docs/API.md)** - Complete API documentation  
- **[Audit System](./docs/AUDIT.md)** - Comprehensive audit and compliance guide
- **[Examples](./examples/)** - Production-ready configuration patterns

## License

Argus is licensed under the [Mozilla Public License 2.0](./LICENSE.md).

---

Argus • an AGILira fragment
