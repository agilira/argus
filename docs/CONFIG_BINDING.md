# Configuration Binding System

Argus provides a high-performance configuration binding system that eliminates reflection overhead while maintaining type safety and excellent developer experience.

## Overview

The binding system uses a fluent API pattern to bind configuration values directly to Go variables with zero reflection and minimal allocations. This approach is significantly faster than traditional unmarshaling while providing compile-time type safety.

## Performance Characteristics

```
Benchmark Results:
- 744ns per 15-binding operation
- 1,609,530 operations per second
- Only 1 allocation per bind operation
- Zero reflection overhead
- 50x faster than reflection-based unmarshaling
```

## Basic Usage

### Simple Binding

```go
package main

import (
    "encoding/json"
    "fmt"
    "time"
    "github.com/agilira/argus"
)

func main() {
    // Parse your configuration (from JSON, YAML, etc.)
    var config map[string]interface{}
    json.Unmarshal(configBytes, &config)
    
    // Declare variables to bind to
    var (
        appName    string
        port       int
        debug      bool
        timeout    time.Duration
    )
    
    // Bind configuration values
    err := argus.BindFromConfig(config).
        BindString(&appName, "app_name").
        BindInt(&port, "port").
        BindBool(&debug, "debug").
        BindDuration(&timeout, "timeout").
        Apply()
    
    if err != nil {
        log.Fatal(err)
    }
    
    // Variables are now populated and ready to use
    fmt.Printf("App: %s, Port: %d, Debug: %t\n", appName, port, debug)
}
```

### With Default Values

```go
var (
    host    string
    port    int
    enabled bool
)

err := argus.BindFromConfig(config).
    BindString(&host, "server.host", "localhost").    // Default: "localhost"
    BindInt(&port, "server.port", 8080).              // Default: 8080
    BindBool(&enabled, "server.enabled", true).       // Default: true
    Apply()
```

## Supported Types

The binding system supports all common Go types with intelligent type conversion:

### Basic Types

| Method | Go Type | JSON Types | Notes |
|--------|---------|------------|-------|
| `BindString` | `*string` | string, number, bool | Converts any value to string |
| `BindInt` | `*int` | number, string | Parses string numbers |
| `BindInt64` | `*int64` | number, string | Supports large integers |
| `BindBool` | `*bool` | bool, string, number | "true"/"false", 0/non-zero |
| `BindFloat64` | `*float64` | number, string | Parses string floats |
| `BindDuration` | `*time.Duration` | string, number | Parses "30s", "5m", etc. |

### Type Conversion Examples

```go
config := map[string]interface{}{
    "string_int":    "42",           // String "42" → int 42
    "float_to_int":  42.7,           // float64 42.7 → int 42
    "int_to_bool":   1,              // int 1 → bool true
    "zero_to_bool":  0,              // int 0 → bool false
    "string_bool":   "true",         // string "true" → bool true
    "int_duration":  5000000000,     // int64 nanoseconds → 5s duration
}

var (
    stringInt   int
    floatToInt  int
    intToBool   bool
    zeroToBool  bool
    stringBool  bool
    intDuration time.Duration
)

err := argus.BindFromConfig(config).
    BindInt(&stringInt, "string_int").
    BindInt(&floatToInt, "float_to_int").
    BindBool(&intToBool, "int_to_bool").
    BindBool(&zeroToBool, "zero_to_bool").
    BindBool(&stringBool, "string_bool").
    BindDuration(&intDuration, "int_duration").
    Apply()
```

## Nested Configuration Keys

The binding system supports dot-notation for nested configuration structures:

```go
config := map[string]interface{}{
    "database": map[string]interface{}{
        "host": "localhost",
        "port": 5432,
        "pool": map[string]interface{}{
            "max_connections": 20,
            "idle_timeout":    "5m",
        },
    },
}

var (
    dbHost        string
    dbPort        int
    maxConns      int
    idleTimeout   time.Duration
)

err := argus.BindFromConfig(config).
    BindString(&dbHost, "database.host").
    BindInt(&dbPort, "database.port").
    BindInt(&maxConns, "database.pool.max_connections").
    BindDuration(&idleTimeout, "database.pool.idle_timeout").
    Apply()
```

## Error Handling

The binding system provides detailed error messages for debugging:

```go
config := map[string]interface{}{
    "invalid_int":  "not-a-number",
    "invalid_bool": "maybe",
}

var (
    invalidInt  int
    invalidBool bool
)

err := argus.BindFromConfig(config).
    BindInt(&invalidInt, "invalid_int").
    BindBool(&invalidBool, "invalid_bool").
    Apply()

if err != nil {
    // Error: failed to bind key 'invalid_int': strconv.Atoi: parsing "not-a-number": invalid syntax
    fmt.Printf("Binding error: %v\n", err)
}
```

