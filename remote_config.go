// remote_config.go: Remote Configuration Sources Plugin System
//
// This implements a plugin-based architecture for loading configuration from remote sources.
// The core remains dependency-free while production plugins provide full-featured integrations.
//
// PRODUCTION USAGE:
//   import _ "github.com/agilira/argus-redis"     // Auto-registers Redis provider
//   import _ "github.com/agilira/argus-consul"   // Auto-registers Consul provider
//   import _ "github.com/agilira/argus-etcd"     // Auto-registers etcd provider
//
// MANUAL REGISTRATION:
//   argus.RegisterRemoteProvider(&MyCustomProvider{})
//
// USAGE:
//   config, err := argus.LoadRemoteConfig("redis://localhost:6379/config")
//   config, err := argus.LoadRemoteConfig("consul://localhost:8500/config/myapp")
//   config, err := argus.LoadRemoteConfig("etcd://localhost:2379/config/myapp")
//   config, err := argus.LoadRemoteConfig("https://config.mycompany.com/api/config")
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

// Package argus provides remote configuration loading with retry mechanisms and watching.
// This file implements the remote provider system for distributed configuration management.

package argus

import (
	"context"
	goerrors "errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/agilira/go-errors"
)

// RemoteConfigProvider defines the interface for remote configuration sources.
// Implementations provide access to distributed configuration systems like Redis,
// Consul, etcd, or HTTP APIs. Providers are registered globally and selected
// automatically based on URL scheme.
//
// Example implementations:
//   - Redis: redis://localhost:6379/config
//   - Consul: consul://localhost:8500/config/myapp
//   - etcd: etcd://localhost:2379/config/myapp
//   - HTTP: https://config.mycompany.com/api/config
type RemoteConfigProvider interface {
	// Name returns a human-readable name for this provider (for debugging)
	Name() string

	// Scheme returns the URL scheme this provider handles (e.g., "redis", "consul", "etcd", "http", "https")
	Scheme() string

	// Load loads configuration from the remote source
	// The URL contains the full connection information
	Load(ctx context.Context, configURL string) (map[string]interface{}, error)

	// Watch starts watching for configuration changes (optional)
	// Returns a channel that sends new configurations when they change
	// If the provider doesn't support watching, it should return nil, nil
	Watch(ctx context.Context, configURL string) (<-chan map[string]interface{}, error)

	// Validate validates that the provider can handle the given URL
	Validate(configURL string) error

	// HealthCheck performs a health check on the remote source
	HealthCheck(ctx context.Context, configURL string) error
}

// RemoteConfigOptions provides options for remote configuration loading.
// Controls timeouts, retries, authentication, and watching behavior.
// Use DefaultRemoteConfigOptions() for sensible defaults.
type RemoteConfigOptions struct {
	// Timeout for remote operations
	Timeout time.Duration

	// RetryAttempts for failed requests
	RetryAttempts int

	// RetryDelay between retry attempts
	RetryDelay time.Duration

	// Watch enables automatic configuration reloading
	Watch bool

	// WatchInterval for polling-based providers (fallback if native watching not supported)
	WatchInterval time.Duration

	// Headers for HTTP-based providers
	Headers map[string]string

	// TLSConfig for secure connections (provider-specific)
	TLSConfig map[string]interface{}

	// Authentication credentials (provider-specific)
	Auth map[string]interface{}
}

// DefaultRemoteConfigOptions provides sensible defaults for remote configuration.
// Returns a new options instance with production-ready timeout and retry settings.
func DefaultRemoteConfigOptions() *RemoteConfigOptions {
	return &RemoteConfigOptions{
		Timeout:       30 * time.Second,
		RetryAttempts: 3,
		RetryDelay:    1 * time.Second,
		Watch:         false,
		WatchInterval: 30 * time.Second,
		Headers:       make(map[string]string),
		TLSConfig:     make(map[string]interface{}),
		Auth:          make(map[string]interface{}),
	}
}

// Global registry of remote configuration providers
var (
	remoteProviders []RemoteConfigProvider
	remoteMutex     sync.RWMutex
)

// RegisterRemoteProvider registers a custom remote configuration provider.
// Providers are tried in registration order. Duplicate schemes are rejected.
//
// Example:
//
//	argus.RegisterRemoteProvider(&MyCustomProvider{})
//
// Or via import-based auto-registration:
//
//	import _ "github.com/agilira/argus-redis"  // Auto-registers in init()
func RegisterRemoteProvider(provider RemoteConfigProvider) error {
	if provider == nil {
		return errors.New(ErrCodeInvalidConfig, "remote provider cannot be nil")
	}

	scheme := provider.Scheme()
	if scheme == "" {
		return errors.New(ErrCodeInvalidConfig, "remote provider scheme cannot be empty")
	}

	remoteMutex.Lock()
	defer remoteMutex.Unlock()

	// Check for duplicate schemes
	for _, existing := range remoteProviders {
		if existing.Scheme() == scheme {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("remote provider for scheme '%s' already registered", scheme))
		}
	}

	remoteProviders = append(remoteProviders, provider)
	return nil
}

