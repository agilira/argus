# Argus Quick Start Guide

Get **ultra-fast configuration watching** running in **2 minutes**!

## 30-Second Setup

### 1. Install Argus

```bash
go get github.com/agilira/argus
```

### 2. Basic File Watching

```go
package main

import (
    "fmt"
    "time"
    "github.com/agilira/argus"
)

func main() {
    // Create watcher with sensible defaults
    watcher := argus.New(argus.Config{
        PollInterval: 1 * time.Second,
    }.WithDefaults())
    defer watcher.Stop()

    // Watch any file
    watcher.Watch("config.json", func(event argus.ChangeEvent) {
        fmt.Printf("File changed: %s\n", event.Path)
        // Your reload logic here
    })

    watcher.Start()
    select {} // Keep running
}
```

### 3. Test It!

```bash
# Run your program
go run main.go &

# Change the config file
echo '{"version": 2}' > config.json

# See the change detected!
# Output: File changed: config.json
```

**That's it!** Your app now responds to configuration changes in **real-time**. ⚡

---

## Common Use Cases (Copy & Paste Ready)

### **JSON Config Reloading**

```go
package main

import (
    "encoding/json"
    "log"
    "os"
    "time"
    "github.com/agilira/argus"
)

type Config struct {
    Port     int    `json:"port"`
    LogLevel string `json:"log_level"`
    Debug    bool   `json:"debug"`
}

var currentConfig Config

func loadConfig(path string) error {
    data, err := os.ReadFile(path)
    if err != nil {
        return err
    }
    return json.Unmarshal(data, &currentConfig)
}

func main() {
    configPath := "app.json"
    
    // Load initial config
    if err := loadConfig(configPath); err != nil {
        log.Fatal(err)
    }
    
    // Watch for changes
    watcher := argus.New(argus.Config{PollInterval: 2 * time.Second}.WithDefaults())
    defer watcher.Stop()

    watcher.Watch(configPath, func(event argus.ChangeEvent) {
        log.Printf("Config changed, reloading...")
        if err := loadConfig(configPath); err != nil {
            log.Printf("Failed to reload config: %v", err)
            return
        }
        log.Printf("Config reloaded: Port=%d, LogLevel=%s", 
            currentConfig.Port, currentConfig.LogLevel)
    })

    watcher.Start()
    
    // Your application logic here
    runApp()
}

func runApp() {
    // Simulate app running
    for {
        log.Printf("App running on port %d (debug: %v)", 
            currentConfig.Port, currentConfig.Debug)
        time.Sleep(10 * time.Second)
    }
}
```

**Create `app.json`:**
```json
{
    "port": 8080,
    "log_level": "info",
    "debug": false
}
```

**Test it:**
```bash
# Run the app
go run main.go &

# Change config while running
echo '{"port": 9090, "log_level": "debug", "debug": true}' > app.json

# Watch the app reload automatically!
```

---

### **Universal Config Parser (Any Format)**

**One-liner** that handles JSON, YAML, TOML, HCL, INI, and Properties:

```go
package main

import (
    "fmt"
    "log"
    "time"
    "github.com/agilira/argus"
)

func main() {
    configFile := "config.yaml"  // Works with .json, .toml, .hcl, .ini too!
    
    // Universal watcher auto-detects format
    watcher, err := argus.UniversalConfigWatcher(configFile, 
        func(config map[string]interface{}) {
            fmt.Printf("Config updated: %+v\n", config)
            
            // Access your config values
            if port, ok := config["port"]; ok {
                fmt.Printf("New port: %v\n", port)
            }
            if dbHost, ok := config["database"].(map[string]interface{})["host"]; ok {
                fmt.Printf("New DB host: %v\n", dbHost)
            }
        },
    )
    
    if err != nil {
        log.Fatal(err)
    }
    defer watcher.Stop()
    
    watcher.Start()
    select {}
}
```

**Create `config.yaml`:**
```yaml
port: 8080
database:
  host: localhost
  port: 5432
features:
  - auth
  - logging
```

**Test with different formats:**
```bash
# YAML
echo 'port: 9090\ndatabase:\n  host: prod-db' > config.yaml

# JSON  
echo '{"port": 9090, "database": {"host": "prod-db"}}' > config.json

# TOML
echo 'port = 9090\n[database]\nhost = "prod-db"' > config.toml
```

