# Environment Variables Configuration Guide

## Overview

Argus provides comprehensive environment variables support for container-native deployments and cloud-native configuration management. This enables easy configuration loading with intelligent defaults, type safety, and multi-source precedence.

**Key Benefits:**
- **Container-Native**: Perfect for Docker, Kubernetes, and cloud deployments
- **Type Safety**: Automatic type conversion with validation and error handling
- **Multi-Source**: Environment > File > Defaults precedence system
- **Zero Dependencies**: No external configuration files required

## Quick Start

### Basic Environment Configuration

```bash
# Set Argus configuration via environment variables
export ARGUS_POLL_INTERVAL=5s
export ARGUS_MAX_WATCHED_FILES=100
export ARGUS_OPTIMIZATION_STRATEGY=auto
export ARGUS_AUDIT_ENABLED=true
```

```go
// Load configuration from environment variables
config, err := argus.LoadConfigFromEnv()
if err != nil {
    log.Fatal(err)
}

watcher := argus.New(*config)
```

### Multi-Source Configuration

```go
// Load with precedence: Environment > File > Defaults
config, err := argus.LoadConfigMultiSource("config.yaml")
if err != nil {
    log.Fatal(err)
}

watcher := argus.New(*config)
```

## Environment Variables Reference

### Core Configuration

| Environment Variable | Type | Default | Description |
|---------------------|------|---------|-------------|
| `ARGUS_POLL_INTERVAL` | Duration | `5s` | How often to check files for changes |
| `ARGUS_CACHE_TTL` | Duration | `2.5s` | Cache lifetime for file stat operations |
| `ARGUS_MAX_WATCHED_FILES` | Integer | `100` | Maximum number of files to watch |

**Examples:**
```bash
export ARGUS_POLL_INTERVAL=10s        # Every 10 seconds
export ARGUS_CACHE_TTL=1s             # 1 second cache
export ARGUS_MAX_WATCHED_FILES=500    # Monitor up to 500 files
```

### Performance Configuration

| Environment Variable | Type | Default | Description |
|---------------------|------|---------|-------------|
| `ARGUS_OPTIMIZATION_STRATEGY` | String | `auto` | Performance optimization strategy |
| `ARGUS_BOREAS_CAPACITY` | Integer | `128` | Internal buffer capacity |

**Optimization Strategies:**
- `auto` - Automatic selection based on file count
- `single` or `singleevent` - Ultra-low latency for 1-2 files
- `small` or `smallbatch` - Balanced performance for 3-20 files  
- `large` or `largebatch` - High throughput for 20+ files

**Examples:**
```bash
export ARGUS_OPTIMIZATION_STRATEGY=smallbatch
export ARGUS_BOREAS_CAPACITY=256
```

### Audit Configuration

| Environment Variable | Type | Default | Description |
|---------------------|------|---------|-------------|
| `ARGUS_AUDIT_ENABLED` | Boolean | `false` | Enable audit logging |
| `ARGUS_AUDIT_OUTPUT_FILE` | String | `/var/log/argus/audit.jsonl` | Audit log file path |
| `ARGUS_AUDIT_MIN_LEVEL` | String | `info` | Minimum audit level |
| `ARGUS_AUDIT_BUFFER_SIZE` | Integer | `1000` | Audit buffer size |
| `ARGUS_AUDIT_FLUSH_INTERVAL` | Duration | `5s` | How often to flush audit buffer |

**Audit Levels:**
- `info` - General configuration changes
- `warn` - Performance warnings and parsing issues
- `critical` - Critical errors and system issues
- `security` - Security-related events

**Examples:**
```bash
export ARGUS_AUDIT_ENABLED=true
export ARGUS_AUDIT_OUTPUT_FILE=/var/log/app/argus.jsonl
export ARGUS_AUDIT_MIN_LEVEL=warn
export ARGUS_AUDIT_BUFFER_SIZE=2000
export ARGUS_AUDIT_FLUSH_INTERVAL=10s
```

## Boolean Values

Environment variables support flexible boolean parsing:

**True Values:** `true`, `1`, `yes`, `on`, `enabled`
**False Values:** `false`, `0`, `no`, `off`, `disabled`

```bash
export ARGUS_AUDIT_ENABLED=true      # Boolean true
export ARGUS_AUDIT_ENABLED=1         # Boolean true
export ARGUS_AUDIT_ENABLED=yes       # Boolean true
export ARGUS_AUDIT_ENABLED=enabled   # Boolean true
```

## Duration Format

Duration values use Go's standard duration format:

