# Argus Flags System Documentation

## Overview

The Argus Flags System provides a **lock-free, ultra-high-performance** configuration management solution with command line flags integration. This system is designed to achieve **sub-15ns operation latency** while supporting multiple configuration sources with automatic precedence resolution.

## Key Features

- **Lock-Free Architecture**: Zero locks, atomic operations only
- **Ultra-High Performance**: Target <15ns per operation  
- **Multi-Source Configuration**: Flags, environment variables, config files, defaults
- **Automatic Precedence**: Intelligent source prioritization
- **Type Safety**: Type-safe getters with automatic conversion
- **pflag/Cobra Compatible**: Seamless integration with existing CLI frameworks
- **Zero-Allocation Timestamps**: Uses TimeCaches library for performance

## Architecture

### Core Components

#### 1. LockFreeConfigManager

The main configuration manager that handles all operations atomically.

```go
type LockFreeConfigManager struct {
    // Lock-free storage using atomic operations and copy-on-write
    entries atomic.Pointer[map[lockFreeConfigKey]*lockFreeConfigEntry]
    
    // Flag bindings for CLI integration
    flagBindings map[string]*lockFreeFlagBinding
    
    // Watcher for file changes
    watcher *Watcher
}
```

#### 2. Configuration Sources & Precedence

Sources are prioritized in the following order (highest to lowest):

1. **Explicit** (`sourceLockFreeExplicit`) - Set via `Set()`
2. **Flags** (`sourceLockFreeFlags`) - Command line flags
3. **Environment Variables** (`sourceLockFreeEnvVars`) - Set via `SetEnvVar()`
4. **Configuration Files** (`sourceLockFreeConfigFile`) - Set via `SetConfigFile()`
5. **Defaults** (`sourceLockFreeDefaults`) - Set via `SetDefault()`

#### 3. Interfaces

##### LockFreeFlag Interface
```go
type LockFreeFlag interface {
    Name() string
    Value() interface{}
    Type() string
    Changed() bool
}
```

##### LockFreeFlagSet Interface
```go
type LockFreeFlagSet interface {
    VisitAll(func(LockFreeFlag))
    Lookup(name string) LockFreeFlag
}
```

## Basic Usage

### 1. Creating a Configuration Manager

```go
import "github.com/agilira/argus"

// Create a new lock-free configuration manager
config := argus.NewLockFreeConfigManager()
```

### 2. Setting Configuration Values

```go
// Set default values (lowest precedence)
config.SetDefault("server.port", 8080)
config.SetDefault("server.host", "localhost")

// Set from config file
config.SetConfigFile("database.timeout", "30s")

// Set from environment variables
config.SetEnvVar("debug.enabled", true)

// Set from command line flags
config.SetFlag("server.port", 9090)

// Set explicit values (highest precedence)
config.Set("app.version", "1.0.0")
```

### 3. Getting Configuration Values

#### Type-Safe Getters

```go
// String values
host := config.GetString("server.host")

// Integer values  
port := config.GetInt("server.port")

// Boolean values
debug := config.GetBool("debug.enabled")

// Duration values
timeout := config.GetDuration("database.timeout")

// String slice values
hosts := config.GetStringSlice("allowed.hosts")

// Generic interface{} getter
value := config.Get("any.key")
```

## Command Line Flags Integration

### 1. pflag/Cobra Integration

```go
import (
    "github.com/agilira/argus"
    "github.com/spf13/pflag"
)

// Create Argus config manager
config := argus.NewLockFreeConfigManager()

// Create pflag command line flags
flagSet := pflag.NewFlagSet("myapp", pflag.ExitOnError)
flagSet.String("server-host", "localhost", "Server host")
flagSet.Int("server-port", 8080, "Server port") 
flagSet.Bool("debug", false, "Enable debug mode")

// Parse command line
flagSet.Parse(os.Args[1:])

// Create adapter for pflag compatibility
type PFlagAdapter struct {
    flag *pflag.Flag
}

func (p *PFlagAdapter) Name() string        { return p.flag.Name }
func (p *PFlagAdapter) Value() interface{}  { /* implementation */ }
func (p *PFlagAdapter) Type() string        { return p.flag.Value.Type() }
func (p *PFlagAdapter) Changed() bool       { return p.flag.Changed }

type PFlagSetAdapter struct {
    flagSet *pflag.FlagSet
}

func (p *PFlagSetAdapter) VisitAll(fn func(argus.LockFreeFlag)) {
    p.flagSet.VisitAll(func(flag *pflag.Flag) {
        fn(&PFlagAdapter{flag: flag})
    })
}

func (p *PFlagSetAdapter) Lookup(name string) argus.LockFreeFlag {
    flag := p.flagSet.Lookup(name)
    if flag == nil {
        return nil
    }
    return &PFlagAdapter{flag: flag}
}

// Bind all flags to Argus
adapter := &PFlagSetAdapter{flagSet: flagSet}
err := config.BindPFlags(adapter)
```

