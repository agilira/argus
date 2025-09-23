# Integration Guide - Remote Configuration Sources

## Table of Contents

- [Quick Start](#quick-start)
- [Redis Provider Setup](#redis-provider-setup)
- [Common Integration Patterns](#common-integration-patterns)
- [Error Handling Strategies](#error-handling-strategies)
- [Performance Optimization](#performance-optimization)
- [Production Deployment](#production-deployment)

## Quick Start

### 1. Basic Configuration Loading

```go
package main

import (
    "log"
    "github.com/agilira/argus"
)

func main() {
    // Load configuration from Redis
    config, err := argus.LoadRemoteConfig("redis://localhost:6379/0/myapp:config")
    if err != nil {
        log.Fatal(err)
    }
    
    // Access configuration values
    dbHost, ok := config["database_host"].(string)
    if !ok {
        log.Fatal("database_host not found or invalid type")
    }
    
    log.Printf("Database host: %s", dbHost)
}
```

### 2. Health Check Before Loading

```go
func loadConfigSafely(url string) (map[string]interface{}, error) {
    // Check provider health first
    if err := argus.HealthCheckRemoteProvider(url); err != nil {
        return nil, fmt.Errorf("provider health check failed: %w", err)
    }
    
    // Load configuration
    return argus.LoadRemoteConfig(url)
}
```

### 3. Configuration Watching

```go
func startConfigWatcher(url string) {
    configChan, err := argus.WatchRemoteConfig(url)
    if err != nil {
        log.Fatal(err)
    }
    
    go func() {
        for config := range configChan {
            log.Printf("Configuration updated: %+v", config)
            applyConfiguration(config)
        }
    }()
}

func applyConfiguration(config map[string]interface{}) {
    // Update application configuration
    if dbHost, ok := config["database_host"].(string); ok {
        updateDatabaseConnection(dbHost)
    }
}
```

## Redis Provider Setup

### Redis Server Requirements

- **Version**: Redis 3.0 or higher
- **Network**: Accessible from application
- **Authentication**: Optional (user:password@host format)
- **Database**: Use databases 0-15

### URL Format Validation

The Redis provider validates URLs strictly:

**Valid URL Formats**:
```
redis://localhost:6379/0/config
redis://user:password@redis.example.com:6379/1/app:settings
redis://127.0.0.1:6379/0/myapp:database
```

**Invalid URL Examples**:
```go
// Wrong scheme
"http://localhost:6379/0/config"
// Error: [ARGUS_INVALID_CONFIG]: URL scheme must be 'redis'

// Missing database/key
"redis://localhost:6379/config"
// Error: [ARGUS_INVALID_CONFIG]: Redis URL path must be in format: /database/key

// Invalid database number
"redis://localhost:6379/99/config"
// Error: [ARGUS_INVALID_CONFIG]: Redis database number must be between 0 and 15
```

### Redis Configuration Storage

Store JSON configuration in Redis:

```bash
# Set configuration in Redis
redis-cli -n 0 SET "myapp:config" '{"database_host":"localhost","port":5432,"timeout":30}'

# Verify storage
redis-cli -n 0 GET "myapp:config"
```

## Common Integration Patterns

### 1. Application Configuration Structure

```go
type AppConfig struct {
    Database DatabaseConfig `json:"database"`
    Cache    CacheConfig    `json:"cache"`
    API      APIConfig      `json:"api"`
}

type DatabaseConfig struct {
    Host     string `json:"host"`
    Port     int    `json:"port"`
    Username string `json:"username"`
    Password string `json:"password"`
}

func loadAppConfig() (*AppConfig, error) {
    configData, err := argus.LoadRemoteConfig("redis://localhost:6379/0/app:config")
    if err != nil {
        return nil, err
    }
    
    var config AppConfig
    configJSON, _ := json.Marshal(configData)
    if err := json.Unmarshal(configJSON, &config); err != nil {
        return nil, err
    }
    
    return &config, nil
}
```

### 2. Environment-Specific Configuration

```go
func getConfigURL() string {
    env := os.Getenv("APP_ENV")
    if env == "" {
        env = "development"
    }
    
    switch env {
    case "production":
        return "redis://prod-redis:6379/0/app:config"
    case "staging":
        return "redis://staging-redis:6379/0/app:config"
    default:
        return "redis://localhost:6379/0/app:config"
    }
}

func loadEnvironmentConfig() (map[string]interface{}, error) {
    url := getConfigURL()
    return argus.LoadRemoteConfig(url)
}
```

### 3. Configuration with Fallback

```go
func loadConfigWithFallback(primaryURL, fallbackURL string) (map[string]interface{}, error) {
    // Try primary source
    config, err := argus.LoadRemoteConfig(primaryURL)
    if err == nil {
        return config, nil
    }
    
    log.Printf("Primary config source failed: %v, trying fallback", err)
    
    // Try fallback source
    config, err = argus.LoadRemoteConfig(fallbackURL)
    if err != nil {
        return nil, fmt.Errorf("both primary and fallback config sources failed: %w", err)
    }
    
    return config, nil
}
```

### 4. Hot Configuration Reloading

```go
type ConfigManager struct {
    config     map[string]interface{}
    configMux  sync.RWMutex
    updateChan <-chan map[string]interface{}
}

func NewConfigManager(url string) (*ConfigManager, error) {
    // Load initial configuration
    initialConfig, err := argus.LoadRemoteConfig(url)
    if err != nil {
        return nil, err
    }
    
    // Start watching for updates
    updateChan, err := argus.WatchRemoteConfig(url)
    if err != nil {
        return nil, err
    }
    
    cm := &ConfigManager{
        config:     initialConfig,
        updateChan: updateChan,
    }
    
    // Start update handler
    go cm.handleUpdates()
    
    return cm, nil
}

func (cm *ConfigManager) handleUpdates() {
    for newConfig := range cm.updateChan {
        cm.configMux.Lock()
        cm.config = newConfig
        cm.configMux.Unlock()
        
        log.Println("Configuration updated")
        cm.notifyApplicationComponents()
    }
}

func (cm *ConfigManager) GetConfig(key string) interface{} {
    cm.configMux.RLock()
    defer cm.configMux.RUnlock()
    return cm.config[key]
}

func (cm *ConfigManager) notifyApplicationComponents() {
    // Notify other parts of the application about config changes
    // Could use channels, callbacks, or event system
}
```

## Error Handling Strategies

### 1. Comprehensive Error Handling

```go
func handleConfigError(err error) {
    if err == nil {
        return
    }
    
    switch {
    case strings.Contains(err.Error(), "ARGUS_INVALID_CONFIG"):
        log.Printf("Configuration error: %v", err)
        // Handle invalid configuration
        
    case strings.Contains(err.Error(), "ARGUS_CONFIG_NOT_FOUND"):
        log.Printf("Configuration not found: %v", err)
        // Use default values or fail gracefully
        
    case strings.Contains(err.Error(), "ARGUS_CONNECTION_ERROR"):
        log.Printf("Connection error: %v", err)
        // Retry with exponential backoff
        
    case strings.Contains(err.Error(), "context deadline exceeded"):
        log.Printf("Operation timed out: %v", err)
        // Handle timeout scenario
        
    default:
        log.Printf("Unknown error: %v", err)
        // Generic error handling
    }
}
```

### 2. Retry Logic with Exponential Backoff

```go
func loadConfigWithRetry(url string, maxRetries int) (map[string]interface{}, error) {
    var lastErr error
    
    for attempt := 0; attempt < maxRetries; attempt++ {
        config, err := argus.LoadRemoteConfig(url)
        if err == nil {
            return config, nil
        }
        
        lastErr = err
        
        // Don't retry on configuration errors
        if strings.Contains(err.Error(), "ARGUS_INVALID_CONFIG") {
            return nil, err
        }
        
        // Exponential backoff
        backoff := time.Duration(1<<uint(attempt)) * time.Second
        log.Printf("Config load attempt %d failed: %v, retrying in %v", attempt+1, err, backoff)
        time.Sleep(backoff)
    }
    
    return nil, fmt.Errorf("failed to load config after %d attempts: %w", maxRetries, lastErr)
}
```

### 3. Circuit Breaker Pattern

```go
type ConfigCircuitBreaker struct {
    url           string
    failureCount  int
    maxFailures   int
    lastFailTime  time.Time
    resetTimeout  time.Duration
    isOpen        bool
    mutex         sync.RWMutex
}

func (cb *ConfigCircuitBreaker) LoadConfig() (map[string]interface{}, error) {
    cb.mutex.Lock()
    defer cb.mutex.Unlock()
    
    // Check if circuit is open
    if cb.isOpen {
        if time.Since(cb.lastFailTime) > cb.resetTimeout {
            cb.isOpen = false
            cb.failureCount = 0
        } else {
            return nil, fmt.Errorf("circuit breaker is open")
        }
    }
    
    // Attempt to load configuration
    config, err := argus.LoadRemoteConfig(cb.url)
    if err != nil {
        cb.failureCount++
        cb.lastFailTime = time.Now()
        
        if cb.failureCount >= cb.maxFailures {
            cb.isOpen = true
        }
        
        return nil, err
    }
    
    // Reset on success
    cb.failureCount = 0
    return config, nil
}
```

## Performance Optimization

### 1. Custom Timeout Configuration

```go
func optimizedConfigLoad(url string) (map[string]interface{}, error) {
    opts := &argus.RemoteConfigOptions{
        Timeout:       2 * time.Second,  // Fast timeout
        RetryAttempts: 2,                // Fewer retries
        RetryDelay:    500 * time.Millisecond,
    }
    
    return argus.LoadRemoteConfig(url, opts)
}
```

### 2. Parallel Configuration Loading

```go
func loadMultipleConfigs(urls []string) map[string]map[string]interface{} {
    type result struct {
        url    string
        config map[string]interface{}
        err    error
    }
    
    resultChan := make(chan result, len(urls))
    
    // Load configurations in parallel
    for _, url := range urls {
        go func(u string) {
            config, err := argus.LoadRemoteConfig(u)
            resultChan <- result{url: u, config: config, err: err}
        }(url)
    }
    
    // Collect results
    configs := make(map[string]map[string]interface{})
    for i := 0; i < len(urls); i++ {
        res := <-resultChan
        if res.err == nil {
            configs[res.url] = res.config
        } else {
            log.Printf("Failed to load config from %s: %v", res.url, res.err)
        }
    }
    
    return configs
}
```

### 3. Configuration Caching

```go
type CachedConfigLoader struct {
    cache     map[string]cachedConfig
    cacheMux  sync.RWMutex
    ttl       time.Duration
}

type cachedConfig struct {
    data      map[string]interface{}
    timestamp time.Time
}

func (ccl *CachedConfigLoader) LoadConfig(url string) (map[string]interface{}, error) {
    ccl.cacheMux.RLock()
    if cached, exists := ccl.cache[url]; exists {
        if time.Since(cached.timestamp) < ccl.ttl {
            ccl.cacheMux.RUnlock()
            return cached.data, nil
        }
    }
    ccl.cacheMux.RUnlock()
    
    // Load fresh configuration
    config, err := argus.LoadRemoteConfig(url)
    if err != nil {
        return nil, err
    }
    
    // Update cache
    ccl.cacheMux.Lock()
    ccl.cache[url] = cachedConfig{
        data:      config,
        timestamp: time.Now(),
    }
    ccl.cacheMux.Unlock()
    
    return config, nil
}
```

## Production Deployment

### 1. Health Check Integration

```go
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
    configURL := "redis://prod-redis:6379/0/app:config"
    
    if err := argus.HealthCheckRemoteProvider(configURL); err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(map[string]string{
            "status": "unhealthy",
            "error":  err.Error(),
        })
        return
    }
    
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "status": "healthy",
    })
}
```

### 2. Monitoring and Metrics

```go
type ConfigMetrics struct {
    LoadDuration    prometheus.Histogram
    LoadErrors      prometheus.Counter
    HealthChecks    prometheus.Counter
    ConfigUpdates   prometheus.Counter
}

func (m *ConfigMetrics) LoadConfigWithMetrics(url string) (map[string]interface{}, error) {
    start := time.Now()
    
    config, err := argus.LoadRemoteConfig(url)
    
    m.LoadDuration.Observe(time.Since(start).Seconds())
    
    if err != nil {
        m.LoadErrors.Inc()
        return nil, err
    }
    
    return config, nil
}
```

### 3. Graceful Degradation

```go
type ProductionConfigManager struct {
    primaryURL   string
    fallbackURL  string
    defaultConfig map[string]interface{}
}

func (pcm *ProductionConfigManager) LoadConfig() map[string]interface{} {
    // Try primary source
    if config, err := argus.LoadRemoteConfig(pcm.primaryURL); err == nil {
        return config
    }
    
    // Try fallback source
    if config, err := argus.LoadRemoteConfig(pcm.fallbackURL); err == nil {
        log.Println("Using fallback configuration source")
        return config
    }
    
    // Use default configuration
    log.Println("Using default configuration - all remote sources failed")
    return pcm.defaultConfig
}
```

### 4. Configuration Validation

```go
func validateConfiguration(config map[string]interface{}) error {
    required := []string{"database_host", "database_port", "api_key"}
    
    for _, key := range required {
        if _, exists := config[key]; !exists {
            return fmt.Errorf("required configuration key missing: %s", key)
        }
    }
    
    // Type validation
    if port, ok := config["database_port"].(float64); !ok || port <= 0 || port > 65535 {
        return fmt.Errorf("invalid database_port value")
    }
    
    return nil
}

func loadAndValidateConfig(url string) (map[string]interface{}, error) {
    config, err := argus.LoadRemoteConfig(url)
    if err != nil {
        return nil, err
    }
    
    if err := validateConfiguration(config); err != nil {
        return nil, fmt.Errorf("configuration validation failed: %w", err)
    }
    
    return config, nil
}
```

## Best Practices Summary

1. **Always handle errors**: Check return values and handle specific error types
2. **Use health checks**: Verify provider availability before critical operations
3. **Implement retry logic**: Handle transient failures with exponential backoff
4. **Cache configurations**: Reduce load on remote sources with appropriate TTL
5. **Monitor performance**: Track load times, error rates, and health status
6. **Validate configuration**: Ensure loaded configuration meets application requirements
7. **Plan for failure**: Implement fallback mechanisms and default configurations
8. **Use contexts**: Leverage context-aware methods for timeout control
9. **Test thoroughly**: Verify behavior under various failure scenarios
10. **Document URLs**: Maintain clear documentation of configuration sources and formats

---

Argus • an AGILira fragment