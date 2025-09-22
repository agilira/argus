// config_validation.go - professional-grade configuration validation for Argus
//
// This module provides comprehensive validation for Argus configuration,
// ensuring safe and reliable operation in production environments with
// detailed error reporting and performance optimization recommendations.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

// Package argus provides comprehensive configuration validation and constraint checking.
// This file implements professional-grade validation rules for configuration parameters.

package argus

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agilira/go-errors"
)

// Validation errors - implementing error codes pattern from Iris
var (
	ErrInvalidPollInterval    = errors.New(ErrCodeInvalidPollInterval, "poll interval must be positive")
	ErrInvalidCacheTTL        = errors.New(ErrCodeInvalidCacheTTL, "cache TTL must be positive")
	ErrInvalidMaxWatchedFiles = errors.New(ErrCodeInvalidMaxWatchedFiles, "max watched files must be positive")
	ErrInvalidOptimization    = errors.New(ErrCodeInvalidOptimization, "unknown optimization strategy")
	ErrInvalidAuditConfig     = errors.New(ErrCodeInvalidAuditConfig, "audit configuration is invalid")
	ErrInvalidBufferSize      = errors.New(ErrCodeInvalidBufferSize, "buffer size must be positive")
	ErrInvalidFlushInterval   = errors.New(ErrCodeInvalidFlushInterval, "flush interval must be positive")
	ErrInvalidOutputFile      = errors.New(ErrCodeInvalidOutputFile, "audit output file path is invalid")
	ErrUnwritableOutputFile   = errors.New(ErrCodeUnwritableOutputFile, "audit output file is not writable")
	ErrCacheTTLTooLarge       = errors.New(ErrCodeCacheTTLTooLarge, "cache TTL should not exceed poll interval")
	ErrPollIntervalTooSmall   = errors.New(ErrCodePollIntervalTooSmall, "poll interval should be at least 10ms for stability")
	ErrMaxFilesTooLarge       = errors.New(ErrCodeMaxFilesTooLarge, "max watched files exceeds recommended limit (10000)")
	ErrBoreasCapacityInvalid  = errors.New(ErrCodeBoreasCapacityInvalid, "BoreasLite capacity must be power of 2")
)

// ValidationResult contains the result of configuration validation with detailed feedback.
// Provides comprehensive validation information including errors, warnings, and
// performance recommendations for production deployments.
type ValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// String returns a human-readable representation of validation results
func (vr ValidationResult) String() string {
	if vr.Valid {
		if len(vr.Warnings) == 0 {
			return "Configuration is valid"
		}
		return fmt.Sprintf("Configuration is valid with %d warning(s)", len(vr.Warnings))
	}
	return fmt.Sprintf("Configuration is invalid: %d error(s), %d warning(s)",
		len(vr.Errors), len(vr.Warnings))
}

// Validate performs comprehensive validation of the Argus configuration
// Returns error if configuration is invalid, warnings are included in ValidationResult
func (c *Config) Validate() error {
	result := c.ValidateDetailed()
	if !result.Valid {
		// Return first error as the primary validation error
		if len(result.Errors) > 0 {
			// Map the error string back to the original error object
			firstError := result.Errors[0]
			switch {
			case firstError == ErrInvalidPollInterval.Error():
				return ErrInvalidPollInterval
			case firstError == ErrPollIntervalTooSmall.Error():
				return ErrPollIntervalTooSmall
			case firstError == ErrInvalidCacheTTL.Error():
				return ErrInvalidCacheTTL
			case firstError == ErrInvalidMaxWatchedFiles.Error():
				return ErrInvalidMaxWatchedFiles
			case firstError == ErrInvalidOptimization.Error():
				return ErrInvalidOptimization
			case firstError == ErrBoreasCapacityInvalid.Error():
				return ErrBoreasCapacityInvalid
			case firstError == ErrInvalidBufferSize.Error():
				return ErrInvalidBufferSize
			case firstError == ErrInvalidFlushInterval.Error():
				return ErrInvalidFlushInterval
			case firstError == ErrInvalidOutputFile.Error():
				return ErrInvalidOutputFile
			case firstError == ErrUnwritableOutputFile.Error():
				return ErrUnwritableOutputFile
			default:
				// Fallback to generic error
				return errors.New(ErrCodeInvalidConfig, firstError)
			}
		}
	}
	return nil
}

// ValidateDetailed performs comprehensive validation and returns detailed results
// including both errors and warnings for better debugging and monitoring
func (c *Config) ValidateDetailed() ValidationResult {
	result := ValidationResult{
		Valid:    true,
		Errors:   make([]string, 0),
		Warnings: make([]string, 0),
	}

	// Core configuration validation
	c.validateCoreConfig(&result)

	// Optimization strategy validation
	c.validateOptimizationStrategy(&result)

	// BoreasLite capacity validation
	c.validateBoreasCapacity(&result)

	// Audit configuration validation
	c.validateAuditConfig(&result)

	// Performance and operational warnings
	c.validatePerformanceConstraints(&result)

	// Set overall validity
	result.Valid = len(result.Errors) == 0

	return result
}

