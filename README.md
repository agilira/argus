# Argus: Dynamic Configuration Framework for Go

![Argus Banner](assets/banner.png)

High-performance configuration management framework for Go applications with zero-allocation performance, universal format support (JSON, YAML, TOML, HCL, INI, Properties), and an ultra-fast CLI powered by [Orpheus](https://github.com/agilira/orpheus).

[![CI/CD Pipeline](https://github.com/agilira/argus/actions/workflows/ci.yml/badge.svg)](https://github.com/agilira/argus/actions/workflows/ci.yml)
[![CodeQL](https://github.com/agilira/argus/actions/workflows/codeql.yml/badge.svg)](https://github.com/agilira/argus/actions/workflows/codeql.yml)
[![Security](https://img.shields.io/badge/security-gosec-brightgreen.svg)](https://github.com/agilira/argus/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/agilira/argus?v=2)](https://goreportcard.com/report/github.com/agilira/argus)
[![Test Coverage](https://img.shields.io/badge/coverage-88.7%25-brightgreen)](https://github.com/agilira/argus)
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

- **Unified Entry Point**: `argus.Setup(...)` gathers files, directories, environment, flags, remote providers and documents behind one call, with fixed precedence and atomic revisions
- **Documents**: system prompts and skills read as text, hot-reloaded, in a namespace of their own
- **Typed Binding**: `argus.Bind[Config](settings)` delivers a new struct per revision, never rewriting the one in use
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

One call declares where configuration comes from and returns a handle that
stays current.

```go
settings, err := argus.Setup("agent").
    File("config.json").
    Env("AGENT_").
    Documents("prompts", "prompts/*.md").
    Start()
if err != nil {
    log.Fatal(err)
}
defer settings.Close()

model := settings.GetString("model")
prompt, _ := settings.Doc("prompts", "system")
```

`Start` fails when the first load does not come together: a file that is
missing or does not parse, a required document that is absent, a remote that is
the only source and is unreachable.

The other entry points are in the [API reference](./docs/API-REFERENCE.md).

### Sources and precedence

Declare the sources the application has. The order between them is fixed:

| | Source | Builder |
|---|---|---|
| 1 | explicit overrides | `Overrides(map[string]interface{}{...})` |
| 2 | command-line flags | `Flags(fs)`, an already-parsed flash-flags set |
| 3 | environment | `Env("APP_")`: `server.port` reads `APP_SERVER_PORT` |
| 4 | files and directories | `File(path)`, `FileIfPresent(path)`, `Dir(path)` |
| 5 | remote providers | `Remote("consul://...")`, read again on `RemoteInterval` (30s) |
| 6 | defaults | `Defaults(map[string]interface{}{...})` |

The local file overrides the remote. `FileIfPresent` accepts a file that is not
there; one that is there and does not parse is an error. `Overrides` takes the
values of an application that parses its own command line with cobra, pflag or
the standard library.

### Documents

A system prompt or a skill is text, and Argus reads it as text: no parser, no
format detection, and a namespace of its own, so `Doc("prompts", "system")` and
`GetString("system")` are different values.

```go
settings, err := argus.Setup("agent").
    File("config.json").
    RequiredDocuments("prompts", "prompts/*.md").
    Documents("skills", "skills/**/*.md").   // ** walks the tree
    Start()

for _, skill := range settings.Docs("skills") {
    register(skill.Name, skill.String())
}
```

Limits are 1 MB per document, 16 MB in total, 1000 documents. An oversized
optional document is skipped and reported; a required one that is missing or
unreadable fails the load.

### Reloads

Everything readable at an instant belongs to one **revision**. A reload builds
a candidate, validates it and swaps it in atomically; a candidate that does not
hold together leaves the previous revision serving.

```go
settings, err := argus.Setup("agent").
    File("config.json").
    Documents("prompts", "prompts/*.md").
    MaxStaleness(200 * time.Millisecond).
    OnReload(func(s *argus.Settings, changed argus.Change) {
        if changed.HasDocument("prompts", "system") {
            agent.Reprompt(mustDoc(s, "prompts", "system"))
        }
    }).
    OnError(func(err error) { log.Printf("argus: %v", err) }).
    Start()
```

Files saved together produce one revision, and a revision is spent only when a
value changes. `Change` carries names, never values.

`MaxStaleness` is a latency budget. `Explain()` reports how it is met, and the
rest of a running instance's state:

```go
report := settings.Explain() // marshals as JSON for a debug endpoint
report.KeySource("port")     // argus.SourceEnv
report.Revision              // a counter, local to this process
report.Digest                // the content identity, the same on every machine
report.Backend               // "polling every 200ms"
report.Issues                // a skipped document, a remote that is down
report.LastRefusal           // the candidate that did not become a revision
```

`Revision` counts; `Digest` identifies. Two instances resolving every key to
the same value and holding the same documents carry the same digest, whatever
supplied those values — which is how a run names the configuration it ran on,
in a way that still means something on another machine. The audit trail
records it with every revision.

A refused candidate never becomes a revision, so `LastRefusal` is where it
goes: reason, when, and how many cycles it has been failing. An application
that declares no `OnError` can still tell that what it serves is no longer
what is on disk.

### Binding

`Bind` maps a revision onto a struct type and keeps it in step.

```go
type Config struct {
    Model   string        `argus:"model,required"`
    Timeout time.Duration `argus:"timeout"`
    Server  struct {
        Port int `argus:"port"`
    } `argus:"server"`
}

bound, err := argus.Bind[Config](settings)
cfg := bound.Value()   // *Config for the revision in force
```

Every revision is a new value, so the struct an application already holds never
changes underneath it. A revision that does not satisfy a bound type is
refused, and the last good value keeps serving. `Value()` costs 0.5 ns;
building one costs ~900 ns, once per revision.

### Audit

`Audit()` records every revision to the unified trail, `AuditTo(logger)` to one
the application owns. Changes are recorded by name and hash, never by content.

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

The layer underneath `Setup`, for the jobs it does not cover: writing
configuration files, and binding a parsed map onto variables.

### ConfigWriter System
Atomic configuration file management with type-safe operations across all supported formats:

```go
config := map[string]interface{}{}

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

Binds a parsed configuration map onto package-level variables. For a struct
kept in step with a running `Settings`, see [Binding](#binding).

```go
config, err := argus.LoadConfigMultiSource("config.yaml")
if err != nil {
    return err
}

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
- **[Examples](./examples/)** - Production-ready configuration patterns, starting with [settings](./examples/settings/)

## License

Argus is licensed under the [Mozilla Public License 2.0](./LICENSE.md).

---

Argus • an AGILira fragment
