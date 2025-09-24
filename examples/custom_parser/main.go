// Package main demonstrates how to create and register custom configuration parsers with Argus.
// The example covers parser registration, integration, error handling, and live reload.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/agilira/argus"
)

// AdvancedYAMLParser is a demonstration of a YAML parser implementation.
// In a real scenario, this would use a full-featured YAML library and be placed in a dedicated package.
type AdvancedYAMLParser struct{}

func (p *AdvancedYAMLParser) Parse(data []byte) (map[string]interface{}, error) {
	// NOTE: In production, use a library like gopkg.in/yaml.v3 for full YAML support.
	// This demo simulates advanced parsing features.
	result := map[string]interface{}{
		"_parser_info": map[string]interface{}{
			"name":     "Advanced YAML Parser",
			"version":  "1.0.0",
			"features": []string{"anchors", "multi-doc", "type-coercion"},
		},
		"_note": "This would use a real YAML library like gopkg.in/yaml.v3",
	}

	// Parse the actual content using built-in parser for demo
	builtinResult, err := parseSimpleYAML(data)
	if err != nil {
		return nil, fmt.Errorf("advanced YAML parser: %w", err)
	}

	// Merge results (in real implementation, this would be the advanced parsing)
	for k, v := range builtinResult {
		result[k] = v
	}

	return result, nil
}

func (p *AdvancedYAMLParser) Supports(format argus.ConfigFormat) bool {
	return format == argus.FormatYAML
}

func (p *AdvancedYAMLParser) Name() string {
	return "Advanced YAML Parser (Demo)"
}

// parseSimpleYAML is a helper for demo purposes
func parseSimpleYAML(data []byte) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	lines := string(data)

	// Very simple YAML parsing for demo
	if len(lines) > 0 {
		result["raw_content"] = string(data)
		result["line_count"] = len([]rune(lines))
	}

	return result, nil
}

func main() {

	fmt.Println("Argus Custom Parser Example")
	fmt.Println("============================")

	// Create a temporary YAML config file
	configContent := `# Demo YAML configuration
app_name: "demo-app"
version: "1.0.0"
features:
  - authentication
  - logging
  - metrics
environment: production
debug: false
`

	configFile := "/tmp/demo_config.yaml"
       err := os.WriteFile(configFile, []byte(configContent), 0600)
       if err != nil {
	       log.Fatalf("Failed to create config file: %v", err)
       }
       defer func() {
	       if removeErr := os.Remove(configFile); removeErr != nil {
		       log.Printf("Failed to remove config file: %v", removeErr)
	       }
       }()

	fmt.Printf("📄 Created demo config: %s\n\n", configFile)


	// Step 1: Parse with built-in parser
	fmt.Println("Step 1: Parsing with built-in parser")
	fmt.Println("   (Simple, minimal dependencies)")

	watcher1, err := argus.UniversalConfigWatcher(configFile, func(config map[string]interface{}) {
		fmt.Printf("   📦 Built-in result: %v\n", config)
	})
	if err != nil {
		log.Fatalf("Failed to create watcher: %v", err)
	}
       defer func() {
	       if err := watcher1.Stop(); err != nil {
		       log.Printf("Failed to stop watcher1: %v", err)
	       }
       }()

	time.Sleep(100 * time.Millisecond) // Give it time to read initial config


	// Step 2: Register custom parser and parse again
	fmt.Println("\nStep 2: Registering custom parser")
	fmt.Println("   (Demonstrates extensibility and advanced features)")

	// Register our custom parser
	argus.RegisterParser(&AdvancedYAMLParser{})

	// List registered parsers
	// Note: We need to access the function differently since it's not exported

	fmt.Println("   Custom parser registered.")

       // Create new watcher that will use the custom parser
       watcher2, err := argus.UniversalConfigWatcher(configFile, func(config map[string]interface{}) {
	       fmt.Printf("   Custom parser result:\n")
	       for k, v := range config {
		       fmt.Printf("      %s: %v\n", k, v)
	       }
       })
	if err != nil {
		log.Fatalf("Failed to create watcher with custom parser: %v", err)
	}
       defer func() {
	       if err := watcher2.Stop(); err != nil {
		       log.Printf("Failed to stop watcher2: %v", err)
	       }
       }()

	time.Sleep(100 * time.Millisecond) // Give it time to read initial config


	// Step 3: Compare parser behaviors
	fmt.Println("\nStep 3: Key differences")
	fmt.Println("   Built-in: Fast, simple, minimal dependencies")
	fmt.Println("   Custom:   Full spec compliance, advanced features")
	fmt.Println("   Priority: Custom parsers are tried first, built-in as fallback")


	// Step 4: Update config to show live reloading
	fmt.Println("\nStep 4: Testing live reload")

	updatedContent := `# Updated YAML configuration
app_name: "demo-app-updated"
version: "2.0.0"
features:
  - authentication
  - logging
  - metrics
  - monitoring
environment: staging
debug: true
`

       err = os.WriteFile(configFile, []byte(updatedContent), 0600)
       if err != nil {
	       log.Printf("Failed to update config: %v", err)
       }

	time.Sleep(200 * time.Millisecond) // Give watchers time to detect changes


	fmt.Println("\nDemo completed.")
	fmt.Println("\nKey points:")
	fmt.Println("   • Import-based registration: import _ \"github.com/your-org/argus-yaml-pro\"")
	fmt.Println("   • Manual registration: argus.RegisterParser(&MyParser{})")
	fmt.Println("   • Build tags: go build -tags \"yaml_pro,toml_pro\"")
	fmt.Println("   • Minimal dependencies by default; advanced features available via custom parsers.")
}
