# Remote Configuration Sources

## Overview

The Remote Configuration Sources feature provides a plugin-based architecture for loading configuration data from remote sources. The system supports provider auto-registration and comprehensive error handling.

## Provider System

### Registered Providers

The system currently has **1 provider** registered:

- **Redis Provider**: `Redis Remote Configuration Provider v1.0`
  - Scheme: `redis`
  - Auto-registered via `init()` function

### Provider Interface

All remote configuration providers implement the `RemoteConfigProvider` interface:

```go
type RemoteConfigProvider interface {
    Name() string
    Scheme() string
    Validate(url string) error
    Load(url string, options *RemoteConfigOptions) (map[string]interface{}, error)
    Watch(url string, options *RemoteConfigOptions) (<-chan map[string]interface{}, error)
    HealthCheck(url string, options *RemoteConfigOptions) error
}
```

#### Method Behaviors (Redis Provider)

**Name()**: Returns `Redis Remote Configuration Provider v1.0`

**Scheme()**: Returns `redis`

**Validate()**: 
- ✅ Valid URLs:
  - `redis://localhost:6379/0/config`
  - `redis://user:pass@redis.example.com:6379/1/app:settings`
  - `redis://127.0.0.1:6379/0/test:key`
- ❌ Invalid URLs:
  - `http://localhost/config` → `[ARGUS_INVALID_CONFIG]: URL scheme must be 'redis'`
  - `redis://localhost:6379/config` → `[ARGUS_INVALID_CONFIG]: Redis URL path must be in format: /database/key`
  - `not-a-url` → `[ARGUS_INVALID_CONFIG]: URL scheme must be 'redis'`
  - `redis://localhost:6379/99/config` → `[ARGUS_INVALID_CONFIG]: Redis database number must be between 0 and 15`

**Load()**: 
- Returns configuration data as `map[string]interface{}`
- Error for missing keys: `[ARGUS_CONFIG_NOT_FOUND]: Redis key 'keyname' not found in database X`

**Watch()**: 
- Returns channel: `<-chan map[string]interface{}`
- Enables real-time configuration updates

**HealthCheck()**: 
- Returns `nil` for healthy connections
- Returns error for connection issues

## Public API Reference

### Core Functions

#### LoadRemoteConfig
```go
func LoadRemoteConfig(url string, opts ...*RemoteConfigOptions) (map[string]interface{}, error)
```
Loads configuration from remote source.

**Error Handling**:
- Empty URL: `[ARGUS_INVALID_CONFIG]: URL cannot be empty`
- Invalid URL: `[ARGUS_INVALID_CONFIG]: invalid URL format: <details>`
- Unsupported scheme: `[ARGUS_UNSUPPORTED_PROVIDER]: no provider registered for scheme 'scheme'`

#### LoadRemoteConfigWithContext
```go
func LoadRemoteConfigWithContext(ctx context.Context, url string, opts ...*RemoteConfigOptions) (map[string]interface{}, error)
```
Context-aware configuration loading with timeout support.

**Timeout Behavior**: 
- 50ms timeout executes in ~50.123ms with error: `context deadline exceeded`

#### WatchRemoteConfig
```go
func WatchRemoteConfig(url string, opts ...*RemoteConfigOptions) (<-chan map[string]interface{}, error)
```
Starts watching for configuration changes.

#### WatchRemoteConfigWithContext
```go
func WatchRemoteConfigWithContext(ctx context.Context, url string, opts ...*RemoteConfigOptions) (<-chan map[string]interface{}, error)
```
Context-aware configuration watching.

#### HealthCheckRemoteProvider
```go
func HealthCheckRemoteProvider(url string, opts ...*RemoteConfigOptions) error
```
Checks health of remote provider.

#### HealthCheckRemoteProviderWithContext
```go
func HealthCheckRemoteProviderWithContext(ctx context.Context, url string, opts ...*RemoteConfigOptions) error
```
Context-aware health checking.

### Provider Management

#### RegisterRemoteProvider
```go
func RegisterRemoteProvider(provider RemoteConfigProvider) error
```
Registers a new remote configuration provider.

**Error Handling**:
- Duplicate registration: `[ARGUS_PROVIDER_EXISTS]: provider for scheme 'redis' already registered`

#### GetRemoteProvider
```go
func GetRemoteProvider(scheme string) (RemoteConfigProvider, error)
```
Retrieves a registered provider by scheme.

**Example**: `GetRemoteProvider('redis')` returns the Redis provider instance.

#### ListRemoteProviders
```go
func ListRemoteProviders() []RemoteConfigProvider
```
Returns all registered providers. Currently returns array with 1 element (Redis provider).

#### DefaultRemoteConfigOptions
```go
func DefaultRemoteConfigOptions() *RemoteConfigOptions
```
Returns default configuration options.

**Default Values**:
- `Timeout`: 30s
- `RetryAttempts`: 3
- `RetryDelay`: 1s
- `Watch`: false
- `WatchInterval`: 30s

## Configuration Options

### RemoteConfigOptions Structure

```go
type RemoteConfigOptions struct {
    Timeout       time.Duration         // Default: 30s
    RetryAttempts int                   // Default: 3
    RetryDelay    time.Duration         // Default: 1s
    Watch         bool                  // Default: false
    WatchInterval time.Duration         // Default: 30s
    Headers       map[string]string     // HTTP headers
    TLSConfig     map[string]interface{} // TLS configuration
    Auth          map[string]interface{} // Authentication
}
```

