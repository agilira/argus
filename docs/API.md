# Argus API Reference

Complete technical reference for all Argus APIs, types, and configuration options.

**For getting started quickly, see [Quick Start Guide](./QUICK_START.md)**

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

##### `IsRunning() bool`

Returns whether the watcher is currently active.

**Returns:** `bool` - True if watcher is running

##### `GetStats() Stats`

Returns performance and operational statistics.

**Returns:** `Stats` - Current watcher statistics

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

## Error Codes

Argus uses structured error codes for better error handling:

- `ARGUS_INVALID_CONFIG`: Invalid configuration provided
- `ARGUS_FILE_NOT_FOUND`: Watched file does not exist
- `ARGUS_WATCHER_STOPPED`: Operation attempted on stopped watcher
- `ARGUS_WATCHER_BUSY`: Watcher is already running

## Configuration File Parsing

Argus includes universal configuration parsers for common formats:

### Supported Formats

- **JSON** (.json): Full production support
- **YAML** (.yml, .yaml): Built-in + plugin support
- **TOML** (.toml): Built-in + plugin support  
- **HCL** (.hcl, .tf): Built-in + plugin support
- **INI** (.ini, .conf, .cfg): Built-in + plugin support
- **Properties** (.properties): Built-in + plugin support

### Parser Interface

```go
type ConfigParser interface {
    Parse(data []byte) (map[string]interface{}, error)
    Supports(filename string) bool
}
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

Built-in audit capabilities for security-sensitive environments:

```go
config := argus.Config{
    Audit: argus.AuditConfig{
        Enabled:    true,
        LogLevel:   argus.AuditInfo,
        OutputPath: "/var/log/argus-audit.log",
    },
}
```

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

## License

Argus is licensed under the Mozilla Public License 2.0 (MPL-2.0).

## Support

For issues, feature requests, and contributions, visit the [GitHub repository](https://github.com/agilira/argus).

---

Argus • an AGILira fragment