```bash
export ARGUS_POLL_INTERVAL=5s         # 5 seconds
export ARGUS_POLL_INTERVAL=500ms      # 500 milliseconds
export ARGUS_POLL_INTERVAL=2m         # 2 minutes
export ARGUS_POLL_INTERVAL=1h30m      # 1 hour 30 minutes
```

## Configuration Precedence

When using `LoadConfigMultiSource()`, configuration is loaded with the following precedence:

1. **Environment Variables** (highest priority)
2. **Configuration File** (medium priority)
3. **Default Values** (lowest priority)

```go
// Example: JSON sets poll_interval=10s, ENV sets ARGUS_POLL_INTERVAL=5s
// Result: poll_interval=5s (environment wins)
config, err := argus.LoadConfigMultiSource("config.json")
```

## Container Deployment Examples

### Docker

```dockerfile
# Dockerfile
FROM golang:1.23-alpine AS builder
COPY . /app
WORKDIR /app
RUN go build -o myapp

FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /root/

# Set Argus configuration
ENV ARGUS_POLL_INTERVAL=10s
ENV ARGUS_OPTIMIZATION_STRATEGY=largebatch
ENV ARGUS_AUDIT_ENABLED=true
ENV ARGUS_AUDIT_OUTPUT_FILE=/var/log/argus/audit.jsonl
ENV ARGUS_MAX_WATCHED_FILES=200

COPY --from=builder /app/myapp .
CMD ["./myapp"]
```

### Docker Compose

```yaml
# docker-compose.yml
version: '3.8'
services:
  myapp:
    image: myapp:latest
    environment:
      - ARGUS_POLL_INTERVAL=5s
      - ARGUS_OPTIMIZATION_STRATEGY=auto
      - ARGUS_AUDIT_ENABLED=true
      - ARGUS_AUDIT_OUTPUT_FILE=/var/log/argus/audit.jsonl
      - ARGUS_MAX_WATCHED_FILES=100
    volumes:
      - ./config:/app/config:ro
      - ./logs:/var/log/argus
```

### Kubernetes

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  template:
    spec:
      containers:
      - name: myapp
        image: myapp:latest
        env:
        - name: ARGUS_POLL_INTERVAL
          value: "10s"
        - name: ARGUS_OPTIMIZATION_STRATEGY
          value: "smallbatch"
        - name: ARGUS_AUDIT_ENABLED
          value: "true"
        - name: ARGUS_AUDIT_OUTPUT_FILE
          value: "/var/log/argus/audit.jsonl"
        - name: ARGUS_MAX_WATCHED_FILES
          value: "150"
        volumeMounts:
        - name: config-volume
          mountPath: /app/config
        - name: log-volume
          mountPath: /var/log/argus
```

### Kubernetes ConfigMap

```yaml
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argus-config
data:
  ARGUS_POLL_INTERVAL: "5s"
  ARGUS_OPTIMIZATION_STRATEGY: "auto"
  ARGUS_AUDIT_ENABLED: "true"
  ARGUS_MAX_WATCHED_FILES: "200"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  template:
    spec:
      containers:
      - name: myapp
        image: myapp:latest
        envFrom:
        - configMapRef:
            name: argus-config
```

## Helper Functions

Argus provides type-safe helper functions for environment variable access:

```go
// String with default
value := argus.GetEnvWithDefault("ARGUS_CUSTOM_VALUE", "default")

// Duration with default
interval := argus.GetEnvDurationWithDefault("ARGUS_POLL_INTERVAL", 5*time.Second)

// Integer with default
maxFiles := argus.GetEnvIntWithDefault("ARGUS_MAX_WATCHED_FILES", 100)

// Boolean with default
enabled := argus.GetEnvBoolWithDefault("ARGUS_AUDIT_ENABLED", false)
```

## Validation and Error Handling

Environment variables are validated during loading:

```go
config, err := argus.LoadConfigFromEnv()
if err != nil {
    // Handle validation errors
    if strings.Contains(err.Error(), "ARGUS_POLL_INTERVAL") {
        log.Fatal("Invalid poll interval format")
    }
    if strings.Contains(err.Error(), "ARGUS_OPTIMIZATION_STRATEGY") {
        log.Fatal("Invalid optimization strategy")
    }
}
```

**Common Validation Errors:**
- Invalid duration format (e.g., `ARGUS_POLL_INTERVAL=invalid`)
- Invalid integer values (e.g., `ARGUS_MAX_WATCHED_FILES=not-a-number`)
- Invalid optimization strategy (e.g., `ARGUS_OPTIMIZATION_STRATEGY=invalid-strategy`)
- Invalid capacity values (e.g., `ARGUS_BOREAS_CAPACITY=negative`)

## Production Best Practices

### Security Considerations

```bash
# ✅ Good: Use environment management tools
kubectl create secret generic argus-config \
  --from-literal=ARGUS_AUDIT_OUTPUT_FILE=/secure/logs/audit.jsonl

