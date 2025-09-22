// env_config.go: Environment Variables Support for Argus Configuration Framework
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

// Package argus provides environment variable configuration loading and processing.
// This file implements comprehensive environment-based configuration management.

package argus

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/agilira/go-errors"
)

// EnvConfig represents configuration loaded from environment variables
// This provides environment variable support with automatic type detection
type EnvConfig struct {
	// Core Configuration
	PollInterval    time.Duration `env:"ARGUS_POLL_INTERVAL"`
	CacheTTL        time.Duration `env:"ARGUS_CACHE_TTL"`
	MaxWatchedFiles int           `env:"ARGUS_MAX_WATCHED_FILES"`

	// Performance Configuration
	OptimizationStrategy string `env:"ARGUS_OPTIMIZATION_STRATEGY"`
	BoreasLiteCapacity   int64  `env:"ARGUS_BOREAS_CAPACITY"`

	// Audit Configuration
	AuditEnabled       bool          `env:"ARGUS_AUDIT_ENABLED"`
	AuditOutputFile    string        `env:"ARGUS_AUDIT_OUTPUT_FILE"`
	AuditMinLevel      string        `env:"ARGUS_AUDIT_MIN_LEVEL"`
	AuditBufferSize    int           `env:"ARGUS_AUDIT_BUFFER_SIZE"`
	AuditFlushInterval time.Duration `env:"ARGUS_AUDIT_FLUSH_INTERVAL"`

	// Remote Configuration Sources
	RemoteURL      string        `env:"ARGUS_REMOTE_URL"`
	RemoteInterval time.Duration `env:"ARGUS_REMOTE_INTERVAL"`
	RemoteTimeout  time.Duration `env:"ARGUS_REMOTE_TIMEOUT"`
	RemoteHeaders  string        `env:"ARGUS_REMOTE_HEADERS"` // JSON format

	// Validation Configuration
	ValidationEnabled bool   `env:"ARGUS_VALIDATION_ENABLED"`
	ValidationSchema  string `env:"ARGUS_VALIDATION_SCHEMA"`
	ValidationStrict  bool   `env:"ARGUS_VALIDATION_STRICT"`
}

// LoadConfigFromEnv loads Argus configuration from environment variables
// This provides an intuitive interface for container deployments
func LoadConfigFromEnv() (*Config, error) {
	config := &Config{}
	envConfig := &EnvConfig{}

	// Load environment variables into EnvConfig struct
	if err := loadEnvVars(envConfig); err != nil {
		return nil, errors.Wrap(err, ErrCodeInvalidConfig, "failed to load environment configuration")
	}

	// Convert EnvConfig to standard Config
	if err := convertEnvToConfig(envConfig, config); err != nil {
		return nil, errors.Wrap(err, ErrCodeInvalidConfig, "failed to convert environment configuration")
	}

	// Apply defaults for any unset values
	config = config.WithDefaults()

	return config, nil
}

// LoadConfigMultiSource loads configuration with precedence:
// 1. Environment variables (highest priority)
// 2. File configuration
// 3. Default values (lowest priority)
//
// This provides multi-source configuration loading with precedence
func LoadConfigMultiSource(configFile string) (*Config, error) {
	// Start with file-based configuration
	config := &Config{}

	// Load from file if provided
	if configFile != "" {
		if _, err := os.Stat(configFile); err == nil {
			// File exists, we could load it here when we implement file loading
			// For now, start with defaults
			config = config.WithDefaults()
		} else {
			// File doesn't exist, start with defaults
			config = config.WithDefaults()
		}
	} else {
		config = config.WithDefaults()
	}

	// Override with environment variables
	envConfig, err := LoadConfigFromEnv()
	if err != nil {
		return config, err // Return file config with error
	}

	// Apply environment overrides
	if err := mergeConfigs(config, envConfig); err != nil {
		return config, errors.Wrap(err, ErrCodeInvalidConfig, "failed to merge configurations")
	}

	return config, nil
}

// loadEnvVars loads environment variables into the EnvConfig struct
func loadEnvVars(envConfig *EnvConfig) error {
	// Load configurations in logical groups
	if err := loadCoreConfig(envConfig); err != nil {
		return err
	}
	if err := loadPerformanceConfig(envConfig); err != nil {
		return err
	}
	if err := loadAuditConfig(envConfig); err != nil {
		return err
	}
	if err := loadRemoteConfig(envConfig); err != nil {
		return err
	}
	return loadValidationConfig(envConfig)
}

