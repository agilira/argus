// Package argus provides a comprehensive dynamic configuration management framework
// for Go applications, combining ultra-fast file monitoring, universal format parsing,
// and zero-reflection configuration binding in a single, cohesive system.
//
// # Philosophy: Dynamic Configuration for the Modern Era
//
// Argus is built on the principle that configuration should be dynamic, type-safe,
// and ultra-performant. It transforms static configuration files into reactive,
// real-time configuration sources that adapt to changes without application restarts.
//
// # Architecture Overview
//
// Argus consists of six integrated subsystems:
//  1. **BoreasLite Ring Buffer**: Ultra-fast MPSC event processing (1.6M+ ops/sec)
//  2. **Universal Format Parsers**: Support for JSON, YAML, TOML, HCL, INI, Properties
//  3. **Zero-Reflection Config Binding**: Type-safe binding with unsafe.Pointer optimization
//  4. **Comprehensive Audit System**: Security and compliance logging with SQLite backend
//  5. **Security Hardening Layer**: Multi-layer protection against path traversal and DoS attacks
//  6. **Remote Configuration**: Distributed config management with graceful failover
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
// tamper detection, structured logging, and unified SQLite backend for persistence.
//
//	auditConfig := argus.AuditConfig{
//		Enabled:       true,
//		OutputFile:    "/var/log/argus/audit.jsonl",
//		SQLiteFile:    "/var/log/argus/audit.db",      // Unified SQLite backend
//		MinLevel:      argus.AuditInfo,
//		BufferSize:    1000,
//		FlushInterval: 5 * time.Second,
//		EnableSQLite:  true,                           // Enable database persistence
//	}
//
// Audit events include:
//   - Configuration file changes with before/after values
//   - File watch start/stop events
//   - Security events (path traversal attempts, DoS detection, watch limit exceeded)
//   - Tamper-detection checksums using SHA-256
//   - Process context and timestamps
//   - Unified storage in SQLite for queryable audit trails and compliance reporting
//
// # Security Hardening Layer
//
// Argus implements comprehensive security controls to protect against common
// attack vectors, making it safe for production environments with untrusted input.
//
// Multi-layer path validation prevents directory traversal attacks:
//
//	// All file paths are automatically validated through 7 security layers:
//	// 1. Empty/null path rejection
//	// 2. Directory traversal pattern detection (.., ../, ..\\, /.., \\.., ./)
//	// 3. Path length limits (max 4096 characters)
//	// 4. Directory depth limits (max 50 levels)
//	// 5. Control character filtering (\x00-\x1f, \x7f-\x9f)
//	// 6. Symlink resolution with safety checks
//	// 7. Windows Alternate Data Stream (ADS) protection
//
// DoS protection through resource limits:
//
//	config := argus.Config{
//		MaxWatchedFiles:  1000,           // Prevent file descriptor exhaustion
//		PollInterval:     100*time.Millisecond, // Min 100ms to prevent CPU DoS
//		CacheTTL:         1*time.Second,  // Min 1s cache TTL
//	}
//
// Environment variable validation prevents injection attacks:
//
//	// All environment variables are validated for:
//	// - Valid UTF-8 encoding
//	// - Safe numeric ranges (poll intervals, cache TTL, file limits)
//	// - Path safety (config file paths, audit log paths)
//	// - Prevention of performance degradation attacks
//
// # Remote Configuration Management
//
// Argus supports distributed configuration management with built-in failover,
// synchronization, and conflict resolution for multi-instance deployments.
//
//	remoteConfig := argus.RemoteConfig{
//		Enabled:        true,
//		PrimaryURL:     "https://config.example.com/api/v1",
//		FallbackPath:   "/etc/argus/fallback.json",
//		SyncInterval:   30 * time.Second,
//		TimeoutConfig: argus.TimeoutConfig{
//			Connection: 5 * time.Second,
//			Read:       10 * time.Second,
//		},
//	}
//
// Remote configuration features:
//   - Automatic failover to local fallback files
//   - Conflict resolution with configurable merge strategies
//   - Encrypted transport with TLS certificate validation
//   - Graceful degradation when remote endpoints are unavailable
//   - Audit logging of all remote configuration changes
//
// # Graceful Shutdown System
//
// Built-in graceful shutdown ensures clean resource cleanup and prevents
// data loss during application termination.
//
//	// Automatic graceful shutdown on SIGINT/SIGTERM
//	watcher := argus.New(config)
//	defer watcher.GracefulShutdown(30 * time.Second) // 30s timeout
//
//	// Manual shutdown with custom timeout
//	shutdownComplete := watcher.InitiateShutdown()
//	select {
//	case <-shutdownComplete:
//		log.Info("Argus shutdown completed successfully")
//	case <-time.After(10 * time.Second):
//		log.Warn("Argus shutdown timeout, forcing termination")
//	}
//
// Shutdown sequence includes:
//   - Stop accepting new file watch requests
//   - Complete processing of pending events in BoreasLite buffer
//   - Flush audit logs to SQLite database
//   - Close all file descriptors and cleanup watchers
//   - Release system resources with proper synchronization
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
// Argus is designed for high-performance production environments with security-first optimizations:
//
//   - **Lock-free caching**: Atomic pointers for zero-contention os.Stat() caching
//   - **Zero-allocation polling**: Reusable buffers and value types prevent GC pressure
//   - **Intelligent batching**: Event processing adapts to load patterns
//   - **Time optimization**: Uses go-timecache for 121x faster timestamps
//   - **Memory efficiency**: Sync.Pool for map reuse and careful allocation patterns
//   - **Security-optimized validation**: Multi-layer path validation with minimal performance impact
//   - **SQLite backend optimization**: Prepared statements and transaction batching for audit performance
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
//   - examples/error_handling/ - Error handling patterns and best practices
//   - examples/enterprise_validation/ - Production security validation examples
//   - docs/CONFIG_BINDING.md - Complete technical documentation
//   - docs/QUICK_START.md - Getting started guide
//   - docs/API.md - Full API reference
//   - docs/AUDIT.md - Audit system and SQLite backend configuration
//   - docs/SECURITY.md - Security hardening and best practices
//   - docs/REMOTE_CONFIGURATION.md - Distributed configuration management
//
// For production deployments, see docs/ARCHITECTURE.md for scaling,
// performance tuning, and security configuration recommendations.
//
// Repository: https://github.com/agilira/argus
package argus
