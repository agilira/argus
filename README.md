# Argus: Dynamic Configuration Framework for Go
### an AGILira fragment

Argus is a high-performance, OS-independent dynamic configuration framework for Go, engineered for applications that demand real-time configuration updates, universal format support, and production-grade reliability without service restarts.

[![CI/CD Pipeline](https://github.com/agilira/argus/actions/workflows/ci.yml/badge.svg)](https://github.com/agilira/argus/actions/workflows/ci.yml)
[![Security](https://img.shields.io/badge/security-gosec%20verified-brightgreen.svg)](https://github.com/securecodewarrior/gosec)
[![Go Report Card](https://img.shields.io/badge/go%20report-A%2B-brightgreen.svg)](https://goreportcard.com/report/github.com/agilira/iris/argus)
[![Test Coverage](https://img.shields.io/badge/coverage-92%25-brightgreen.svg)](.)
[![Xantos Powered](https://img.shields.io/badge/-Xantos%20Powered-8A2BE2?style=for-the-badge)](https://github.com/agilira)

## Architecture

Argus provides intelligent configuration management through polling-based optimization and universal format support:

### Core Framework
- **Universal Format Support**: JSON, YAML, TOML, HCL, INI, Properties with auto-detection
- **Polling Optimization**: Four strategies (SingleEvent, SmallBatch, LargeBatch, Auto) 
- **Zero Allocations**: Pre-allocated buffers eliminate GC pressure in hot paths
- **Audit System**: Tamper-resistant logging with sub-microsecond performance impact
- **Performance**: 12.11ns polling overhead with intelligent caching strategies
- **Built to Scale** - monitor hundreds to thousands of files simultaneously

```
Configuration Flow Architecture:

[Config Files] ──► [Format Detection] ──► [Universal Parser] ──┐
    │                                                          │
    ▼                                                          ▼
[File Monitor] ──► [BoreasLite Buffer] ──► [Optimization] ──► [Business Logic]
    │                      │                     │
    ▼                      ▼                     ▼
[Audit Trail] ──► [Tamper Detection] ──► [Compliance Logging]

- Zero-allocation monitoring with 12.11ns overhead
- Universal parsing eliminates format lock-in
- Audit system provides forensic-quality trails
```

## Performance

Argus is engineered for production configuration management. The following benchmarks demonstrate sustained monitoring with minimal overhead and intelligent optimization.

### Performance Characteristics
```
Configuration Monitoring Overhead:    12.11 ns/op     (99.998% efficiency)
Universal Parser Detection:           ~100 ns/op      (format auto-detection)
Audit System Impact:                  <0.5 μs/op      (121x faster than time.Now())
Memory Footprint:                     8KB fixed       + configurable buffers
File Count Scalability:               1-1000+ files   (auto-optimization)
```

**Key Features:**
- **12.11ns polling overhead** with intelligent caching
- **Sub-microsecond audit** with tamper detection
- **Universal format support** with zero configuration
- **Adaptive optimization** based on workload patterns
- **Production-grade reliability** with comprehensive error handling

### Configuration Parser Support

Argus includes built-in parsers optimized for the **80% use case** with line-based parsing for rapid deployment:

**Fully Supported Formats:**
- **JSON** - Complete RFC 7159 compliance
- **Properties** - Java-style key=value parsing
- **INI** - Section-based configuration files

**Simplified Parsers (80% Use Case):**
- **YAML** - Line-based parser for simple key-value configurations
- **TOML** - Basic parsing for standard use cases  
- **HCL** - HashiCorp Configuration Language essentials

> **For Spec Compliance**: Complex YAML/TOML/HCL configurations should use plugin parsers.
> Register custom parsers with `argus.RegisterParser()` for full specification compliance.
> See [docs/PARSERS.md](docs/PARSERS.md) for parser plugin development.

## Installation

```bash
go get github.com/agilira/argus
```

## Quick Start

```go
import "github.com/agilira/argus"

// Watch any configuration format - auto-detected
watcher, err := argus.UniversalConfigWatcher("config.yaml", 
    func(config map[string]interface{}) {
        fmt.Printf("Config updated: %+v\n", config)
    })

watcher.Start()
defer watcher.Stop()
```

**[Complete Quick Start Guide →](./docs/QUICK_START.md)** - Get running in 2 minutes with detailed examples


## Core Features

### Universal Format Support
- **Auto-Detection**: Format determined by file extension and content analysis
- **Six Formats**: JSON, YAML, TOML, HCL (.hcl, .tf), INI, Properties
- **Zero Configuration**: Works out-of-the-box with any supported format

### Optimization Strategies
- **OptimizationAuto**: Adaptive strategy selection based on file count
- **OptimizationSingleEvent**: Ultra-low latency for single file (24ns processing)
- **OptimizationSmallBatch**: Balanced performance for 3-20 files
- **OptimizationLargeBatch**: High throughput for 20+ files with 4x unrolling

### Audit System
- **Tamper Detection**: Cryptographic checksums on every audit entry
- **Compliance Ready**: SOX, PCI-DSS, GDPR compatible logging
- **Sub-Microsecond Impact**: Cached timestamps for minimal overhead

**[Full API Reference →](./docs/API.md)** - Complete API documentation with all types and methods

## Use Cases

- **Microservices Configuration**: Real-time config updates without service restarts
- **Feature Flag Management**: Dynamic feature enabling/disabling  
- **Database Connection Management**: Hot-swapping connection parameters
- **Kubernetes ConfigMaps**: Automatic detection of mounted ConfigMap changes
- **Security Policy Updates**: Real-time security configuration enforcement

## The Philosophy Behind Argus

In Greek mythology, Argus Panoptes was the all-seeing giant with a hundred eyes, known for his unwavering vigilance and ability to watch over everything simultaneously. Unlike reactive systems that miss changes, Argus maintained constant awareness while remaining efficient and unobtrusive.

This embodies Argus' design philosophy: intelligent vigilance over configuration changes through universal format support and adaptive optimization. The framework provides comprehensive visibility into configuration state while adapting its monitoring strategy to current conditions. The audit system ensures complete accountability without sacrificing performance.

Argus doesn't just watch files—it understands configuration, adapting to your application's needs while maintaining the reliability and transparency that production systems demand.

## Security & Quality Assurance

Argus maintains production-grade security standards with comprehensive automated validation:

### Security Verification
```bash
# Run security analysis with gosec
./scripts/security-check.sh

# Manual security scan
gosec --exclude=G104,G306,G301 ./...
```

### Audit System Configuration

Argus includes production-grade audit logging with SHA-256 tamper detection:

```go
// Default audit configuration (Linux-optimized)
config := argus.DefaultAuditConfig()
// Default path: /var/log/argus/audit.jsonl

// Cross-platform configuration
config := argus.AuditConfig{
    Enabled:    true,
    OutputFile: filepath.Join(os.TempDir(), "argus-audit.jsonl"), // Portable
    MinLevel:   argus.AuditInfo,
}
```

> **Platform Notes**: Default audit path `/var/log/argus/audit.jsonl` assumes Linux/Unix.
> For Windows or restricted environments, customize `OutputFile` to writable location.
> Ensure proper directory permissions and log rotation for production deployments.

## Documentation

**Quick Links:**
- **[Quick Start Guide](./docs/QUICK_START.md)** - Get running in 2 minutes
- **[API Reference](./docs/API.md)** - Complete API documentation  
- **[Architecture Guide](./docs/ARCHITECTURE.md)** - Deep dive into dynamic configuration design
- **[Audit System](./docs/AUDIT.md)** - Comprehensive audit and compliance guide
- **[Examples](./examples/)** - Production-ready configuration patterns

## License

Argus is licensed under the [Mozilla Public License 2.0](./LICENSE.md).

---

Argus • an AGILira fragment
