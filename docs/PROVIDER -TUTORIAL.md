# Tutorial: Creating a Custom HTTP Provider

## Overview

This tutorial demonstrates how to create a custom remote configuration provider for Argus. We'll build a simple HTTP provider that can fetch configuration from HTTP/HTTPS endpoints. This serves as a complete example of how users can extend Argus with their own providers.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Provider Interface](#provider-interface)
- [Implementation Steps](#implementation-steps)
- [Testing Your Provider](#testing-your-provider)
- [Advanced Features](#advanced-features)
- [Distribution](#distribution)
- [Complete Example](#complete-example)

## Prerequisites

- Go 1.21 or later
- Basic understanding of HTTP clients
- Familiarity with Argus configuration system

## Provider Interface

All Argus remote configuration providers must implement the `RemoteConfigProvider` interface:

```go
type RemoteConfigProvider interface {
    Name() string
    Scheme() string
    Validate(configURL string) error
    Load(ctx context.Context, configURL string) (map[string]interface{}, error)
    Watch(ctx context.Context, configURL string) (<-chan map[string]interface{}, error)
    HealthCheck(ctx context.Context, configURL string) error
}
```

## Implementation Steps

### Step 1: Create the Provider Structure

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "time"

    "github.com/agilira/argus"
    "github.com/agilira/go-errors"
)

// HTTPProvider implements RemoteConfigProvider for HTTP/HTTPS endpoints
type HTTPProvider struct {
    client *http.Client
}

// NewHTTPProvider creates a new HTTP provider with default settings
func NewHTTPProvider() *HTTPProvider {
    return &HTTPProvider{
        client: &http.Client{
            Timeout: 30 * time.Second,
        },
    }
}
```

### Step 2: Implement Required Methods

#### Name and Scheme

```go
// Name returns a human-readable name for this provider
func (h *HTTPProvider) Name() string {
    return "HTTP Remote Configuration Provider v1.0"
}

// Scheme returns the URL schemes this provider handles
func (h *HTTPProvider) Scheme() string {
    return "http"  // Will handle both http and https
}
```

#### URL Validation

```go
// Validate checks if the provider can handle the given URL
func (h *HTTPProvider) Validate(configURL string) error {
    parsedURL, err := url.Parse(configURL)
    if err != nil {
        return errors.Wrap(err, argus.ErrCodeInvalidConfig, "invalid URL format")
    }

    // Check if scheme is supported
    if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
        return errors.New(argus.ErrCodeInvalidConfig,
            fmt.Sprintf("URL scheme must be 'http' or 'https', got '%s'", parsedURL.Scheme))
    }

    // Check if host is present
    if parsedURL.Host == "" {
        return errors.New(argus.ErrCodeInvalidConfig, "URL must have a host")
    }

    return nil
}
```

#### Configuration Loading

```go
// Load fetches configuration from the HTTP endpoint
func (h *HTTPProvider) Load(ctx context.Context, configURL string) (map[string]interface{}, error) {
    // Validate URL first
    if err := h.Validate(configURL); err != nil {
        return nil, err
    }

    // Create HTTP request with context
    req, err := http.NewRequestWithContext(ctx, "GET", configURL, nil)
    if err != nil {
        return nil, errors.Wrap(err, argus.ErrCodeRemoteConfigError, "failed to create request")
    }

    // Set appropriate headers
    req.Header.Set("Accept", "application/json")
    req.Header.Set("User-Agent", "Argus-HTTP-Provider/1.0")

    // Execute request
    resp, err := h.client.Do(req)
    if err != nil {
        return nil, errors.Wrap(err, argus.ErrCodeRemoteConfigError, "HTTP request failed")
    }
    defer resp.Body.Close()

    // Check response status
    if resp.StatusCode != http.StatusOK {
        return nil, errors.New(argus.ErrCodeRemoteConfigError,
            fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status))
    }

    // Read response body
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, errors.Wrap(err, argus.ErrCodeRemoteConfigError, "failed to read response")
    }

    // Parse JSON configuration
    var config map[string]interface{}
    if err := json.Unmarshal(body, &config); err != nil {
        return nil, errors.Wrap(err, argus.ErrCodeInvalidConfig, "invalid JSON response")
    }

    return config, nil
}
```

#### Health Check

```go
// HealthCheck verifies the HTTP endpoint is accessible
func (h *HTTPProvider) HealthCheck(ctx context.Context, configURL string) error {
    // Validate URL first
    if err := h.Validate(configURL); err != nil {
        return err
    }

    // Create HEAD request for efficient health check
    req, err := http.NewRequestWithContext(ctx, "HEAD", configURL, nil)
    if err != nil {
        return errors.Wrap(err, argus.ErrCodeRemoteConfigError, "failed to create health check request")
    }

    // Execute health check
    resp, err := h.client.Do(req)
    if err != nil {
        return errors.Wrap(err, argus.ErrCodeRemoteConfigError, "health check failed")
    }
    defer resp.Body.Close()

    // Check if endpoint is healthy
    if resp.StatusCode < 200 || resp.StatusCode >= 400 {
        return errors.New(argus.ErrCodeRemoteConfigError,
            fmt.Sprintf("unhealthy endpoint: HTTP %d", resp.StatusCode))
    }

    return nil
}
```

#### Configuration Watching

```go
// Watch implements polling-based configuration watching
func (h *HTTPProvider) Watch(ctx context.Context, configURL string) (<-chan map[string]interface{}, error) {
    // Validate URL first
    if err := h.Validate(configURL); err != nil {
        return nil, err
    }

    // Create channel for configuration updates
    configChan := make(chan map[string]interface{}, 1)

    // Start watching goroutine
    go h.watchConfig(ctx, configURL, configChan)

    return configChan, nil
}

// watchConfig implements the actual watching logic
func (h *HTTPProvider) watchConfig(ctx context.Context, configURL string, configChan chan<- map[string]interface{}) {
    defer close(configChan)

    // Poll interval
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    var lastConfig map[string]interface{}

    // Load initial configuration
    if config, err := h.Load(ctx, configURL); err == nil {
        lastConfig = config
        select {
        case configChan <- config:
        case <-ctx.Done():
            return
        }
    }

    // Watch for changes
    for {
        select {
        case <-ticker.C:
            // Load current configuration
            currentConfig, err := h.Load(ctx, configURL)
            if err != nil {
                // Log error and continue watching
                continue
            }

            // Check if configuration changed
            if !configEqual(lastConfig, currentConfig) {
                lastConfig = currentConfig
                select {
                case configChan <- currentConfig:
                case <-ctx.Done():
                    return
                }
            }

        case <-ctx.Done():
            return
        }
    }
}

// configEqual compares two configurations for equality
func configEqual(a, b map[string]interface{}) bool {
    if len(a) != len(b) {
        return false
    }

    for key, valueA := range a {
        if valueB, exists := b[key]; !exists || fmt.Sprintf("%v", valueA) != fmt.Sprintf("%v", valueB) {
            return false
        }
    }

    return true
}
```

### Step 3: Provider Registration

```go
// RegisterHTTPProvider registers the HTTP provider with Argus
func RegisterHTTPProvider() error {
    provider := NewHTTPProvider()
    return argus.RegisterRemoteProvider(provider)
}

// For auto-registration when imported as a package
func init() {
    if err := RegisterHTTPProvider(); err != nil {
        // Handle registration error if needed
        _ = err
    }
}
```

## Testing Your Provider

### Step 1: Create Test Server

```go
package main

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/agilira/argus"
    "github.com/agilira/go-errors"
)

