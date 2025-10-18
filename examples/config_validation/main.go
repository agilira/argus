// Package main demonstrates advanced configuration validation using Argus.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/agilira/argus"
)

func main() {
	fmt.Println("=== Argus Configuration Validation Demo ===")
	fmt.Println("Demonstrates comprehensive configuration validation capabilities")
	fmt.Println("Features unified SQLite audit backend for cross-app correlation")
	fmt.Println()

	// Create temporary directory for examples
	tempDir, err := os.MkdirTemp("", "argus_validation_demo")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Example 1: Valid Configuration
	fmt.Println("1. Testing VALID configuration:")
	validConfig := &argus.Config{
		PollInterval:         2 * time.Second,
		CacheTTL:             1 * time.Second,
		MaxWatchedFiles:      50,
		OptimizationStrategy: argus.OptimizationAuto,
		BoreasLiteCapacity:   256, // Power of 2
		Audit: argus.AuditConfig{
			Enabled:       true,
			BufferSize:    1000,
			FlushInterval: 5 * time.Second,
			OutputFile:    filepath.Join(tempDir, "audit.log"),
		},
	}

	result := validConfig.ValidateDetailed()
	fmt.Printf("   Valid: %t\n", result.Valid)
	fmt.Printf("   Errors: %d\n", len(result.Errors))
	fmt.Printf("   Warnings: %d\n", len(result.Warnings))
	if len(result.Warnings) > 0 {
		for _, warning := range result.Warnings {
			fmt.Printf("   ⚠️  %s\n", warning)
		}
	}
	fmt.Println()

	// Example 2: Invalid Configuration
	fmt.Println("2. Testing INVALID configuration:")
	invalidConfig := &argus.Config{
		PollInterval:         -1 * time.Second,                // INVALID: negative
		CacheTTL:             5 * time.Second,                 // WARNING: larger than poll interval
		MaxWatchedFiles:      0,                               // INVALID: zero
		OptimizationStrategy: argus.OptimizationStrategy(999), // INVALID: unknown strategy
		BoreasLiteCapacity:   15,                              // INVALID: not power of 2
		Audit: argus.AuditConfig{
			Enabled:       true,
			BufferSize:    -1,                       // INVALID: negative
			FlushInterval: -2 * time.Second,         // INVALID: negative
			OutputFile:    "invalid/path/audit.log", // INVALID: invalid path
		},
	}

	result = invalidConfig.ValidateDetailed()
	fmt.Printf("   Valid: %t\n", result.Valid)
	fmt.Printf("   Errors: %d\n", len(result.Errors))
	fmt.Printf("   Warnings: %d\n", len(result.Warnings))

	if len(result.Errors) > 0 {
		fmt.Println("   🚫 Errors found:")
		for i, err := range result.Errors {
			fmt.Printf("      %d. %s\n", i+1, err)
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Println("   ⚠️  Warnings found:")
		for i, warning := range result.Warnings {
			fmt.Printf("      %d. %s\n", i+1, warning)
		}
	}
	fmt.Println()

	// Example 3: Configuration with Performance Warnings
	fmt.Println("3. Testing configuration with PERFORMANCE warnings:")
	performanceConfig := &argus.Config{
		PollInterval:         50 * time.Millisecond,         // Fast polling
		CacheTTL:             25 * time.Millisecond,         // Half of poll interval
		MaxWatchedFiles:      500,                           // Many files
		OptimizationStrategy: argus.OptimizationSingleEvent, // Suboptimal for many files
		BoreasLiteCapacity:   4096,                          // Large capacity
		Audit: argus.AuditConfig{
			Enabled:       true,
			BufferSize:    20000,                  // Large buffer
			FlushInterval: 100 * time.Millisecond, // Frequent flushing
			OutputFile:    filepath.Join(tempDir, "audit.log"),
		},
	}

	result = performanceConfig.ValidateDetailed()
	fmt.Printf("   Valid: %t\n", result.Valid)
	fmt.Printf("   Errors: %d\n", len(result.Errors))
	fmt.Printf("   Warnings: %d\n", len(result.Warnings))

	if len(result.Warnings) > 0 {
		fmt.Println("   ⚠️  Performance warnings:")
		for i, warning := range result.Warnings {
			fmt.Printf("      %d. %s\n", i+1, warning)
		}
	}
	fmt.Println()

	// Example 4: Environment Validation
	fmt.Println("4. Testing ENVIRONMENT configuration validation:")

	// Set some environment variables
	_ = os.Setenv("ARGUS_POLL_INTERVAL", "3s")                                        // #nosec G104 -- demo env var, error not critical
	_ = os.Setenv("ARGUS_CACHE_TTL", "1s")                                            // #nosec G104 -- demo env var, error not critical
	_ = os.Setenv("ARGUS_MAX_WATCHED_FILES", "200")                                   // #nosec G104 -- demo env var, error not critical
	_ = os.Setenv("ARGUS_AUDIT_OUTPUT_FILE", filepath.Join(tempDir, "env_audit.log")) // #nosec G104 -- demo env var, error not critical
	_ = os.Setenv("ARGUS_AUDIT_ENABLED", "true")                                      // #nosec G104 -- demo env var, error not critical
	defer func() {
		os.Unsetenv("ARGUS_POLL_INTERVAL")
		os.Unsetenv("ARGUS_CACHE_TTL")
		os.Unsetenv("ARGUS_MAX_WATCHED_FILES")
		os.Unsetenv("ARGUS_AUDIT_OUTPUT_FILE")
		os.Unsetenv("ARGUS_AUDIT_ENABLED")
	}()

	err = argus.ValidateEnvironmentConfig()
	if err != nil {
		fmt.Printf("   ❌ Environment validation failed: %v\n", err)
	} else {
		fmt.Printf("   ✅ Environment configuration is valid\n")
	}
	fmt.Println()

	// Example 5: Quick validation using Validate() method
	fmt.Println("5. Quick validation (errors only):")

	quickConfig := &argus.Config{
		PollInterval:    -5 * time.Second, // Invalid
		CacheTTL:        1 * time.Second,
		MaxWatchedFiles: 100,
		Audit: argus.AuditConfig{
			Enabled: false, // Disable audit to focus on poll interval error
		},
	}

	err = quickConfig.Validate()
	if err != nil {
		fmt.Printf("   ❌ Validation error: %v\n", err)
	} else {
		fmt.Printf("   ✅ Configuration is valid\n")
	}

	fmt.Println("\n=== Configuration Validation Complete ===")
	fmt.Println("Argus provides comprehensive configuration validation:")
	fmt.Println("   • Detailed error reporting with specific error codes")
	fmt.Println("   • Performance warnings and recommendations")
	fmt.Println("   • File system path validation")
	fmt.Println("   • Memory usage warnings")
	fmt.Println("   • Optimization strategy recommendations")
	fmt.Println("   • Environment variable validation")
	fmt.Println("   • Audit configuration validation")
}
