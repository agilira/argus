# Multi-Source Configuration Loading Example

This example demonstrates Argus's powerful **LoadConfigMultiSource** functionality, which provides automatic configuration precedence handling across multiple sources.

## Key Features Demonstrated

### 1. **Automatic Precedence Resolution**
```
Environment Variables (highest) > Configuration Files (medium) > Defaults (lowest)
```

### 2. **Universal Format Support**
- ✅ **JSON** (.json) - Native high-performance parsing
- ✅ **YAML** (.yml, .yaml) - Built-in parser with 2.79ns format detection
- ✅ **TOML** (.toml) - Configuration format popular in Go ecosystem
- ✅ **INI** (.ini, .conf, .cfg) - Traditional configuration format
- ✅ **Properties** (.properties) - Java-style properties files
- ✅ **HCL** (.hcl, .tf) - HashiCorp Configuration Language

### 3. **Production-Ready Features**
- **Security validation** - Path traversal protection
- **Ultra-fast performance** - 2.79ns format detection, zero allocations
- **Graceful fallback** - Continues operation with missing/invalid files
- **Real-time integration** - Works seamlessly with file watching

## Quick Start

```bash
# Run the example
go run main.go
```

## 📋 Example Output

```
Argus Multi-Source Configuration Loading Demo
==============================================

Demo 1: Configuration File + Environment Override
PollInterval: 3s (ENV override from file's 10s)
CacheTTL: 5s (from file, no ENV override)
MaxWatchedFiles: 200 (ENV override from file's 100)
Audit.Enabled: true (ENV override from file's false)

Demo 2: Environment Variables Only
Environment-only PollInterval: 3s
Environment-only MaxWatchedFiles: 200

Demo 3: Multiple Configuration Formats  
config.yaml loaded successfully
config.toml loaded successfully
config.ini loaded successfully

Demo 4: Graceful Fallback Behavior
Non-existent file: graceful fallback to defaults
Invalid file: graceful fallback to defaults

Demo 5: Real-time Watching with Multi-Source Config
Watching watched_demo.json with multi-source configuration...
Using PollInterval: 3s (from precedence resolution)
Change detected #1: watched_demo.json
Watcher gracefully shut down
```

## How It Works

### 1. **Configuration Precedence**
```go
// Automatic precedence handling
config, err := argus.LoadConfigMultiSource("config.yaml")

// Precedence order:
// 1. ARGUS_POLL_INTERVAL=5s        (ENV - highest priority)
// 2. poll_interval: 10s            (YAML file - medium priority) 
// 3. PollInterval: 5*time.Second   (defaults - lowest priority)
// Result: config.PollInterval = 5s (ENV wins)
```

### 2. **Format Auto-Detection**
```go
// Detects format from file extension (2.79ns performance)
argus.DetectFormat("config.yaml") // → FormatYAML
argus.DetectFormat("config.json") // → FormatJSON
argus.DetectFormat("config.toml") // → FormatTOML
```

### 3. **Security Validation**
```go
// Prevents path traversal attacks
loadConfigFromFile("../../../etc/passwd") // → Error: security validation failed
loadConfigFromFile("config.yaml")         // → Success: safe path
```

### 4. **Error Resilience**
```go
// Graceful fallback behavior
argus.LoadConfigMultiSource("/nonexistent.json")  // → Uses env + defaults
argus.LoadConfigMultiSource("invalid.json")       // → Uses env + defaults  
```

## 🔧 Environment Variables

Set these environment variables to override file-based configuration:

```bash
# Core Configuration  
export ARGUS_POLL_INTERVAL=3s
export ARGUS_CACHE_TTL=2s
export ARGUS_MAX_WATCHED_FILES=200

# Performance Configuration
export ARGUS_OPTIMIZATION_STRATEGY=auto
export ARGUS_BOREAS_CAPACITY=256

# Audit Configuration  
export ARGUS_AUDIT_ENABLED=true
export ARGUS_AUDIT_MIN_LEVEL=info
export ARGUS_AUDIT_BUFFER_SIZE=1000
export ARGUS_AUDIT_FLUSH_INTERVAL=5s
```

## Integration Example

```go
// Production usage pattern
func main() {
    // Load configuration with precedence
    config, err := argus.LoadConfigMultiSource("config.yaml")
    if err != nil {
        log.Fatal(err)
    }
    
    // Create watcher with multi-source config
    watcher := argus.New(*config)
    
    // Set up file watching
    watcher.Watch("/app/config.json", func(event argus.ChangeEvent) {
        log.Printf("Config changed: %s", event.Path)
        // Handle hot-reload logic
    })
    
    // Start with graceful shutdown
    watcher.Start()
    defer watcher.GracefulShutdown(30 * time.Second)
}
```

## Performance Characteristics

- **Format Detection**: 2.79ns per operation
- **File Loading**: I/O bound (~1-3ms for typical configs)  
- **Parsing**: Zero allocations in hot paths
- **Memory Usage**: 8KB fixed + config size
- **Precedence Resolution**: O(1) complexity

## Next Steps

1. **Type-Safe Binding**: Use `argus.BindFromConfig()` for zero-reflection binding
2. **Real-Time Watching**: Integrate with `argus.UniversalConfigWatcher()`
3. **Remote Configuration**: Combine with `argus.NewRemoteConfigWithFallback()`
4. **Production Deployment**: Add audit logging and monitoring

---

Argus • an AGILira fragment