### 2. Flag Name Mapping

Flag names are automatically converted to configuration keys:
- Hyphens (`-`) are converted to dots (`.`)
- Example: `--server-port` becomes config key `server.port`

### 3. Binding Individual Flags

```go
// Bind a single flag
err := config.BindPFlag("server.port", flagAdapter)
if err != nil {
    log.Fatalf("Failed to bind flag: %v", err)
}
```

## Advanced Features

### 1. Cache Statistics

```go
total, valid := config.GetCacheStats()
fmt.Printf("Cache: %d total entries, %d valid\n", total, valid)
```

### 2. Bound Flags Inspection

```go
boundFlags := config.GetBoundFlags()
for configKey, flagName := range boundFlags {
    fmt.Printf("Config '%s' -> Flag '%s'\n", configKey, flagName)
}
```

### 3. Flag Refresh

```go
// Refresh all bound flags from their current values
err := config.RefreshFlags()
if err != nil {
    log.Printf("Failed to refresh flags: %v", err)
}
```

## Performance Characteristics

### Benchmarks

The lock-free implementation achieves the following performance characteristics:

- **Target**: <15ns per operation
- **Achieved**: ~15.5ns per operation (isolated environment)
- **Production**: ~18-31ns per operation (depending on overhead)

### Performance Optimizations

1. **Atomic Operations**: All reads/writes use `atomic.Pointer`
2. **Copy-on-Write**: Map updates use COW semantics
3. **Zero-Allocation Timestamps**: TimeCaches library integration
4. **Inlined Precedence**: Direct if-else chains for maximum speed
5. **Struct Keys**: Fast lookup using structured keys vs string formatting

### Benchmark Example

```go
func BenchmarkConfigManager(b *testing.B) {
    config := argus.NewLockFreeConfigManager()
    config.Set("test.key", "test-value")
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = config.GetString("test.key")
    }
}
```

## Error Handling

### Flag Binding Errors

```go
err := config.BindPFlags(flagSet)
if err != nil {
    // Handle binding errors
    log.Fatalf("Flag binding failed: %v", err)
}
```

### Type Conversion Errors

The system handles type conversions gracefully:
- Invalid conversions return zero values
- String representations are attempted for complex types
- Reflection fallback for unknown types

## Thread Safety

### Lock-Free Guarantees

- **Read Operations**: Always lock-free using `atomic.LoadPointer`
- **Write Operations**: Lock-free using copy-on-write semantics
- **Concurrent Access**: Safe for unlimited concurrent readers and writers
- **Memory Ordering**: All operations use proper memory barriers

### Safe Usage Patterns

```go
// Safe concurrent reads
go func() {
    for {
        value := config.GetString("some.key")
        // Process value...
    }
}()

// Safe concurrent writes
go func() {
    for {
        config.Set("some.key", newValue)
        // Continue...
    }
}()
```

## Integration Examples

### 1. Web Server Configuration