// loadCoreConfig loads core configuration from environment variables with security validation
func loadCoreConfig(envConfig *EnvConfig) error {
	// Load and validate poll interval
	if err := loadPollInterval(envConfig); err != nil {
		return err
	}

	// Load and validate cache TTL
	if err := loadCacheTTL(envConfig); err != nil {
		return err
	}

	// Load and validate max watched files
	if err := loadMaxWatchedFiles(envConfig); err != nil {
		return err
	}

	return nil
}

// loadPollInterval loads and validates poll interval from environment
func loadPollInterval(envConfig *EnvConfig) error {
	pollStr := os.Getenv("ARGUS_POLL_INTERVAL")
	if pollStr == "" {
		return nil
	}

	duration, err := time.ParseDuration(pollStr)
	if err != nil {
		return errors.New(ErrCodeInvalidConfig, "invalid ARGUS_POLL_INTERVAL format")
	}

	// SECURITY: Prevent excessively fast polling that could cause DoS
	if duration < 100*time.Millisecond {
		return errors.New(ErrCodeInvalidConfig, "poll interval too fast (minimum 100ms)")
	}
	// SECURITY: Prevent excessively slow polling that could cause missed events
	if duration > 10*time.Minute {
		return errors.New(ErrCodeInvalidConfig, "poll interval too slow (maximum 10 minutes)")
	}

	envConfig.PollInterval = duration
	return nil
}

// loadCacheTTL loads and validates cache TTL from environment
func loadCacheTTL(envConfig *EnvConfig) error {
	cacheStr := os.Getenv("ARGUS_CACHE_TTL")
	if cacheStr == "" {
		return nil
	}

	duration, err := time.ParseDuration(cacheStr)
	if err != nil {
		return errors.New(ErrCodeInvalidConfig, "invalid ARGUS_CACHE_TTL format")
	}

	// SECURITY: Ensure cache TTL is reasonable for security and performance
	if duration < 1*time.Second {
		return errors.New(ErrCodeInvalidConfig, "cache TTL too short (minimum 1 second)")
	}
	if duration > 1*time.Hour {
		return errors.New(ErrCodeInvalidConfig, "cache TTL too long (maximum 1 hour)")
	}

	envConfig.CacheTTL = duration
	return nil
}

// loadMaxWatchedFiles loads and validates max watched files from environment
func loadMaxWatchedFiles(envConfig *EnvConfig) error {
	maxStr := os.Getenv("ARGUS_MAX_WATCHED_FILES")
	if maxStr == "" {
		return nil
	}

	maxFiles, err := strconv.Atoi(maxStr)
	if err != nil {
		return errors.New(ErrCodeInvalidConfig, "invalid ARGUS_MAX_WATCHED_FILES value")
	}

	// SECURITY: Enforce reasonable limits to prevent resource exhaustion
	if maxFiles < 1 {
		return errors.New(ErrCodeInvalidConfig, "max watched files must be at least 1")
	}
	if maxFiles > 10000 { // Reasonable upper limit for most use cases
		return errors.New(ErrCodeInvalidConfig, "max watched files too high (maximum 10000)")
	}

	envConfig.MaxWatchedFiles = maxFiles
	return nil
}

// loadPerformanceConfig loads performance configuration from environment variables with security validation
func loadPerformanceConfig(envConfig *EnvConfig) error {
	// Load and validate optimization strategy
	if err := loadOptimizationStrategy(envConfig); err != nil {
		return err
	}

	// Load and validate BoreasLite capacity
	if err := loadBoreasLiteCapacity(envConfig); err != nil {
		return err
	}

	return nil
}

// loadOptimizationStrategy loads and validates optimization strategy from environment
func loadOptimizationStrategy(envConfig *EnvConfig) error {
	optimizationStr := os.Getenv("ARGUS_OPTIMIZATION_STRATEGY")
	if optimizationStr == "" {
		return nil
	}

	// SECURITY: Only allow known valid optimization strategies
	validStrategies := []string{"auto", "single", "singleevent", "small", "smallbatch", "large", "largebatch"}
	lowerStrategy := strings.ToLower(optimizationStr)

	for _, valid := range validStrategies {
		if lowerStrategy == valid {
			envConfig.OptimizationStrategy = optimizationStr
			return nil
		}
	}

	return errors.New(ErrCodeInvalidConfig, "invalid optimization strategy")
}