**All formats work automatically!**

---

### **Production Setup with Audit Trail**

```go
package main

import (
    "log"
    "os"
    "time"
    "github.com/agilira/argus"
)

func main() {
    configFile := "production-config.json"
    
    // Production-grade configuration
    config := argus.Config{
        PollInterval: 5 * time.Second,
        OptimizationStrategy: argus.OptimizationAuto,
        
        // Enable audit trail for compliance
        Audit: argus.AuditConfig{
            Enabled:       true,
            OutputFile:    "/var/log/app/config-audit.jsonl",
            MinLevel:      argus.AuditCritical,
            BufferSize:    1000,
            FlushInterval: 10 * time.Second,
        },
        
        // Error handling
        ErrorHandler: func(err error, path string) {
            log.Printf("ALERT: Config error for %s: %v", path, err)
            // Send to monitoring system
            // alertmanager.SendAlert("config-error", err.Error())
        },
    }
    
    watcher, err := argus.UniversalConfigWatcherWithConfig(configFile,
        func(configData map[string]interface{}) {
            log.Printf("Production config updated")
            applyProductionConfig(configData)
        },
        config,
    )
    
    if err != nil {
        log.Fatal(err)
    }
    defer watcher.Stop()
    
    watcher.Start()
    
    // Keep app running
    select {}
}

func applyProductionConfig(config map[string]interface{}) {
    // Your production config application logic
    log.Printf("Applying new production config...")
    
    // Example: Update database connection
    if dbConfig, ok := config["database"].(map[string]interface{}); ok {
        updateDatabaseConnection(dbConfig)
    }
    
    // Example: Update feature flags
    if features, ok := config["features"].(map[string]interface{}); ok {
        updateFeatureFlags(features)
    }
}

func updateDatabaseConnection(dbConfig map[string]interface{}) {
    // Database connection update logic
}

func updateFeatureFlags(features map[string]interface{}) {
    // Feature flag update logic
}
```

**Features:**
- ✅ **Audit trail** for compliance (SOX, PCI-DSS, GDPR)
- ✅ **Error handling** with monitoring integration
- ✅ **Auto-optimization** for best performance
- ✅ **Production-ready** defaults

---

### ⚡ **High-Performance Setup**

For **high-frequency** config changes or **performance-critical** applications:

```go
package main

import (
    "time"
    "github.com/agilira/argus"
)

func main() {
    // Ultra-fast configuration
    config := argus.Config{
        PollInterval:         100 * time.Millisecond,  // Very fast polling
        OptimizationStrategy: argus.OptimizationSmallBatch,  // Optimized for frequent changes
        
        // Minimal audit for performance
        Audit: argus.AuditConfig{
            Enabled:       true,
            MinLevel:      argus.AuditCritical,  // Only critical events
            BufferSize:    5000,                 // Large buffer
            FlushInterval: 30 * time.Second,     // Less frequent flushes
        },
    }
    
    watcher := argus.New(*config.WithDefaults())
    defer watcher.Stop()

    // Watch multiple config files
    files := []string{
        "feature-flags.json",
        "rate-limits.json", 
        "circuit-breakers.json",
    }
    
    for _, file := range files {
        watcher.Watch(file, func(event argus.ChangeEvent) {
            // Ultra-fast reload logic
            reloadConfigFile(event.Path)
        })
    }

    watcher.Start()
    select {}
}

func reloadConfigFile(path string) {
    // Your optimized reload logic
    switch path {
    case "feature-flags.json":
        reloadFeatureFlags()
    case "rate-limits.json":
        reloadRateLimits()
    case "circuit-breakers.json":
        reloadCircuitBreakers()
    }
}

func reloadFeatureFlags() { /* ... */ }
func reloadRateLimits() { /* ... */ }
func reloadCircuitBreakers() { /* ... */ }
```

**Performance:** Sub-millisecond config reload with **12.11ns polling overhead**!

---

## **Configuration Options**

### Essential Settings

```go
config := argus.Config{
    PollInterval: 1 * time.Second,           // How often to check files
    OptimizationStrategy: argus.OptimizationAuto,  // Auto, SingleEvent, SmallBatch, LargeBatch
    
    // Audit (optional)
    Audit: argus.AuditConfig{
        Enabled:    true,
        OutputFile: "/var/log/config-audit.jsonl",
        MinLevel:   argus.AuditCritical,
    },
    
    // Error handling (optional)
    ErrorHandler: func(err error, path string) {
        log.Printf("Config error: %v", err)
    },
}
```

