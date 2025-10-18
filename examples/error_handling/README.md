# Error Handling Example

This example demonstrates comprehensive error handling strategies with Argus using the [go-errors](https://github.com/agilira/go-errors) for structured error handling.

## Overview

The error handling example showcases:

- **Structured Error Handling**: Using `go-errors` for consistent error codes and messages
- **Custom Error Handlers**: Implementing custom error handling logic
- **Error Code Integration**: Using Argus error codes with `go-errors`
- **Error Wrapping**: Demonstrating error wrapping and cause tracking
- **Performance**: Error creation and handling performance characteristics

## Features Demonstrated

### 1. Custom Error Handler
Shows how to implement custom error handlers that integrate with Argus watchers:

```go
errorHandler := func(err error, filepath string) {
    fmt.Printf("Custom Error Handler: File %s had error: %v\n", filepath, err)
    
    // Demonstrate go-errors structured error handling
    errorMsg := err.Error()
    if strings.Contains(errorMsg, "ARGUS_INVALID_CONFIG") {
        fmt.Printf("Identified as invalid config error\n")
    }
}
```

### 2. Error Code Integration
Demonstrates using Argus error codes with `go-errors`:

```go
// Create error with go-errors using Argus error codes
customErr := errors.New(argus.ErrCodeInvalidConfig, "Custom error message")

// Wrap error with another Argus error code
wrappedErr := errors.Wrap(customErr, argus.ErrCodeWatcherStopped, "Wrapped error")
```

### 3. File Not Found Handling
Shows how to handle file not found errors:

```go
// Error handler for file not found
errorHandler := func(err error, filepath string) {
    errorMsg := err.Error()
    if strings.Contains(errorMsg, "ARGUS_FILE_NOT_FOUND") {
        fmt.Printf("Correctly identified as file not found error\n")
    }
}
```

### 4. Parse Error Handling
Demonstrates handling configuration parsing errors:

```go
// Error handler for parse errors
errorHandler := func(err error, filepath string) {
    errorMsg := err.Error()
    if strings.Contains(errorMsg, "INVALID_CONFIG") {
        fmt.Printf("Correctly identified as config parsing error\n")
    }
}
```

## Argus Error Codes

The example uses the following Argus error codes:

- `ARGUS_INVALID_CONFIG`: Invalid configuration or parsing errors
- `ARGUS_FILE_NOT_FOUND`: File not found errors
- `ARGUS_WATCHER_STOPPED`: Watcher stopped errors
- `ARGUS_WATCHER_BUSY`: Watcher busy errors
- `ARGUS_INVALID_POLL_INTERVAL`: Invalid poll interval errors

## Dependencies

- [go-errors](https://github.com/agilira/go-errors): Structured error handling library
- [argus](https://github.com/agilira/argus): Configuration watcher library

## Running the Example

```bash
# Run the example
go run main.go

# Run the test suite
go test -v

# Run with race detection
go test -race -v
```

## Performance Characteristics

- **Error Creation**: < 1µs per error (tested with 1000 iterations)
- **Error Wrapping**: Efficient error wrapping with cause tracking
- **Concurrent Safety**: Thread-safe error handling
- **Memory Efficient**: Minimal allocations in error handling paths

## Best Practices

1. **Use Argus Error Codes**: Always use `argus.ErrCode*` constants for consistency
2. **Error Wrapping**: Use `errors.Wrap()` to preserve error context
3. **Error Code Checking**: Check error codes using string matching on `err.Error()`
4. **Custom Handlers**: Implement custom error handlers for specific use cases
5. **Performance**: Error handling should be fast and non-blocking

## Integration with Argus

The example shows how to integrate `go-errors` with Argus:

```go
config := argus.Config{
    PollInterval: 50 * time.Millisecond,
    ErrorHandler: customErrorHandler,
}

watcher, err := argus.UniversalConfigWatcherWithConfig(configFile, callback, config)
```

## Error Message Format

Argus errors follow the format: `[ERROR_CODE]: Error message`

Example: `[ARGUS_INVALID_CONFIG]: failed to parse JSON config`

---

Argus • an AGILira fragment