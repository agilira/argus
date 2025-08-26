# Argus Flags API Reference

## Types

### LockFreeConfigManager

The main configuration manager providing lock-free, high-performance configuration management.

```go
type LockFreeConfigManager struct {
    // Private fields - use only through methods
}
```

### LockFreeFlag Interface

Interface for command line flags that can be bound to configuration.

```go
type LockFreeFlag interface {
    Name() string           // Flag name (e.g., "server-port")
    Value() interface{}     // Current flag value
    Type() string          // Flag type ("string", "int", "bool", etc.)
    Changed() bool         // Whether flag was set on command line
}
```

### LockFreeFlagSet Interface

Interface for a collection of flags (compatible with pflag.FlagSet).

```go
type LockFreeFlagSet interface {
    VisitAll(func(LockFreeFlag))              // Iterate over all flags
    Lookup(name string) LockFreeFlag          // Find flag by name
}
```

## Constructor

### NewLockFreeConfigManager

```go
func NewLockFreeConfigManager() *LockFreeConfigManager
```

Creates a new lock-free configuration manager with initialized atomic storage.

**Returns:**
- `*LockFreeConfigManager`: New configuration manager instance

**Example:**
```go
config := argus.NewLockFreeConfigManager()
```

## Configuration Setters

All setters are **thread-safe** and use **atomic operations**. Values are stored with source precedence information.

### Set

```go
func (cm *LockFreeConfigManager) Set(key string, value interface{})
```

Sets a configuration value with **highest precedence** (explicit source).

**Parameters:**
- `key`: Configuration key (e.g., "server.port")
- `value`: Any value (string, int, bool, time.Duration, etc.)

**Example:**
```go
config.Set("server.port", 8080)
config.Set("app.name", "MyApp")
config.Set("debug.enabled", true)
```

### SetDefault

```go
func (cm *LockFreeConfigManager) SetDefault(key string, value interface{})
```

Sets a default configuration value with **lowest precedence**.

**Parameters:**
- `key`: Configuration key
- `value`: Default value

**Example:**
```go
config.SetDefault("server.port", 8080)
config.SetDefault("server.host", "localhost")
config.SetDefault("database.timeout", "30s")
```

### SetConfigFile

```go
func (cm *LockFreeConfigManager) SetConfigFile(key string, value interface{})
```

Sets a configuration value from config file source.

**Parameters:**
- `key`: Configuration key
- `value`: Value from config file

**Example:**
```go
config.SetConfigFile("database.url", "postgres://localhost/mydb")
config.SetConfigFile("log.level", "INFO")
```

### SetEnvVar

```go
func (cm *LockFreeConfigManager) SetEnvVar(key string, value interface{})
```

Sets a configuration value from environment variable source.

**Parameters:**
- `key`: Configuration key
- `value`: Value from environment variable

**Example:**
```go
config.SetEnvVar("database.password", os.Getenv("DB_PASSWORD"))
config.SetEnvVar("api.key", os.Getenv("API_KEY"))
```

### SetFlag

```go
func (cm *LockFreeConfigManager) SetFlag(key string, value interface{})
```

Sets a configuration value from command line flag source.

**Parameters:**
- `key`: Configuration key
- `value`: Value from command line flag

**Example:**
```go
config.SetFlag("server.port", 9090)
config.SetFlag("verbose", true)
```

## Configuration Getters

All getters are **lock-free** and provide **type-safe** access with automatic type conversion.

### Get

```go
func (cm *LockFreeConfigManager) Get(key string) interface{}
```

Gets a configuration value as `interface{}`. Returns highest precedence value.

**Parameters:**
- `key`: Configuration key

**Returns:**
- `interface{}`: Configuration value or `nil` if not found

**Example:**
```go
value := config.Get("server.port")
if value != nil {
    port := value.(int)
}
```

### GetString

```go
func (cm *LockFreeConfigManager) GetString(key string) string
```

Gets a configuration value as string with automatic type conversion.

**Parameters:**
- `key`: Configuration key

**Returns:**
- `string`: String value or empty string if not found

**Type Conversions:**
- `string` → direct return
- `int`, `int64` → converted using `strconv`
- `float64` → converted using `strconv`
- `bool` → "true" or "false"
- Other types → `fmt.Sprintf("%v", value)`

