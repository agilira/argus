# Remote Configuration API Reference

## Table of Contents

- [Overview](#overview)
- [Core API Functions](#core-api-functions)
- [Provider Management](#provider-management)
- [Configuration Options](#configuration-options)
- [Error Codes](#error-codes)
- [Type Definitions](#type-definitions)

## Overview

The Remote Configuration API provides programmatic access to remote configuration sources through a unified interface. All functions are thread-safe and support context-based timeout control.

## Core API Functions

### LoadRemoteConfig

```go
func LoadRemoteConfig(url string, opts ...*RemoteConfigOptions) (map[string]interface{}, error)
```

**Description**: Loads configuration data from a remote source.

**Parameters**:
- `url`: Remote configuration URL (e.g., `redis://localhost:6379/0/config`)
- `opts`: Optional configuration parameters

**Returns**:
- `map[string]interface{}`: Configuration data
- `error`: Error if operation fails

**Error Conditions**:
- Empty URL: `[ARGUS_INVALID_CONFIG]: URL cannot be empty`
- Invalid URL format: `[ARGUS_INVALID_CONFIG]: invalid URL format: <details>`
- Unsupported scheme: `[ARGUS_UNSUPPORTED_PROVIDER]: no provider registered for scheme 'scheme'`
- Connection failure: Provider-specific error

**Example**:
```go
config, err := argus.LoadRemoteConfig("redis://localhost:6379/0/myapp:config")
if err != nil {
    log.Fatal(err)
}
dbHost := config["database_host"].(string)
```

---

### LoadRemoteConfigWithContext

```go
func LoadRemoteConfigWithContext(ctx context.Context, url string, opts ...*RemoteConfigOptions) (map[string]interface{}, error)
```

**Description**: Context-aware version of LoadRemoteConfig with timeout support.

**Parameters**:
- `ctx`: Context for timeout and cancellation
- `url`: Remote configuration URL
- `opts`: Optional configuration parameters

**Returns**:
- `map[string]interface{}`: Configuration data
- `error`: Error if operation fails or times out

**Timeout Behavior**:
- 50ms timeout executes in approximately 50.123ms
- Returns `context deadline exceeded` error on timeout

**Example**:
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

config, err := argus.LoadRemoteConfigWithContext(ctx, "redis://localhost:6379/0/config")
if err != nil {
    if err == context.DeadlineExceeded {
        log.Println("Operation timed out")
    }
    return err
}
```

---

### WatchRemoteConfig

```go
func WatchRemoteConfig(url string, opts ...*RemoteConfigOptions) (<-chan map[string]interface{}, error)
```

**Description**: Starts watching for configuration changes and returns a channel for updates.

**Parameters**:
- `url`: Remote configuration URL
- `opts`: Optional configuration parameters (set `Watch: true` for automatic watching)

**Returns**:
- `<-chan map[string]interface{}`: Channel receiving configuration updates
- `error`: Error if watch setup fails

**Channel Type**: `<-chan map[string]interface{}`

**Example**:
```go
configChan, err := argus.WatchRemoteConfig("redis://localhost:6379/0/app:config")
if err != nil {
    log.Fatal(err)
}

go func() {
    for config := range configChan {
        log.Printf("Configuration updated: %+v", config)
        // Apply new configuration
    }
}()
```

---

### WatchRemoteConfigWithContext

```go
func WatchRemoteConfigWithContext(ctx context.Context, url string, opts ...*RemoteConfigOptions) (<-chan map[string]interface{}, error)
```

**Description**: Context-aware version of WatchRemoteConfig.

**Parameters**:
- `ctx`: Context for timeout and cancellation
- `url`: Remote configuration URL
- `opts`: Optional configuration parameters

**Returns**:
- `<-chan map[string]interface{}`: Channel receiving configuration updates
- `error`: Error if watch setup fails

**Context Behavior**:
- Watch stops when context is cancelled
- Timeout applies to initial setup, not ongoing watching

---

### HealthCheckRemoteProvider

```go
func HealthCheckRemoteProvider(url string, opts ...*RemoteConfigOptions) error
```

**Description**: Checks the health of a remote configuration provider.

**Parameters**:
- `url`: Remote configuration URL
- `opts`: Optional configuration parameters

**Returns**:
- `error`: `nil` if healthy, error describing the problem otherwise

**Health Check Results**:
- `nil`: Provider is healthy and accessible
- Error: Connection or authentication issues

**Example**:
```go
err := argus.HealthCheckRemoteProvider("redis://localhost:6379/0/config")
if err != nil {
    log.Printf("Provider health check failed: %v", err)
}
```

---

### HealthCheckRemoteProviderWithContext

```go
func HealthCheckRemoteProviderWithContext(ctx context.Context, url string, opts ...*RemoteConfigOptions) error
```

**Description**: Context-aware version of HealthCheckRemoteProvider.

**Parameters**:
- `ctx`: Context for timeout and cancellation
- `url`: Remote configuration URL
- `opts`: Optional configuration parameters

**Returns**:
- `error`: `nil` if healthy, error otherwise

## Provider Management

### RegisterRemoteProvider

```go
func RegisterRemoteProvider(provider RemoteConfigProvider) error
```

**Description**: Registers a new remote configuration provider.

**Parameters**:
- `provider`: Implementation of RemoteConfigProvider interface

**Returns**:
- `error`: Error if registration fails

**Error Conditions**:
- Duplicate scheme: `[ARGUS_PROVIDER_EXISTS]: provider for scheme 'redis' already registered`

**Example**:
```go
provider := &MyCustomProvider{}
err := argus.RegisterRemoteProvider(provider)
if err != nil {
    log.Fatal(err)
}
```

---

### GetRemoteProvider

```go
func GetRemoteProvider(scheme string) (RemoteConfigProvider, error)
```

**Description**: Retrieves a registered provider by URL scheme.

**Parameters**:
- `scheme`: URL scheme (e.g., "redis", "http")

**Returns**:
- `RemoteConfigProvider`: Provider instance
- `error`: Error if provider not found

**Example**:
```go
provider, err := argus.GetRemoteProvider("redis")
if err != nil {
    log.Fatal(err)
}
name := provider.Name() // "Redis Remote Configuration Provider v1.0"
```

---

### ListRemoteProviders

```go
func ListRemoteProviders() []RemoteConfigProvider
```

**Description**: Returns all registered remote configuration providers.

**Returns**:
- `[]RemoteConfigProvider`: Array of registered providers

**Current Providers**: Returns 1 provider (Redis)

**Example**:
```go
providers := argus.ListRemoteProviders()
log.Printf("Registered providers: %d", len(providers))
for _, provider := range providers {
    log.Printf("Provider: %s (scheme: %s)", provider.Name(), provider.Scheme())
}
```

---

### DefaultRemoteConfigOptions

```go
func DefaultRemoteConfigOptions() *RemoteConfigOptions
```

**Description**: Returns default configuration options.

**Returns**:
- `*RemoteConfigOptions`: Default options instance

**Default Values**:
- `Timeout`: 30s
- `RetryAttempts`: 3
- `RetryDelay`: 1s
- `Watch`: false
- `WatchInterval`: 30s

**Example**:
```go
opts := argus.DefaultRemoteConfigOptions()
opts.Timeout = 10 * time.Second
opts.RetryAttempts = 5

config, err := argus.LoadRemoteConfig("redis://localhost:6379/0/config", opts)
```

## Configuration Options

### RemoteConfigOptions Structure

```go
type RemoteConfigOptions struct {
    Timeout       time.Duration         // Request timeout (default: 30s)
    RetryAttempts int                   // Number of retry attempts (default: 3)
    RetryDelay    time.Duration         // Delay between retries (default: 1s)
    Watch         bool                  // Enable watching (default: false)
    WatchInterval time.Duration         // Watch poll interval (default: 30s)
    Headers       map[string]string     // HTTP headers for requests
    TLSConfig     map[string]interface{} // TLS configuration
    Auth          map[string]interface{} // Authentication parameters
}
```

### Field Descriptions

- **Timeout**: Maximum time to wait for a single operation
- **RetryAttempts**: Number of times to retry failed operations
- **RetryDelay**: Time to wait between retry attempts
- **Watch**: Whether to enable automatic configuration watching
- **WatchInterval**: How often to check for configuration changes
- **Headers**: Custom HTTP headers for HTTP-based providers
- **TLSConfig**: TLS/SSL configuration options
- **Auth**: Authentication credentials and options

### Custom Options Example

```go
customOpts := &argus.RemoteConfigOptions{
    Timeout:       5 * time.Second,
    RetryAttempts: 5,
    RetryDelay:    2 * time.Second,
    Watch:         true,
    WatchInterval: 10 * time.Second,
}

config, err := argus.LoadRemoteConfig("redis://localhost:6379/0/config", customOpts)
```

## Error Codes

### Standard Error Codes

| Error Code | Description | Context |
|------------|-------------|---------|
| `ARGUS_INVALID_CONFIG` | Invalid configuration parameter | URL format, scheme validation |
| `ARGUS_CONFIG_NOT_FOUND` | Configuration key not found | Loading non-existent keys |
| `ARGUS_UNSUPPORTED_PROVIDER` | No provider for URL scheme | Unknown or unregistered schemes |
| `ARGUS_PROVIDER_EXISTS` | Provider already registered | Duplicate provider registration |
| `ARGUS_CONNECTION_ERROR` | Connection to provider failed | Network or authentication issues |
| `ARGUS_TIMEOUT` | Operation timed out | Context deadline exceeded |

### Error Examples

```go
// Invalid URL scheme
_, err := argus.LoadRemoteConfig("http://localhost/config")
// Error: [ARGUS_UNSUPPORTED_PROVIDER]: no provider registered for scheme 'http'

// Empty URL
_, err := argus.LoadRemoteConfig("")
// Error: [ARGUS_INVALID_CONFIG]: URL cannot be empty

// Invalid Redis URL format
_, err := argus.LoadRemoteConfig("redis://localhost:6379/config")
// Error: [ARGUS_INVALID_CONFIG]: Redis URL path must be in format: /database/key

// Redis database out of range
_, err := argus.LoadRemoteConfig("redis://localhost:6379/99/config")
// Error: [ARGUS_INVALID_CONFIG]: Redis database number must be between 0 and 15

// Configuration key not found
_, err := argus.LoadRemoteConfig("redis://localhost:6379/0/nonexistent")
// Error: [ARGUS_CONFIG_NOT_FOUND]: Redis key 'nonexistent' not found in database 0
```

## Type Definitions

### RemoteConfigProvider Interface

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

### Function Signatures Summary

- `LoadRemoteConfig(url string, opts ...*RemoteConfigOptions) (map[string]interface{}, error)`
- `LoadRemoteConfigWithContext(ctx context.Context, url string, opts ...*RemoteConfigOptions) (map[string]interface{}, error)`
- `WatchRemoteConfig(url string, opts ...*RemoteConfigOptions) (<-chan map[string]interface{}, error)`
- `WatchRemoteConfigWithContext(ctx context.Context, url string, opts ...*RemoteConfigOptions) (<-chan map[string]interface{}, error)`
- `HealthCheckRemoteProvider(url string, opts ...*RemoteConfigOptions) error`
- `HealthCheckRemoteProviderWithContext(ctx context.Context, url string, opts ...*RemoteConfigOptions) error`
- `RegisterRemoteProvider(provider RemoteConfigProvider) error`
- `GetRemoteProvider(scheme string) (RemoteConfigProvider, error)`
- `ListRemoteProviders() []RemoteConfigProvider`
- `DefaultRemoteConfigOptions() *RemoteConfigOptions`

## Performance Notes

- **Typical Response Time**: ~50ms for local Redis operations
- **Context Timeout Precision**: Timeouts are honored within millisecond precision
- **Thread Safety**: All public APIs are thread-safe
- **Auto-registration**: Providers register automatically via `init()` functions
- **Connection Pooling**: Provider-dependent (Redis provider uses connection pooling)

---

Argus • an AGILira fragment
