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
	config, err := loadConfigFromEnvRaw()
	if err != nil {
		return nil, err
	}

	// Apply defaults for any unset values
	return config.WithDefaults(), nil
}

// loadConfigFromEnvRaw reads the environment into a Config WITHOUT applying
// defaults, so every field left at its zero value means "the environment did
// not set this".
//
// That distinction is what makes precedence work. LoadConfigFromEnv applies
// defaults because its caller wants a usable Config; LoadConfigMultiSource
// must not, because mergeConfigs decides "did the environment set this?" by
// testing for the zero value. Merging a defaulted Config made every default —
// PollInterval 5s, MaxWatchedFiles 100 — look like an explicit environment
// setting and silently overwrite the configuration file underneath.
func loadConfigFromEnvRaw() (*Config, error) {
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

	return config, nil
}

// LoadConfigMultiSource loads configuration with precedence:
// 1. Environment variables (highest priority)
// 2. Configuration file (medium priority) - supports JSON, YAML, TOML, HCL, INI, Properties
// 3. Default values (lowest priority)
//
// This provides complete multi-source configuration loading with:
//   - Universal format auto-detection from file extension
//   - Security validation for configuration file paths
//   - Graceful fallback when file doesn't exist or fails to parse
//   - Environment variable override of any file-based settings
//
// Example:
//
//	config, err := LoadConfigMultiSource("config.yaml")
//	// Loads config.yaml, overrides with ARGUS_* environment variables
//
// Parameters:
//   - configFile: Path to configuration file (optional, "" for env-only config)
//
// Returns:
//   - *Config: Merged configuration with precedence applied
//   - error: Any critical errors in environment variable parsing
func LoadConfigMultiSource(configFile string) (*Config, error) {
	// Start with file-based configuration
	config := &Config{}

	// Load from file if provided.
	//
	// A file that is absent is a legitimate configuration: the caller asked for
	// an optional file and the other two sources cover the settings. A file
	// that is present but malformed is not — reporting it is the whole point of
	// a configuration framework, and falling through to defaults in silence
	// started applications on settings nobody chose.
	if configFile != "" {
		if _, err := os.Stat(configFile); err == nil {
			fileConfig, err := loadConfigFromFile(configFile)
			if err != nil {
				return config.WithDefaults(), errors.Wrap(err, ErrCodeInvalidConfig,
					"failed to load configuration file")
			}
			config = fileConfig
		} else {
			// File doesn't exist, start with defaults
			config = config.WithDefaults()
		}
	} else {
		config = config.WithDefaults()
	}

	// Override with environment variables. The raw form is deliberate: only
	// fields the environment actually set are non-zero, so the merge below
	// cannot mistake a default for an override of the file.
	envConfig, err := loadConfigFromEnvRaw()
	if err != nil {
		return config, err // Return file config with error
	}

	// Apply environment overrides
	if err := mergeConfigs(config, envConfig); err != nil {
		return config, errors.Wrap(err, ErrCodeInvalidConfig, "failed to merge configurations")
	}

	// Re-apply defaults: the merge may have introduced values whose guard rails
	// (CacheTTL <= PollInterval, capacity a power of two) need re-checking.
	return config.WithDefaults(), nil
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

	// SECURITY: Only allow known valid optimization strategies. The allow-list
	// lives in parseOptimizationStrategy so the environment and the config file
	// can never drift apart on which names exist.
	if _, err := parseOptimizationStrategy(optimizationStr); err != nil {
		return err
	}

	envConfig.OptimizationStrategy = optimizationStr
	return nil
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

	// A malformed duration is an error here as it is everywhere else in this
	// file. Swallowing it left the operator with a silently ignored setting.
	if remoteStr := os.Getenv("ARGUS_REMOTE_INTERVAL"); remoteStr != "" {
		duration, err := time.ParseDuration(remoteStr)
		if err != nil {
			return errors.New(ErrCodeInvalidConfig, "invalid ARGUS_REMOTE_INTERVAL format")
		}
		envConfig.RemoteInterval = duration
	}

	if timeoutStr := os.Getenv("ARGUS_REMOTE_TIMEOUT"); timeoutStr != "" {
		duration, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return errors.New(ErrCodeInvalidConfig, "invalid ARGUS_REMOTE_TIMEOUT format")
		}
		envConfig.RemoteTimeout = duration
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
		strategy, err := parseOptimizationStrategy(envConfig.OptimizationStrategy)
		if err != nil {
			return err
		}
		config.OptimizationStrategy = strategy
	}
	if envConfig.BoreasLiteCapacity > 0 {
		config.BoreasLiteCapacity = envConfig.BoreasLiteCapacity
	}
	return nil
}