**Example:**
```go
host := config.GetString("server.host")
version := config.GetString("app.version")
```

### GetInt

```go
func (cm *LockFreeConfigManager) GetInt(key string) int
```

Gets a configuration value as integer with automatic type conversion.

**Parameters:**
- `key`: Configuration key

**Returns:**
- `int`: Integer value or 0 if not found or conversion fails

**Type Conversions:**
- `int` → direct return
- `int32`, `int64` → converted to `int`
- `float64` → converted to `int`
- `string` → parsed using `strconv.Atoi`
- `bool` → 1 for true, 0 for false

**Example:**
```go
port := config.GetInt("server.port")
workers := config.GetInt("app.workers")
```

### GetBool

```go
func (cm *LockFreeConfigManager) GetBool(key string) bool
```

Gets a configuration value as boolean with automatic type conversion.

**Parameters:**
- `key`: Configuration key

**Returns:**
- `bool`: Boolean value or false if not found

**Type Conversions:**
- `bool` → direct return
- `string` → parsed ("true", "1", "yes", "on" = true)
- `int` → non-zero = true, zero = false
- `float64` → non-zero = true, zero = false

**Example:**
```go
debug := config.GetBool("debug.enabled")
ssl := config.GetBool("server.ssl")
```

### GetDuration

```go
func (cm *LockFreeConfigManager) GetDuration(key string) time.Duration
```

Gets a configuration value as `time.Duration` with automatic parsing.

**Parameters:**
- `key`: Configuration key

**Returns:**
- `time.Duration`: Duration value or 0 if not found or parsing fails

**Type Conversions:**
- `time.Duration` → direct return
- `string` → parsed using `time.ParseDuration` ("30s", "5m", "1h")
- `int`, `int64` → nanoseconds
- `float64` → nanoseconds

**Example:**
```go
timeout := config.GetDuration("database.timeout")
interval := config.GetDuration("health.check.interval")
```

### GetStringSlice

```go
func (cm *LockFreeConfigManager) GetStringSlice(key string) []string
```

Gets a configuration value as string slice.

**Parameters:**
- `key`: Configuration key

**Returns:**
- `[]string`: String slice or empty slice if not found

**Type Conversions:**
- `[]string` → direct return
- `string` → split by comma
- `[]interface{}` → convert each element to string

**Example:**
```go
hosts := config.GetStringSlice("allowed.hosts")
tags := config.GetStringSlice("app.tags")
```

## Flag Integration

### BindPFlags

```go
func (cm *LockFreeConfigManager) BindPFlags(flagSet LockFreeFlagSet) error
```

Binds all flags from a flag set to configuration keys.

**Parameters:**
- `flagSet`: Flag set implementing `LockFreeFlagSet` interface

**Returns:**
- `error`: Binding error or nil on success

**Flag Name Mapping:**
- Hyphens (`-`) are converted to dots (`.`)
- Example: `--server-port` → `server.port`

**Example:**
```go
adapter := &PFlagSetAdapter{flagSet: pflagSet}
err := config.BindPFlags(adapter)
if err != nil {
    log.Fatalf("Failed to bind flags: %v", err)
}
```

### BindPFlag

```go
func (cm *LockFreeConfigManager) BindPFlag(configKey string, flag LockFreeFlag) error
```

Binds a single flag to a specific configuration key.

**Parameters:**
- `configKey`: Target configuration key
- `flag`: Flag implementing `LockFreeFlag` interface

**Returns:**
- `error`: Binding error or nil on success

**Example:**
```go
flagAdapter := &PFlagAdapter{flag: pflagInstance}
err := config.BindPFlag("server.port", flagAdapter)
```

### RefreshFlags

```go
func (cm *LockFreeConfigManager) RefreshFlags() error
```

Refreshes all bound flags by re-reading their current values.

**Returns:**
- `error`: Refresh error or nil on success

**Example:**
```go
err := config.RefreshFlags()
if err != nil {
    log.Printf("Failed to refresh flags: %v", err)
}
```

## Utility Methods

### GetBoundFlags

```go
func (cm *LockFreeConfigManager) GetBoundFlags() map[string]string
```

