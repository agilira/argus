# Argus CLI Example

Complete command-line interface for Argus configuration management, powered by [Orpheus](https://github.com/agilira/orpheus).

## Overview

This example demonstrates how to build a production-ready CLI for Argus using the ultra-fast Orpheus framework. The CLI provides comprehensive configuration management capabilities with zero-allocation performance.

## Features

- **Configuration Operations**: Get, set, delete, and list configuration values
- **Format Conversion**: Convert between JSON, YAML, TOML, HCL, INI, and Properties
- **Validation**: Comprehensive configuration validation with detailed error reporting
- **Real-time Monitoring**: Watch configuration files for changes
- **Audit Logging**: Optional audit trail for all operations
- **Performance Benchmarks**: Built-in benchmarking capabilities
- **Shell Completion**: Automatic completion for bash/zsh/fish

## Performance

Built on Orpheus framework providing:
- **7x-47x faster** than traditional CLI frameworks
- **512 ns/op** command parsing (vs 3,727 ns/op for alternatives)
- **Zero allocations** in hot paths
- Sub-microsecond command routing

## Installation

### Build from Source

```bash
# Clone the repository
git clone https://github.com/agilira/argus.git
cd argus/examples/cli

# Build the binary
go build -o argus main.go

# Optional: Install to $GOPATH/bin
go install
```

### Quick Build

```bash
# From the argus repository root
go build -o argus ./examples/cli/main.go
```

## Usage

### Basic Commands

```bash
# Show help
./argus --help

# Show version
./argus --version
```

### Configuration Operations

```bash
# Get a configuration value
./argus config get config.yaml server.port

# Set a configuration value
./argus config set config.yaml server.port 9000

# Delete a configuration key
./argus config delete config.yaml old.setting

# List all keys (optionally filter by prefix)
./argus config list config.yaml
./argus config list config.yaml --prefix=database

# Initialize a new configuration file
./argus config init myconfig.yaml --format=yaml --template=server
```

### Format Conversion

```bash
# Convert between formats (auto-detected)
./argus config convert config.yaml config.json

# Explicit format specification
./argus config convert input.yaml output.toml --from=yaml --to=toml

# Supported formats: json, yaml, toml, hcl, ini, properties
```

### Validation

```bash
# Validate a configuration file
./argus config validate config.yaml

# Validate with explicit format
./argus config validate config.ini --format=ini
```

### Real-time Monitoring

```bash
# Watch a configuration file for changes
./argus watch config.yaml

# Custom polling interval
./argus watch config.yaml --interval=2s

# Verbose output
./argus watch config.yaml --verbose
```

### Audit Operations

```bash
# Query audit logs
./argus audit query --since=24h
./argus audit query --event=config_change --limit=50

# Cleanup old audit logs
./argus audit cleanup --older-than=30d
./argus audit cleanup --older-than=90d --dry-run
```

### Performance Benchmarks

```bash
# Run performance benchmarks
./argus benchmark

# Custom iteration count
./argus benchmark --iterations=5000
```

### System Information

```bash
# Display system and runtime information
./argus info
```

### Shell Completion

```bash
# Generate completion for bash
./argus completion bash > /etc/bash_completion.d/argus

# Generate completion for zsh
./argus completion zsh > /usr/local/share/zsh/site-functions/_argus

# Generate completion for fish
./argus completion fish > ~/.config/fish/completions/argus.fish
```

## Architecture

The CLI is built using these components:

- **Orpheus Framework**: Ultra-fast CLI framework with zero external dependencies
- **Argus Core**: Configuration management library with universal format support
- **CLI Manager**: High-performance command orchestration and routing
- **Command Handlers**: Zero-allocation implementations for all operations

## Examples

### Complete Workflow

```bash
# 1. Initialize a new configuration
./argus config init app.yaml --template=server

# 2. Set some values
./argus config set app.yaml server.host "0.0.0.0"
./argus config set app.yaml server.port 8080
./argus config set app.yaml database.url "postgres://localhost/mydb"

# 3. Validate the configuration
./argus config validate app.yaml

# 4. Convert to JSON for deployment
./argus config convert app.yaml app.json

# 5. Watch for runtime changes
./argus watch app.json --interval=5s
```

### Multi-Format Configuration

```bash
# Work with different formats seamlessly
./argus config get config.yaml server.port
./argus config get config.json server.port
./argus config get config.toml server.port
./argus config get config.ini server.port

# Format auto-detection based on file extension
# Manual format specification available via --format flag
```

## Development

### Running Tests

```bash
# Run all tests
go test -v ./...

# Run with race detector
go test -race -v ./...

# Run with coverage
go test -cover -v ./...
```

### Building for Production

```bash
# Build with optimizations
go build -ldflags="-w -s" -o argus main.go

# Cross-compile for different platforms
GOOS=linux GOARCH=amd64 go build -o argus-linux main.go
GOOS=darwin GOARCH=amd64 go build -o argus-darwin main.go
GOOS=windows GOARCH=amd64 go build -o argus.exe main.go
```

## Integration

### Using in Your Application

You can use the CLI manager in your own applications:

```go
package main

import (
    "fmt"
    "os"
    
    "github.com/agilira/argus/cmd/cli"
)

func main() {
    // Create CLI manager with optional audit logging
    manager := cli.NewManager()
    
    // Run with custom arguments
    if err := manager.Run(os.Args[1:]); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}
```

## Performance Comparison

```
Benchmark Results (command parsing with 3 flags):

Argus (Orpheus)    512 ns/op      96 B/op       3 allocs/op
Cobra CLI       18,440 ns/op   3,145 B/op      33 allocs/op
Urfave CLI      30,097 ns/op   8,549 B/op     318 allocs/op

Argus is 36x faster than Cobra, 59x faster than Urfave
```

## License

This example is licensed under the [Mozilla Public License 2.0](../../LICENSE.md).

## Related Examples

- [Configuration Binding](../config_binding/) - Type-safe configuration binding
- [Multi-Source Config](../multi_source_config/) - Loading from multiple sources
- [OTEL Integration](../otel_integration/) - OpenTelemetry integration
- [Error Handling](../error_handling/) - Advanced error handling patterns

## Links

- [Argus Repository](https://github.com/agilira/argus)
- [Orpheus CLI Framework](https://github.com/agilira/orpheus)
- [Complete Documentation](../../docs/)
- [API Reference](../../docs/API.md)

---

Argus CLI Example • an AGILira fragment