// GetRemoteProvider returns the provider for the given URL scheme.
// Used internally to find the appropriate provider for a remote config URL.
func GetRemoteProvider(scheme string) (RemoteConfigProvider, error) {
	remoteMutex.RLock()
	defer remoteMutex.RUnlock()

	for _, provider := range remoteProviders {
		if provider.Scheme() == scheme {
			return provider, nil
		}
	}

	return nil, errors.New(ErrCodeInvalidConfig,
		fmt.Sprintf("no remote provider registered for scheme '%s'", scheme))
}

// ListRemoteProviders returns a list of all registered remote providers.
// Returns a copy to prevent external modification of the provider registry.
// Useful for debugging and discovering available remote configuration sources.
func ListRemoteProviders() []RemoteConfigProvider {
	remoteMutex.RLock()
	defer remoteMutex.RUnlock()

	// Return a copy to prevent modification
	providers := make([]RemoteConfigProvider, len(remoteProviders))
	copy(providers, remoteProviders)
	return providers
}

// LoadRemoteConfig loads configuration from a remote source using default context.
// Automatically detects provider based on URL scheme and handles retries.
//
// Example:
//
//	config, err := argus.LoadRemoteConfig("redis://localhost:6379/config")
//	config, err := argus.LoadRemoteConfig("consul://localhost:8500/config/myapp")
func LoadRemoteConfig(configURL string, opts ...*RemoteConfigOptions) (map[string]interface{}, error) {
	return LoadRemoteConfigWithContext(context.Background(), configURL, opts...)
}

// LoadRemoteConfigWithContext loads configuration from a remote source with custom context.
// Provides full control over timeouts and cancellation for remote configuration loading.
func LoadRemoteConfigWithContext(ctx context.Context, configURL string, opts ...*RemoteConfigOptions) (map[string]interface{}, error) {
	// Validate and setup
	provider, options, err := setupRemoteConfig(configURL, opts...)
	if err != nil {
		return nil, err
	}

	// Load with retries
	return loadWithRetries(ctx, provider, configURL, options)
}

// setupRemoteConfig validates URL and gets provider
func setupRemoteConfig(configURL string, opts ...*RemoteConfigOptions) (RemoteConfigProvider, *RemoteConfigOptions, error) {
	// Validate and parse URL
	provider, err := validateAndGetProvider(configURL)
	if err != nil {
		return nil, nil, err
	}

	// Get options with defaults
	options := getRemoteOptions(opts...)

	return provider, options, nil
}

// validateAndGetProvider validates URL and gets appropriate provider
func validateAndGetProvider(configURL string) (RemoteConfigProvider, error) {
	if configURL == "" {
		return nil, errors.New(ErrCodeInvalidConfig, "remote config URL cannot be empty")
	}

	parsedURL, err := url.Parse(configURL)
	if err != nil {
		return nil, errors.Wrap(err, ErrCodeInvalidConfig, "invalid remote config URL")
	}

	if parsedURL.Scheme == "" {
		return nil, errors.New(ErrCodeInvalidConfig, "remote config URL must have a scheme")
	}

	provider, err := GetRemoteProvider(parsedURL.Scheme)
	if err != nil {
		return nil, err
	}

	if err := provider.Validate(configURL); err != nil {
		return nil, errors.Wrap(err, ErrCodeInvalidConfig, "remote config URL validation failed")
	}

	return provider, nil
}

// getRemoteOptions returns options with defaults applied
func getRemoteOptions(opts ...*RemoteConfigOptions) *RemoteConfigOptions {
	options := DefaultRemoteConfigOptions()
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
	}
	return options
}

// loadWithRetries loads config with retry logic
func loadWithRetries(ctx context.Context, provider RemoteConfigProvider, configURL string, options *RemoteConfigOptions) (map[string]interface{}, error) {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()

	var config map[string]interface{}
	var lastErr error

	for attempt := 0; attempt <= options.RetryAttempts; attempt++ {
		if attempt > 0 {
			if err := waitForRetry(ctxWithTimeout, options.RetryDelay); err != nil {
				return nil, err
			}
		}

		config, lastErr = provider.Load(ctxWithTimeout, configURL)
		if lastErr == nil {
			break
		}

		if shouldStopRetrying(lastErr) {
			break
		}
	}

	if lastErr != nil {
		return nil, errors.Wrap(lastErr, ErrCodeRemoteConfigError, "failed to load remote configuration")
	}

	if config == nil {
		return nil, errors.New(ErrCodeRemoteConfigError, "remote provider returned nil configuration")
	}

	return config, nil
}

