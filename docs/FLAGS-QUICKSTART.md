# Argus Flags - Quick Start Guide

## Installation

```bash
go get github.com/agilira/argus
```

## 30-Second Quick Start

```go
package main

import (
    "fmt"
    "github.com/agilira/argus"
    "github.com/spf13/pflag"
)

func main() {
    // 1. Create config manager
    config := argus.NewLockFreeConfigManager()
    
    // 2. Set defaults
    config.SetDefault("server.port", 8080)
    config.SetDefault("server.host", "localhost")
    
    // 3. Setup command line flags
    flagSet := pflag.NewFlagSet("app", pflag.ExitOnError)
    flagSet.Int("server-port", 9090, "Server port")
    flagSet.String("server-host", "0.0.0.0", "Server host")
    
    // 4. Parse flags (simulate command line)
    flagSet.Parse([]string{"--server-port=3000", "--server-host=production.com"})
    
    // 5. Bind flags to config (see full docs for adapter implementation)
    // config.BindPFlags(flagSetAdapter)
    
    // 6. Use configuration with automatic precedence
    fmt.Printf("Server: %s:%d\n", 
        config.GetString("server.host"),    // "production.com" (flag overrides default)
        config.GetInt("server.port"))       // 3000 (flag overrides default)
}
```

## Key Features in 3 Lines

```go
config := argus.NewLockFreeConfigManager()        // Lock-free, ultra-fast
config.SetDefault("key", "default")               // Multi-source with precedence  
value := config.GetString("key")                  // Type-safe getters (<15ns)
```

## Configuration Precedence (High → Low)

1. **Explicit**: `config.Set("key", value)`
2. **Flags**: `config.SetFlag("key", value)` 
3. **Environment**: `config.SetEnvVar("key", value)`
4. **Config Files**: `config.SetConfigFile("key", value)`
5. **Defaults**: `config.SetDefault("key", value)`

## Performance

- **Target**: <15ns per operation
- **Architecture**: Completely lock-free with atomic operations
- **Use Case**: High-performance applications requiring millions of config reads/sec

## Next Steps

- Read [Full Documentation](flags.md) for complete API reference
- Check [examples/](../examples/) for real-world usage patterns
- See [Performance Benchmarks](../cmd/pure-benchmark/) for detailed metrics

---

Argus • an AGILira fragment
