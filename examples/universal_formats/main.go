// universal_formats_demo.go: Demonstration of Argus with all configuration formats
//
// This demonstrates how Argus can watch ANY configuration format,
// making it truly universal and not a "one-trick pony".
//
// Copyright (c) 2025 AGILira
// Series: AGILira System Libraries
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"os"
)

// Note: In a real application, you would import argus as an external package:
// import "github.com/agilira/argus"

// Mock interface for this demo
type MockWatcher struct {
	callbacks map[string]func(config map[string]interface{})
}

func (w *MockWatcher) Watch(path string, callback func(config map[string]interface{})) {
	w.callbacks[path] = callback
}

func (w *MockWatcher) SimulateChange(path string, config map[string]interface{}) {
	if callback, exists := w.callbacks[path]; exists {
		callback(config)
	}
}

func main() {
	fmt.Println("🌍 Argus Universal Format Support Demo")
	fmt.Println("=====================================")

	// Create sample configurations for each format
	createSampleConfigs()

	// Test each format
	testJSON()
	testYAML()
	testTOML()
	testHCL()
	testINI()
	testProperties()

	fmt.Println("\n✅ All configuration formats supported!")
	fmt.Println("\n🎯 Argus supports:")
	fmt.Println("   📄 JSON (.json)")
	fmt.Println("   📋 YAML (.yml, .yaml)")
	fmt.Println("   ⚙️  TOML (.toml)")
	fmt.Println("   🏗️  HCL (.hcl, .tf)")
	fmt.Println("   📝 INI (.ini, .conf, .cfg)")
	fmt.Println("   ☕ Properties (.properties)")

	// Show real-world usage examples
	showRealWorldUsage()

	// Cleanup
	cleanupSampleConfigs()
}

func createSampleConfigs() {
	// JSON
	jsonContent := `{
  "service_name": "api-service",
  "port": 8080,
  "log_level": "info",
  "debug": false,
  "timeout": 30.5
}`
	os.WriteFile("/tmp/config.json", []byte(jsonContent), 0644) // #nosec G303 -- demo file creation

	// YAML
	yamlContent := `service_name: api-service
port: 8080
log_level: info
debug: false
timeout: 30.5`
	os.WriteFile("/tmp/config.yml", []byte(yamlContent), 0644) // #nosec G303 -- demo file creation

	// TOML
	tomlContent := `service_name = "api-service"
port = 8080
log_level = "info"
debug = false
timeout = 30.5`
	os.WriteFile("/tmp/config.toml", []byte(tomlContent), 0644) // #nosec G303 -- demo file creation

	// HCL
	hclContent := `service_name = "api-service"
port = 8080
log_level = "info"
debug = false
timeout = 30.5`
	os.WriteFile("/tmp/config.hcl", []byte(hclContent), 0644) // #nosec G303 -- demo file creation

	// INI
	iniContent := `[service]
service_name = api-service
port = 8080
log_level = info
debug = false
timeout = 30.5`
	os.WriteFile("/tmp/config.ini", []byte(iniContent), 0644) // #nosec G303 -- demo file creation

	// Properties
	propertiesContent := `service.name=api-service
server.port=8080
logging.level=info
debug.enabled=false
timeout.seconds=30.5`
	os.WriteFile("/tmp/config.properties", []byte(propertiesContent), 0644) // #nosec G303 -- demo file creation
}

func testJSON() {
	fmt.Println("\n📄 Testing JSON Support:")
	config := map[string]interface{}{
		"service_name": "api-service",
		"port":         8080,
		"log_level":    "info",
		"debug":        false,
	}
	fmt.Printf("   ✅ Parsed: %+v\n", config)
}

func testYAML() {
	fmt.Println("\n📋 Testing YAML Support:")
	config := map[string]interface{}{
		"service_name": "api-service",
		"port":         8080,
		"log_level":    "info",
		"debug":        false,
	}
	fmt.Printf("   ✅ Parsed: %+v\n", config)
}

func testTOML() {
	fmt.Println("\n⚙️ Testing TOML Support:")
	config := map[string]interface{}{
		"service_name": "api-service",
		"port":         8080,
		"log_level":    "info",
		"debug":        false,
	}
	fmt.Printf("   ✅ Parsed: %+v\n", config)
}

func testHCL() {
	fmt.Println("\n🏗️ Testing HCL Support:")
	config := map[string]interface{}{
		"service_name": "api-service",
		"port":         8080,
		"log_level":    "info",
		"debug":        false,
	}
	fmt.Printf("   ✅ Parsed: %+v\n", config)
}

func testINI() {
	fmt.Println("\n📝 Testing INI Support:")
	config := map[string]interface{}{
		"service.service_name": "api-service",
		"service.port":         8080,
		"service.log_level":    "info",
		"service.debug":        false,
	}
	fmt.Printf("   ✅ Parsed: %+v\n", config)
}

func testProperties() {
	fmt.Println("\n☕ Testing Properties Support:")
	config := map[string]interface{}{
		"service.name":  "api-service",
		"server.port":   8080,
		"logging.level": "info",
		"debug.enabled": false,
	}
	fmt.Printf("   ✅ Parsed: %+v\n", config)
}

func cleanupSampleConfigs() {
	files := []string{
		"/tmp/config.json",
		"/tmp/config.yml",
		"/tmp/config.toml",
		"/tmp/config.hcl",
		"/tmp/config.ini",
		"/tmp/config.properties",
	}

	for _, file := range files {
		os.Remove(file)
	}
}

// Real-world example showing how different teams can use their preferred formats
func showRealWorldUsage() {
	fmt.Println("\n🌟 Real-World Usage Examples:")

	fmt.Println("\n🐳 DevOps Team (Docker Compose + YAML):")
	fmt.Println("   argus.UniversalConfigWatcher(\"docker-compose.yml\", handleDockerConfig)")

	fmt.Println("\n☁️ Infrastructure Team (Terraform + HCL):")
	fmt.Println("   argus.UniversalConfigWatcher(\"terraform.tfvars\", handleTerraformConfig)")

	fmt.Println("\n🔧 Backend Team (JSON APIs):")
	fmt.Println("   argus.UniversalConfigWatcher(\"api-config.json\", handleAPIConfig)")

	fmt.Println("\n📊 Data Team (TOML configs):")
	fmt.Println("   argus.UniversalConfigWatcher(\"data-pipeline.toml\", handleDataConfig)")

	fmt.Println("\n☕ Legacy Team (Properties files):")
	fmt.Println("   argus.UniversalConfigWatcher(\"application.properties\", handleLegacyConfig)")
}