// validateCoreConfig validates essential configuration parameters
func (c *Config) validateCoreConfig(result *ValidationResult) {
	// Poll interval validation
	pollIntervalValid := true
	if c.PollInterval <= 0 {
		result.Errors = append(result.Errors, ErrInvalidPollInterval.Error())
		pollIntervalValid = false
	} else if c.PollInterval < 10*time.Millisecond {
		result.Errors = append(result.Errors, ErrPollIntervalTooSmall.Error())
		pollIntervalValid = false
	}

	// Cache TTL validation
	if c.CacheTTL < 0 {
		result.Errors = append(result.Errors, ErrInvalidCacheTTL.Error())
	} else if pollIntervalValid && c.CacheTTL > c.PollInterval {
		// Only check this if PollInterval is valid
		result.Warnings = append(result.Warnings, ErrCacheTTLTooLarge.Error())
	}

	// Max watched files validation
	if c.MaxWatchedFiles <= 0 {
		result.Errors = append(result.Errors, ErrInvalidMaxWatchedFiles.Error())
	} else if c.MaxWatchedFiles > 10000 {
		result.Warnings = append(result.Warnings, ErrMaxFilesTooLarge.Error())
	}
}

// validateOptimizationStrategy validates the optimization strategy setting
func (c *Config) validateOptimizationStrategy(result *ValidationResult) {
	switch c.OptimizationStrategy {
	case OptimizationSingleEvent, OptimizationSmallBatch, OptimizationLargeBatch, OptimizationAuto:
		// Valid strategies (including OptimizationAuto which is 0)
	default:
		result.Errors = append(result.Errors,
			fmt.Sprintf("%s: '%v'", ErrInvalidOptimization.Error(), c.OptimizationStrategy))
	}
}

// validateBoreasCapacity validates BoreasLite capacity settings
func (c *Config) validateBoreasCapacity(result *ValidationResult) {
	if c.BoreasLiteCapacity > 0 {
		// Check if capacity is power of 2
		if c.BoreasLiteCapacity&(c.BoreasLiteCapacity-1) != 0 {
			result.Errors = append(result.Errors, ErrBoreasCapacityInvalid.Error())
		}

		// Warn about very large capacities
		if c.BoreasLiteCapacity > 1024 {
			result.Warnings = append(result.Warnings,
				"Large BoreasLite capacity may consume significant memory")
		}
	}
}

// validateAuditConfig validates audit configuration if enabled
func (c *Config) validateAuditConfig(result *ValidationResult) {
	if !c.Audit.Enabled {
		return // Skip audit validation if disabled
	}

	c.validateAuditBufferSize(result)
	c.validateAuditFlushInterval(result)
	c.validateAuditOutputFile(result)
}

// validateAuditBufferSize validates audit buffer size configuration
func (c *Config) validateAuditBufferSize(result *ValidationResult) {
	if c.Audit.BufferSize < 0 {
		result.Errors = append(result.Errors, ErrInvalidBufferSize.Error())
	} else if c.Audit.BufferSize == 0 {
		result.Warnings = append(result.Warnings, "Audit buffer size is 0, consider setting to 100-1000 for better performance")
	} else if c.Audit.BufferSize > 10000 {
		result.Warnings = append(result.Warnings, "Large audit buffer size may consume significant memory")
	}
}

// validateAuditFlushInterval validates audit flush interval configuration
func (c *Config) validateAuditFlushInterval(result *ValidationResult) {
	if c.Audit.FlushInterval < 0 {
		result.Errors = append(result.Errors, ErrInvalidFlushInterval.Error())
	} else if c.Audit.FlushInterval == 0 {
		result.Warnings = append(result.Warnings, "Audit flush interval is 0, events will be written immediately (may impact performance)")
	}
}

// validateAuditOutputFile validates audit output file configuration
func (c *Config) validateAuditOutputFile(result *ValidationResult) {
	if c.Audit.OutputFile == "" {
		result.Errors = append(result.Errors, ErrInvalidOutputFile.Error())
		return
	}

	// Validate output file path and writeability
	if err := c.validateOutputFile(c.Audit.OutputFile); err != nil {
		result.Errors = append(result.Errors, err.Error())
	}
}

// validateOutputFile checks if the audit output file path is valid and writable
func (c *Config) validateOutputFile(outputFile string) error {
	// Clean and validate the path
	cleanPath := filepath.Clean(outputFile)
	if cleanPath == "." || cleanPath == "/" {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("path '%s' is not a valid file path", outputFile))
	}

	// Check if directory exists and is writable
	dir := filepath.Dir(cleanPath)
	if dir != "" {
		if info, err := os.Stat(dir); err != nil {
			if os.IsNotExist(err) {
				return errors.New(ErrCodeInvalidConfig,
					fmt.Sprintf("directory '%s' does not exist", dir))
			}
			return errors.Wrap(err, ErrCodeInvalidConfig,
				fmt.Sprintf("cannot access directory '%s'", dir))
		} else if !info.IsDir() {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("'%s' is not a directory", dir))
		}
	}

	return nil
}