// loadBoreasLiteCapacity loads and validates BoreasLite capacity from environment
func loadBoreasLiteCapacity(envConfig *EnvConfig) error {
	capacityStr := os.Getenv("ARGUS_BOREAS_CAPACITY")
	if capacityStr == "" {
		return nil
	}

	capacity, err := strconv.ParseInt(capacityStr, 10, 64)
	if err != nil || capacity <= 0 {
		return errors.New(ErrCodeInvalidConfig, "invalid ARGUS_BOREAS_CAPACITY value")
	}

	// SECURITY: Enforce reasonable capacity limits to prevent memory exhaustion
	if capacity < 32 {
		return errors.New(ErrCodeInvalidConfig, "BoreasLite capacity too low (minimum 32)")
	}
	if capacity > 1048576 { // 1MB entries max
		return errors.New(ErrCodeInvalidConfig, "BoreasLite capacity too high (maximum 1048576)")
	}

	// SECURITY: Ensure capacity is power of 2 (prevents certain attacks)
	if capacity&(capacity-1) != 0 {
		return errors.New(ErrCodeInvalidConfig, "BoreasLite capacity must be power of 2")
	}

	envConfig.BoreasLiteCapacity = capacity
	return nil
}

// loadAuditConfig loads audit configuration from environment variables with security validation
func loadAuditConfig(envConfig *EnvConfig) error {
	// Load audit enable/disable settings with security validation
	if err := loadAuditEnabledSetting(envConfig); err != nil {
		return err
	}

	// Load and validate audit output file path
	if err := loadAuditOutputFile(envConfig); err != nil {
		return err
	}

	// Load audit min level
	envConfig.AuditMinLevel = os.Getenv("ARGUS_AUDIT_MIN_LEVEL")

	// Load audit buffer and flush settings with security limits
	if err := loadAuditBufferSettings(envConfig); err != nil {
		return err
	}

	return nil
}

// loadAuditEnabledSetting loads and validates audit enabled setting with security policy
func loadAuditEnabledSetting(envConfig *EnvConfig) error {
	// SECURITY POLICY: Audit should generally remain enabled in production environments
	// Only allow disabling in specific development/test scenarios
	auditStr := os.Getenv("ARGUS_AUDIT_ENABLED")
	if auditStr == "" {
		return nil
	}

	requestedEnabled := parseBool(auditStr)

	// SECURITY CHECK: Prevent audit disabling unless explicitly allowed
	if !requestedEnabled {
		// Check for explicit development/test override
		devOverride := os.Getenv("ARGUS_ALLOW_AUDIT_DISABLE")
		if devOverride == "" || !parseBool(devOverride) {
			// Log security event but don't fail - keep audit enabled for security
			// In production, this should be logged to a secure audit trail
			envConfig.AuditEnabled = true // Force enable for security
		} else {
			envConfig.AuditEnabled = requestedEnabled
		}
	} else {
		envConfig.AuditEnabled = requestedEnabled
	}

	return nil
}

// loadAuditOutputFile loads and validates audit output file path
func loadAuditOutputFile(envConfig *EnvConfig) error {
	auditOutputFile := os.Getenv("ARGUS_AUDIT_OUTPUT_FILE")
	if auditOutputFile == "" {
		return nil
	}

	// SECURITY VALIDATION: Validate audit output file path
	// Use the same path validation as file watching to prevent path traversal
	if err := validateSecureAuditPath(auditOutputFile); err != nil {
		return errors.Wrap(err, ErrCodeInvalidConfig, "audit output file path is unsafe").
			WithContext("audit_path", auditOutputFile)
	}
	envConfig.AuditOutputFile = auditOutputFile
	return nil
}

