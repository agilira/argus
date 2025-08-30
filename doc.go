// Package argus provides a comprehensive dynamic configuration management framework
// for Go applications, combining ultra-fast file monitoring, universal format parsing,
// and zero-reflection configuration binding in a single, cohesive system.
//
// # Philosophy: Dynamic Configuration Done Right
//
// Argus is built on the principle that configuration should be dynamic, type-safe,
// and ultra-performant. It transforms static configuration files into reactive,
// real-time configuration sources that adapt to changes without application restarts.
//
// # Architecture Overview
//
// Argus consists of four integrated subsystems:
//  1. **BoreasLite Ring Buffer**: Ultra-fast MPSC event processing (1.6M+ ops/sec)
//  2. **Universal Format Parsers**: Support for JSON, YAML, TOML, HCL, INI, Properties
//  3. **Zero-Reflection Config Binding**: Type-safe binding with unsafe.Pointer optimization
//  4. **Comprehensive Audit System**: Security and compliance logging with tamper detection
//
// # Universal Configuration Watching
//
// Argus automatically detects and parses any configuration format, making it truly
// universal for modern applications that use diverse configuration sources.
//
// Quick start with automatic format detection:
//
//	watcher, err := argus.UniversalConfigWatcher("config.yml", func(config map[string]interface{}) {
//		if level, ok := config["log_level"].(string); ok {
//			logger.SetLevel(level)
//		}
//		if port, ok := config["server"].(map[string]interface{})["port"].(int); ok {
//			server.UpdatePort(port)
//		}
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer watcher.Close()
//
// Supported formats with zero configuration:
//   - JSON (.json) - Native high-performance parsing
//   - YAML (.yml, .yaml) - Built-in parser + plugin support
//   - TOML (.toml) - Built-in parser + plugin support
//   - HCL (.hcl, .tf) - HashiCorp configuration language
//   - INI (.ini, .conf, .cfg) - Traditional configuration files
//   - Properties (.properties) - Java-style key=value format
//
// # Ultra-Fast Configuration Binding
//
// The zero-reflection binding system provides type-safe configuration access
// with unprecedented performance through unsafe.Pointer optimizations.
//
// High-performance typed configuration binding:
//
//	var dbHost string
//	var dbPort int
//	var enableSSL bool
//	var timeout time.Duration
//
//	err := argus.BindFromConfig(configMap).
//		BindString(&dbHost, "database.host", "localhost").
//		BindInt(&dbPort, "database.port", 5432).
//		BindBool(&enableSSL, "database.ssl", true).
//		BindDuration(&timeout, "database.timeout", 30*time.Second).
//		Apply()
//
// Performance characteristics:
//   - 1,609,530+ binding operations per second
//   - ~744 nanoseconds per binding operation
//   - Zero reflection overhead using unsafe.Pointer
//   - Minimal memory allocations (1 per operation)
//   - Support for nested keys with dot notation (e.g., "database.pool.max_connections")
//
// # BoreasLite: Ultra-Fast Event Processing
//
// At the heart of Argus is BoreasLite, a specialized MPSC ring buffer optimized
// for configuration file events with adaptive performance strategies.
//
// Adaptive optimization strategies:
//
//   - **SingleEvent**: Ultra-low latency for 1-2 files (24ns per event)
//
//   - **SmallBatch**: Balanced performance for 3-20 files
//
//   - **LargeBatch**: High throughput for 20+ files with 4x unrolling
//
//   - **Auto**: Automatically adapts based on file count
//
//     config := argus.Config{
//     PollInterval:         5 * time.Second,
//     OptimizationStrategy: argus.OptimizationAuto,
//     BoreasLiteCapacity:   128,
//     }
//     watcher := argus.New(config)
//
// # Production-Grade Monitoring
//
// Argus uses intelligent polling rather than OS-specific APIs for maximum
// portability and predictable behavior across all platforms.
//
// Advanced file monitoring with caching:
//
//	config := argus.Config{
//		PollInterval:    5 * time.Second,
//		CacheTTL:        2 * time.Second,  // Cache os.Stat() calls
//		MaxWatchedFiles: 100,
//		ErrorHandler: func(err error, path string) {
//			metrics.Increment("config.errors")
//			log.Printf("Config error in %s: %v", path, err)
//		},
//	}
//
//	watcher := argus.New(config)
//	err := watcher.Watch("/app/config.json", func(event argus.ChangeEvent) {
//		if event.IsModify {
//			// Reload configuration
//			reloadConfig(event.Path)
//		}
//	})
//	watcher.Start()
//	defer watcher.Close()
//
// # Comprehensive Audit System
//
// Built-in audit logging provides security and compliance capabilities with
// tamper detection and structured logging.
//
//	auditConfig := argus.AuditConfig{
//		Enabled:       true,
//		OutputFile:    "/var/log/argus/audit.jsonl",
//		MinLevel:      argus.AuditInfo,
//		BufferSize:    1000,
//		FlushInterval: 5 * time.Second,
//	}
//
// Audit events include:
//   - Configuration file changes with before/after values
//   - File watch start/stop events
//   - Security events (watch limit exceeded, etc.)
//   - Tamper-detection checksums using SHA-256
//   - Process context and timestamps
//
// # Plugin Architecture
//
// Argus supports pluggable parsers for production environments requiring
// full specification compliance or advanced features.
//
// Register custom parsers at startup:
//
//	import _ "github.com/your-org/argus-yaml-pro"  // Auto-registers in init()
//	import _ "github.com/your-org/argus-toml-pro"  // Advanced TOML features
//
// Or manually:
//
//	argus.RegisterParser(&MyAdvancedYAMLParser{})
//
// # Performance Optimizations
//
// Argus is designed for high-performance production environments:
//
//   - **Lock-free caching**: Atomic pointers for zero-contention os.Stat() caching
//   - **Zero-allocation polling**: Reusable buffers and value types prevent GC pressure
//   - **Intelligent batching**: Event processing adapts to load patterns
//   - **Time optimization**: Uses go-timecache for 121x faster timestamps
//   - **Memory efficiency**: Sync.Pool for map reuse and careful allocation patterns
//
// # Cross-Platform Compatibility
//
// Argus works identically on all platforms with platform-specific optimizations:
//   - **Linux**: Optimized for container environments and high file counts
//   - **macOS**: Native performance with efficient polling
//   - **Windows**: Proper path handling and JSON escaping
//
// # Integration Patterns
//
// Common integration patterns for different use cases:
//
// **Microservice Configuration**:
//
//	// Hot-reload service configuration
//	watcher, _ := argus.UniversalConfigWatcher("service.yml", func(config map[string]interface{}) {
//		service.UpdateConfig(config)
//	})
//
// **Feature Flags**:
//
//	// Real-time feature flag updates
//	var enableNewAPI bool
//	var rateLimitRPS int
//	argus.BindFromConfig(config).
//		BindBool(&enableNewAPI, "features.new_api", false).
//		BindInt(&rateLimitRPS, "rate_limit.rps", 1000).
//		Apply()
//
// **Database Connection Pools**:
//
//	// Dynamic connection pool sizing
//	var maxConns int
//	var idleTimeout time.Duration
//	watcher.Watch("db.toml", func(event argus.ChangeEvent) {
//		argus.BindFromConfig(config).
//			BindInt(&maxConns, "pool.max_connections", 10).
//			BindDuration(&idleTimeout, "pool.idle_timeout", 5*time.Minute).
//			Apply()
//		db.UpdatePoolConfig(maxConns, idleTimeout)
//	})
//
// # Thread Safety and Concurrency
//
// All Argus components are thread-safe and optimized for concurrent access:
//   - Configuration binding supports concurrent reads
//   - File watching uses atomic operations for state management
//   - Audit logging uses buffered writes with proper synchronization
//   - BoreasLite ring buffer is MPSC (Multiple Producer, Single Consumer)
//
// # Error Handling and Observability
//
// Argus provides comprehensive error handling and observability:
//   - Structured error messages with context
//   - Configurable error handlers for custom logging/metrics
//   - Built-in statistics for monitoring ring buffer performance
//   - Audit trail for all configuration changes and system events
//
// # Getting Started
//
// For detailed examples and documentation:
//   - examples/config_binding/ - Ultra-fast binding system demo
//   - docs/CONFIG_BINDING.md - Complete technical documentation
//   - docs/QUICK_START.md - Getting started guide
//   - docs/API.md - Full API reference
//
// For production deployments, see docs/ARCHITECTURE.md for scaling
// and performance tuning recommendations.
//
// Repository: https://github.com/agilira/argus
package argus