### Error Behavior

- **Fail-fast**: The first binding error stops execution and returns immediately
- **Detailed context**: Error messages include the key name and conversion details
- **Type safety**: Compile-time guarantees prevent most runtime errors

## Integration with File Watching

Combine configuration binding with Argus file watching for dynamic configuration:

```go
package main

import (
    "encoding/json"
    "log"
    "sync/atomic"
    "time"
    "github.com/agilira/argus"
)

// Application configuration
var (
    serverHost     atomic.Value // string
    serverPort     atomic.Value // int
    debugEnabled   atomic.Value // bool
    requestTimeout atomic.Value // time.Duration
)

func main() {
    // Watch configuration file
    watcher := argus.New(argus.Config{
        PollInterval: 5 * time.Second,
    })
    
    watcher.Watch("config.json", func(event argus.ChangeEvent) {
        updateConfig(event.Path)
    })
    
    // Initial load
    updateConfig("config.json")
    
    watcher.Start()
    defer watcher.Stop()
    
    // Use configuration in your application
    startServer()
}

func updateConfig(configPath string) {
    // Load and parse configuration
    data, err := os.ReadFile(configPath)
    if err != nil {
        log.Printf("Failed to read config: %v", err)
        return
    }
    
    var config map[string]interface{}
    if err := json.Unmarshal(data, &config); err != nil {
        log.Printf("Failed to parse config: %v", err)
        return
    }
    
    // Local variables for binding
    var (
        host    string
        port    int
        debug   bool
        timeout time.Duration
    )
    
    // Bind configuration
    err = argus.BindFromConfig(config).
        BindString(&host, "server.host", "localhost").
        BindInt(&port, "server.port", 8080).
        BindBool(&debug, "debug", false).
        BindDuration(&timeout, "request_timeout", 30*time.Second).
        Apply()
    
    if err != nil {
        log.Printf("Configuration binding failed: %v", err)
        return
    }
    
    // Atomically update global configuration
    serverHost.Store(host)
    serverPort.Store(port)
    debugEnabled.Store(debug)
    requestTimeout.Store(timeout)
    
    log.Printf("Configuration updated: %s:%d (debug=%t)", host, port, debug)
}

func startServer() {
    // Use atomic values in your application
    host := serverHost.Load().(string)
    port := serverPort.Load().(int)
    
    // Start your server with current configuration
    log.Printf("Starting server on %s:%d", host, port)
}
```

## Performance Optimization

### Pre-allocation for High-Frequency Binding

```go
// For applications that bind configuration frequently,
// pre-allocate the binder to avoid slice growth
func optimizedBinding(config map[string]interface{}) error {
    binder := argus.NewConfigBinder(config)
    
    // Pre-allocate capacity if you know the number of bindings
    // This is done automatically, but can be tuned for specific workloads
    
    return binder.
        BindString(&var1, "key1").
        BindInt(&var2, "key2").
        // ... more bindings
        Apply()
}
```

### Benchmark Your Usage

```go
func BenchmarkYourConfigBinding(b *testing.B) {
    config := map[string]interface{}{
        "key1": "value1",
        "key2": 42,
        // ... your configuration
    }
    
    var (
        var1 string
        var2 int
    )
    
    b.ResetTimer()
    b.ReportAllocs()
    
    for i := 0; i < b.N; i++ {
        err := argus.BindFromConfig(config).
            BindString(&var1, "key1").
            BindInt(&var2, "key2").
            Apply()
        
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

## API Reference

### Core Functions

#### `BindFromConfig(config map[string]interface{}) *ConfigBinder`

Creates a new configuration binder from a parsed configuration map.

**Parameters:**
- `config`: A map containing configuration data (typically from JSON, YAML, etc.)

**Returns:**
- `*ConfigBinder`: A new binder instance ready for chaining

#### `NewConfigBinder(config map[string]interface{}) *ConfigBinder`

Alternative constructor for creating a configuration binder.

### Binding Methods

All binding methods follow the same pattern and support optional default values:

#### `BindString(target *string, key string, defaultValue ...string) *ConfigBinder`

Binds a string value from configuration.

**Parameters:**
- `target`: Pointer to the string variable to populate
- `key`: Configuration key (supports dot notation for nested keys)
- `defaultValue`: Optional default value if key is not found

#### `BindInt(target *int, key string, defaultValue ...int) *ConfigBinder`

Binds an integer value from configuration.

#### `BindInt64(target *int64, key string, defaultValue ...int64) *ConfigBinder`

Binds a 64-bit integer value from configuration.

#### `BindBool(target *bool, key string, defaultValue ...bool) *ConfigBinder`

Binds a boolean value from configuration.

#### `BindFloat64(target *float64, key string, defaultValue ...float64) *ConfigBinder`

Binds a 64-bit floating point value from configuration.

#### `BindDuration(target *time.Duration, key string, defaultValue ...time.Duration) *ConfigBinder`

Binds a time.Duration value from configuration. Supports string duration formats like "30s", "5m", "1h".

### Execution Methods

#### `Apply() error`

Executes all accumulated bindings in a single optimized pass.

**Returns:**
- `error`: nil on success, or detailed error information on failure

## Common Patterns

### Configuration Struct Alternative

Instead of using structs with reflection, use the binding pattern:

```go
// Instead of this (reflection-based):
type Config struct {
    Database struct {
        Host     string `json:"host"`
        Port     int    `json:"port"`
        Password string `json:"password"`
    } `json:"database"`
    Server struct {
        Port    int           `json:"port"`
        Timeout time.Duration `json:"timeout"`
    } `json:"server"`
}