// convertAuditConfig converts audit configuration from EnvConfig to Config.
//
// Any audit variable being present is enough to convert. Gating on
// AuditEnabled alone meant that setting only ARGUS_AUDIT_MIN_LEVEL or
// ARGUS_AUDIT_BUFFER_SIZE was silently ignored, because ARGUS_AUDIT_ENABLED
// being unset leaves AuditEnabled at false.
func convertAuditConfig(envConfig *EnvConfig, config *Config) error {
	if envConfig.AuditEnabled ||
		envConfig.AuditOutputFile != "" ||
		envConfig.AuditMinLevel != "" ||
		envConfig.AuditBufferSize > 0 ||
		envConfig.AuditFlushInterval > 0 {
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

// loadConfigFromFile loads and parses a configuration file using the universal parser system.
// This provides the same functionality as the CLI's loadConfig but for LoadConfigMultiSource.
//
// Features:
//   - Universal format support: JSON, YAML, TOML, HCL, INI, Properties
//   - Automatic format detection from file extension (2.9ns)
//   - Security validation to prevent path traversal attacks
//   - Graceful error handling for malformed files
//
// Performance:
//   - 13 us/op, 2,752 B, 47 allocs for a small JSON file on a local disk
//   - Zero allocations for format detection; parsing allocates the map it returns
//   - Uses the same optimized parsers as the rest of Argus
//
// Parameters:
//   - configFile: Path to configuration file (validated for security)
//
// Returns:
//   - *Config: Parsed configuration with defaults applied
//   - error: Parsing or security validation errors
func loadConfigFromFile(configFile string) (*Config, error) {
	// SECURITY: Validate path to prevent directory traversal attacks
	if err := ValidateSecurePath(configFile); err != nil {
		return nil, errors.Wrap(err, ErrCodeInvalidConfig, "security validation failed")
	}

	// Auto-detect format from file extension
	format := DetectFormat(configFile)
	if format == FormatUnknown {
		return nil, errors.New(ErrCodeInvalidConfig, "unsupported configuration file format")
	}

	// Read file content
	// #nosec G304 -- Path is validated by ValidateSecurePath() above
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, errors.Wrap(err, ErrCodeFileNotFound, "failed to read configuration file")
	}

	// Parse using universal parser
	configMap, err := ParseConfig(data, format)
	if err != nil {
		return nil, errors.Wrap(err, ErrCodeInvalidConfig, "failed to parse configuration file")
	}

	config := &Config{}
	if err := bindConfigMap(configMap, config); err != nil {
		return nil, errors.Wrap(err, ErrCodeInvalidConfig, "invalid value in configuration file")
	}

	return config.WithDefaults(), nil
}

// bindConfigMap fills a Config from a parsed configuration map.
//
// Every key is optional; a key that is absent leaves the field at its zero
// value so WithDefaults and the environment layer can still fill it. Keys are
// resolved through lookupConfigValue, so both a flat "audit.enabled" (what the
// Properties and INI parsers emit) and a nested audit: { enabled: } (what
// JSON, YAML and TOML emit) reach the same field.
//
// The names mirror the ARGUS_* environment variables one for one, and the
// audit/remote sub-keys mirror the json tags already declared on AuditConfig
// and RemoteConfig, so the three sources describe the same settings with the
// same vocabulary.
func bindConfigMap(configMap map[string]interface{}, config *Config) error {
	if err := bindCoreConfigMap(configMap, config); err != nil {
		return err
	}
	if err := bindAuditConfigMap(configMap, config); err != nil {
		return err
	}
	return bindRemoteConfigMap(configMap, config)
}

// bindMapDuration assigns a duration-valued key when present.
func bindMapDuration(configMap map[string]interface{}, key string, target *time.Duration) error {
	value, ok := lookupConfigValue(configMap, key)
	if !ok {
		return nil
	}
	converted, err := convertToDuration(value)
	if err != nil {
		return errors.Wrap(err, ErrCodeInvalidConfig, "invalid duration for "+key)
	}
	*target = converted
	return nil
}

// bindMapInt assigns an int-valued key when present.
func bindMapInt(configMap map[string]interface{}, key string, target *int) error {
	value, ok := lookupConfigValue(configMap, key)
	if !ok {
		return nil
	}
	converted, err := convertToInt(value)
	if err != nil {
		return errors.Wrap(err, ErrCodeInvalidConfig, "invalid integer for "+key)
	}
	*target = converted
	return nil
}

// bindMapInt64 assigns an int64-valued key when present.
func bindMapInt64(configMap map[string]interface{}, key string, target *int64) error {
	value, ok := lookupConfigValue(configMap, key)
	if !ok {
		return nil
	}
	converted, err := convertToInt64(value)
	if err != nil {
		return errors.Wrap(err, ErrCodeInvalidConfig, "invalid integer for "+key)
	}
	*target = converted
	return nil
}

// bindMapBool assigns a bool-valued key when present.
func bindMapBool(configMap map[string]interface{}, key string, target *bool) error {
	value, ok := lookupConfigValue(configMap, key)
	if !ok {
		return nil
	}
	converted, err := convertToBool(value)
	if err != nil {
		return errors.Wrap(err, ErrCodeInvalidConfig, "invalid boolean for "+key)
	}
	*target = converted
	return nil
}

// bindMapString assigns a string-valued key when present.
func bindMapString(configMap map[string]interface{}, key string, target *string) {
	if value, ok := lookupConfigValue(configMap, key); ok {
		*target = convertToString(value)
	}
}

func bindCoreConfigMap(configMap map[string]interface{}, config *Config) error {
	if err := bindMapDuration(configMap, "poll_interval", &config.PollInterval); err != nil {
		return err
	}
	if err := bindMapDuration(configMap, "cache_ttl", &config.CacheTTL); err != nil {
		return err
	}
	if err := bindMapInt(configMap, "max_watched_files", &config.MaxWatchedFiles); err != nil {
		return err
	}
	if err := bindMapInt64(configMap, "boreas_capacity", &config.BoreasLiteCapacity); err != nil {
		return err
	}
	if err := bindMapBool(configMap, "disable_audit", &config.DisableAudit); err != nil {
		return err
	}

	if value, ok := lookupConfigValue(configMap, "optimization_strategy"); ok {
		strategy, err := parseOptimizationStrategy(convertToString(value))
		if err != nil {
			return err
		}
		config.OptimizationStrategy = strategy
	}

	return nil
}

func bindAuditConfigMap(configMap map[string]interface{}, config *Config) error {
	if err := bindMapBool(configMap, "audit.enabled", &config.Audit.Enabled); err != nil {
		return err
	}
	bindMapString(configMap, "audit.output_file", &config.Audit.OutputFile)
	if err := bindMapInt(configMap, "audit.buffer_size", &config.Audit.BufferSize); err != nil {
		return err
	}
	if err := bindMapDuration(configMap, "audit.flush_interval", &config.Audit.FlushInterval); err != nil {
		return err
	}
	if err := bindMapBool(configMap, "audit.include_stack", &config.Audit.IncludeStack); err != nil {
		return err
	}

	if value, ok := lookupConfigValue(configMap, "audit.min_level"); ok {
		level, err := parseAuditLevel(convertToString(value))
		if err != nil {
			return err
		}
		config.Audit.MinLevel = level
	}

	return nil
}

func bindRemoteConfigMap(configMap map[string]interface{}, config *Config) error {
	if err := bindMapBool(configMap, "remote.enabled", &config.Remote.Enabled); err != nil {
		return err
	}
	bindMapString(configMap, "remote.primary_url", &config.Remote.PrimaryURL)
	bindMapString(configMap, "remote.fallback_url", &config.Remote.FallbackURL)
	bindMapString(configMap, "remote.fallback_path", &config.Remote.FallbackPath)
	if err := bindMapDuration(configMap, "remote.sync_interval", &config.Remote.SyncInterval); err != nil {
		return err
	}
	if err := bindMapDuration(configMap, "remote.timeout", &config.Remote.Timeout); err != nil {
		return err
	}
	if err := bindMapInt(configMap, "remote.max_retries", &config.Remote.MaxRetries); err != nil {
		return err
	}
	return bindMapDuration(configMap, "remote.retry_delay", &config.Remote.RetryDelay)
}

// parseOptimizationStrategy maps a strategy name to its constant. It is the
// single place that spelling is decided, shared by the environment loader and
// the configuration-file loader so a name accepted by one is accepted by both.
func parseOptimizationStrategy(name string) (OptimizationStrategy, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "auto":
		return OptimizationAuto, nil
	case "single", "singleevent":
		return OptimizationSingleEvent, nil
	case "small", "smallbatch":
		return OptimizationSmallBatch, nil
	case "large", "largebatch":
		return OptimizationLargeBatch, nil
	case "light":
		return OptimizationLight, nil
	default:
		return OptimizationAuto, errors.New(ErrCodeInvalidConfig, "invalid optimization strategy: "+name)
	}
}

// validateSecureAuditPath validates audit file paths using the same security checks as file watching.
// This prevents path traversal attacks via audit configuration environment variables.
func validateSecureAuditPath(path string) error {
	// Reuse the comprehensive path validation from the main security function
	return ValidateSecurePath(path)
}
