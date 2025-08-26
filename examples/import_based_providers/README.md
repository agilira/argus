# Argus Remote Configuration Sources - Import-Based Provider System

## Overview

This demonstrates the **import-based provider registration pattern** for Argus remote configuration sources, following the same successful pattern used by Argus parser plugins.

## The Problem

The current system requires manual registration and recompilation when adding new providers:

```go
// ❌ Current: Manual registration (requires recompilation)
argus.RegisterRemoteProvider(&RedisProvider{})
argus.RegisterRemoteProvider(&ConsulProvider{})
go build  // Must recompile when adding providers
```

## The Solution: Import-Based Registration

Following the Argus parser plugin pattern, providers auto-register through imports:

```go
// ✅ Target: Import-based (zero recompilation for new providers)
import _ "github.com/agilira/argus/providers/redis"
import _ "github.com/agilira/argus/providers/consul"
import _ "github.com/agilira/argus/providers/etcd"
// Auto-registered via init() functions
```

## User-Friendly Workflow

1. **Add Provider**: `go get github.com/agilira/argus/providers/redis`
2. **Import Provider**: Add `import _` line
3. **Build Once**: `go build` (one-time rebuild)
4. **Use Immediately**: Provider automatically available

## Architecture Benefits

### ✅ UX-Friendly
- No manual registration code
- No recompilation when adding providers
- Clean, declarative imports

### ✅ Zero Dependencies Default
- Core Argus remains dependency-free
- Production features available when imported
- Follows "batteries included, but removable" philosophy

### ✅ Proven Pattern
- Same system as successful parser plugins
- Consistent with Go ecosystem patterns
- Battle-tested architecture

## Provider Examples

### Redis Provider
```go
import _ "github.com/agilira/argus/providers/redis"

config, err := argus.LoadRemoteConfig("redis://localhost:6379/config:myapp")
```

**Features:**
- Native Redis KEYSPACE notifications for watching
- Authentication support
- Cluster and Sentinel compatibility
- Health checks via PING

### HTTP/HTTPS Provider
```go
import _ "github.com/agilira/argus/providers/http"

config, err := argus.LoadRemoteConfig("https://config.company.com/api/myapp")
```

**Features:**
- Authentication (Bearer, Basic, API Keys)
- Custom headers and request configuration
- Retry logic with exponential backoff
- TLS configuration

### Consul Provider
```go
import _ "github.com/agilira/argus/providers/consul"

config, err := argus.LoadRemoteConfig("consul://localhost:8500/config/myapp")
```

**Features:**
- Native Consul watch API
- ACL token support
- Service discovery integration
- Blocking queries for efficient watching

### etcd Provider
```go
import _ "github.com/agilira/argus/providers/etcd"

config, err := argus.LoadRemoteConfig("etcd://localhost:2379/config/myapp")
```

**Features:**
- etcd watch API for real-time updates
- Authentication and TLS
- Cluster support
- Lease-based configuration

## Implementation Pattern

Each provider package follows this pattern:

```go
package redis

import "github.com/agilira/argus"

type RedisProvider struct {
    // Provider implementation
}

func (r *RedisProvider) Name() string { return "Redis Provider" }
func (r *RedisProvider) Scheme() string { return "redis" }
func (r *RedisProvider) Load(ctx context.Context, configURL string) (map[string]interface{}, error) {
    // Implementation
}
func (r *RedisProvider) Watch(ctx context.Context, configURL string) (<-chan map[string]interface{}, error) {
    // Implementation
}

// Auto-registration via init()
func init() {
    argus.RegisterRemoteProvider(&RedisProvider{})
}
```

## Usage Examples

### Basic Loading
```go
config, err := argus.LoadRemoteConfig("redis://localhost:6379/config:myapp")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Database URL: %s\n", config["database_url"])
```

### Configuration Watching
```go
watcher, err := argus.WatchRemoteConfig("consul://localhost:8500/config/myapp")
if err != nil {
    log.Fatal(err)
}

for config := range watcher {
    fmt.Printf("Configuration updated: %+v\n", config)
    applyConfiguration(config)
}
```

### Multi-Environment
```go
env := os.Getenv("ENVIRONMENT")
configURL := fmt.Sprintf("https://config.company.com/api/%s/myapp", env)
config, err := argus.LoadRemoteConfig(configURL)
```

### With Options
```go
opts := argus.DefaultRemoteConfigOptions()
opts.Timeout = 30 * time.Second
opts.RetryAttempts = 5
opts.Headers["Authorization"] = "Bearer " + token

config, err := argus.LoadRemoteConfig("https://config.company.com/api/myapp", opts)
```

## Comparison with Parser Plugins

| Feature | Parser Plugins | Remote Config Providers |
|---------|---------------|-------------------------|
| **Registration** | `import _ "parser-pkg"` | `import _ "provider-pkg"` |
| **Interface** | `ConfigParser` | `RemoteConfigProvider` |
| **Auto-Registration** | ✅ via `init()` | ✅ via `init()` |
| **Zero Dependencies** | ✅ Built-in fallbacks | ✅ Built-in fallbacks |
| **Production Features** | ✅ When imported | ✅ When imported |
| **User Experience** | ✅ Import and use | ✅ Import and use |

## Current Status

### ✅ Completed
- Remote Configuration Sources core system
- Plugin registration architecture
- Examples and demonstration code
- Documentation and patterns

### 🚧 In Progress
- Provider package structure creation
- Interface refinement (`map[string]interface{}` return type)
- Provider implementation examples

### 📋 TODO
- Create actual provider packages
- Publish to separate repositories
- Community provider ecosystem
- Registry and discovery system

## Running the Examples

```bash
# View the import-based pattern concept
go run ./examples/import_based_providers/main.go

# View current working examples
go run ./examples/remote/remote_config_demo.go
```

## Contributing

This follows the established Argus patterns. When creating new providers:

1. Follow the `RemoteConfigProvider` interface
2. Include `init()` function for auto-registration
3. Provide comprehensive error handling
4. Include examples and documentation
5. Support both Load and Watch operations when possible

---

**Argus Remote Configuration Sources** • Part of the AGILira System Libraries