// waitForRetry waits for retry delay or context cancellation
func waitForRetry(ctx context.Context, delay time.Duration) error {
	select {
	case <-time.After(delay):
		return nil
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), ErrCodeRemoteConfigError, "context canceled during retry")
	}
}

// shouldStopRetrying determines if retry attempts should be stopped based on error type.
//
// This function implements intelligent retry logic by categorizing errors into:
// 1. Context errors (canceled/timeout) - stop retrying
// 2. HTTP client errors (4xx) - stop retrying as they indicate permanent issues
// 3. Specific HTTP server errors that are not recoverable - stop retrying
// 4. Network and temporary errors - continue retrying
//
// Returns true if retrying should be stopped, false if it should continue.
func shouldStopRetrying(err error) bool {
	if err == nil {
		return false
	}

	// Context cancellation or timeout - stop immediately
	if goerrors.Is(err, context.Canceled) || goerrors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Check for HTTP-specific errors that are not recoverable
	if isNonRecoverableHTTPError(err) {
		return true
	}

	// For all other errors (network, temporary, etc.), continue retrying
	return false
}

// isNonRecoverableHTTPError checks if an error represents an HTTP error
// that should not be retried (client errors and specific server errors).
func isNonRecoverableHTTPError(err error) bool {
	// Convert error to string for pattern matching
	// This approach works with most HTTP client implementations
	errStr := strings.ToLower(err.Error())

	// HTTP 4xx client errors - these indicate permanent configuration issues
	clientErrors := []string{
		"400 bad request",
		"401 unauthorized",
		"402 payment required",
		"403 forbidden",
		"404 not found",
		"405 method not allowed",
		"406 not acceptable",
		"407 proxy authentication required",
		"408 request timeout",
		"409 conflict",
		"410 gone",
		"411 length required",
		"412 precondition failed",
		"413 payload too large",
		"414 uri too long",
		"415 unsupported media type",
		"416 range not satisfiable",
		"417 expectation failed",
		"418 i'm a teapot", // RFC 2324
		"421 misdirected request",
		"422 unprocessable entity",
		"423 locked",
		"424 failed dependency",
		"425 too early",
		"426 upgrade required",
		"428 precondition required",
		"429 too many requests", // Rate limiting - could be recoverable, but usually indicates config issue
		"431 request header fields too large",
		"451 unavailable for legal reasons",
	}

	// Check for client error patterns
	for _, clientError := range clientErrors {
		if strings.Contains(errStr, clientError) {
			return true
		}
	}

	// Specific server errors that indicate permanent issues
	permanentServerErrors := []string{
		"501 not implemented",
		"505 http version not supported",
		"506 variant also negotiates",
		"507 insufficient storage",
		"508 loop detected",
		"510 not extended",
		"511 network authentication required",
	}

	// Check for permanent server error patterns
	for _, serverError := range permanentServerErrors {
		if strings.Contains(errStr, serverError) {
			return true
		}
	}

	// Authentication and authorization related errors (various formats)
	authErrors := []string{
		"authentication failed",
		"unauthorized access",
		"invalid credentials",
		"access denied",
		"permission denied",
		"forbidden",
		"invalid api key",
		"invalid token",
		"token expired",
		"certificate verify failed",
		"ssl certificate problem",
		"tls handshake failure",
	}

	// Check for authentication/authorization error patterns
	for _, authError := range authErrors {
		if strings.Contains(errStr, authError) {
			return true
		}
	}

	return false
}

// WatchRemoteConfig starts watching a remote configuration source for changes
func WatchRemoteConfig(configURL string, opts ...*RemoteConfigOptions) (<-chan map[string]interface{}, error) {
	return WatchRemoteConfigWithContext(context.Background(), configURL, opts...)
}

// WatchRemoteConfigWithContext starts watching a remote configuration source with context
func WatchRemoteConfigWithContext(ctx context.Context, configURL string, opts ...*RemoteConfigOptions) (<-chan map[string]interface{}, error) {
	// Setup provider and options
	provider, options, err := setupRemoteConfig(configURL, opts...)
	if err != nil {
		return nil, err
	}

	// Try native watching first
	return startWatching(ctx, provider, configURL, options)
}