// Use this (zero-reflection):
var (
    dbHost     string
    dbPort     int
    dbPassword string
    srvPort    int
    srvTimeout time.Duration
)

err := argus.BindFromConfig(config).
    BindString(&dbHost, "database.host").
    BindInt(&dbPort, "database.port").
    BindString(&dbPassword, "database.password").
    BindInt(&srvPort, "server.port").
    BindDuration(&srvTimeout, "server.timeout").
    Apply()
```

### Configuration Validation

```go
// Bind configuration and validate
var (
    port    int
    threads int
)

err := argus.BindFromConfig(config).
    BindInt(&port, "port", 8080).
    BindInt(&threads, "threads", 4).
    Apply()

if err != nil {
    return fmt.Errorf("configuration binding failed: %w", err)
}

// Validate bound values
if port < 1 || port > 65535 {
    return fmt.Errorf("invalid port: %d (must be 1-65535)", port)
}

if threads < 1 || threads > 32 {
    return fmt.Errorf("invalid threads: %d (must be 1-32)", threads)
}
```

## Troubleshooting

### Common Issues

1. **Missing Key with No Default**
   ```
   Error: failed to bind key 'database.host': key not found
   ```
   Solution: Provide a default value or ensure the key exists in configuration.

2. **Type Conversion Failure**
   ```
   Error: failed to bind key 'port': strconv.Atoi: parsing "abc": invalid syntax
   ```
   Solution: Ensure configuration values are compatible with target types.

3. **Nested Key Not Found**
   ```
   Error: failed to bind key 'database.pool.size': key not found
   ```
   Solution: Verify the nested structure exists in your configuration map.

### Debugging Tips

1. **Print Configuration Structure**
   ```go
   fmt.Printf("Config: %+v\n", config)
   ```

2. **Test Individual Bindings**
   ```go
   // Test one binding at a time to isolate issues
   err := argus.BindFromConfig(config).
       BindString(&var1, "key1").
       Apply()
   ```

3. **Validate Configuration Keys**
   ```go
   if _, exists := config["expected_key"]; !exists {
       log.Printf("Warning: expected_key not found in configuration")
   }
   ```

## Migration Guide

### From reflect-based unmarshaling

```go
// Before (with reflection)
type Config struct {
    Host string `json:"host"`
    Port int    `json:"port"`
}

var config Config
json.Unmarshal(data, &config)

// After (zero reflection)
var (
    host string
    port int
)

var configMap map[string]interface{}
json.Unmarshal(data, &configMap)

err := argus.BindFromConfig(configMap).
    BindString(&host, "host").
    BindInt(&port, "port").
    Apply()
```

### From manual key access

```go
// Before (manual with type assertions)
host := config["host"].(string)
port := config["port"].(int)

// After (type-safe binding)
var (
    host string
    port int
)

err := argus.BindFromConfig(config).
    BindString(&host, "host").
    BindInt(&port, "port").
    Apply()
```

## Best Practices

1. **Use defaults for optional configuration**
2. **Validate critical values after binding**
3. **Handle binding errors appropriately**
4. **Use atomic values for concurrent access in dynamic configuration**
5. **Benchmark your specific binding patterns**
6. **Group related bindings logically in your code**
7. **Document your configuration schema**

The configuration binding system provides exceptional performance while maintaining the safety and readability that Go developers expect. It's particularly effective for high-performance applications that need frequent configuration updates.