Returns a mapping of configuration keys to flag names.

**Returns:**
- `map[string]string`: Map of config key → flag name

**Example:**
```go
boundFlags := config.GetBoundFlags()
for configKey, flagName := range boundFlags {
    fmt.Printf("Config '%s' bound to flag '%s'\n", configKey, flagName)
}
```

### GetCacheStats

```go
func (cm *LockFreeConfigManager) GetCacheStats() (total, valid int)
```

Returns cache statistics for performance monitoring.

**Returns:**
- `total`: Total number of cache entries
- `valid`: Number of valid cache entries

**Example:**
```go
total, valid := config.GetCacheStats()
fmt.Printf("Cache efficiency: %d/%d (%.1f%%)\n", 
    valid, total, float64(valid)/float64(total)*100)
```

## Thread Safety Guarantees

### Lock-Free Operations

All operations are **completely lock-free** using atomic operations:

- **Read Operations**: Use `atomic.LoadPointer` for zero-contention reads
- **Write Operations**: Use copy-on-write semantics for atomic updates
- **Concurrent Access**: Unlimited concurrent readers and writers
- **Memory Ordering**: Proper memory barriers ensure consistency

### Safe Usage Patterns

```go
// Safe: Concurrent reads from multiple goroutines
for i := 0; i < 100; i++ {
    go func() {
        value := config.GetString("some.key")
        // Process value...
    }()
}

// Safe: Concurrent writes from multiple goroutines  
for i := 0; i < 100; i++ {
    go func(i int) {
        config.Set(fmt.Sprintf("key.%d", i), i)
    }(i)
}

// Safe: Mixed concurrent reads and writes
go func() {
    for {
        config.Set("timestamp", time.Now().Unix())
        time.Sleep(time.Second)
    }
}()

go func() {
    for {
        timestamp := config.GetInt("timestamp")
        // Use timestamp...
    }
}()
```

## Performance Characteristics

### Benchmark Results

- **Target Performance**: <15ns per operation
- **Achieved Performance**: ~15.5ns (isolated environment)
- **Production Performance**: ~18-31ns (with I/O overhead)

### Performance Tips

1. **Use Type-Specific Getters**: `GetString()` is faster than `Get()`
2. **Set Defaults Early**: Reduces nil checks in hot paths
3. **Avoid Frequent Rebinding**: Bind flags once during initialization
4. **Monitor Cache Stats**: Use `GetCacheStats()` for performance tuning

### Memory Usage

- **Zero Allocations**: For timestamp operations (TimeCaches integration)
- **Copy-on-Write**: Minimal memory overhead for writes
- **Atomic Pointers**: Direct memory access without locking overhead

## Error Handling

### Common Errors

1. **Flag Binding Errors**: Invalid flag types or binding conflicts
2. **Type Conversion Errors**: Handled gracefully with fallbacks
3. **Memory Allocation**: Extremely rare due to lock-free design

### Error Handling Patterns

```go
// Flag binding with error handling
err := config.BindPFlags(flagSet)
if err != nil {
    log.Fatalf("Critical: Failed to bind flags: %v", err)
}

// Graceful type conversion handling
port := config.GetInt("server.port")
if port == 0 {
    port = 8080 // fallback default
}

// Safe string access
host := config.GetString("server.host")
if host == "" {
    host = "localhost" // fallback default
}
```

## Source Precedence Rules

Configuration values follow strict precedence ordering:

1. **Explicit** (`Set`) - Highest precedence
2. **Flags** (`SetFlag`, bound flags)
3. **Environment Variables** (`SetEnvVar`)
4. **Configuration Files** (`SetConfigFile`)
5. **Defaults** (`SetDefault`) - Lowest precedence

### Precedence Example

```go
config := argus.NewLockFreeConfigManager()

config.SetDefault("server.port", 8080)      // Precedence: 5 (lowest)
config.SetConfigFile("server.port", 9090)   // Precedence: 4
config.SetEnvVar("server.port", 3000)       // Precedence: 3
config.SetFlag("server.port", 4000)         // Precedence: 2
config.Set("server.port", 5000)             // Precedence: 1 (highest)

port := config.GetInt("server.port")        // Returns: 5000
```


---

Argus • an AGILira fragment
