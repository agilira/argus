// config.go: Configuration management for Argus Dynamic Configuration Framework
//
// Copyright (c) 2025 AGILira
// Series: AGILira System Libraries
// SPDX-License-Identifier: MPL-2.0

package argus

import "time"

// IdleStrategy defines how the watcher should behave when no file changes
// are detected. This allows for power management and CPU optimization.
type IdleStrategy interface {
	// Wait is called between polling cycles when no changes are detected
	Wait()

	// Reset is called when file changes are detected to reset any backoff
	Reset()
}

// SleepStrategy implements IdleStrategy using simple sleep
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

// WithDefaults applies sensible defaults to the configuration
func (c *Config) WithDefaults() *Config {
	config := *c

	if config.PollInterval <= 0 {
		config.PollInterval = 5 * time.Second
	}

	if config.CacheTTL <= 0 {
		config.CacheTTL = config.PollInterval / 2
	}

	// GUARD RAIL: Ensure CacheTTL <= PollInterval for effectiveness
	if config.CacheTTL > config.PollInterval {
		config.CacheTTL = config.PollInterval / 2
	}

	if config.MaxWatchedFiles <= 0 {
		config.MaxWatchedFiles = 100
	}

	// Set audit defaults if not configured
	if config.Audit == (AuditConfig{}) {
		config.Audit = DefaultAuditConfig()
	}

	// Set BoreasLite optimization defaults
	if config.OptimizationStrategy == OptimizationAuto {
		// Auto-strategy remains, will be determined at runtime based on file count
		config.OptimizationStrategy = OptimizationAuto
	}

	// Set BoreasLite capacity based on strategy if not explicitly set
	if config.BoreasLiteCapacity <= 0 {
		switch config.OptimizationStrategy {
		case OptimizationSingleEvent:
			config.BoreasLiteCapacity = 64 // Minimal for 1-2 files
		case OptimizationSmallBatch:
			config.BoreasLiteCapacity = 128 // Balanced for 3-20 files
		case OptimizationLargeBatch:
			config.BoreasLiteCapacity = 256 // High throughput for 20+ files
		default: // OptimizationAuto
			config.BoreasLiteCapacity = 128 // Safe default, will adjust at runtime
		}
	}

	// Ensure capacity is power of 2
	if config.BoreasLiteCapacity > 0 && (config.BoreasLiteCapacity&(config.BoreasLiteCapacity-1)) != 0 {
		// Find next power of 2
		capacity := int64(1)
		for capacity < config.BoreasLiteCapacity {
			capacity <<= 1
		}
		config.BoreasLiteCapacity = capacity
	}

	return &config
}