// loadAuditBufferSettings loads audit buffer size and flush interval with security limits
func loadAuditBufferSettings(envConfig *EnvConfig) error {
	// SECURITY LIMITS: Enforce reasonable buffer size limits
	if bufferStr := os.Getenv("ARGUS_AUDIT_BUFFER_SIZE"); bufferStr != "" {
		buffer, err := strconv.Atoi(bufferStr)
		if err != nil || buffer <= 0 {
			return errors.New(ErrCodeInvalidConfig, "invalid ARGUS_AUDIT_BUFFER_SIZE value")
		}

		// SECURITY: Limit buffer size to prevent memory exhaustion attacks
		if buffer > 100000 { // Max 100k events in buffer
			return errors.New(ErrCodeInvalidConfig, "audit buffer size too large (max 100000)")
		}
		envConfig.AuditBufferSize = buffer
	}

	if flushStr := os.Getenv("ARGUS_AUDIT_FLUSH_INTERVAL"); flushStr != "" {
		duration, err := time.ParseDuration(flushStr)
		if err != nil {
			return errors.New(ErrCodeInvalidConfig, "invalid ARGUS_AUDIT_FLUSH_INTERVAL value")
		}

		// SECURITY: Prevent excessively long flush intervals that could lose audit data
		if duration > 5*time.Minute {
			return errors.New(ErrCodeInvalidConfig, "audit flush interval too long (max 5 minutes)")
		}
		envConfig.AuditFlushInterval = duration
	}

	return nil
}

// loadRemoteConfig loads remote configuration from environment variables
func loadRemoteConfig(envConfig *EnvConfig) error {
	// Remote Configuration Sources
	envConfig.RemoteURL = os.Getenv("ARGUS_REMOTE_URL")

	if remoteStr := os.Getenv("ARGUS_REMOTE_INTERVAL"); remoteStr != "" {
		if duration, err := time.ParseDuration(remoteStr); err == nil {
			envConfig.RemoteInterval = duration
		}
	}

	if timeoutStr := os.Getenv("ARGUS_REMOTE_TIMEOUT"); timeoutStr != "" {
		if duration, err := time.ParseDuration(timeoutStr); err == nil {
			envConfig.RemoteTimeout = duration
		}
	}

	envConfig.RemoteHeaders = os.Getenv("ARGUS_REMOTE_HEADERS")
	return nil
}

// loadValidationConfig loads validation configuration from environment variables
func loadValidationConfig(envConfig *EnvConfig) error {
	// Validation Configuration
	if validationStr := os.Getenv("ARGUS_VALIDATION_ENABLED"); validationStr != "" {
		envConfig.ValidationEnabled = parseBool(validationStr)
	}

	envConfig.ValidationSchema = os.Getenv("ARGUS_VALIDATION_SCHEMA")

	if strictStr := os.Getenv("ARGUS_VALIDATION_STRICT"); strictStr != "" {
		envConfig.ValidationStrict = parseBool(strictStr)
	}

	return nil
}

// convertEnvToConfig converts EnvConfig to standard Config
func convertEnvToConfig(envConfig *EnvConfig, config *Config) error {
	// Convert configurations in logical groups
	convertCoreConfig(envConfig, config)
	if err := convertPerformanceConfig(envConfig, config); err != nil {
		return err
	}
	if err := convertAuditConfig(envConfig, config); err != nil {
		return err
	}
	return nil
}

// convertCoreConfig converts core configuration from EnvConfig to Config
func convertCoreConfig(envConfig *EnvConfig, config *Config) {
	if envConfig.PollInterval != 0 {
		config.PollInterval = envConfig.PollInterval
	}
	if envConfig.CacheTTL != 0 {
		config.CacheTTL = envConfig.CacheTTL
	}
	if envConfig.MaxWatchedFiles != 0 {
		config.MaxWatchedFiles = envConfig.MaxWatchedFiles
	}
}

// convertPerformanceConfig converts performance configuration from EnvConfig to Config
func convertPerformanceConfig(envConfig *EnvConfig, config *Config) error {
	if envConfig.OptimizationStrategy != "" {
		switch strings.ToLower(envConfig.OptimizationStrategy) {
		case "auto":
			config.OptimizationStrategy = OptimizationAuto
		case "single", "singleevent":
			config.OptimizationStrategy = OptimizationSingleEvent
		case "small", "smallbatch":
			config.OptimizationStrategy = OptimizationSmallBatch
		case "large", "largebatch":
			config.OptimizationStrategy = OptimizationLargeBatch
		default:
			return errors.New(ErrCodeInvalidConfig, "invalid optimization strategy")
		}
	}
	if envConfig.BoreasLiteCapacity > 0 {
		config.BoreasLiteCapacity = envConfig.BoreasLiteCapacity
	}
	return nil
}

// convertAuditConfig converts audit configuration from EnvConfig to Config
func convertAuditConfig(envConfig *EnvConfig, config *Config) error {
	if envConfig.AuditEnabled || envConfig.AuditOutputFile != "" {
		return convertAuditSettings(envConfig, config)
	}
	return nil
}

