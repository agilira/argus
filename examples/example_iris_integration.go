// example_iris_integration.go - Integrazione di Argus con Iris logging
//
// Questo esempio dimostra come implementare il punto 4 di Gemini:
// "Dynamic log level changes at runtime" con audit trail completo
//
// Copyright (c) 2025 AGILira
// Series: AGILira System Libraries

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"argus"
)

// IrisConfig rappresenta la configurazione di Iris logging
type IrisConfig struct {
	LogLevel    string `json:"log_level"`
	EnableAudit bool   `json:"enable_audit"`
	MaxFileSize int64  `json:"max_file_size"`
	Port        int    `json:"port"`
}

// MockIrisLogger simula il logger di Iris
type MockIrisLogger struct {
	level    string
	auditLog *argus.AuditLogger
}

func (l *MockIrisLogger) SetLevel(level string) {
	oldLevel := l.level
	l.level = level

	// Log del cambio di livello per audit trail
	if l.auditLog != nil {
		l.auditLog.LogConfigChange("iris_logger",
			map[string]interface{}{"log_level": oldLevel},
			map[string]interface{}{"log_level": level})
	}

	fmt.Printf("📝 Iris log level changed: %s -> %s\n", oldLevel, level)
}

func (l *MockIrisLogger) Info(msg string) {
	if l.level == "debug" || l.level == "info" {
		fmt.Printf("[INFO] %s\n", msg)
	}
}

func (l *MockIrisLogger) Debug(msg string) {
	if l.level == "debug" {
		fmt.Printf("[DEBUG] %s\n", msg)
	}
}

func main() {
	fmt.Println("🎯 Demo: Argus + Iris Dynamic Log Level Changes")
	fmt.Println("===================================================")

	// 1. Crea file di configurazione esempio
	configFile := "/tmp/iris_config.json"
	initialConfig := IrisConfig{
		LogLevel:    "info",
		EnableAudit: true,
		MaxFileSize: 10485760, // 10MB
		Port:        8080,
	}

	configData, _ := json.MarshalIndent(initialConfig, "", "  ")
	os.WriteFile(configFile, configData, 0644)
	fmt.Printf("📄 Created config file: %s\n", configFile)

	// 2. Setup audit trail
	auditConfig := argus.AuditConfig{
		Enabled:       true,
		OutputFile:    "/tmp/iris_audit.jsonl",
		MinLevel:      argus.AuditInfo,
		BufferSize:    100,
		FlushInterval: 1 * time.Second,
		IncludeStack:  false,
	}

	auditor, err := argus.NewAuditLogger(auditConfig)
	if err != nil {
		log.Fatal("Failed to create audit logger:", err)
	}
	defer auditor.Close()

	// 3. Crea mock Iris logger
	irisLogger := &MockIrisLogger{
		level:    "info",
		auditLog: auditor,
	}

	// 4. Setup Argus file watcher con audit trail
	watcherConfig := argus.Config{
		PollInterval: 500 * time.Millisecond,
		CacheTTL:     1 * time.Second,
		Audit:        auditConfig,
	}

	fmt.Printf("🔍 Starting Argus watcher with audit trail...\n")

	watcher, err := argus.UniversalConfigWatcherWithConfig(configFile,
		func(config map[string]interface{}) {
			// Parse della nuova configurazione
			if logLevel, ok := config["log_level"].(string); ok {
				irisLogger.SetLevel(logLevel)
			}

			if port, ok := config["port"].(float64); ok {
				fmt.Printf("🌐 Port updated to: %.0f\n", port)
			}

			fmt.Printf("⚙️  Full config: %+v\n", config)
		}, watcherConfig)

	if err != nil {
		log.Fatal("Failed to create config watcher:", err)
	}
	defer watcher.Stop()

	// 5. Demo dei log con livello iniziale
	fmt.Printf("\n🧪 Testing logs with initial level (%s):\n", irisLogger.level)
	irisLogger.Info("This is an info message")
	irisLogger.Debug("This debug message should NOT appear")

	// 6. Simula modifiche alla configurazione
	time.Sleep(1 * time.Second)

	fmt.Printf("\n🔄 Changing log level to 'debug' in config file...\n")
	updatedConfig := initialConfig
	updatedConfig.LogLevel = "debug"
	updatedConfig.Port = 9090

	configData, _ = json.MarshalIndent(updatedConfig, "", "  ")
	os.WriteFile(configFile, configData, 0644)

	// Aspetta che Argus rilevi il cambiamento
	time.Sleep(1 * time.Second)

	fmt.Printf("\n🧪 Testing logs with new level (%s):\n", irisLogger.level)
	irisLogger.Info("This is an info message")
	irisLogger.Debug("This debug message SHOULD appear now!")

	// 7. Altra modifica
	time.Sleep(1 * time.Second)

	fmt.Printf("\n🔄 Changing log level back to 'info'...\n")
	updatedConfig.LogLevel = "info"
	updatedConfig.Port = 8080

	configData, _ = json.MarshalIndent(updatedConfig, "", "  ")
	os.WriteFile(configFile, configData, 0644)

	time.Sleep(1 * time.Second)

	fmt.Printf("\n🧪 Testing logs with level back to (%s):\n", irisLogger.level)
	irisLogger.Info("This is an info message")
	irisLogger.Debug("This debug message should NOT appear again")

	// 8. Flush audit e mostra il contenuto
	auditor.Flush()
	time.Sleep(500 * time.Millisecond)

	fmt.Printf("\n📋 Audit Trail Summary:\n")
	fmt.Printf("=======================\n")

	auditData, err := os.ReadFile("/tmp/iris_audit.jsonl")
	if err != nil {
		fmt.Printf("Error reading audit file: %v\n", err)
	} else {
		fmt.Printf("%s\n", string(auditData))
	}

	fmt.Printf("\n✅ Demo completata! Argus ha gestito:\n")
	fmt.Printf("   - Rilevamento automatico dei cambiamenti di configurazione\n")
	fmt.Printf("   - Aggiornamento dinamico del log level di Iris\n")
	fmt.Printf("   - Audit trail completo per compliance e sicurezza\n")
	fmt.Printf("   - Support multi-formato (JSON, YAML, TOML, HCL, INI, Properties)\n")
	fmt.Printf("   - Performance ultra-ottimizzate (4.073ns format detection)\n")

	// Cleanup
	os.Remove(configFile)
	os.Remove("/tmp/iris_audit.jsonl")
}