# ❌ Avoid: Hardcoding sensitive paths in Docker images
ENV ARGUS_AUDIT_OUTPUT_FILE=/hardcoded/path/audit.jsonl
```

### Performance Tuning

```bash
# High-performance microservice
export ARGUS_POLL_INTERVAL=1s
export ARGUS_OPTIMIZATION_STRATEGY=smallbatch
export ARGUS_BOREAS_CAPACITY=256
export ARGUS_CACHE_TTL=500ms

# High-throughput application
export ARGUS_POLL_INTERVAL=5s
export ARGUS_OPTIMIZATION_STRATEGY=largebatch
export ARGUS_BOREAS_CAPACITY=512
export ARGUS_MAX_WATCHED_FILES=1000
```

### Memory-Constrained Environments

```bash
# Reduce memory usage
export ARGUS_BOREAS_CAPACITY=64
export ARGUS_AUDIT_BUFFER_SIZE=500
export ARGUS_MAX_WATCHED_FILES=50
export ARGUS_CACHE_TTL=1s
```

## Troubleshooting

### Common Issues

**Issue: Configuration not loading from environment**
```go
// Check if environment variables are set
if os.Getenv("ARGUS_POLL_INTERVAL") == "" {
    log.Println("ARGUS_POLL_INTERVAL not set, using defaults")
}
```

**Issue: Invalid duration format**
```bash
# ❌ Wrong
export ARGUS_POLL_INTERVAL=5seconds

# ✅ Correct
export ARGUS_POLL_INTERVAL=5s
```

**Issue: Case sensitivity**
```bash
# ❌ Wrong (case sensitive)
export argus_poll_interval=5s

# ✅ Correct
export ARGUS_POLL_INTERVAL=5s
```

### Debug Configuration Loading

```go
// Enable debug logging
config, err := argus.LoadConfigFromEnv()
if err != nil {
    log.Printf("Config loading error: %v", err)
}

log.Printf("Loaded config: PollInterval=%v, MaxFiles=%d, Strategy=%v",
    config.PollInterval, config.MaxWatchedFiles, config.OptimizationStrategy)
```

## Integration Examples

### Complete Application Example

```go
package main

import (
    "log"
    "os"
    "os/signal"
    "syscall"
    
    "github.com/agilira/argus"
)

func main() {
    // Load configuration from environment with fallback to file
    config, err := argus.LoadConfigMultiSource("config.yaml")
    if err != nil {
        log.Fatalf("Failed to load configuration: %v", err)
    }
    
    // Create watcher with environment configuration
    watcher := argus.New(*config)
    
    // Watch configuration files
    watcher.Watch("/app/config/app.json", func(event argus.ChangeEvent) {
        log.Printf("Configuration changed: %s", event.Path)
        // Handle configuration update
    })
    
    // Start watching
    if err := watcher.Start(); err != nil {
        log.Fatalf("Failed to start watcher: %v", err)
    }
    
    log.Printf("Started with PollInterval=%v, Strategy=%v", 
        config.PollInterval, config.OptimizationStrategy)
    
    // Wait for shutdown signal
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)
    <-c
    
    watcher.Stop()
    log.Println("Shutdown complete")
}
```

## Migration from Other Libraries

### Migration Example

```go
// Legacy approach
```go
// Legacy approach
cfg.SetEnvPrefix("APP")
cfg.AutomaticEnv()
pollInterval := cfg.GetDuration("poll_interval")

// After (Argus)
config, err := argus.LoadConfigFromEnv()
if err != nil {
    log.Fatal(err)
}
pollInterval := config.PollInterval
```

### From Environment Manual Parsing

```go
// Before (Manual)
pollIntervalStr := os.Getenv("POLL_INTERVAL")
if pollIntervalStr == "" {
    pollIntervalStr = "5s"
}
pollInterval, err := time.ParseDuration(pollIntervalStr)

// After (Argus)
pollInterval := argus.GetEnvDurationWithDefault("ARGUS_POLL_INTERVAL", 5*time.Second)
```
---

Argus • an AGILira fragment
