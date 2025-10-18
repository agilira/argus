# Argus API Reference

**Quick Navigation:** [Core Types](#core-types) | [ConfigWriter](#configwriter-system) | [ConfigBinder](#configuration-binding-system) | [Parsers](#configuration-file-parsing) | [Error Codes](#error-codes) | [Utils](#global-utility-functions)

---

## Table of Contents

### [Core Types](#core-types)
- [Watcher](#watcher) - Main file watching functionality
- [Config](#config) - Core configuration options  
- [RemoteConfig](#remoteconfig) - Remote configuration management
- [ChangeEvent](#changeevent) - File change event structure
- [OptimizationStrategy](#optimizationstrategy) - Performance tuning options

### [ConfigWriter System](#configwriter-system)
- [ConfigWriter](#configwriter) - Atomic configuration file operations
- [Constructor Functions](#constructor-functions) - Creating writers
- [CRUD Methods](#methods) - Get, Set, Delete, List operations
- [File Operations](#file-operations) - Write, backup, format conversion

### [Configuration Binding System](#configuration-binding-system)
- [ConfigBinder](#configbinder) - Ultra-fast configuration binding
- [Binding Methods](#binding-methods) - Struct binding operations
- [Advanced Binding](#advanced-binding) - Complex data types

### [Configuration File Parsing](#configuration-file-parsing)
- [Supported Formats](#supported-formats) - JSON, YAML, TOML, HCL, INI, Properties
- [ConfigParser Interface](#configparser-interface) - Custom parser implementation
- [Format Detection](#format-detection) - Automatic format recognition

### [Error Handling](#error-codes)
- [Error Codes](#error-codes) - Complete error code reference
- [Error Types](#error-types) - Categorized error handling

### [Advanced Features](#advanced-features)
- [Audit System](#audit-system) - Enterprise audit trails
- [Remote Configuration](#remote-configuration) - Distributed configuration
- [Plugin System](#plugin-system) - Extensibility features

### [Performance & Best Practices](#performance-characteristics)
- [Performance Characteristics](#performance-characteristics) - Benchmarks and optimization
- [Best Practices](#best-practices) - Production usage patterns
- [Thread Safety](#thread-safety) - Concurrency considerations

### [Utilities](#global-utility-functions)
- [Global Utility Functions](#global-utility-functions) - Helper functions and utilities

---

## Core Types

### Watcher

The main watcher instance that monitors files for changes.

```go
type Watcher struct {
    // Internal fields not exposed
}
```

#### Constructor

##### `New(config Config) *Watcher`

Creates a new Argus watcher instance with the specified configuration.

**Parameters:**
- `config Config`: Configuration options for the watcher

**Returns:** `*Watcher` - New watcher instance

**Example:**
```go
config := argus.Config{
    PollInterval: 2 * time.Second,
    OptimizationStrategy: argus.OptimizationSingleEvent,
}
watcher := argus.New(*config.WithDefaults())
```

#### Methods

##### `Watch(filePath string, callback UpdateCallback) error`

Adds a file to the watch list with a callback function that executes when the file changes.

**Parameters:**
- `filePath string`: Absolute or relative path to the file to watch
- `callback UpdateCallback`: Function called when file changes

**Returns:** `error` - Error if file cannot be watched

**Example:**
```go
err := watcher.Watch("/etc/myapp/config.json", func(event argus.ChangeEvent) {
    if event.IsModify {
        reloadConfig(event.Path)
    }
})
```

##### `Unwatch(filePath string) error`

Removes a file from the watch list.

**Parameters:**
- `filePath string`: Path of the file to stop watching

**Returns:** `error` - Error if file was not being watched

##### `Start() error`

Starts the file watching process in a background goroutine.

**Returns:** `error` - Error if watcher is already running

##### `Stop() error`

Stops the file watching process and cleans up resources.

**Returns:** `error` - Error if watcher was not running

##### `GracefulShutdown(timeout time.Duration) error`

Performs a graceful shutdown with timeout control. Enterprise feature for production deployments requiring controlled shutdown behavior.

**Parameters:**
- `timeout time.Duration`: Maximum time to wait for graceful shutdown

**Returns:** `error` - Error if shutdown times out or watcher was not running

**Example:**
```go
// Graceful shutdown with 30 second timeout (Kubernetes)
err := watcher.GracefulShutdown(30 * time.Second)
if err != nil {
    log.Printf("Graceful shutdown failed: %v", err)
}
```

**Use Cases:**
- Kubernetes pod termination handling
- Docker container shutdown hooks  
- Production service graceful restarts
- Integration testing cleanup

##### `IsRunning() bool`

Returns whether the watcher is currently active.

**Returns:** `bool` - True if watcher is running

##### `GetStats() Stats`

Returns performance and operational statistics.

**Returns:** `Stats` - Current watcher statistics

##### `WatchedFiles() int`

Returns the number of currently watched files.

**Returns:** `int` - Number of files currently being watched

##### `GetWriter(filePath string, format ConfigFormat, initialConfig map[string]interface{}) (*ConfigWriter, error)`

Creates a ConfigWriter for the specified file with atomic write operations.

**Parameters:**
- `filePath string`: Path to the configuration file
- `format ConfigFormat`: Configuration format (JSON, YAML, etc.)
- `initialConfig map[string]interface{}`: Initial configuration data

**Returns:** 
- `*ConfigWriter`: New ConfigWriter instance for atomic operations
- `error`: Error if writer creation fails

**Performance:** ~500 ns/op, zero allocations for writer creation

**Example:**
```go
writer, err := watcher.GetWriter("config.json", argus.FormatJSON, initialConfig)
if err != nil {
    log.Fatal(err)
}
writer.SetValue("port", 8080)
writer.WriteConfig()
```

##### `ClearCache()`

Forces clearing of the internal file stat cache.

**Use Cases:**
- Testing scenarios requiring fresh file stats
- Debugging cache-related issues  
- Manual cache invalidation after external file changes

**Performance:** Zero allocations, immediate effect

##### `GetCacheStats() CacheStats`

Returns statistics about the internal cache for monitoring and debugging.

**Returns:** `CacheStats` - Current cache performance metrics

**Example:**
```go
stats := watcher.GetCacheStats()
fmt.Printf("Cache entries: %d, oldest: %v\n", stats.Entries, stats.OldestAge)
```

##### `Close() error`

Alias for Stop() that implements the common Close() interface for better resource management patterns.

**Returns:** `error` - Error if watcher was not running

**Example:**
```go
defer watcher.Close() // Can be used with defer for automatic cleanup
```

### Config

Configuration structure for customizing watcher behavior.

```go
type Config struct {
    PollInterval          time.Duration
    CacheTTL             time.Duration
    MaxWatchedFiles      int
    Audit                AuditConfig
    ErrorHandler         ErrorHandler
    OptimizationStrategy OptimizationStrategy
    BoreasLiteCapacity   int64
    Remote               RemoteConfig
}
```

#### Fields

##### `PollInterval time.Duration`

How often to check for file changes.
- **Default:** 5 seconds
- **Recommended:** 1-10 seconds for config files
- **Performance:** Lower values increase CPU usage

##### `CacheTTL time.Duration`

How long to cache `os.Stat()` results to reduce syscalls.
- **Default:** `PollInterval / 2`
- **Constraint:** Must be ≤ `PollInterval`
- **Performance:** Longer TTL reduces I/O overhead

##### `MaxWatchedFiles int`

Maximum number of files that can be watched simultaneously.
- **Default:** 100
- **Range:** 1-1000 (practical limits)

##### `OptimizationStrategy OptimizationStrategy`

Strategy for optimizing performance based on workload.
- **Default:** `OptimizationAuto`
- **Options:** Auto, SingleEvent, SmallBatch, LargeBatch

##### `BoreasLiteCapacity int64`

Ring buffer size for event processing (must be power of 2).
- **Default:** Auto-calculated based on strategy
- **Range:** 64-4096

##### `Remote RemoteConfig`

Remote configuration with automatic fallback capabilities.
- **Default:** Disabled for backward compatibility
- **Purpose:** Distributed configuration management with resilient fallback

---

### RemoteConfig

Remote configuration management with automatic fallback sequence.

```go
type RemoteConfig struct {
    Enabled     bool
    PrimaryURL  string
    FallbackURL string
    LocalPath   string
    Timeout     time.Duration
    MaxRetries  int
    RetryDelay  time.Duration
    SyncInterval time.Duration
}
```

#### Fields

##### `Enabled bool`

Enables remote configuration loading with fallback sequence.
- **Default:** `false`
- **Note:** Must be explicitly enabled

##### `PrimaryURL string`

Primary remote configuration endpoint (Consul, etcd, HTTP API).
- **Required:** When `Enabled` is true
- **Example:** `"https://consul.internal:8500/v1/kv/app/config"`

##### `FallbackURL string`

Secondary remote configuration endpoint for failover scenarios.
- **Optional:** Provides additional resilience
- **Example:** `"https://backup-consul.internal:8500/v1/kv/app/config"`

##### `LocalPath string`

Local file path for last-resort fallback when remote endpoints fail.
- **Optional:** Final fallback in the sequence  
- **Example:** `"/etc/myapp/config.json"`

##### `Timeout time.Duration`

Maximum time to wait for remote configuration loading.
- **Default:** `10 * time.Second`
- **Range:** 1s-300s

##### `MaxRetries int`

Maximum retry attempts for failed remote requests.
- **Default:** `3`
- **Range:** 0-10

##### `RetryDelay time.Duration`

Base delay for exponential backoff retry strategy.
- **Default:** `1 * time.Second`
- **Pattern:** `RetryDelay * 2^(attempt-1)`

##### `SyncInterval time.Duration`

Interval for periodic remote configuration synchronization.
- **Default:** `5 * time.Minute`  
- **Range:** 1m-24h

#### Methods

##### `NewRemoteConfigWithFallback(primaryURL, fallbackURL, localPath string) *RemoteConfigManager`

Creates a new RemoteConfig manager with automatic fallback sequence for.

**Parameters:**
- `primaryURL string`: Primary remote configuration endpoint
- `fallbackURL string`: Secondary endpoint (can be empty)  
- `localPath string`: Local fallback file path (can be empty)

**Returns:** `*RemoteConfigManager` - Configured remote config manager

**Example:**
```go
remoteManager := argus.NewRemoteConfigWithFallback(
    "https://consul.internal:8500/v1/kv/app/config",
    "https://backup-consul.internal:8500/v1/kv/app/config", 
    "/etc/myapp/fallback.json",
)

config := argus.Config{
    Remote: remoteManager.Config(),
}
watcher := argus.New(config)
```

**Fallback Sequence:**
1. **Primary URL** → Try primary remote endpoint
2. **Fallback URL** → Try secondary remote endpoint  
3. **Local Path** → Load local configuration file
4. **Error** → All sources failed

#### Methods

##### `WithDefaults() *Config`

Applies sensible defaults to configuration fields.

**Returns:** `*Config` - Configuration with defaults applied

**Example:**
```go
config := argus.Config{
    PollInterval: 3 * time.Second,
    // Other fields will get defaults
}
finalConfig := config.WithDefaults()
```

---

## ConfigWriter System

### ConfigWriter

The ConfigWriter provides atomic configuration file operations with zero-allocation performance for programmatic configuration management.

```go
type ConfigWriter struct {
    // Internal fields - not exposed for thread safety
}
```

#### Constructor Functions

##### `NewConfigWriter(filePath string, format ConfigFormat, initialConfig map[string]interface{}) (*ConfigWriter, error)`

Creates a new ConfigWriter for atomic configuration file operations.

**Parameters:**
- `filePath string`: Path to the configuration file
- `format ConfigFormat`: Configuration format (JSON, YAML, TOML, HCL, INI, Properties)
- `initialConfig map[string]interface{}`: Initial configuration data (can be nil)

**Returns:**
- `*ConfigWriter`: New ConfigWriter instance
- `error`: Error if creation fails

**Performance:** ~500 ns/op, zero allocations for writer creation

**Example:**
```go
writer, err := argus.NewConfigWriter("config.yaml", argus.FormatYAML, config)
if err != nil {
    return err
}
```

##### `NewConfigWriterWithAudit(filePath string, format ConfigFormat, initialConfig map[string]interface{}, auditLogger *AuditLogger) (*ConfigWriter, error)`

Creates a new ConfigWriter with optional audit logging for compliance requirements.

**Parameters:**
- `filePath string`: Path to configuration file
- `format ConfigFormat`: Configuration format
- `initialConfig map[string]interface{}`: Initial configuration (can be nil)
- `auditLogger *AuditLogger`: Optional audit logger (can be nil for performance)

**Returns:**
- `*ConfigWriter`: New ConfigWriter with audit capability
- `error`: Error if creation fails

**Performance:** ~500 ns/op when audit disabled, ~750 ns/op when enabled

#### Methods

##### `SetValue(key string, value interface{}) error`

Sets a configuration value using dot notation for nested keys.

**Parameters:**
- `key string`: Configuration key in dot notation (e.g., "database.host")
- `value interface{}`: Value to set (any JSON-serializable type)

**Returns:** `error` - Error if key is invalid or operation fails

**Performance:** 127 ns/op, 0 allocs for simple keys; 295 ns/op, 1 alloc for nested keys

**Examples:**
```go
writer.SetValue("port", 8080)                    // Simple key
writer.SetValue("database.host", "localhost")    // Nested key
writer.SetValue("features.auth.enabled", true)   // Deep nesting
```

##### `GetValue(key string) interface{}`

Retrieves a configuration value using dot notation.

**Parameters:**
- `key string`: Configuration key in dot notation

**Returns:** `interface{}` - Value or nil if key doesn't exist

**Performance:** 89 ns/op, 0 allocs for simple lookups

**Example:**
```go
host := writer.GetValue("database.host")
if host != nil {
    fmt.Printf("Database host: %s\n", host.(string))
}
```

##### `DeleteValue(key string) bool`

Removes a configuration key using dot notation.

**Parameters:**
- `key string`: Configuration key to remove

**Returns:** `bool` - True if key existed and was deleted

**Performance:** 156 ns/op, 0 allocs for simple keys

**Examples:**
```go
deleted := writer.DeleteValue("old.setting")     // Returns true if existed
writer.DeleteValue("features.beta.enabled")     // Remove nested key
```

##### `WriteConfig() error`

Atomically writes the current configuration to disk using temporary file + rename.

**Returns:** `error` - Error if write operation fails

**Performance:** I/O bound, typically 2-5ms

**Atomicity:** Either succeeds completely or leaves original file unchanged

**Example:**
```go
writer.SetValue("database.port", 5432)
if err := writer.WriteConfig(); err != nil {
    log.Printf("Failed to write config: %v", err)
}
```

##### `WriteConfigAs(filePath string) error`

Writes the configuration to a different file path (useful for backups).

**Parameters:**
- `filePath string`: Target file path for export

**Returns:** `error` - Error if write fails

**Use Cases:**
- Configuration backups
- Exporting to different locations
- Creating configuration templates

##### `HasChanges() bool`

Returns true if the configuration has unsaved changes.

**Returns:** `bool` - True if changes exist

**Performance:** O(1) using fast hash comparison

##### `GetConfig() map[string]interface{}`

Returns a deep copy of the current configuration.

**Returns:** `map[string]interface{}` - Deep copy of configuration

**Performance:** O(n) where n is config size

##### `ListKeys(prefix string) []string`

Returns all configuration keys in dot notation format, optionally filtered by prefix.

**Parameters:**
- `prefix string`: Optional prefix filter (empty string returns all keys)

**Returns:** `[]string` - List of keys matching the prefix

**Performance:** O(n) where n is total number of keys

**Examples:**
```go
allKeys := writer.ListKeys("")           // All keys
dbKeys := writer.ListKeys("database")    // Only database.* keys
```

##### `Reset() error`

Discards all changes and reverts to the last saved state by reloading from file.

**Returns:** `error` - Error if reload fails (resets to empty config)

**Performance:** I/O bound, reads and parses original file

**Use Cases:**
- Canceling unsaved operations
- Error recovery
- Reverting to known good state

---

## Configuration Binding System

### ConfigBinder

Configuration binding system that eliminates reflection overhead while providing excellent developer experience.

```go
type ConfigBinder struct {
    // Internal fields - optimized for zero-allocation performance
}
```

#### Constructor Functions

##### `BindFromConfig(config map[string]interface{}) *ConfigBinder`

Creates a new ConfigBinder from a parsed configuration map. This is the main entry point for configuration binding.

**Parameters:**
- `config map[string]interface{}`: Parsed configuration data

**Returns:** `*ConfigBinder` - New ConfigBinder instance

**Example:**
```go
binder := argus.BindFromConfig(parsedConfig)
```

##### `NewConfigBinder(config map[string]interface{}) *ConfigBinder`

Creates a new high-performance configuration binder. Prefer using `BindFromConfig()` for better API consistency.

**Parameters:**
- `config map[string]interface{}`: Configuration source

**Returns:** `*ConfigBinder` - New ConfigBinder instance

#### Binding Methods

##### `BindString(target *string, key string, defaultValue ...string) *ConfigBinder`

Binds a string configuration value with optional default.

**Parameters:**
- `target *string`: Pointer to target variable
- `key string`: Configuration key (supports dot notation)
- `defaultValue ...string`: Optional default value

**Returns:** `*ConfigBinder` - Self for method chaining

**Example:**
```go
var dbHost string
binder.BindString(&dbHost, "database.host", "localhost")
```

##### `BindInt(target *int, key string, defaultValue ...int) *ConfigBinder`

Binds an integer configuration value with optional default.

**Parameters:**
- `target *int`: Pointer to target variable
- `key string`: Configuration key (supports dot notation)
- `defaultValue ...int`: Optional default value

**Returns:** `*ConfigBinder` - Self for method chaining

##### `BindInt64(target *int64, key string, defaultValue ...int64) *ConfigBinder`

Binds an int64 configuration value with optional default.

**Parameters:**
- `target *int64`: Pointer to target variable
- `key string`: Configuration key
- `defaultValue ...int64`: Optional default value

**Returns:** `*ConfigBinder` - Self for method chaining

##### `BindBool(target *bool, key string, defaultValue ...bool) *ConfigBinder`

Binds a boolean configuration value with optional default.

**Parameters:**
- `target *bool`: Pointer to target variable
- `key string`: Configuration key
- `defaultValue ...bool`: Optional default value

**Returns:** `*ConfigBinder` - Self for method chaining

##### `BindFloat64(target *float64, key string, defaultValue ...float64) *ConfigBinder`

Binds a float64 configuration value with optional default.

**Parameters:**
- `target *float64`: Pointer to target variable
- `key string`: Configuration key
- `defaultValue ...float64`: Optional default value

**Returns:** `*ConfigBinder` - Self for method chaining

##### `BindDuration(target *time.Duration, key string, defaultValue ...time.Duration) *ConfigBinder`

Binds a time.Duration configuration value with optional default.

**Parameters:**
- `target *time.Duration`: Pointer to target variable
- `key string`: Configuration key
- `defaultValue ...time.Duration`: Optional default value

**Returns:** `*ConfigBinder` - Self for method chaining

**Example:**
```go
var timeout time.Duration
binder.BindDuration(&timeout, "database.timeout", 30*time.Second)
```

##### `Apply() error`

Executes all bindings in a single optimized pass with ultra-fast batch processing.

**Returns:** `error` - Error if any binding fails

**Performance:** 1,645,489 operations/second with single allocation per bind

**Example:**
```go
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

if err != nil {
    log.Fatal(err)
}

// Variables are now populated and ready to use!
```

### ChangeEvent

Represents a file change notification.

```go
type ChangeEvent struct {
    Path     string
    ModTime  time.Time
    Size     int64
    IsCreate bool
    IsDelete bool
    IsModify bool
}
```

#### Fields

##### `Path string`
Absolute path of the file that changed.

##### `ModTime time.Time`
New modification timestamp of the file.

##### `Size int64`
New size of the file in bytes.

##### `IsCreate bool`
True if the file was newly created.

##### `IsDelete bool`
True if the file was deleted.

##### `IsModify bool`
True if the file was modified (most common case).

### OptimizationStrategy

Enumeration of performance optimization strategies.

```go
type OptimizationStrategy int
```

#### Constants

##### `OptimizationAuto`
Automatically selects the best strategy based on file count:
- 1-3 files: SingleEvent strategy
- 4-20 files: SmallBatch strategy  
- 21+ files: LargeBatch strategy

##### `OptimizationSingleEvent`
Optimized for 1-2 files with ultra-low latency:
- **Performance:** 24ns per operation
- **Best for:** Single config file scenarios
- **Memory:** 64-event ring buffer

##### `OptimizationSmallBatch`
Balanced optimization for 3-20 files:
- **Performance:** 28ns per operation
- **Best for:** Multi-config applications
- **Memory:** 128-event ring buffer

##### `OptimizationLargeBatch`
High throughput optimization for 20+ files:
- **Performance:** 35ns per operation
- **Best for:** Configuration management systems
- **Memory:** 256+ event ring buffer

### UpdateCallback

Function type for handling file change notifications.

```go
type UpdateCallback func(event ChangeEvent)
```

**Parameters:**
- `event ChangeEvent`: Details about the file change

**Example:**
```go
callback := func(event argus.ChangeEvent) {
    switch {
    case event.IsCreate:
        fmt.Printf("File created: %s\n", event.Path)
    case event.IsDelete:
        fmt.Printf("File deleted: %s\n", event.Path)
    case event.IsModify:
        fmt.Printf("File modified: %s (size: %d)\n", event.Path, event.Size)
    }
}
```

### ErrorHandler

Function type for handling errors during file watching.

```go
type ErrorHandler func(err error, filepath string)
```

**Parameters:**
- `err error`: The error that occurred
- `filepath string`: Path of the file where error occurred

**Example:**
```go
errorHandler := func(err error, path string) {
    log.Printf("Argus error for %s: %v", path, err)
    metrics.Increment("argus.errors")
}

config := argus.Config{
    ErrorHandler: errorHandler,
}
```

### Stats

Performance and operational statistics.

```go
type Stats struct {
    FilesWatched        int64
    TotalPolls          int64
    TotalChanges        int64
    CacheHits           int64
    CacheMisses         int64
    LastPollDuration    time.Duration
    AverageLatency      time.Duration
    ErrorCount          int64
}
```

#### Fields

##### `FilesWatched int64`
Current number of files being monitored.

##### `TotalPolls int64`
Total number of polling cycles completed.

##### `TotalChanges int64`
Total number of file changes detected.

##### `CacheHits int64` / `CacheMisses int64`
Statistics for the internal `os.Stat()` cache.

##### `LastPollDuration time.Duration`
Duration of the most recent polling cycle.

##### `AverageLatency time.Duration`
Average time from file change detection to callback execution.

##### `ErrorCount int64`
Total number of errors encountered.

### CacheStats

Statistics about the internal file stat cache for monitoring and debugging.

```go
type CacheStats struct {
    Entries   int           // Number of cached entries
    OldestAge time.Duration // Age of oldest cache entry
    NewestAge time.Duration // Age of newest cache entry
}
```

#### Fields

##### `Entries int`
Number of cached file stat entries currently in memory.

##### `OldestAge time.Duration`
Age of the oldest cache entry, indicating how long the oldest cached stat has been retained.

##### `NewestAge time.Duration`  
Age of the newest cache entry, indicating how recently the cache was last updated.

**Use Cases:**
- Cache efficiency monitoring
- Memory usage optimization
- Performance debugging
- Cache hit ratio analysis

## Error Codes

Argus uses structured error codes for better error handling:

- `ARGUS_INVALID_CONFIG`: Invalid configuration provided
- `ARGUS_FILE_NOT_FOUND`: Watched file does not exist
- `ARGUS_WATCHER_STOPPED`: Operation attempted on stopped watcher
- `ARGUS_WATCHER_BUSY`: Watcher is already running

## Configuration File Parsing

Argus includes universal configuration parsers for common formats:

### ConfigFormat

Enumeration of supported configuration file formats for auto-detection and parsing.

```go
type ConfigFormat int

const (
    FormatJSON       ConfigFormat = iota
    FormatYAML
    FormatTOML
    FormatHCL
    FormatINI
    FormatProperties
    FormatUnknown
)
```

#### Constants

##### `FormatJSON`
JSON format (.json files) - Full production support with zero dependencies.

##### `FormatYAML`
YAML format (.yml, .yaml files) - Built-in parser + plugin support for advanced features.

##### `FormatTOML`
TOML format (.toml files) - Built-in parser + plugin support for full specification compliance.

##### `FormatHCL`
HashiCorp Configuration Language (.hcl, .tf files) - Built-in parser + plugin support.

##### `FormatINI`
INI/Configuration format (.ini, .conf, .cfg files) - Built-in parser with section support.

##### `FormatProperties`
Java Properties format (.properties files) - Built-in parser with dot notation flattening.

##### `FormatUnknown`
Unknown or unsupported format - returned by DetectFormat() when format cannot be determined.

### Supported Formats

- **JSON** (.json): Full production support
- **YAML** (.yml, .yaml): Built-in + plugin support
- **TOML** (.toml): Built-in + plugin support  
- **HCL** (.hcl, .tf): Built-in + plugin support
- **INI** (.ini, .conf, .cfg): Built-in + plugin support
- **Properties** (.properties): Built-in + plugin support

### ConfigParser Interface

Interface for pluggable configuration parsers that enable production-grade parsing with full specification compliance.

```go
type ConfigParser interface {
    Parse(data []byte) (map[string]interface{}, error)
    Supports(format ConfigFormat) bool
    Name() string
}
```

#### Methods

##### `Parse(data []byte) (map[string]interface{}, error)`

Parses configuration data for supported formats.

**Parameters:**
- `data []byte`: Raw configuration file data

**Returns:**
- `map[string]interface{}`: Parsed configuration data  
- `error`: Parse error if data is invalid

##### `Supports(format ConfigFormat) bool`

Returns true if this parser can handle the given format.

**Parameters:**
- `format ConfigFormat`: Configuration format to check

**Returns:** `bool` - True if parser supports the format

##### `Name() string`

Returns a human-readable name for this parser for debugging and logging.

**Returns:** `string` - Parser name (e.g., "Advanced YAML Parser v2.1")

#### Parser Registration

Custom parsers are tried before built-in parsers, allowing for full specification compliance or advanced features not available in built-in parsers.

**Import-based Registration (Recommended):**
```go
import _ "github.com/your-org/argus-yaml-pro"   // Auto-registers in init()
import _ "github.com/your-org/argus-toml-pro"   // Auto-registers in init()
```

**Manual Registration:**
```go
argus.RegisterParser(&MyAdvancedYAMLParser{})
```

**Build Tags (Advanced):**
```bash
go build -tags "yaml_pro,toml_pro" ./...
```

### Using Parsers

```go
// Register custom parser
argus.RegisterParser("json", &MyJSONParser{})

// Parse configuration in callback
watcher.Watch("config.json", func(event argus.ChangeEvent) {
    if event.IsModify {
        data, _ := os.ReadFile(event.Path)
        config, err := argus.ParseConfig(event.Path, data)
        if err == nil {
            applyConfig(config)
        }
    }
})
```

## Advanced Features

### Audit and Compliance

Argus provides professional audit capabilities with unified SQLite backend for cross-application correlation.

#### AuditConfig

Configuration structure for the audit system:

```go
type AuditConfig struct {
    Enabled       bool          // Enable/disable audit logging
    OutputFile    string        // Path to audit storage (empty = SQLite, .jsonl = JSONL)
    MinLevel      AuditLevel    // Minimum audit level to log
    BufferSize    int           // Number of events to buffer
    FlushInterval time.Duration // How often to flush buffer
    IncludeStack  bool          // Include stack traces (for debugging)
}
```

#### Backend Selection

Argus automatically selects the appropriate audit backend:

```go
// Unified SQLite backend (recommended)
config := argus.Config{
    Audit: argus.DefaultAuditConfig(), // Uses unified SQLite
}

// Or explicit SQLite configuration
config := argus.Config{
    Audit: argus.AuditConfig{
        Enabled:    true,
        OutputFile: "",  // Empty = unified SQLite backend
        MinLevel:   argus.AuditCritical,
    },
}

// Legacy JSONL backend (backward compatibility)
config := argus.Config{
    Audit: argus.AuditConfig{
        Enabled:    true,
        OutputFile: "/var/log/argus-audit.jsonl", // .jsonl = JSONL backend
        MinLevel:   argus.AuditInfo,
    },
}
```

#### AuditLevel Constants

```go
const (
    AuditInfo     AuditLevel = iota // File watch events, system status
    AuditWarn                       // Performance warnings, parsing issues  
    AuditCritical                   // Configuration changes with before/after
    AuditSecurity                   // Access violations, suspicious activity
)
```

**See [Audit System Documentation](./audit-system.md) for comprehensive usage examples and best practices.**

### Performance Monitoring

Real-time performance metrics:

```go
go func() {
    ticker := time.NewTicker(30 * time.Second)
    for range ticker.C {
        stats := watcher.GetStats()
        fmt.Printf("Files: %d, Changes: %d, Cache Hit Rate: %.2f%%\n",
            stats.FilesWatched,
            stats.TotalChanges,
            float64(stats.CacheHits)/float64(stats.CacheHits+stats.CacheMisses)*100,
        )
    }
}()
```

## Performance Characteristics

### Overhead Analysis

Based on comprehensive benchmarking:

- **Polling overhead:** 12.11 nanoseconds per cycle
- **Memory footprint:** 8KB fixed + (64 bytes × files watched)
- **HTTP request impact:** +0.061ns per request (0.002%)
- **System impact:** 1.44µs every 5 seconds

### Optimization Guidelines

1. **Single config file:** Use `OptimizationSingleEvent`
2. **Few config files (3-20):** Use `OptimizationSmallBatch`  
3. **Many config files (20+):** Use `OptimizationLargeBatch`
4. **Unknown workload:** Use `OptimizationAuto` (default)

### Cache Tuning

```go
config := argus.Config{
    PollInterval: 5 * time.Second,
    CacheTTL:     2 * time.Second,  // 40% of poll interval
}
```

## Best Practices

### Configuration Management

```go
// Centralized configuration reloading
type ConfigManager struct {
    watcher *argus.Watcher
    config  atomic.Value
}

func (cm *ConfigManager) Start() error {
    cm.watcher.Watch("config.json", func(event argus.ChangeEvent) {
        if newConfig, err := LoadConfig(event.Path); err == nil {
            cm.config.Store(newConfig)
        }
    })
    return cm.watcher.Start()
}

func (cm *ConfigManager) GetConfig() *Config {
    return cm.config.Load().(*Config)
}
```

### Error Handling

```go
config := argus.Config{
    ErrorHandler: func(err error, path string) {
        switch {
        case errors.Is(err, os.ErrNotExist):
            log.Printf("Config file removed: %s", path)
            // Use default configuration
        case errors.Is(err, os.ErrPermission):
            log.Printf("Permission denied: %s", path)
            // Alert operations team
        default:
            log.Printf("Unexpected error for %s: %v", path, err)
        }
    },
}
```

### Graceful Shutdown

```go
func main() {
    watcher := argus.New(config)
    
    // Handle shutdown signals
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)
    
    go func() {
        <-c
        log.Println("Shutting down...")
        watcher.Stop()
        os.Exit(0)
    }()
    
    watcher.Start()
    select {} // Keep running
}
```

## Thread Safety

Argus is fully thread-safe and designed for concurrent use:

- **Watch/Unwatch:** Safe to call from multiple goroutines
- **Callbacks:** Executed sequentially to prevent race conditions
- **Statistics:** Atomic operations ensure consistent reads
- **Configuration:** Immutable after watcher creation

---

## Global Utility Functions

### Configuration Loading and Parsing

##### `LoadConfigMultiSource(configFile string) (*Config, error)`

Loads configuration with automatic precedence: Environment Variables > Configuration File > Defaults.

**Parameters:**
- `configFile string`: Path to configuration file

**Returns:**
- `*Config`: Loaded configuration with applied precedence
- `error`: Error if loading fails

**Precedence Order:**
1. Environment variables (highest priority)
2. Configuration file values  
3. Default values (lowest priority)

**Example:**
```go
config, err := argus.LoadConfigMultiSource("config.yaml")
if err != nil {
    log.Fatal(err)
}
watcher := argus.New(*config)
```

##### `ParseConfig(data []byte, format ConfigFormat) (map[string]interface{}, error)`

Parses configuration data in the specified format using built-in or registered parsers.

**Parameters:**
- `data []byte`: Raw configuration data
- `format ConfigFormat`: Configuration format (JSON, YAML, TOML, etc.)

**Returns:**
- `map[string]interface{}`: Parsed configuration data
- `error`: Parse error if format is invalid

**Parser Priority:**
1. Custom registered parsers (tried first)
2. Built-in parsers (fallback)

**Example:**
```go
data, _ := os.ReadFile("config.json")
config, err := argus.ParseConfig(data, argus.FormatJSON)
if err != nil {
    log.Fatal(err)
}
```

##### `DetectFormat(filePath string) ConfigFormat`

Automatically detects configuration format from file extension.

**Parameters:**
- `filePath string`: Path to configuration file

**Returns:** `ConfigFormat` - Detected format or FormatUnknown

**Supported Extensions:**
- `.json` → FormatJSON
- `.yml`, `.yaml` → FormatYAML  
- `.toml` → FormatTOML
- `.hcl`, `.tf` → FormatHCL
- `.ini`, `.conf`, `.cfg` → FormatINI
- `.properties` → FormatProperties

**Example:**
```go
format := argus.DetectFormat("config.yaml")
if format == argus.FormatUnknown {
    log.Fatal("Unsupported file format")
}
```

##### `RegisterParser(parser ConfigParser) `

Registers a custom parser for production use cases requiring full specification compliance.

**Parameters:**
- `parser ConfigParser`: Custom parser implementation

**Use Cases:**  
- Full YAML specification compliance
- Advanced TOML features  
- Custom configuration formats
- Enterprise parsing requirements

**Example:**
```go
argus.RegisterParser(&MyAdvancedYAMLParser{})
```

Or via import-based registration:
```go
import _ "github.com/your-org/argus-yaml-pro"
```

### Universal Configuration Watchers

##### `UniversalConfigWatcher(configPath string, callback func(config map[string]interface{})) (*Watcher, error)`

Creates a watcher for ANY configuration format with automatic format detection and parsing.

**Parameters:**
- `configPath string`: Path to configuration file (format auto-detected)
- `callback func(config map[string]interface{})`: Function called when configuration changes

**Returns:**
- `*Watcher`: Configured and started watcher
- `error`: Initialization error

**Features:**
- Automatic format detection from file extension
- Built-in parsing for all supported formats
- Automatic watcher startup
- Initial configuration loading

**Example:**
```go
watcher, err := argus.UniversalConfigWatcher("config.yml", func(config map[string]interface{}) {
    if level, ok := config["level"].(string); ok {
        log.Printf("Log level changed to: %s", level)
    }
    if port, ok := config["port"].(int); ok {
        log.Printf("Port changed to: %d", port)
    }
})
defer watcher.Stop()
```

##### `UniversalConfigWatcherWithConfig(configPath string, callback func(config map[string]interface{}), config Config) (*Watcher, error)`

Creates a universal configuration watcher with custom Argus configuration for performance tuning.

**Parameters:**
- `configPath string`: Path to configuration file
- `callback func(config map[string]interface{})`: Configuration change callback
- `config Config`: Custom Argus configuration for performance tuning

**Returns:**
- `*Watcher`: Configured and started watcher  
- `error`: Initialization error

**Example:**
```go
config := argus.Config{
    PollInterval: 1 * time.Second,
    OptimizationStrategy: argus.OptimizationSingleEvent,
}

watcher, err := argus.UniversalConfigWatcherWithConfig("config.json", 
    func(cfg map[string]interface{}) {
        // Handle configuration changes
    }, config)
```

##### `SimpleFileWatcher(filePath string, callback func(path string)) (*Watcher, error)`

Creates a basic file watcher without configuration parsing for simple use cases.

**Parameters:**
- `filePath string`: Path to file to watch
- `callback func(path string)`: Function called with file path when changes occur

**Returns:**
- `*Watcher`: Configured watcher (NOT automatically started)
- `error`: Setup error

**Note:** Unlike Universal watchers, SimpleFileWatcher does NOT auto-start. Call `Start()` manually.

**Example:**
```go
watcher, err := argus.SimpleFileWatcher("/var/log/app.log", func(path string) {
    log.Printf("Log file changed: %s", path)
})
if err != nil {
    log.Fatal(err)
}

// Must start manually
watcher.Start()
defer watcher.Stop()
```

##### `GenericConfigWatcher(configPath string, callback func(config map[string]interface{})) (*Watcher, error)`

**DEPRECATED:** Use UniversalConfigWatcher for better format support and future-proofing.

This function is maintained for existing codebases but new code should use UniversalConfigWatcher.

### Security Functions

##### `ValidateSecurePath(path string) error`

Validates that a file path is safe from path traversal attacks and security vulnerabilities.

**Parameters:**
- `path string`: File path to validate

**Returns:** `error` - Error if path is unsafe

**Security Protection:**
- Directory traversal prevention (CWE-22)
- URL decoding attack detection  
- System file protection
- Windows device name protection
- Symlink traversal validation
- Path length and complexity limits

**Example:**
```go
if err := argus.ValidateSecurePath(userProvidedPath); err != nil {
    return fmt.Errorf("unsafe path: %w", err)
}
```

**Critical:** Call this function on ALL user-provided paths before file operations.

### Remote Configuration

##### `NewRemoteConfigWithFallback(primaryURL, fallbackURL, localPath string) *RemoteConfigManager`

Creates a new RemoteConfigManager with automatic fallback sequence for enterprise deployments.

**Parameters:**
- `primaryURL string`: Primary remote configuration endpoint (required)
- `fallbackURL string`: Secondary endpoint for failover (optional, can be empty)  
- `localPath string`: Local fallback file path (optional, can be empty)

**Returns:** `*RemoteConfigManager` - Configured remote config manager

**Fallback Sequence:**
1. **Primary URL** → Try primary remote endpoint
2. **Fallback URL** → Try secondary remote endpoint (if provided)
3. **Local Path** → Load local configuration file (if provided)
4. **Error** → All sources failed

**Example:**
```go
// Full enterprise setup with all fallback layers
remoteManager := argus.NewRemoteConfigWithFallback(
    "https://consul.prod:8500/v1/kv/app/config",
    "https://consul.backup:8500/v1/kv/app/config", 
    "/etc/myapp/fallback.json",
)

// Use with watcher
watcher := argus.New(argus.Config{
    Remote: remoteManager.Config(),
})

```
---

Argus • an AGILira fragment
