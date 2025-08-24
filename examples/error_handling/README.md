# ErrorHandler Examples

The `ErrorHandler` feature in Argus provides powerful error handling capabilities for production environments.

## Overview

The `ErrorHandler` is a function type that gets called whenever errors occur during file watching or configuration parsing:

```go
type ErrorHandler func(err error, filepath string)
```

## Usage Patterns

### 1. Default Behavior (Backward Compatible)

When `ErrorHandler` is `nil`, Argus uses default error logging:

```go
config := argus.Config{
    PollInterval: 5 * time.Second,
    // ErrorHandler: nil (default)
}
watcher, err := argus.UniversalConfigWatcherWithConfig(path, callback, config)
```

### 2. Custom Error Handler

```go
errorHandler := func(err error, filepath string) {
    // Custom error handling logic
    log.Printf("Config error in %s: %v", filepath, err)
    
    // Integration examples:
    metrics.ConfigErrors.WithLabelValues(filepath).Inc()
    alertManager.SendAlert("Config error", err)
}

config := argus.Config{
    ErrorHandler: errorHandler,
}
```

### 3. Production Integrations

#### Prometheus Metrics
```go
var configErrors = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "config_errors_total",
        Help: "Total configuration errors",
    },
    []string{"file", "error_type"},
)

errorHandler := func(err error, filepath string) {
    configErrors.WithLabelValues(filepath, "parse_error").Inc()
}
```

#### Structured Logging
```go
errorHandler := func(err error, filepath string) {
    logger.Error("config_error",
        zap.String("file", filepath),
        zap.Error(err),
        zap.String("component", "argus"),
    )
}
```

#### Error Tracking
```go
errorHandler := func(err error, filepath string) {
    sentry.WithScope(func(scope *sentry.Scope) {
        scope.SetTag("component", "config-watcher")
        scope.SetTag("file", filepath)
        sentry.CaptureException(err)
    })
}
```

#### Circuit Breaker
```go
var errorCount atomic.Int64

errorHandler := func(err error, filepath string) {
    if errorCount.Add(1) > maxErrors {
        circuitBreaker.Open()
        log.Fatal("Too many config errors, opening circuit breaker")
    }
}
```

## Error Types

The ErrorHandler receives different types of errors:

1. **File Read Errors**: Permission denied, file locked, etc.
2. **Parse Errors**: Invalid JSON, YAML, TOML syntax
3. **Stat Errors**: File system errors (excluding file not found)

## Benefits

- **Observability**: Integration with monitoring systems
- **Alerting**: Real-time notifications on config issues  
- **Debugging**: Enhanced error context with file paths
- **Reliability**: Circuit breaker and graceful degradation
- **Backward Compatible**: Optional feature, doesn't break existing code

## Testing

See `error_handler_test.go` for comprehensive test examples demonstrating:
- Custom error handler integration
- Default behavior validation
- Error propagation testing