func TestHTTPProvider(t *testing.T) {
    // Create test configuration
    testConfig := map[string]interface{}{
        "database_host": "localhost",
        "database_port": 5432,
        "debug_mode":    true,
        "app_name":      "test-app",
    }

    // Create test HTTP server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(testConfig)
    }))
    defer server.Close()

    // Register HTTP provider
    provider := NewHTTPProvider()
    err := argus.RegisterRemoteProvider(provider)
    if err != nil {
        t.Fatalf("Failed to register HTTP provider: %v", err)
    }

    // Test configuration loading
    t.Run("Load Configuration", func(t *testing.T) {
        config, err := argus.LoadRemoteConfig(server.URL)
        if err != nil {
            t.Fatalf("Failed to load config: %v", err)
        }

        // Verify configuration
        if config["database_host"] != "localhost" {
            t.Errorf("Expected database_host=localhost, got %v", config["database_host"])
        }

        if config["database_port"] != float64(5432) { // JSON numbers are float64
            t.Errorf("Expected database_port=5432, got %v", config["database_port"])
        }
    })

    // Test health check
    t.Run("Health Check", func(t *testing.T) {
        err := argus.HealthCheckRemoteProvider(server.URL)
        if err != nil {
            t.Fatalf("Health check failed: %v", err)
        }
    })

    // Test URL validation
    t.Run("URL Validation", func(t *testing.T) {
        validURLs := []string{
            "http://localhost:8080/config",
            "https://api.example.com/config",
            "http://192.168.1.1/app/config.json",
        }

        invalidURLs := []string{
            "ftp://example.com/config",
            "http://",
            "not-a-url",
            "",
        }

        for _, url := range validURLs {
            if err := provider.Validate(url); err != nil {
                t.Errorf("Valid URL %s rejected: %v", url, err)
            }
        }

        for _, url := range invalidURLs {
            if err := provider.Validate(url); err == nil {
                t.Errorf("Invalid URL %s accepted", url)
            }
        }
    })
}
```

### Step 2: Run Tests

```bash
go test -v ./...
```

## Advanced Features

### Authentication Support

```go
// HTTPProviderWithAuth extends HTTPProvider with authentication
type HTTPProviderWithAuth struct {
    *HTTPProvider
    username string
    password string
    apiKey   string
}