```go
func main() {
    config := argus.NewLockFreeConfigManager()
    
    // Set defaults
    config.SetDefault("server.host", "localhost")
    config.SetDefault("server.port", 8080)
    config.SetDefault("server.timeout", "30s")
    
    // Setup CLI flags
    flagSet := pflag.NewFlagSet("webserver", pflag.ExitOnError)
    flagSet.String("host", "", "Server host")
    flagSet.Int("port", 0, "Server port")
    flagSet.Duration("timeout", 0, "Request timeout")
    
    flagSet.Parse(os.Args[1:])
    
    // Bind flags
    adapter := &PFlagSetAdapter{flagSet: flagSet}
    config.BindPFlags(adapter)
    
    // Use configuration
    server := &http.Server{
        Addr:         fmt.Sprintf("%s:%d", 
                        config.GetString("server.host"),
                        config.GetInt("server.port")),
        ReadTimeout:  config.GetDuration("server.timeout"),
        WriteTimeout: config.GetDuration("server.timeout"),
    }
    
    log.Fatal(server.ListenAndServe())
}
```

### 2. Database Configuration

```go
type DatabaseConfig struct {
    Host     string
    Port     int  
    Database string
    Timeout  time.Duration
    SSL      bool
}

func LoadDatabaseConfig(config *argus.LockFreeConfigManager) DatabaseConfig {
    return DatabaseConfig{
        Host:     config.GetString("db.host"),
        Port:     config.GetInt("db.port"),
        Database: config.GetString("db.name"),
        Timeout:  config.GetDuration("db.timeout"),
        SSL:      config.GetBool("db.ssl"),
    }
}
```

## Best Practices

### 1. Configuration Organization

```go
// Use hierarchical keys
config.SetDefault("server.http.port", 8080)
config.SetDefault("server.grpc.port", 9090)
config.SetDefault("database.read.timeout", "5s")
config.SetDefault("database.write.timeout", "10s")
```

### 2. Initialization Pattern

```go
func NewAppConfig() *argus.LockFreeConfigManager {
    config := argus.NewLockFreeConfigManager()
    
    // Set all defaults first
    setDefaults(config)
    
    // Load config files
    loadConfigFiles(config)
    
    // Apply environment variables
    loadEnvironment(config)
    
    // Bind command line flags
    bindFlags(config)
    
    return config
}
```

### 3. Performance Considerations

- Prefer specific getters (`GetString`, `GetInt`) over generic `Get()`
- Set defaults early to avoid nil checks
- Use cache statistics to monitor performance
- Avoid frequent flag rebinding in hot paths

## Troubleshooting

### Common Issues

1. **Flag Not Found**: Check flag name mapping (hyphens to dots)
2. **Type Conversion Errors**: Verify flag types match expected types
3. **Performance Issues**: Use benchmark tools to identify bottlenecks
4. **Memory Leaks**: Ensure proper flag binding lifecycle

### Debug Information

```go
// Check bound flags
boundFlags := config.GetBoundFlags()
fmt.Printf("Bound flags: %+v\n", boundFlags)

// Check cache statistics  
total, valid := config.GetCacheStats()
fmt.Printf("Cache stats: %d/%d\n", valid, total)

// Performance test
start := time.Now()
for i := 0; i < 1000000; i++ {
    _ = config.GetString("test.key")
}
fmt.Printf("1M operations: %v\n", time.Since(start))
```

## API Reference

### Constructor
- `NewLockFreeConfigManager() *LockFreeConfigManager`

### Setters
- `Set(key string, value interface{})` - Explicit (highest precedence)
- `SetFlag(key string, value interface{})` - Flag source
- `SetEnvVar(key string, value interface{})` - Environment variable source  
- `SetConfigFile(key string, value interface{})` - Config file source
- `SetDefault(key string, value interface{})` - Default (lowest precedence)

### Getters
- `Get(key string) interface{}` - Generic getter
- `GetString(key string) string` - Type-safe string getter
- `GetInt(key string) int` - Type-safe integer getter
- `GetBool(key string) bool` - Type-safe boolean getter
- `GetDuration(key string) time.Duration` - Type-safe duration getter
- `GetStringSlice(key string) []string` - Type-safe string slice getter

### Flag Integration
- `BindPFlags(flagSet LockFreeFlagSet) error` - Bind entire flag set
- `BindPFlag(configKey string, flag LockFreeFlag) error` - Bind single flag
- `RefreshFlags() error` - Refresh all bound flags

### Utilities
- `GetBoundFlags() map[string]string` - Get flag binding mappings
- `GetCacheStats() (total, valid int)` - Get cache statistics

---

Argus • an AGILira fragment