// startWatching starts the actual watching process
func startWatching(ctx context.Context, provider RemoteConfigProvider, configURL string, options *RemoteConfigOptions) (<-chan map[string]interface{}, error) {
	configChan, err := provider.Watch(ctx, configURL)
	if err != nil {
		return nil, errors.Wrap(err, ErrCodeRemoteConfigError, "failed to start watching remote configuration")
	}

	if configChan != nil {
		return configChan, nil
	}

	// Fallback to polling
	return startPollingWatch(ctx, provider, configURL, options), nil
}

// startPollingWatch starts polling-based watching
func startPollingWatch(ctx context.Context, provider RemoteConfigProvider, configURL string, options *RemoteConfigOptions) <-chan map[string]interface{} {
	pollingChan := make(chan map[string]interface{}, 1)

	go func() {
		defer close(pollingChan)
		pollForChanges(ctx, provider, configURL, options, pollingChan)
	}()

	return pollingChan
}

// pollForChanges polls for configuration changes
func pollForChanges(ctx context.Context, provider RemoteConfigProvider, configURL string, options *RemoteConfigOptions, pollingChan chan<- map[string]interface{}) {
	ticker := time.NewTicker(options.WatchInterval)
	defer ticker.Stop()

	var lastConfig map[string]interface{}

	// Load initial configuration
	if config, err := provider.Load(ctx, configURL); err == nil {
		lastConfig = config
		select {
		case pollingChan <- config:
		case <-ctx.Done():
			return
		}
	}

	for {
		select {
		case <-ticker.C:
			if newConfig := checkForChanges(ctx, provider, configURL, lastConfig); newConfig != nil {
				lastConfig = newConfig
				select {
				case pollingChan <- newConfig:
				case <-ctx.Done():
					return
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

// checkForChanges checks if configuration has changed
func checkForChanges(ctx context.Context, provider RemoteConfigProvider, configURL string, lastConfig map[string]interface{}) map[string]interface{} {
	newConfig, err := provider.Load(ctx, configURL)
	if err != nil {
		return nil
	}

	// Simple comparison - in production you might want a more sophisticated diff
	if !configEquals(lastConfig, newConfig) {
		return newConfig
	}

	return nil
}

// ConfigEquals performs a basic equality check for configurations.
// This utility function compares two configuration maps for equality by comparing
// their keys and values. It uses string representation for value comparison,
// making it suitable for basic configuration comparison needs.
//
// Example:
//
//	config1 := map[string]interface{}{"key": "value", "count": 42}
//	config2 := map[string]interface{}{"key": "value", "count": 42}
//	if argus.ConfigEquals(config1, config2) {
//	    log.Println("Configurations are identical")
//	}
//
// Note: This function uses string comparison for values, so it may not handle
// complex nested structures or type-sensitive comparisons perfectly.
// For production use cases requiring deep equality, consider using reflect.DeepEqual
// or specialized comparison libraries.
func ConfigEquals(config1, config2 map[string]interface{}) bool {
	// Handle nil cases
	if config1 == nil && config2 == nil {
		return true
	}
	if config1 == nil || config2 == nil {
		return false
	}

	if len(config1) != len(config2) {
		return false
	}

	for key, value1 := range config1 {
		if value2, exists := config2[key]; !exists || fmt.Sprintf("%v", value1) != fmt.Sprintf("%v", value2) {
			return false
		}
	}

	return true
}

// configEquals is an internal alias for backward compatibility
func configEquals(config1, config2 map[string]interface{}) bool {
	return ConfigEquals(config1, config2)
}

// HealthCheckRemoteProvider performs a health check on a remote configuration provider
func HealthCheckRemoteProvider(configURL string, opts ...*RemoteConfigOptions) error {
	return HealthCheckRemoteProviderWithContext(context.Background(), configURL, opts...)
}

// HealthCheckRemoteProviderWithContext performs a health check with context
func HealthCheckRemoteProviderWithContext(ctx context.Context, configURL string, opts ...*RemoteConfigOptions) error {
	if configURL == "" {
		return errors.New(ErrCodeInvalidConfig, "remote config URL cannot be empty")
	}

	// Parse URL to get scheme
	parsedURL, err := url.Parse(configURL)
	if err != nil {
		return errors.Wrap(err, ErrCodeInvalidConfig, "invalid remote config URL")
	}

	scheme := parsedURL.Scheme
	if scheme == "" {
		return errors.New(ErrCodeInvalidConfig, "remote config URL must have a scheme")
	}

	// Get provider for scheme
	provider, err := GetRemoteProvider(scheme)
	if err != nil {
		return err
	}

	// Get options
	options := DefaultRemoteConfigOptions()
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
	}

	// Create context with timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()

	return provider.HealthCheck(ctxWithTimeout, configURL)
}