// NewHTTPProviderWithAuth creates an authenticated HTTP provider
func NewHTTPProviderWithAuth(username, password, apiKey string) *HTTPProviderWithAuth {
    return &HTTPProviderWithAuth{
        HTTPProvider: NewHTTPProvider(),
        username:     username,
        password:     password,
        apiKey:       apiKey,
    }
}

// Load extends the base Load method with authentication
func (h *HTTPProviderWithAuth) Load(ctx context.Context, configURL string) (map[string]interface{}, error) {
    // Validate URL first
    if err := h.Validate(configURL); err != nil {
        return nil, err
    }

    // Create HTTP request with context
    req, err := http.NewRequestWithContext(ctx, "GET", configURL, nil)
    if err != nil {
        return nil, errors.Wrap(err, argus.ErrCodeRemoteConfigError, "failed to create request")
    }

    // Add authentication headers
    if h.username != "" && h.password != "" {
        req.SetBasicAuth(h.username, h.password)
    }

    if h.apiKey != "" {
        req.Header.Set("Authorization", "Bearer "+h.apiKey)
        // or req.Header.Set("X-API-Key", h.apiKey)
    }

    // Set other headers
    req.Header.Set("Accept", "application/json")
    req.Header.Set("User-Agent", "Argus-HTTP-Provider/1.0")

    // Rest of the implementation...
    return h.HTTPProvider.Load(ctx, configURL)
}
```

### Custom HTTP Client

```go
// NewHTTPProviderWithClient creates a provider with custom HTTP client
func NewHTTPProviderWithClient(client *http.Client) *HTTPProvider {
    return &HTTPProvider{
        client: client,
    }
}

// Example with custom TLS configuration
func NewHTTPProviderWithTLS() *HTTPProvider {
    transport := &http.Transport{
        TLSClientConfig: &tls.Config{
            InsecureSkipVerify: false, // Set to true for self-signed certs (not recommended for production)
        },
    }

    client := &http.Client{
        Transport: transport,
        Timeout:   30 * time.Second,
    }

    return NewHTTPProviderWithClient(client)
}
```

### Configuration Caching

```go
type CachedHTTPProvider struct {
    *HTTPProvider
    cache     map[string]cachedConfig
    cacheMutex sync.RWMutex
    ttl       time.Duration
}

type cachedConfig struct {
    data      map[string]interface{}
    timestamp time.Time
}