// convertAuditSettings converts individual audit settings
func convertAuditSettings(envConfig *EnvConfig, config *Config) error {
	config.Audit.Enabled = envConfig.AuditEnabled

	if envConfig.AuditOutputFile != "" {
		config.Audit.OutputFile = envConfig.AuditOutputFile
	}

	if err := convertAuditLevel(envConfig, config); err != nil {
		return err
	}

	convertAuditBufferSettings(envConfig, config)
	return nil
}

// convertAuditLevel converts audit level setting
func convertAuditLevel(envConfig *EnvConfig, config *Config) error {
	if envConfig.AuditMinLevel != "" {
		level, err := parseAuditLevel(envConfig.AuditMinLevel)
		if err != nil {
			return err
		}
		config.Audit.MinLevel = level
	}
	return nil
}

// convertAuditBufferSettings converts audit buffer and flush settings
func convertAuditBufferSettings(envConfig *EnvConfig, config *Config) {
	if envConfig.AuditBufferSize > 0 {
		config.Audit.BufferSize = envConfig.AuditBufferSize
	}
	if envConfig.AuditFlushInterval > 0 {
		config.Audit.FlushInterval = envConfig.AuditFlushInterval
	}
}

// parseAuditLevel parses audit level string to AuditLevel type
func parseAuditLevel(levelStr string) (AuditLevel, error) {
	switch strings.ToLower(levelStr) {
	case "info":
		return AuditInfo, nil
	case "warn", "warning":
		return AuditWarn, nil
	case "critical", "error":
		return AuditCritical, nil
	case "security":
		return AuditSecurity, nil
	default:
		return AuditInfo, errors.New(ErrCodeInvalidConfig, "invalid audit level")
	}
}

// mergeConfigs merges environment configuration into base configuration
func mergeConfigs(base, env *Config) error {
	mergeCoreConfig(base, env)
	mergeAuditConfig(base, env)
	return nil
}

// mergeCoreConfig merges core configuration settings
func mergeCoreConfig(base, env *Config) {
	if env.PollInterval > 0 {
		base.PollInterval = env.PollInterval
	}
	if env.CacheTTL > 0 {
		base.CacheTTL = env.CacheTTL
	}
	if env.MaxWatchedFiles > 0 {
		base.MaxWatchedFiles = env.MaxWatchedFiles
	}
	if env.OptimizationStrategy != OptimizationAuto {
		base.OptimizationStrategy = env.OptimizationStrategy
	}
	if env.BoreasLiteCapacity > 0 {
		base.BoreasLiteCapacity = env.BoreasLiteCapacity
	}
}

// mergeAuditConfig merges audit configuration settings
func mergeAuditConfig(base, env *Config) {
	if env.Audit.Enabled {
		base.Audit.Enabled = env.Audit.Enabled
	}
	if env.Audit.OutputFile != "" {
		base.Audit.OutputFile = env.Audit.OutputFile
	}
	if env.Audit.MinLevel != AuditInfo {
		base.Audit.MinLevel = env.Audit.MinLevel
	}
	if env.Audit.BufferSize > 0 {
		base.Audit.BufferSize = env.Audit.BufferSize
	}
	if env.Audit.FlushInterval > 0 {
		base.Audit.FlushInterval = env.Audit.FlushInterval
	}
}

// parseBool parses boolean values from environment variables
// Supports: true/false, 1/0, yes/no, on/off, enabled/disabled
func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on", "enabled":
		return true
	case "false", "0", "no", "off", "disabled":
		return false
	default:
		return false
	}
}

// GetEnvWithDefault returns environment variable value or default if not set
func GetEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetEnvDurationWithDefault returns environment variable as duration or default
func GetEnvDurationWithDefault(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

// GetEnvIntWithDefault returns environment variable as int or default
func GetEnvIntWithDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// GetEnvBoolWithDefault returns environment variable as bool or default
func GetEnvBoolWithDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return parseBool(value)
	}
	return defaultValue
}

// validateSecureAuditPath validates audit file paths using the same security checks as file watching.
// This prevents path traversal attacks via audit configuration environment variables.
func validateSecureAuditPath(path string) error {
	// Reuse the comprehensive path validation from the main security function
	return validateSecurePath(path)
}