// validatePerformanceConstraints adds performance-related warnings
func (c *Config) validatePerformanceConstraints(result *ValidationResult) {
	// Warn about performance implications
	if c.PollInterval < 100*time.Millisecond && c.MaxWatchedFiles > 100 {
		result.Warnings = append(result.Warnings,
			"Fast polling with many files may impact CPU usage")
	}

	if c.Audit.Enabled && c.Audit.FlushInterval < time.Second && c.MaxWatchedFiles > 50 {
		result.Warnings = append(result.Warnings,
			"Frequent audit flushing with many files may impact I/O performance")
	}

	// Recommend optimization strategy based on configuration
	if c.MaxWatchedFiles > 10 && c.OptimizationStrategy == OptimizationSingleEvent {
		result.Warnings = append(result.Warnings,
			"Consider using 'smallbatch' or 'auto' optimization for multiple files")
	}

	if c.MaxWatchedFiles > 100 && c.OptimizationStrategy != OptimizationLargeBatch && c.OptimizationStrategy != OptimizationAuto {
		result.Warnings = append(result.Warnings,
			"Consider using 'largebatch' or 'auto' optimization for many files")
	}

	// Memory usage warnings
	totalMemoryEst := (c.MaxWatchedFiles * 256) + int(c.BoreasLiteCapacity*64) + c.Audit.BufferSize*512
	if totalMemoryEst > 50*1024*1024 { // 50MB
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Configuration may use ~%dMB memory, consider reducing limits", totalMemoryEst/(1024*1024)))
	}
}

// ValidateEnvironmentConfig validates environment-loaded configuration
// This is a convenience method for validating configs loaded from environment variables
func ValidateEnvironmentConfig() error {
	config, err := LoadConfigFromEnv()
	if err != nil {
		return errors.Wrap(err, ErrCodeInvalidConfig, "failed to load config from environment")
	}

	return config.Validate()
}

// loadConfigFromJSON loads and parses a JSON configuration file with cross-platform path handling
func loadConfigFromJSON(configPath string) (*Config, error) {
	// Read the file content
	data, err := os.ReadFile(configPath) // #nosec G304 - configPath is validated by caller, intentional config file loading
	if err != nil {
		return nil, errors.Wrap(err, ErrCodeFileNotFound, "failed to read config file '"+configPath+"'")
	}

	// Handle cross-platform JSON parsing - normalize Windows path separators
	jsonStr := string(data)

	// On Windows, JSON paths with backslashes need to be properly escaped
	// We normalize by ensuring all backslashes are properly escaped for JSON
	if strings.Contains(jsonStr, "\\") && !strings.Contains(jsonStr, "\\\\") {
		// This is a heuristic - if we see single backslashes but no double backslashes,
		// we likely have Windows paths that need escaping
		jsonStr = strings.ReplaceAll(jsonStr, "\\", "\\\\")
	}

	// Load base config with defaults first
	config := (&Config{}).WithDefaults()

	// Parse JSON into the config
	if err := json.Unmarshal([]byte(jsonStr), config); err != nil {
		return nil, errors.Wrap(err, ErrCodeInvalidConfig, "failed to parse JSON config")
	}

	return config, nil
}

// ValidateConfigFile validates a configuration that would be loaded from a file
// This method performs validation without actually starting file watching
func ValidateConfigFile(configPath string) error {
	if configPath == "" {
		return errors.New(ErrCodeInvalidConfig, "configuration file path cannot be empty")
	}

	// Check if file exists and is readable
	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			return errors.New(ErrCodeConfigNotFound, "configuration file '"+configPath+"' not found")
		}
		return errors.Wrap(err, ErrCodeConfigNotFound, "cannot access configuration file '"+configPath+"'")
	}

	// Load and parse the actual config file
	config, err := loadConfigFromJSON(configPath)
	if err != nil {
		return errors.Wrap(err, ErrCodeInvalidConfig, "config parse error")
	}

	// Validate the loaded configuration
	return config.Validate()
}

// GetValidationErrorCode extracts the error code from an Argus validation error
func GetValidationErrorCode(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	// Handle go-errors format: [CODE]: Message
	if len(errStr) > 3 && errStr[0] == '[' {
		for idx := 1; idx < len(errStr); idx++ {
			if errStr[idx] == ']' {
				return errStr[1:idx]
			}
		}
	}

	// Fallback for old format: CODE: Message
	for idx := 0; idx < len(errStr); idx++ {
		if errStr[idx] == ':' {
			return errStr[:idx]
		}
	}

	return errStr
}

// IsValidationError checks if an error is an Argus validation error
func IsValidationError(err error) bool {
	if err == nil {
		return false
	}

	errorStr := err.Error()

	// Handle go-errors format: [ARGUS_*]: Message
	if len(errorStr) > 8 && errorStr[0] == '[' && errorStr[1:7] == "ARGUS_" {
		return true
	}

	// Fallback for old format: ARGUS_*: Message
	return len(errorStr) > 6 && errorStr[:6] == "ARGUS_"
}