### Optimization Strategies

| Strategy | Best For | Performance |
|----------|----------|-------------|
| `OptimizationSingleEvent` | Single config file | **Fastest** |
| `OptimizationSmallBatch` | 2-10 files | **Fast** |
| `OptimizationLargeBatch` | 10+ files | **Efficient** |
| `OptimizationAuto` | **Recommended** | **Adaptive** |

### Audit Levels

| Level | Use Case | Events |
|-------|----------|--------|
| `AuditInfo` | Development | All events |
| `AuditWarn` | Staging | Warnings + errors |
| `AuditCritical` | **Production** | **Config changes** |
| `AuditSecurity` | High-security | **Security events** |

---

## **Common Pitfalls & Solutions**

### ❌ **File Doesn't Exist**

```go
// BAD: Will panic if file doesn't exist
watcher.Watch("missing-config.json", handler)

// GOOD: Check file existence first
if _, err := os.Stat("config.json"); err == nil {
    watcher.Watch("config.json", handler)
} else {
    log.Printf("Config file not found, using defaults")
}
```

### ❌ **Forgetting to Start**

```go
// BAD: Watcher created but never started
watcher := argus.New(config)
watcher.Watch("config.json", handler)
// Missing: watcher.Start()

// GOOD: Always call Start()
watcher := argus.New(config)
watcher.Watch("config.json", handler)
watcher.Start()  // ← Don't forget this!
```

### ❌ **No Graceful Shutdown**

```go
// BAD: No cleanup
func main() {
    watcher := argus.New(config)
    watcher.Start()
    select {}
}

// GOOD: Proper cleanup
func main() {
    watcher := argus.New(config)
    defer watcher.Stop()  // ← Always defer Stop()
    
    watcher.Start()
    
    // Handle shutdown gracefully
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)
    <-c
    
    log.Println("Shutting down gracefully...")
}
```

---

## **Performance Tips**

### 1. **Choose Right Poll Interval**

```go
// Development: Fast feedback
PollInterval: 500 * time.Millisecond

// Production: Balance performance vs responsiveness  
PollInterval: 5 * time.Second

// High-frequency: For real-time systems
PollInterval: 100 * time.Millisecond
```

### 2. **Optimize for Your Workload**

```go
// Single config file
OptimizationStrategy: argus.OptimizationSingleEvent

// Multiple files, frequent changes
OptimizationStrategy: argus.OptimizationSmallBatch

// Many files, batch changes
OptimizationStrategy: argus.OptimizationLargeBatch

// Not sure? Let Argus decide
OptimizationStrategy: argus.OptimizationAuto  // ← Recommended
```

### 3. **Efficient Error Handling**

```go
// Efficient error handler
ErrorHandler: func(err error, path string) {
    // Log to structured logger (don't use fmt.Printf in production)
    logger.Error("config-error", 
        "path", path, 
        "error", err.Error(),
    )
    
    // Optional: Send to monitoring
    if isCriticalError(err) {
        metrics.Increment("config.errors.critical")
    }
},
```

---

## **Benchmarks**

**Argus vs Traditional Polling:**

| Method | Overhead | CPU Usage | Memory |
|--------|----------|-----------|--------|
| Traditional | 2.1µs | High | 64KB+ |
| **Argus** | **12.11ns** | **Minimal** | **8KB** |
| **Improvement** | **175x faster** | **90% less** | **8x less** |

**Real-world impact:** In a service handling 10,000 RPS, Argus adds **<0.002%** overhead vs **3.5%** for traditional polling.

---

## 🔗 **Next Steps**

### **Learn More**
- **[API Reference](API.md)** - Complete API documentation
- **[Audit System](AUDIT.md)** - Enterprise audit trails for compliance
- **[Examples](../examples/)** - More code examples

### **Production Deployment**
1. Set up proper **log rotation** for audit files
2. Configure **monitoring** integration
3. Test **graceful shutdown** procedures
4. Review **security settings** and file permissions

### **Get Help**
- GitHub Issues: Report bugs or request features
- Discussions: Ask questions and share experiences

---

Argus • an AGILira fragment