func (c *CachedHTTPProvider) Load(ctx context.Context, configURL string) (map[string]interface{}, error) {
    // Check cache first
    c.cacheMutex.RLock()
    if cached, exists := c.cache[configURL]; exists {
        if time.Since(cached.timestamp) < c.ttl {
            c.cacheMutex.RUnlock()
            return cached.data, nil
        }
    }
    c.cacheMutex.RUnlock()

    // Load from HTTP
    config, err := c.HTTPProvider.Load(ctx, configURL)
    if err != nil {
        return nil, err
    }

    // Update cache
    c.cacheMutex.Lock()
    c.cache[configURL] = cachedConfig{
        data:      config,
        timestamp: time.Now(),
    }
    c.cacheMutex.Unlock()

    return config, nil
}
```

## Distribution

### As a Standalone Package

Create a separate Go module for your provider:

```go
// go.mod
module github.com/yourorg/argus-http-provider

go 1.21

require github.com/agilira/argus v1.0.0
```

```go
// provider.go
package argushttpprovider

import "github.com/agilira/argus"

// Auto-register when imported
func init() {
    provider := NewHTTPProvider()
    argus.RegisterRemoteProvider(provider)
}
```

Users can then import your provider:

```go
import _ "github.com/yourorg/argus-http-provider"
```

### Usage Documentation

```go
// Example usage documentation
/*
HTTP Provider for Argus Remote Configuration

INSTALLATION:
    go get github.com/yourorg/argus-http-provider

USAGE:
    import _ "github.com/yourorg/argus-http-provider"
    
    config, err := argus.LoadRemoteConfig("http://api.example.com/config")
    
URL FORMAT:
    http://host:port/path
    https://host:port/path
    
SUPPORTED FEATURES:
    - HTTP/HTTPS endpoints
    - JSON configuration format
    - Health checking
    - Configuration watching (polling-based)
    - Authentication (basic auth, API keys)
    - Custom TLS configuration
    
EXAMPLES:
    // Basic usage
    config, err := argus.LoadRemoteConfig("https://config.mycompany.com/api/myapp")
    
    // With authentication
    provider := NewHTTPProviderWithAuth("user", "pass", "api-key")
    argus.RegisterRemoteProvider(provider)
    
    // Watching for changes
    watcher, err := argus.WatchRemoteConfig("https://config.mycompany.com/api/myapp")
    for config := range watcher {
        applyConfig(config)
    }
*/
```

## Complete Example

Here's the complete HTTP provider implementation:

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "time"

    "github.com/agilira/argus"
    "github.com/agilira/go-errors"
)

// HTTPProvider implements RemoteConfigProvider for HTTP/HTTPS endpoints
type HTTPProvider struct {
    client *http.Client
}

// NewHTTPProvider creates a new HTTP provider
func NewHTTPProvider() *HTTPProvider {
    return &HTTPProvider{
        client: &http.Client{
            Timeout: 30 * time.Second,
        },
    }
}

func (h *HTTPProvider) Name() string {
    return "HTTP Remote Configuration Provider v1.0"
}

func (h *HTTPProvider) Scheme() string {
    return "http" // Handles both http and https
}

func (h *HTTPProvider) Validate(configURL string) error {
    parsedURL, err := url.Parse(configURL)
    if err != nil {
        return errors.Wrap(err, argus.ErrCodeInvalidConfig, "invalid URL format")
    }

    if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
        return errors.New(argus.ErrCodeInvalidConfig, "URL scheme must be 'http' or 'https'")
    }

    if parsedURL.Host == "" {
        return errors.New(argus.ErrCodeInvalidConfig, "URL must have a host")
    }

    return nil
}

func (h *HTTPProvider) Load(ctx context.Context, configURL string) (map[string]interface{}, error) {
    if err := h.Validate(configURL); err != nil {
        return nil, err
    }

    req, err := http.NewRequestWithContext(ctx, "GET", configURL, nil)
    if err != nil {
        return nil, errors.Wrap(err, argus.ErrCodeRemoteConfigError, "failed to create request")
    }

    req.Header.Set("Accept", "application/json")
    req.Header.Set("User-Agent", "Argus-HTTP-Provider/1.0")

    resp, err := h.client.Do(req)
    if err != nil {
        return nil, errors.Wrap(err, argus.ErrCodeRemoteConfigError, "HTTP request failed")
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, errors.New(argus.ErrCodeRemoteConfigError,
            fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status))
    }

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, errors.Wrap(err, argus.ErrCodeRemoteConfigError, "failed to read response")
    }

    var config map[string]interface{}
    if err := json.Unmarshal(body, &config); err != nil {
        return nil, errors.Wrap(err, argus.ErrCodeInvalidConfig, "invalid JSON response")
    }

    return config, nil
}

func (h *HTTPProvider) Watch(ctx context.Context, configURL string) (<-chan map[string]interface{}, error) {
    if err := h.Validate(configURL); err != nil {
        return nil, err
    }

    configChan := make(chan map[string]interface{}, 1)
    go h.watchConfig(ctx, configURL, configChan)
    return configChan, nil
}

func (h *HTTPProvider) watchConfig(ctx context.Context, configURL string, configChan chan<- map[string]interface{}) {
    defer close(configChan)

    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    var lastConfig map[string]interface{}

    // Load initial configuration
    if config, err := h.Load(ctx, configURL); err == nil {
        lastConfig = config
        select {
        case configChan <- config:
        case <-ctx.Done():
            return
        }
    }

    // Watch for changes
    for {
        select {
        case <-ticker.C:
            if currentConfig, err := h.Load(ctx, configURL); err == nil {
                if !configEqual(lastConfig, currentConfig) {
                    lastConfig = currentConfig
                    select {
                    case configChan <- currentConfig:
                    case <-ctx.Done():
                        return
                    }
                }
            }
        case <-ctx.Done():
            return
        }
    }
}

func (h *HTTPProvider) HealthCheck(ctx context.Context, configURL string) error {
    if err := h.Validate(configURL); err != nil {
        return err
    }

    req, err := http.NewRequestWithContext(ctx, "HEAD", configURL, nil)
    if err != nil {
        return errors.Wrap(err, argus.ErrCodeRemoteConfigError, "failed to create health check request")
    }

    resp, err := h.client.Do(req)
    if err != nil {
        return errors.Wrap(err, argus.ErrCodeRemoteConfigError, "health check failed")
    }
    defer resp.Body.Close()

    if resp.StatusCode < 200 || resp.StatusCode >= 400 {
        return errors.New(argus.ErrCodeRemoteConfigError,
            fmt.Sprintf("unhealthy endpoint: HTTP %d", resp.StatusCode))
    }

    return nil
}

func configEqual(a, b map[string]interface{}) bool {
    if len(a) != len(b) {
        return false
    }
    for key, valueA := range a {
        if valueB, exists := b[key]; !exists || fmt.Sprintf("%v", valueA) != fmt.Sprintf("%v", valueB) {
            return false
        }
    }
    return true
}

func main() {
    // Register the HTTP provider
    provider := NewHTTPProvider()
    if err := argus.RegisterRemoteProvider(provider); err != nil {
        panic(err)
    }

    fmt.Println("HTTP Provider registered successfully!")
    
    // Example usage
    config, err := argus.LoadRemoteConfig("https://api.github.com/repos/agilira/argus")
    if err != nil {
        fmt.Printf("Error: %v\n", err)
    } else {
        fmt.Printf("Config loaded: %+v\n", config)
    }
}
```

## Summary

This tutorial demonstrates how to create a fully functional HTTP provider for Argus. The key points are:

1. **Implement the Interface**: All six methods of `RemoteConfigProvider`
2. **Handle Errors Properly**: Use Argus error codes and provide meaningful messages
3. **Support Context**: Respect context cancellation and timeouts
4. **Validate URLs**: Ensure robust input validation
5. **Test Thoroughly**: Create comprehensive tests for all functionality
6. **Document Usage**: Provide clear examples and usage instructions

Your users can now create their own providers following this pattern and extend Argus with any remote configuration source they need!

---

Argus • an AGILira fragment
