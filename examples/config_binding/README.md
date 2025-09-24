
# Configuration Binding Example

This example demonstrates configuration binding using Argus, focusing on type safety, error handling, and performance benchmarking.


## Features Demonstrated

- Type-safe configuration binding
- Nested key support using dot notation
- Default value handling for missing configuration keys
- Performance benchmarking
- Error handling for invalid configuration values


## Running the Example

```bash
cd examples/config_binding
go run main.go
```


## Example Output

```
Argus Configuration Binding Example
==========================================

Binding configuration...
Configuration bound in 18.365µs

Configuration Results:
=========================
App Name:          my-service
App Version:       1.0.0
App Debug:         true
Server Host:       localhost
Server Port:       8080
Server Timeout:    30s
DB Host:           db.example.com
DB Port:           5432
DB SSL Mode:       require
DB Max Conns:      20
DB Idle Timeout:   5m0s

Performance Benchmark:
=====================
Running 10000 binding operations...
10000 operations completed in 15.039096ms
Average per operation: 1.503µs
Operations per second: 664934

Error Handling Demo:
========================
Error correctly detected: failed to bind key 'invalid_port': strconv.Atoi: parsing "not-a-number": invalid syntax

Demo completed successfully.
All configuration bindings and error handling checks passed.
```


## Usage Highlights

### 1. API Usage
```go
err := argus.BindFromConfig(config).
    BindString(&dbHost, "database.host", "localhost").
    BindInt(&dbPort, "database.port", 5432).
    BindBool(&enableSSL, "database.ssl", true).
    BindDuration(&timeout, "database.timeout", 30*time.Second).
    Apply()
```

### 2. Nested Configuration Keys
The example shows how to access nested configuration values:
```go
BindInt(&maxConns, "database.pool.max_connections")
BindDuration(&idleTimeout, "database.pool.idle_timeout")
```

### 3. Performance Benchmarking
The example includes a performance test showing operations per second and average time per operation.

### 4. Error Handling
Demonstrates handling of invalid configuration values with clear error messages.


## Configuration Format

The example uses this JSON configuration internally:

```json
{
  "app": {
    "name": "my-service",
    "version": "1.0.0",
    "debug": true
  },
  "server": {
    "host": "localhost",
    "port": 8080,
    "timeout": "30s"
  },
  "database": {
    "host": "db.example.com",
    "port": 5432,
    "ssl_mode": "require",
    "pool": {
      "max_connections": 20,
      "idle_timeout": "5m"
    }
  }
}
```


## Modifying the Example

To add new configuration values:
1. Add the value to the `exampleConfig` JSON string
2. Declare a variable to bind to
3. Add a binding call in the chain
4. Use the variable in your application

Example:
```go
// 1. Add to JSON
"cache": {
  "enabled": true,
  "size": 1000
}

// 2. Declare variables
var cacheEnabled bool
var cacheSize int

// 3. Add bindings
BindBool(&cacheEnabled, "cache.enabled", false).
BindInt(&cacheSize, "cache.size", 100).

// 4. Use the variables
fmt.Printf("Cache: enabled=%t, size=%d\n", cacheEnabled, cacheSize)
```

You can also test different types:
- `time.Duration` with string formats ("30s", "5m", "1h")
- `bool` with various representations (true/false, 1/0, "true"/"false")
- Number conversions (string to int, float to int, etc.)


## Performance Notes

Performance will vary based on:
- Number of bindings per operation
- Complexity of nested keys
- System hardware specifications


## Integration with File Watching

Configuration binding can be combined with Argus file watching for dynamic configuration updates. See the main documentation for integration examples.


## Further Reading

- [Configuration Binding Documentation](../../docs/CONFIG_BINDING.md) - Technical guide
- [API Reference](../../docs/API.md) - Full API documentation
- [Quick Start Guide](../../docs/QUICK_START.md) - Getting started with Argus

---

Argus • an AGILira fragment