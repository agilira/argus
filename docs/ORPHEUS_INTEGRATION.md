# Orpheus CLI Integration Guide

Complete guide for integrating Argus configuration management with Orpheus CLI framework.

## Overview

Argus includes a high-performance CLI built with the Orpheus framework, delivering 7x-53x better performance than traditional CLI solutions. The integration provides git-style subcommands with zero-allocation hot paths.
```

## Command Structure

### Configuration Management Commands

#### config get
Retrieve configuration values using dot notation.

```bash
# Get a simple value
argus config get config.yaml server.port

# Get nested values
argus config get app.json database.connection.host

# Specify format explicitly
argus config get -f yaml settings.yml app.debug
```

**Implementation:** Zero-allocation value access with automatic format detection.

#### config set
Set configuration values with automatic type parsing.

```bash
# Set string value
argus config set config.yaml app.name "MyApp"

# Set numeric values (auto-parsed)
argus config set config.yaml server.port 8080

# Set boolean values (auto-parsed)
argus config set config.yaml debug true

# Set with explicit format
argus config set -f toml config.toml database.timeout 30
```

**Type Detection:**
- `"true"`, `"false"` → boolean
- Pure numbers → int or float
- Everything else → string

#### config delete
Remove configuration keys atomically.

```bash
# Delete single key
argus config delete config.yaml old.setting

# Delete nested key
argus config delete config.yaml database.deprecated.option
```

#### config list
List all configuration keys with optional prefix filtering.

```bash
# List all keys
argus config list config.yaml

# Filter by prefix
argus config list config.yaml --prefix=database

# Short flag version
argus config list config.yaml -p server
```

#### config convert
Convert between configuration formats while preserving all data.

```bash
# Auto-detect formats from extensions
argus config convert config.yaml config.json

# Explicit format specification
argus config convert --from=yaml --to=toml input.yml output.toml

# Supported formats: json, yaml, toml, hcl, ini, properties
argus config convert app.ini app.hcl
```

#### config validate
Validate configuration file syntax.

```bash
# Validate with auto-detection
argus config validate config.yaml

# Validate with explicit format
argus config validate -f json config.json
```

#### config init
Initialize new configuration files from templates.

```bash
# Create JSON config with default template
argus config init config.json

# Create with specific format and template
argus config init -f yaml -t server app.yaml

# Available templates: default, server, database, minimal
argus config init -f toml -t database db.toml
```

**Templates:**
- `default`: Basic app configuration
- `server`: HTTP server settings
- `database`: Database connection config
- `minimal`: Bare minimum structure

### Real-Time Monitoring

#### watch
Monitor configuration files for changes with configurable intervals.

```bash
# Watch with default 5s interval
argus watch config.yaml

# Custom interval
argus watch config.yaml --interval=1s

# Verbose output with validation
argus watch config.yaml -v --interval=2s
```

**Features:**
- Real-time change detection
- Optional configuration validation on change
- Configurable polling intervals
- Audit logging integration

### Audit and Compliance

#### audit query
Query audit logs with filtering options.

```bash
# Recent activity (24 hours)
argus audit query

# Custom time range
argus audit query --since=7d

# Filter by event type
argus audit query --event=config_set --since=1h

# Filter by file and limit results
argus audit query --file=config.yaml --limit=50
```

#### audit cleanup
Manage audit log retention.

```bash
# Dry run cleanup (see what would be deleted)
argus audit cleanup --older-than=30d --dry-run

# Actual cleanup
argus audit cleanup --older-than=30d
```

### Performance and Diagnostics

#### benchmark
Run performance benchmarks for different operations.

```bash
# Benchmark all operations
argus benchmark

# Specific operation
argus benchmark --operation=get --iterations=10000

# Available operations: get, set, parse, all
argus benchmark -o parse -i 5000
```

#### info
Display system information and diagnostics.

```bash
# Basic info
argus info

# Verbose system details
argus info --verbose
```

#### completion
Generate shell completion scripts.

```bash
# Bash completion
argus completion bash

# Zsh completion  
argus completion zsh

# Fish completion
argus completion fish
```

**Installation:**
```bash
# Bash
source <(argus completion bash)

# Zsh  
source <(argus completion zsh)

# Fish
argus completion fish | source
```

## Integration in Go Applications

### Basic CLI Application

```go
package main

import (
    "log"
    "github.com/agilira/argus/internal/cli"
)

func main() {
    // Create high-performance CLI manager
    manager := cli.NewManager()
    
    // Optional: Enable audit logging
    auditLogger := argus.NewAuditLogger("audit.log")
    manager.WithAudit(auditLogger)
    
    // Run with OS args
    if err := manager.Run(os.Args[1:]); err != nil {
        log.Fatal(err)
    }
}
```

### Custom CLI with Orpheus

```go
package main

import (
    "github.com/agilira/argus"
    "github.com/agilira/orpheus/pkg/orpheus"
)

