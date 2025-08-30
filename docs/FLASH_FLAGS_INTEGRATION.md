# Argus + FlashFlags Integration

This document describes the complete integration of the **FlashFlags** ultra-fast command-line parsing library into the **Argus** configuration management system.

## Overview

The integration provides a unified configuration management solution that combines:

- **FlashFlags**: Ultra-fast, zero-dependency command-line flag parsing (30-40ns per access)
- **Argus Lock-Free Config**: Multi-source configuration with atomic operations
- **Real-time Config Watching**: File change detection with BoreasLite ring buffer
- **Precedence Management**: Explicit > Flags > Environment > Config Files > Defaults

## Key Features

### 🚀 Ultra-Fast Performance
- **30.61 ns/op** for string config access
- **30.98 ns/op** for integer config access  
- **3565 ns/op** for complete ConfigManager creation
- Lock-free atomic operations throughout

### 🎯 Fluent Interface
```go
config := argus.NewConfigManager("myapp").
    SetDescription("My Application").
    SetVersion("1.0.0").
    StringFlag("host", "localhost", "Server host").
    IntFlag("port", 8080, "Server port").
    BoolFlag("debug", false, "Enable debug mode")
```

### 📊 Multi-Source Configuration
1. **Explicit Sets** (highest priority): `config.Set("key", value)`
2. **Command Line Flags**: `--host=example.com`
3. **Environment Variables**: `MYAPP_HOST=example.com`
4. **Configuration Files**: JSON/YAML/TOML support
5. **Defaults** (lowest priority): Set programmatically

### 🔄 Real-Time Updates
- File watching with sub-second detection
- Atomic configuration updates
- Hot reloading support
- Graceful degradation

## Quick Start

### Basic Usage

```go
package main

import (
    "log"
    "github.com/agilira/argus"
)

func main() {
    // Create configuration manager
    config := argus.NewConfigManager("myapp").
        StringFlag("host", "localhost", "Server host").
        IntFlag("port", 8080, "Server port").
        BoolFlag("debug", false, "Debug mode")

    // Parse command line and environment
    if err := config.ParseArgs(); err != nil {
        log.Fatal(err)
    }

    // Access configuration values
    host := config.GetString("host")      // Ultra-fast access
    port := config.GetInt("port")         // Type-safe
    debug := config.GetBool("debug")      // Lock-free

    log.Printf("Server: %s:%d (debug: %t)", host, port, debug)
}
```

### Advanced Usage with File Watching

```go
config := argus.NewConfigManager("webserver").
    StringFlag("config-file", "config.json", "Config file").
    StringFlag("host", "localhost", "Host").
    IntFlag("port", 8080, "Port")

if err := config.ParseArgs(); err != nil {
    log.Fatal(err)
}

// Enable real-time config watching
configFile := config.GetString("config.file")
if configFile != "" {
    config.WatchConfigFile(configFile, func() {
        log.Println("Configuration reloaded!")
        // Config values automatically updated
        newPort := config.GetInt("port")
        log.Printf("Port updated to: %d", newPort)
    })
    
    config.StartWatching()
    defer config.StopWatching()
}
```

## Command Line Usage

### Basic Commands
```bash
# Use defaults
./myapp

# Override with flags
./myapp --host=0.0.0.0 --port=3000 --debug

# Use environment variables
MYAPP_HOST=0.0.0.0 MYAPP_PORT=3000 ./myapp

# Mixed (CLI takes precedence)
MYAPP_PORT=3000 ./myapp --port=8080  # Uses port 8080

# With config file
./myapp --config-file=server.json --debug
```

### Flag Naming Conventions
- **CLI Flags**: Use dashes (`--server-host`, `--db-port`)
- **Config Keys**: Converted to dots (`server.host`, `db.port`)  
- **Environment**: Uppercase with underscores (`MYAPP_SERVER_HOST`, `MYAPP_DB_PORT`)

## Architecture

### Configuration Precedence (High to Low)
1. **Explicit Sets**: `config.Set("key", value)` - Runtime overrides
2. **Command Line Flags**: `--key=value` - User input
3. **Environment Variables**: `APPNAME_KEY=value` - Deployment config
4. **Configuration Files**: JSON/YAML/TOML - Persistent config
5. **Defaults**: Programmatic defaults - Fallback values

### Key Components

#### ConfigManager
- **Purpose**: Unified configuration interface
- **Performance**: 30ns per access, lock-free
- **Features**: Fluent API, type safety, auto-binding

