// Error Handling Example
//
// This example demonstrates comprehensive error handling strategies with Argus
// using the go-errors library (https://github.com/agilira/go-errors) for
// structured error handling with Argus error codes.
//
// Features:
// - Custom error handlers with go-errors integration
// - Argus error code usage (ARGUS_INVALID_CONFIG, ARGUS_FILE_NOT_FOUND, etc.)
// - Error wrapping and cause tracking
// - Performance-optimized error handling
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
	"strings"
	"time"

	"github.com/agilira/argus"
	"github.com/agilira/go-errors"
)

func main() {
	fmt.Println("🚨 Argus Error Handling Demo")
	fmt.Println("============================")

	// Create temporary directory for testing
	tempDir := "/tmp/argus_error_demo"
	if err := os.MkdirAll(tempDir, 0750); err != nil { // Changed to 0750 for gosec
		log.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			log.Printf("Failed to cleanup temp directory: %v", err)
		}
	}()

	// Test 1: Custom Error Handler
	fmt.Println("\n1️⃣  Testing Custom Error Handler:")
	testCustomErrorHandler(tempDir)

	// Test 2: File Not Found Error
	fmt.Println("\n2️⃣  Testing File Not Found:")
	testFileNotFound(tempDir)

	// Test 3: Parse Error Handling
	fmt.Println("\n3️⃣  Testing Parse Error Handling:")
	testParseError(tempDir)

	// Test 4: Default Error Handler
	fmt.Println("\n4️⃣  Testing Default Error Handler:")
	testDefaultErrorHandler(tempDir)

	// Test 5: Custom Error Creation
	testCustomErrorCreation()

	fmt.Println("\n✅ Error handling demo completed!")
	fmt.Println("\n💡 Key Features Demonstrated:")
	fmt.Println("   • Structured error handling with go-errors")
	fmt.Println("   • Error code checking and identification")
	fmt.Println("   • Error wrapping and cause tracking")
	fmt.Println("   • Custom error creation")
	fmt.Println("   • Integration with Argus error handling")
}

func testCustomErrorHandler(tempDir string) {
	configFile := filepath.Join(tempDir, "custom_error_config.json")

	// Create a valid config first
	validConfig := `{"service": "test", "port": 8080}`
	if err := os.WriteFile(configFile, []byte(validConfig), 0600); err != nil {
		log.Fatalf("Failed to write config file: %v", err)
	}

	// Create custom error handler that demonstrates go-errors usage
	errorHandler := func(err error, filepath string) {
		fmt.Printf("   🔥 Custom Error Handler: File %s had error: %v\n", filepath, err)

		// Demonstrate go-errors structured error handling
		// go-errors implements the standard error interface
		errorMsg := err.Error()
		fmt.Printf("      📝 Error Message: %s\n", errorMsg)

		// Check for specific error codes in the error message
		if strings.Contains(errorMsg, "ARGUS_INVALID_CONFIG") {
			fmt.Printf("      ✅ Identified as invalid config error\n")
		}
	}

	// Create watcher with custom error handler using the utility function
	config := argus.Config{
		PollInterval: 50 * time.Millisecond,
		ErrorHandler: errorHandler,
	}

	watcher, err := argus.UniversalConfigWatcherWithConfig(configFile, func(config map[string]interface{}) {
		fmt.Printf("   📦 Config received: %v\n", config)
	}, config)

	if err != nil {
		log.Printf("Failed to create watcher: %v", err)
		return
	}
	defer watcher.Stop()

	// Give it time to read initial config
	time.Sleep(100 * time.Millisecond)

	// Now write invalid JSON to trigger error
	invalidConfig := `{"service": "test", "port": INVALID_JSON}`
	if err := os.WriteFile(configFile, []byte(invalidConfig), 0600); err != nil {
		log.Fatalf("Failed to write config file: %v", err)
	}

	// Give error handler time to be called
	time.Sleep(200 * time.Millisecond)
}