func main() {
    // Create custom CLI application
    app := orpheus.New("myapp").
        SetDescription("My application with Argus integration").
        SetVersion("1.0.0")
    
    // Add custom commands with Argus integration
    configCmd := orpheus.NewCommand("server", "Start server")
    configCmd.SetHandler(func(ctx *orpheus.Context) error {
        // Load configuration using Argus
        cfg := argus.New()
        cfg.AddFile("server.yaml")
        cfg.AddEnv("MYAPP")
        
        var config ServerConfig
        if err := cfg.Load(&config); err != nil {
            return err
        }
        
        return startServer(config)
    })
    
    app.AddCommand(configCmd)
    app.Run(os.Args[1:])
}
```

### Advanced Integration with Config Writer

```go
func setupConfigCommand() *orpheus.Command {
    cmd := orpheus.NewCommand("config", "Configuration management")
    
    // Dynamic configuration updates
    setCmd := cmd.Subcommand("update", "Update config dynamically", func(ctx *orpheus.Context) error {
        key := ctx.GetArg(0)
        value := ctx.GetArg(1)
        
        // Load current config
        config, err := loadConfig("app.yaml", argus.FormatYAML)
        if err != nil {
            return err
        }
        
        // Create writer with audit
        writer, err := argus.NewConfigWriterWithAudit("app.yaml", argus.FormatYAML, config, auditLogger)
        if err != nil {
            return err
        }
        
        // Update and save atomically
        if err := writer.SetValue(key, parseValue(value)); err != nil {
            return err
        }
        
        return writer.WriteConfig()
    })
    
    return cmd
}
```

## Format Support

All commands automatically detect and support these formats:

| Format | Extension | Auto-Detection |
|--------|-----------|----------------|
| JSON | `.json` | ✅ |
| YAML | `.yaml`, `.yml` | ✅ |
| TOML | `.toml` | ✅ |
| HCL | `.hcl` | ✅ |
| INI | `.ini` | ✅ |
| Properties | `.properties` | ✅ |

### Format Override

Use the `-f` flag to override auto-detection:

```bash
# Force JSON parsing of .txt file
argus config get -f json data.txt app.setting

# Force YAML output
argus config convert -f json --to=yaml input.txt output.yaml
```

## Error Handling

The CLI provides structured error messages with context:

```bash
$ argus config get missing.yaml key
Error: failed to load configuration: open missing.yaml: no such file or directory

$ argus config set invalid.json bad.key value
Error: failed to set value: invalid JSON structure at key 'bad.key'

$ argus config validate broken.yaml  
Invalid YAML configuration: yaml: line 3: mapping values are not allowed in this context
```

## Audit Integration

Enable comprehensive audit logging for compliance:

```go
// Enable audit logging
auditLogger := argus.NewAuditLogger("config-audit.jsonl")
manager := cli.NewManager().WithAudit(auditLogger)

// All CLI operations are automatically logged:
// - Configuration reads/writes  
// - File access patterns
// - Validation results
// - Performance metrics
```

**Audit Log Format:**
```json
{
  "timestamp": "2025-09-22T10:30:00Z",
  "event": "cli_config_set", 
  "file": "/app/config.yaml",
  "key": "server.port",
  "value": 8080,
  "user": "root",
  "duration_ns": 1250000
}
```

## Performance Optimization

### Zero-Allocation Hot Paths

The CLI is optimized for production use with zero allocations in hot paths:

```go
// These operations allocate no memory:
value := writer.GetValue("database.host")     // 0 allocs
writer.SetValue("app.debug", true)            // 0 allocs  
keys := writer.ListKeys("server")             // 0 allocs
```

### Caching Strategy

- Parsed configuration caching prevents repeated parsing
- File modification time tracking for efficient change detection
- Lazy loading parses only requested sections

### Memory Usage

```
Operation Benchmarks:
config get:     30ns,   0 allocs
config set:   2.1ms,   3 allocs (I/O bound)  
config list:  100μs,   0 allocs
config watch:  25ms,   1 alloc (polling interval)
```

## Best Practices

### File Organization

```bash
# Separate concerns by file
argus config init config/database.yaml -f yaml -t database
argus config init config/server.yaml -f yaml -t server  
argus config init config/app.yaml -f yaml -t default
```

### Environment-Specific Configs

```bash
# Base configuration
argus config init config.yaml

# Environment overrides
argus config convert config.yaml config.prod.yaml
argus config set config.prod.yaml database.host prod-db.example.com

# Development settings
argus config convert config.yaml config.dev.yaml  
argus config set config.dev.yaml debug true
```

### Automation Scripts

```bash
#!/bin/bash
# Deployment script with validation

# Validate all configs before deployment
for config in config/*.yaml; do
    if ! argus config validate "$config"; then
        echo "Invalid configuration: $config"
        exit 1
    fi
done

# Convert to production format if needed
argus config convert config.yaml config.prod.json
```

## Troubleshooting

### Common Issues

**File Format Detection:**
```bash
# If auto-detection fails, specify format explicitly
argus config get -f yaml config.txt setting

# Check what format is detected
argus config validate config.yml  # Shows detected format
```

**Permission Issues:**
```bash
# Ensure file permissions allow read/write
ls -la config.yaml

# Check directory permissions for atomic writes
ls -la $(dirname config.yaml)
```

**Performance Issues:**
```bash
# Run benchmarks to identify bottlenecks
argus benchmark --operation=all --iterations=1000

# Enable verbose mode for detailed timing
argus watch config.yaml -v --interval=1s
```

### Debug Mode

```bash
# Set debug environment for detailed logging
ARGUS_DEBUG=true argus config get config.yaml key

# Check system info for diagnostics
argus info --verbose
```

## Migration from Other CLI Tools

### From Manual File Editing

```bash
# Old way: manual editing
vim config.yaml

# New way: programmatic updates
argus config set config.yaml server.port 9090
argus config set config.yaml database.pool_size 20
```

### From Custom Scripts

Replace custom configuration scripts with standardized CLI commands:

```bash
# Instead of custom shell scripts
# ./update_config.sh database.host newhost

# Use Argus CLI
argus config set config.yaml database.host newhost
```

### From Other Configuration Tools

```bash
# Convert from other formats
argus config convert old_config.ini new_config.yaml

# Validate migrated configuration  
argus config validate new_config.yaml
```
---

Argus • an AGILira fragment