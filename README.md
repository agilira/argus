# Argus: Dynamic Configuration Framework for Go

![Argus Banner](assets/banner.png)

High-performance configuration management framework for Go applications with zero-allocation performance, universal format support (JSON, YAML, TOML, HCL, INI, Properties), and an ultra-fast CLI powered by [Orpheus](https://github.com/agilira/orpheus).

[![CI/CD Pipeline](https://github.com/agilira/argus/actions/workflows/ci.yml/badge.svg)](https://github.com/agilira/argus/actions/workflows/ci.yml)
[![CodeQL](https://github.com/agilira/argus/actions/workflows/codeql.yml/badge.svg)](https://github.com/agilira/argus/actions/workflows/codeql.yml)
[![Security](https://img.shields.io/badge/security-gosec-brightgreen.svg)](https://github.com/agilira/argus/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/agilira/argus?v=2)](https://goreportcard.com/report/github.com/agilira/argus)
[![Test Coverage](https://img.shields.io/badge/coverage-87.7%25-brightgreen)](https://github.com/agilira/argus)
[![CLI Coverage](https://img.shields.io/badge/cli_coverage-77.5%25-green)](https://github.com/agilira/argus)
![Xantos Powered](https://img.shields.io/badge/Xantos-Powered-8A2BE2)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/11273/badge)](https://www.bestpractices.dev/projects/11273)
[![Mentioned in Awesome Go](https://awesome.re/mentioned-badge.svg)](https://github.com/avelino/awesome-go)

## Live Demo

<div align="center">

See Argus in action - managing configurations across multiple formats with zero-allocation performance:

<picture>
  <source media="(max-width: 768px)" srcset="https://asciinema.org/a/ZuUskxFCGcotZJ61.svg" width="100%">
  <source media="(max-width: 1024px)" srcset="https://asciinema.org/a/ZuUskxFCGcotZJ61.svg" width="90%">
  <img src="https://asciinema.org/a/ZuUskxFCGcotZJ61.svg" alt="Argus CLI Demo" style="max-width: 100%; height: auto;" width="800">
</picture>

*[Click to view interactive demo](https://asciinema.org/a/ZuUskxFCGcotZJ61)*

</div>

**[Installation](#installation) • [Quick Start](#quick-start) • [Performance](#performance) • [Architecture](#architecture) • [Framework](#core-framework) • [Observability](#observability--integrations) • [Philosophy](#the-philosophy-behind-argus) • [Documentation](#documentation)**


### Features

- **Universal Format Support**: JSON, YAML, TOML, HCL, INI, Properties with auto-detection
- **ConfigWriter System**: Atomic configuration file updates with type-safe operations
- **Ultra-Fast CLI**: [Orpheus](https://github.com/agilira/orpheus)-powered CLI
- **Professional Grade Validation**: With detailed error reporting & performance recommendations
- **Security Hardened**: [Red-team tested](argus_security_test.go) against path traversal, injection, DoS and resource exhaustion attacks
- **Fuzz Tested**: [Comprehensive fuzzing](argus_fuzz_test.go) for ValidateSecurePath and ParseConfig edge cases
- **Zero-Allocation Design**: Pre-allocated buffers eliminate GC pressure in hot paths
- **Remote Config**: Distributed configuration with automatic fallback (Remote → Local). Currently available: [HashiCorp Consul](https://github.com/agilira/argus-provider-consul), [Redis](https://github.com/agilira/argus-provider-redis), [GitOps](https://github.com/agilira/argus-provider-git) with more to come..
- **Graceful Shutdown**: Timeout-controlled shutdown for Kubernetes and production deployments
- **OpenTelemetry Ready**: Async tracing and metrics with zero contamination of core library
- **Type-Safe Binding**: Zero-reflection configuration binding with fluent API (~46 ns per bound field)
- **Adaptive Optimization**: Five strategies (SingleEvent, SmallBatch, LargeBatch, Light, Auto) 
- **Unified Audit System**: SQLite-based cross-application correlation with JSONL fallback
- **Scalable Monitoring**: Handle 1-1000+ files simultaneously with linear performance

## Compatibility and Support

Argus is designed for Go 1.25+ environments and follows Long-Term Support guidelines to ensure consistent performance across production deployments.

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
// Zero-reflection binding: ~46 ns per bound field, one allocation per chain
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
defer watcher.Close()
```

### Remote Configuration
```go
// Distributed configuration with automatic fallback
watcher := argus.New(argus.Config{
    Remote: argus.RemoteConfig{
        Enabled:      true,
        PrimaryURL:   "consul://consul.internal:8500/config/myapp",
        FallbackURL:  "consul://backup-consul.internal:8500/config/myapp",
        FallbackPath: "/etc/myapp/fallback.json",
        SyncInterval: 30 * time.Second,
        Timeout:      10 * time.Second,
    },
})

// Start() begins remote synchronisation along with file watching.
if err := watcher.Start(); err != nil {
    log.Printf("argus: %v", err) // file watching runs; the remote load is retried
}

// The most recently loaded remote configuration, and when it arrived.
config, loadedAt, err := watcher.RemoteConfig()

// Graceful shutdown for Kubernetes deployments
defer watcher.GracefulShutdown(30 * time.Second)
```

The provider for the URL scheme must be registered first — import
`github.com/agilira/argus-provider-consul` (or redis, or git) for its side effect.

### Directory Watching
```go
// Watch entire directory for config files with pattern filtering
watcher, err := argus.WatchDirectory("/etc/myapp/config.d", argus.DirectoryWatchOptions{
    Patterns:  []string{"*.yaml", "*.json"},
    Recursive: true,
    ErrorHandler: func(err error, path string) {
        log.Printf("argus: %s: %v", path, err) // a file that will not parse
    },
}, func(update argus.DirectoryConfigUpdate) {
    if update.IsDelete {
        fmt.Printf("Config removed: %s\n", update.FilePath)
    } else {
        fmt.Printf("Config updated: %s\n", update.FilePath)
    }
})
defer watcher.Close()

// Merged config from all files (alphabetical order, later overrides earlier)
watcher, err := argus.WatchDirectoryMerged("/etc/myapp/config.d", argus.DirectoryWatchOptions{
    Patterns: []string{"*.yaml"},
}, func(merged map[string]interface{}, files []string) {
    // 00-base.yaml + 10-override.yaml = merged config
    applyConfig(merged)
})
```

### CLI Usage
```bash
# Install the CLI
go install github.com/agilira/argus/cmd/cli/argus@latest


# Ultra-fast configuration management CLI
argus config get config.yaml server.port
argus config set config.yaml database.host localhost
argus config convert config.yaml config.json
argus watch config.yaml --interval=1s
```
**[Orpheus CLI Integration →](./docs/cli-integration.md)** - Complete CLI documentation and examples

## Performance

Engineered for production environments with sustained monitoring and minimal overhead:

### Benchmarks

Measured on an 8-core Linux box, Go 1.25, `go test -bench . -benchmem`:

```
Format auto-detection:              2.9 ns/op    0 allocs
Cached stat lookup:                28.4 ns/op    0 allocs
Event write (ring buffer):         12.4 ns/op    0 allocs
Event write + process:             24.7 ns/op    0 allocs
  same work over Go channels:      42.8 ns/op    (3.4x)
Config binding, 15 fields:          685 ns/op    896 B, 1 alloc
Uncached stat (os.Stat + cache):  1,400 ns/op    240 B, 2 allocs
JSON parsing (small):             1,790 ns/op    616 B, 16 allocs
JSON parsing (large):             8,030 ns/op    3,064 B, 86 allocs
```

**Test BoreasLite ring buffer performance**:
```bash
cd benchmarks && go test -bench="BenchmarkBoreasLite.*" -run=^$ -benchmem
```
See [isolated benchmarks](./benchmarks/) for detailed ring buffer performance analysis.

**Scalability**, from `BenchmarkWatcherSetup` and `BenchmarkWatcherPollCycle`:

```
File Count    Watch() setup    Poll cycle
   10 files     12.2 us/file    1,882 ns/file
  100 files     11.9 us/file      923 ns/file
 1000 files     12.7 us/file      530 ns/file
```

Setup is a one-off per file: path validation, the first `os.Stat` and
registration. The poll figures are wall clock on 8 cores: one `os.Stat` costs
~1.4 us of CPU, and the worker pool overlaps them, which is why the per-file
wall clock falls as the file count rises. Sustained, 1000 files polled every
second sit at the measurement floor — under 1% of a core.

*Detection rate: 100% across all scales*

**Optimization Strategies:**

| Strategy | Best For | Batch size | Hot-spin window |
|---|---|---|---|
| `OptimizationSingleEvent` | 1-2 files, real-time systems | 1 | longest |
| `OptimizationSmallBatch` | 3-20 files, balanced workloads | 4 | medium |
| `OptimizationLargeBatch` | 20+ files, high throughput | 16 | short |
| `OptimizationLight` | Config hot-reload, daemons | 1 | none |
| `OptimizationAuto` | Let Argus decide | 1-16, by file count | medium |

A watcher with nothing to do costs nothing on any of them: the event consumer
spins only while events are arriving and blocks once they stop. What the
strategies trade is batching against how long the consumer stays hot.

Event pickup, measured: **24.7 ns** while events are still flowing, **~7 us**
(median; ~30 us worst case) for the first event after an idle period, which
pays the wakeup.

Use `OptimizationLight` for config files that change rarely (daemon processes, CLI tools):

```go
watcher := argus.New(argus.Config{
    PollInterval:         10 * time.Second,
    OptimizationStrategy: argus.OptimizationLight,
})
```

## Architecture

Argus watches configuration by polling: one `os.Stat` per watched file per interval, spread over a small worker pool. Measured on an 8-core Linux box, a cycle over 1000 files takes ~530us of wall clock (~1.4ms of CPU across the pool), so polling 1000 files every second stays under 1% of a core; a handful of config files costs nothing measurable. Format detection is 2.9ns per operation.

**[Complete Architecture Guide →](./docs/ARCHITECTURE.md)**


### Parser Support

Built-in parsers optimized for rapid deployment with full specification compliance available via plugins.

> **Advanced Features**: Complex configurations requiring full spec compliance should use plugin parsers via `argus.RegisterParser()`. See [docs/parser-guide.md](docs/parser-guide.md) for details.


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
host := writer.GetValue("database.host")      // 111ns, 1 alloc (24ns for a top-level key)
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

**[Full API Reference →](./docs/API-REFERENCE.md)**


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
- **[Quick Start Guide](./docs/quick-start.md)** - Get running in 2 minutes
- **[Orpheus CLI Integration](./docs/cli-integration.md)** - Complete CLI documentation and examples
- **[API Reference](./docs/API-REFERENCE.md)** - Complete API documentation  
- **[Audit System](./docs/audit-system.md)** - Comprehensive audit and compliance guide
- **[Examples](./examples/)** - Production-ready configuration patterns

## License

Argus is licensed under the [Mozilla Public License 2.0](./LICENSE.md).

---

Argus • an AGILira fragment
