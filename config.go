// config.go: Configuration management for Argus Dynamic Configuration Framework
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import "time"

// IdleStrategy defines how the watcher should behave when no file changes
// are detected. This allows for power management and CPU optimization.
// Currently serves as a design interface for future power management features.
type IdleStrategy interface {
	// Wait is called between polling cycles when no changes are detected
	Wait()

	// Reset is called when file changes are detected to reset any backoff
	Reset()
}

// SleepStrategy implements IdleStrategy using simple sleep-based waiting.
// This is the default strategy that relies on polling intervals for timing.
type SleepStrategy struct{}

// NewSleepStrategy creates a new sleep-based idle strategy
func NewSleepStrategy() *SleepStrategy {
	return &SleepStrategy{}
}

// Wait implements IdleStrategy by doing nothing (polling interval handles timing)
func (s *SleepStrategy) Wait() {
	// No additional waiting - rely on polling interval
	_ = s // Prevent unused receiver warning
}

// Reset implements IdleStrategy by doing nothing
func (s *SleepStrategy) Reset() {
	// No state to reset
	_ = s // Prevent unused receiver warning
}

// WithDefaults applies sensible defaults to the configuration and validates settings.
// Returns a new Config instance with all required fields populated.
// Ensures proper relationships between settings (e.g., CacheTTL <= PollInterval).
//
// Default values:
//   - PollInterval: 5 seconds
//   - CacheTTL: PollInterval / 2
//   - MaxWatchedFiles: 100
//   - BoreasLiteCapacity: Strategy-dependent (64-256)
//   - Audit: Enabled with secure defaults
func (c *Config) WithDefaults() *Config {
	config := *c

	config.setTimingDefaults()
	config.setFileDefaults()
	config.setAuditDefaults()
	config.setBoreasLiteDefaults()
	config.setRemoteConfigDefaults()

	return &config
}

// setTimingDefaults sets default values for timing-related configuration
func (c *Config) setTimingDefaults() {
	if c.PollInterval <= 0 {
		c.PollInterval = 5 * time.Second
	}

	if c.CacheTTL <= 0 {
		c.CacheTTL = c.PollInterval / 2
	}

	// GUARD RAIL: Ensure CacheTTL <= PollInterval for effectiveness
	if c.CacheTTL > c.PollInterval {
		c.CacheTTL = c.PollInterval / 2
	}
}

// setFileDefaults sets default values for file watching configuration
func (c *Config) setFileDefaults() {
	if c.MaxWatchedFiles <= 0 {
		c.MaxWatchedFiles = 100
	}
}

// setAuditDefaults sets default audit configuration
func (c *Config) setAuditDefaults() {
	if c.Audit == (AuditConfig{}) {
		c.Audit = DefaultAuditConfig()
	}
}

// setBoreasLiteDefaults sets default BoreasLite optimization configuration
func (c *Config) setBoreasLiteDefaults() {
	// Set BoreasLite optimization defaults
	if c.OptimizationStrategy == OptimizationAuto {
		// Auto-strategy remains, will be determined at runtime based on file count
		c.OptimizationStrategy = OptimizationAuto
	}

	// Set BoreasLite capacity based on strategy if not explicitly set
	if c.BoreasLiteCapacity <= 0 {
		c.BoreasLiteCapacity = c.getDefaultCapacityByStrategy()
	}

	// Ensure capacity is power of 2
	c.BoreasLiteCapacity = c.nextPowerOfTwo(c.BoreasLiteCapacity)
}

// getDefaultCapacityByStrategy returns the default capacity for the optimization strategy
func (c *Config) getDefaultCapacityByStrategy() int64 {
	switch c.OptimizationStrategy {
	case OptimizationSingleEvent:
		return 64 // Minimal for 1-2 files
	case OptimizationSmallBatch:
		return 128 // Balanced for 3-20 files
	case OptimizationLargeBatch:
		return 256 // High throughput for 20+ files
	default: // OptimizationAuto
		return 128 // Safe default, will adjust at runtime
	}
}

// nextPowerOfTwo ensures capacity is a power of 2
func (c *Config) nextPowerOfTwo(capacity int64) int64 {
	if capacity > 0 && (capacity&(capacity-1)) != 0 {
		// Find next power of 2
		result := int64(1)
		for result < capacity {
			result <<= 1
		}
		return result
	}
	return capacity
}

// setRemoteConfigDefaults sets default values for remote configuration
func (c *Config) setRemoteConfigDefaults() {
	// Remote config is disabled by default for backward compatibility
	if !c.Remote.Enabled {
		return
	}

	// Set timing defaults for remote operations
	if c.Remote.SyncInterval <= 0 {
		c.Remote.SyncInterval = 30 * time.Second // Balanced default
	}

	if c.Remote.Timeout <= 0 {
		c.Remote.Timeout = 10 * time.Second // Allow for network latency
	}

	if c.Remote.MaxRetries < 0 {
		c.Remote.MaxRetries = 2 // Total 3 attempts (initial + 2 retries)
	}

	if c.Remote.RetryDelay <= 0 {
		c.Remote.RetryDelay = 1 * time.Second // Exponential backoff base
	}

	// Validation: Timeout should allow for retries
	// Safe calculation to prevent integer overflow
	var maxRetryTime time.Duration
	if c.Remote.MaxRetries > 30 {
		// Cap exponential growth to prevent overflow
		maxRetryTime = c.Remote.RetryDelay * time.Duration(1<<30)
	} else {
		maxRetryTime = c.Remote.RetryDelay * time.Duration(1<<c.Remote.MaxRetries) // 2^MaxRetries
	}
	if c.Remote.Timeout <= maxRetryTime {
		// Adjust timeout to accommodate retry attempts
		c.Remote.Timeout = maxRetryTime + (5 * time.Second) // Extra buffer
	}

	// Validation: SyncInterval should be longer than timeout to prevent overlap
	if c.Remote.SyncInterval <= c.Remote.Timeout {
		c.Remote.SyncInterval = c.Remote.Timeout + (10 * time.Second) // Prevent overlap
	}
}