#### FlashFlagSetAdapter  
- **Purpose**: Bridge between FlashFlags and Argus
- **Performance**: Zero-allocation flag parsing
- **Features**: Standard flag types, validation, help text

#### LockFreeConfigManager
- **Purpose**: Multi-source configuration storage
- **Performance**: Atomic operations, copy-on-write
- **Features**: Precedence management, real-time updates

## API Reference

### Configuration Creation

```go
// Create new configuration manager
config := argus.NewConfigManager("app-name")

// Set metadata (optional)
config.SetDescription("Application description")
config.SetVersion("1.0.0")
```

### Flag Registration

```go
// String flags
config.StringFlag("name", "default", "Description")

// Integer flags  
config.IntFlag("count", 42, "Description")

// Boolean flags
config.BoolFlag("enabled", false, "Description")

// Duration flags
config.DurationFlag("timeout", 30*time.Second, "Description")

// Float flags
config.Float64Flag("rate", 1.5, "Description")

// String slice flags
config.StringSliceFlag("tags", []string{"web"}, "Description")
```

### Configuration Access

```go
// Type-safe getters (30ns performance)
host := config.GetString("host")
port := config.GetInt("port")
debug := config.GetBool("debug")
timeout := config.GetDuration("timeout")
rate := config.GetFloat64("rate")
tags := config.GetStringSlice("tags")

// Explicit setting (highest precedence)
config.Set("host", "override.com")
config.SetDefault("timeout", 60*time.Second)
```

### File Watching

```go
// Watch configuration file
err := config.WatchConfigFile("config.json", func() {
    log.Println("Config changed!")
})

// Start/stop watching
config.StartWatching()
defer config.StopWatching()
```

### Utilities

```go
// Performance statistics
total, valid := config.GetStats()

// Flag bindings debug
bindings := config.GetBoundFlags()

// Environment key mapping
envKey := config.FlagToEnvKey("server-port") // "MYAPP_SERVER_PORT"

// Help text
config.PrintUsage()
```

## Examples

### Web Server
See `demo/demo_app.go` for a complete web server example with:
- HTTP/HTTPS support
- Real-time configuration reloading
- CORS configuration
- Graceful shutdown
- API authentication

### CLI Tool
```go
config := argus.NewConfigManager("mytool").
    StringFlag("input", "", "Input file (required)").
    StringFlag("output", "", "Output file (required)").
    StringFlag("format", "json", "Output format").
    BoolFlag("verbose", false, "Verbose output")

if err := config.ParseArgs(); err != nil {
    config.PrintUsage()
    os.Exit(1)
}

// Validation
if config.GetString("input") == "" {
    log.Fatal("--input is required")
}
```

### Microservice
```go
config := argus.NewConfigManager("service").
    IntFlag("http-port", 8080, "HTTP port").
    IntFlag("grpc-port", 9090, "gRPC port").
    StringFlag("consul-addr", "localhost:8500", "Consul address").
    DurationFlag("health-interval", 30*time.Second, "Health check interval")

// Enable real-time config watching for service discovery
config.WatchConfigFile("service.json", func() {
    // Update service registration
    updateServiceDiscovery(config)
})
```

## Performance Benchmarks

### Configuration Access (Lower is Better)
- **GetString**: 30.61 ns/op
- **GetInt**: 30.98 ns/op  
- **GetBool**: 32.42 ns/op
- **GetDuration**: 31.00 ns/op

### Compared to Standard Solutions
- **10-50x faster** than viper
- **5-20x faster** than flag package
- **2-5x faster** than pflag
- **Lock-free** vs mutex-based solutions

### Memory Usage
- **Zero allocations** in hot paths
- **Copy-on-write** for updates
- **Atomic pointers** for lock-free access
- **Minimal overhead**: <1KB per ConfigManager

## Testing

The integration includes comprehensive tests:

```bash
# Run all integration tests
go test -v -run TestConfigManager

# Run performance benchmarks  
go test -bench=BenchmarkConfigManager

# Run example application
cd demo && go run demo_app.go --help
```

### Test Coverage
- ✅ Basic flag parsing
- ✅ Environment variable precedence
- ✅ Configuration precedence rules
- ✅ Real-time updates
- ✅ Error handling
- ✅ Performance benchmarks
- ✅ Fluent interface
- ✅ Key conversion (dashes ↔ dots)

## Migration Guide

### From Standard `flag` Package
```go
// Before (flag package)
var host = flag.String("host", "localhost", "Server host")
var port = flag.Int("port", 8080, "Server port")
flag.Parse()

// After (Argus + FlashFlags)
config := argus.NewConfigManager("myapp").
    StringFlag("host", "localhost", "Server host").
    IntFlag("port", 8080, "Server port")
config.ParseArgs()

host := config.GetString("host")  // Direct value, not pointer
port := config.GetInt("port")     // Type-safe access
```