### Custom Options Usage

Custom options are respected by the system:

```go
// Custom timeout and retry configuration
opts := &argus.RemoteConfigOptions{
    Timeout:       5 * time.Second,
    RetryAttempts: 5,
    RetryDelay:    2 * time.Second,
}

config, err := argus.LoadRemoteConfig("redis://localhost:6379/0/config", opts)
```

## Redis Provider Details

### URL Format

Redis URLs must follow the format: `redis://[user:pass@]host:port/database/key`

**Components**:
- **Scheme**: Must be `redis`
- **Authentication**: Optional `user:pass@`
- **Host/Port**: Redis server location
- **Database**: Number between 0-15
- **Key**: Configuration key name

### Examples

```go
// Basic connection
config, err := argus.LoadRemoteConfig("redis://localhost:6379/0/myapp:config")

// With authentication
config, err := argus.LoadRemoteConfig("redis://user:password@redis.example.com:6379/1/app:settings")

// Health check
err := argus.HealthCheckRemoteProvider("redis://localhost:6379/0/config")
```

### Error Codes

| Error Code | Description |
|------------|-------------|
| `ARGUS_INVALID_CONFIG` | Invalid URL format or parameters |
| `ARGUS_CONFIG_NOT_FOUND` | Configuration key not found |
| `ARGUS_CONNECTION_ERROR` | Redis connection failed |
| `ARGUS_TIMEOUT` | Operation timed out |

## Performance Characteristics

- **Context Timeout**: 50ms timeout executes in ~50.123ms
- **Default Timeout**: 30 seconds
- **Retry Logic**: 3 attempts with 1-second delay by default
- **Auto-registration**: Providers register via `init()` functions at startup

## Thread Safety

All public APIs are thread-safe and can be called concurrently from multiple goroutines.

## Best Practices

1. **Always handle errors**: Check error returns from all API calls
2. **Use contexts**: Prefer context-aware methods for timeout control
3. **Health checks**: Verify provider health before critical operations
4. **Custom options**: Configure appropriate timeouts and retry logic
5. **URL validation**: Validate URLs before attempting to load configuration

## Integration Examples

### Basic Configuration Loading

```go
// Load configuration with default options
config, err := argus.LoadRemoteConfig("redis://localhost:6379/0/app:config")
if err != nil {
    log.Fatal(err)
}

// Access configuration values
dbHost := config["database_host"].(string)
```

### Watch Configuration Changes

```go
// Start watching for changes
configChan, err := argus.WatchRemoteConfig("redis://localhost:6379/0/app:config")
if err != nil {
    log.Fatal(err)
}

// Handle configuration updates
go func() {
    for config := range configChan {
        log.Printf("Configuration updated: %+v", config)
        // Apply new configuration
    }
}()
```

### Health Monitoring

```go
// Regular health checks
ticker := time.NewTicker(30 * time.Second)
go func() {
    for range ticker.C {
        err := argus.HealthCheckRemoteProvider("redis://localhost:6379/0/config")
        if err != nil {
            log.Printf("Provider health check failed: %v", err)
        }
    }
}()
```

## Remote Configuration (v1.0.2+)

### Automatic Fallback System

The new `RemoteConfig` struct provides enterprise-grade distributed configuration with automatic fallback capabilities:

```go
type RemoteConfig struct {
    Enabled      bool          // Enable remote configuration
    PrimaryURL   string        // Primary remote endpoint  
    FallbackURL  string        // Secondary remote endpoint
    LocalPath    string        // Local fallback file
    Timeout      time.Duration // Request timeout
    MaxRetries   int           // Retry attempts
    RetryDelay   time.Duration // Exponential backoff base
    SyncInterval time.Duration // Sync frequency
}
```

### Quick Setup

```go
// Enterprise setup with full fallback
remoteManager := argus.NewRemoteConfigWithFallback(
    "https://consul.prod:8500/v1/kv/app/config",     // Primary
    "https://consul.backup:8500/v1/kv/app/config",   // Fallback  
    "/etc/myapp/emergency-config.json",              // Local
)

watcher := argus.New(argus.Config{
    Remote: remoteManager.Config(),
})

// Graceful shutdown for production
defer watcher.GracefulShutdown(30 * time.Second)
```

### Fallback Sequence

1. **Primary URL** - Try main remote source (Consul, etcd, HTTP API)
2. **Fallback URL** - Try backup remote source (if configured)
3. **Local Path** - Load local configuration file (if configured)
4. **Error** - All sources failed, propagate error

### Production Features

- **Zero-allocation hot path**: No GC pressure during config loading
- **Exponential backoff**: Intelligent retry with overflow protection  
- **Atomic updates**: Thread-safe configuration swapping
- **Audit integration**: Full enterprise audit logging
- **Health monitoring**: Endpoint availability tracking
- **Timeout control**: Prevent hanging requests in production

### Kubernetes Integration

```yaml
# ConfigMap with fallback configuration
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-remote-config
data:
  primary-url: "https://consul.cluster.local:8500/v1/kv/app/config"
  fallback-url: "https://consul-backup.cluster.local:8500/v1/kv/app/config" 
  local-path: "/etc/config/fallback.json"
  timeout: "10s"
  max-retries: "3"
  sync-interval: "5m"
```

---

Argus • an AGILira fragment