func testFileNotFound(tempDir string) {
	nonExistentFile := filepath.Join(tempDir, "does_not_exist.json")

	// Create custom error handler to catch file not found
	errorHandler := func(err error, filepath string) {
		fmt.Printf("   📁 Expected file not found error: %v\n", err)

		// Demonstrate go-errors error code checking
		errorMsg := err.Error()
		if strings.Contains(errorMsg, "ARGUS_FILE_NOT_FOUND") {
			fmt.Printf("      ✅ Correctly identified as file not found error\n")
		}
	}

	config := argus.Config{
		PollInterval: 50 * time.Millisecond,
		ErrorHandler: errorHandler,
	}

	// Try to watch non-existent file
	watcher, err := argus.UniversalConfigWatcherWithConfig(nonExistentFile, func(config map[string]interface{}) {
		fmt.Printf("   📦 This should not be called: %v\n", config)
	}, config)

	if err != nil {
		fmt.Printf("   ✅ Correctly caught watch error: %v\n", err)

		// Demonstrate go-errors error code checking
		errorMsg := err.Error()
		fmt.Printf("      📝 Error Message: %s\n", errorMsg)
	} else {
		defer watcher.Stop()
		time.Sleep(100 * time.Millisecond) // Give it time to trigger error handler
	}
}

func testParseError(tempDir string) {
	configFile := filepath.Join(tempDir, "parse_error_config.yaml")

	// Create invalid YAML
	invalidYAML := `
key: value
  invalid_indentation: bad
badly_formatted: {
`
	if err := os.WriteFile(configFile, []byte(invalidYAML), 0600); err != nil {
		log.Fatalf("Failed to write config file: %v", err)
	}

	// Error handler for parse errors
	errorHandler := func(err error, filepath string) {
		fmt.Printf("   🔧 Parse error as expected: %v\n", err)

		// Demonstrate go-errors error code checking for parse errors
		errorMsg := err.Error()
		if strings.Contains(errorMsg, "INVALID_CONFIG") {
			fmt.Printf("      ✅ Correctly identified as config parsing error\n")
		}
		fmt.Printf("      📝 Error Message: %s\n", errorMsg)
	}

	config := argus.Config{
		PollInterval: 50 * time.Millisecond,
		ErrorHandler: errorHandler,
	}

	watcher, err := argus.UniversalConfigWatcherWithConfig(configFile, func(config map[string]interface{}) {
		fmt.Printf("   📦 This might be called with partial data: %v\n", config)
	}, config)

	if err != nil {
		fmt.Printf("   ✅ Watch setup completed with: %v\n", err)

		// Demonstrate go-errors error code checking
		errorMsg := err.Error()
		fmt.Printf("      📝 Error Message: %s\n", errorMsg)
	} else {
		defer watcher.Stop()
		// Give it time to process
		time.Sleep(200 * time.Millisecond)
	}
}

func testDefaultErrorHandler(tempDir string) {
	configFile := filepath.Join(tempDir, "default_error_config.json")

	// Create invalid JSON
	invalidConfig := `{"service": "test", "port": invalid}`
	if err := os.WriteFile(configFile, []byte(invalidConfig), 0600); err != nil {
		log.Fatalf("Failed to write config file: %v", err)
	}

	fmt.Println("   📝 Using default error handler (logs to stderr):")

	// Use default configuration (includes default error handler)
	watcher, err := argus.UniversalConfigWatcher(configFile, func(config map[string]interface{}) {
		fmt.Printf("   📦 Config: %v\n", config)
	})

	if err != nil {
		fmt.Printf("   ✅ Watch error: %v\n", err)

		// Demonstrate go-errors error code checking
		errorMsg := err.Error()
		fmt.Printf("      📝 Error Message: %s\n", errorMsg)
	} else {
		defer watcher.Stop()
		// Give it time to process and show default error handling
		time.Sleep(200 * time.Millisecond)
	}
}

// Demonstrate creating custom errors with go-errors
func testCustomErrorCreation() {
	fmt.Println("\n5️⃣  Testing Custom Error Creation with go-errors:")

	// Create a custom error using go-errors with Argus error codes
	customErr := errors.New(argus.ErrCodeInvalidConfig, "This is a custom error for demonstration")

	fmt.Printf("   🔧 Custom Error: %v\n", customErr)
	fmt.Printf("      📝 Error Message: %s\n", customErr.Error())

	// Demonstrate error wrapping with Argus error codes
	wrappedErr := errors.Wrap(customErr, argus.ErrCodeWatcherStopped, "Wrapped the custom error")
	fmt.Printf("   🔗 Wrapped Error: %v\n", wrappedErr)
	fmt.Printf("      📝 Wrapped Message: %s\n", wrappedErr.Error())

	// Demonstrate error code checking
	errorMsg := wrappedErr.Error()
	if strings.Contains(errorMsg, "ARGUS_WATCHER_STOPPED") {
		fmt.Printf("      ✅ Correctly identified wrapped error code\n")
	}
}
