// Error Handling Example
//
// This example demonstrates comprehensive error handling strategies with Argus.
//
// Usage:
//   cd examples/error_handling
//   go run main.go

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
	fmt.Println("🚨 Argus Error Handling Demo")
	fmt.Println("============================")

	// Create temporary directory for testing
	tempDir := "/tmp/argus_error_demo"
	os.MkdirAll(tempDir, 0755)
	defer os.RemoveAll(tempDir)

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

	fmt.Println("\n✅ Error handling demo completed!")
}

func testCustomErrorHandler(tempDir string) {
	configFile := filepath.Join(tempDir, "custom_error_config.json")

	// Create a valid config first
	validConfig := `{"service": "test", "port": 8080}`
	os.WriteFile(configFile, []byte(validConfig), 0644)

	// Create custom error handler (note the parameter order: err, filepath)
	errorHandler := func(err error, filepath string) {
		fmt.Printf("   🔥 Custom Error Handler: File %s had error: %v\n", filepath, err)
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
	os.WriteFile(configFile, []byte(invalidConfig), 0644)

	// Give error handler time to be called
	time.Sleep(200 * time.Millisecond)
}

func testFileNotFound(tempDir string) {
	nonExistentFile := filepath.Join(tempDir, "does_not_exist.json")

	// Create custom error handler to catch file not found
	errorHandler := func(err error, filepath string) {
		fmt.Printf("   📁 Expected file not found error: %v\n", err)
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
	os.WriteFile(configFile, []byte(invalidYAML), 0644)

	// Error handler for parse errors
	errorHandler := func(err error, filepath string) {
		fmt.Printf("   🔧 Parse error as expected: %v\n", err)
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
	os.WriteFile(configFile, []byte(invalidConfig), 0644)

	fmt.Println("   📝 Using default error handler (logs to stderr):")

	// Use default configuration (includes default error handler)
	watcher, err := argus.UniversalConfigWatcher(configFile, func(config map[string]interface{}) {
		fmt.Printf("   📦 Config: %v\n", config)
	})

	if err != nil {
		fmt.Printf("   ✅ Watch error: %v\n", err)
	} else {
		defer watcher.Stop()
		// Give it time to process and show default error handling
		time.Sleep(200 * time.Millisecond)
	}
}