### From Viper
```go
// Before (Viper)
viper.SetDefault("host", "localhost")
viper.BindPFlag("host", cmd.Flags().Lookup("host"))
host := viper.GetString("host")

// After (Argus + FlashFlags)  
config := argus.NewConfigManager("myapp").
    StringFlag("host", "localhost", "Server host")
config.ParseArgs()
host := config.GetString("host")  // 10-50x faster
```

## Best Practices

### 1. Application Structure
```go
// Create config manager once, use throughout app
var Config = argus.NewConfigManager("myapp").
    StringFlag("host", "localhost", "Host").
    IntFlag("port", 8080, "Port")

func init() {
    if err := Config.ParseArgs(); err != nil {
        log.Fatal(err)
    }
}

// Use in handlers/services
func handler(w http.ResponseWriter, r *http.Request) {
    debug := Config.GetBool("debug")  // Ultra-fast access
    if debug {
        log.Printf("Request: %s", r.URL.Path)
    }
}
```

### 2. Configuration Validation
```go
config := argus.NewConfigManager("myapp").
    StringFlag("db-host", "", "Database host").
    IntFlag("db-port", 5432, "Database port")

config.ParseArgs()

// Validate required configs
if config.GetString("db.host") == "" {
    log.Fatal("--db-host is required")
}

if port := config.GetInt("db.port"); port < 1 || port > 65535 {
    log.Fatal("--db-port must be between 1 and 65535")
}
```

### 3. Environment-Specific Configs
```go
// Use environment variables for deployment
// MYAPP_DB_HOST=prod.db.com
// MYAPP_DB_PORT=5432
// MYAPP_DEBUG=false

config := argus.NewConfigManager("myapp").
    StringFlag("db-host", "localhost", "Database host").
    IntFlag("db-port", 5432, "Database port").
    BoolFlag("debug", true, "Debug mode")  // Default true for dev

// Environment automatically overrides defaults
// Command line overrides environment
```

### 4. Real-Time Configuration
```go
// Use for feature flags, rate limits, circuit breaker settings
config.WatchConfigFile("features.json", func() {
    // Reload feature flags
    featureFlags.Update(config.GetStringSlice("enabled.features"))
    
    // Update rate limits
    rateLimiter.SetLimit(config.GetInt("rate.limit"))
})
```

## Troubleshooting

### Common Issues

#### 1. Configuration Not Updated
**Problem**: `config.Set()` doesn't override flag values
**Solution**: Check that precedence fix is applied (explicit should be highest)

#### 2. Environment Variables Not Working  
**Problem**: `MYAPP_HOST=test` doesn't set the value
**Solution**: Ensure flag name conversion: `--server-host` → `MYAPP_SERVER_HOST`

#### 3. Performance Issues
**Problem**: Configuration access is slow
**Solution**: Verify lock-free implementation and check for mutex usage

#### 4. Memory Leaks
**Problem**: Memory usage grows over time
**Solution**: Ensure file watchers are stopped with `defer config.StopWatching()`

### Debug Helpers

```go
// Check configuration state
total, valid := config.GetStats()
log.Printf("Config entries: %d/%d", valid, total)

// Debug flag bindings
bindings := config.GetBoundFlags()
for configKey, flagName := range bindings {
    log.Printf("Flag %s -> Config %s", flagName, configKey)
}

// Environment variable mapping
envKey := config.FlagToEnvKey("server-host")
log.Printf("Flag server-host maps to env %s", envKey)
```

## Contributing

### Development Setup
```bash
# Clone and test
git clone https://github.com/agilira/argus
cd argus
go mod tidy
go test -v

# Run benchmarks
go test -bench=.

# Build demo
cd demo && go build
```

### Testing New Features
1. Add unit tests in `flash_flags_integration_test.go`
2. Add performance benchmarks
3. Update documentation
4. Test with demo application

## License

This integration is part of the AGILira ecosystem and is licensed under MPL-2.0.

## Related Projects

- **[flash-flags](https://github.com/agilira/flash-flags)**: Ultra-fast command-line flag parsing
- **[go-timecache](https://github.com/agilira/go-timecache)**: Zero-allocation time caching  
- **[go-errors](https://github.com/agilira/go-errors)**: Structured error handling

---

For more examples and advanced usage, see the `demo/` directory and test files.
