// env_config.go: Environment Variables Support for Argus Configuration Framework
//
// Copyright (c) 2025 AGILira
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
// This provides Viper-compatible environment variable support
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
// This provides a Viper-compatible interface for container deployments
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
// This provides Viper-compatible multi-source configuration loading
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

// loadCoreConfig loads core configuration from environment variables
func loadCoreConfig(envConfig *EnvConfig) error {
	// Core Configuration
	if pollStr := os.Getenv("ARGUS_POLL_INTERVAL"); pollStr != "" {
		if duration, err := time.ParseDuration(pollStr); err == nil {
			envConfig.PollInterval = duration
		} else {
			return errors.New(ErrCodeInvalidConfig, "invalid ARGUS_POLL_INTERVAL format")
		}
	}

	if cacheStr := os.Getenv("ARGUS_CACHE_TTL"); cacheStr != "" {
		if duration, err := time.ParseDuration(cacheStr); err == nil {
			envConfig.CacheTTL = duration
		} else {
			return errors.New(ErrCodeInvalidConfig, "invalid ARGUS_CACHE_TTL format")
		}
	}

	if maxStr := os.Getenv("ARGUS_MAX_WATCHED_FILES"); maxStr != "" {
		if maxFiles, err := strconv.Atoi(maxStr); err == nil {
			envConfig.MaxWatchedFiles = maxFiles
		} else {
			return errors.New(ErrCodeInvalidConfig, "invalid ARGUS_MAX_WATCHED_FILES value")
		}
	}
	return nil
}

// loadPerformanceConfig loads performance configuration from environment variables
func loadPerformanceConfig(envConfig *EnvConfig) error {
	// Performance Configuration
	envConfig.OptimizationStrategy = os.Getenv("ARGUS_OPTIMIZATION_STRATEGY")

	if capacityStr := os.Getenv("ARGUS_BOREAS_CAPACITY"); capacityStr != "" {
		if capacity, err := strconv.ParseInt(capacityStr, 10, 64); err == nil && capacity > 0 {
			envConfig.BoreasLiteCapacity = capacity
		} else {
			return errors.New(ErrCodeInvalidConfig, "invalid ARGUS_BOREAS_CAPACITY value")
		}
	}
	return nil
}

// loadAuditConfig loads audit configuration from environment variables
func loadAuditConfig(envConfig *EnvConfig) error {
	// Audit Configuration
	if auditStr := os.Getenv("ARGUS_AUDIT_ENABLED"); auditStr != "" {
		envConfig.AuditEnabled = parseBool(auditStr)
	}

	envConfig.AuditOutputFile = os.Getenv("ARGUS_AUDIT_OUTPUT_FILE")
	envConfig.AuditMinLevel = os.Getenv("ARGUS_AUDIT_MIN_LEVEL")

	if bufferStr := os.Getenv("ARGUS_AUDIT_BUFFER_SIZE"); bufferStr != "" {
		if buffer, err := strconv.Atoi(bufferStr); err == nil && buffer > 0 {
			envConfig.AuditBufferSize = buffer
		}
	}

	if flushStr := os.Getenv("ARGUS_AUDIT_FLUSH_INTERVAL"); flushStr != "" {
		if duration, err := time.ParseDuration(flushStr); err == nil {
			envConfig.AuditFlushInterval = duration
		}
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
		config.Audit.Enabled = envConfig.AuditEnabled

		if envConfig.AuditOutputFile != "" {
			config.Audit.OutputFile = envConfig.AuditOutputFile
		}

		if envConfig.AuditMinLevel != "" {
			level, err := parseAuditLevel(envConfig.AuditMinLevel)
			if err != nil {
				return err
			}
			config.Audit.MinLevel = level
		}

		if envConfig.AuditBufferSize > 0 {
			config.Audit.BufferSize = envConfig.AuditBufferSize
		}

		if envConfig.AuditFlushInterval > 0 {
			config.Audit.FlushInterval = envConfig.AuditFlushInterval
		}
	}
	return nil
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
	// Only override non-zero values from environment
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

	// Merge audit configuration
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

	return nil
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
