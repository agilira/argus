// demo_universal.go: Standalone demo showing Argus with any application
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync/atomic"
)

// Example: Generic microservice configuration
type ServiceConfig struct {
	ServiceName    string `json:"service_name"`
	Port           int    `json:"port"`
	LogLevel       string `json:"log_level"`
	MetricsEnabled bool   `json:"metrics_enabled"`
	HealthCheckURL string `json:"health_check_url"`
}

type MicroService struct {
	config atomic.Value
}

func (s *MicroService) UpdateConfig(newConfig *ServiceConfig) {
	oldConfig := s.GetConfig()
	s.config.Store(newConfig)

	log.Printf("🚀 Service [%s] config updated:", newConfig.ServiceName)
	log.Printf("   Port: %d -> %d", oldConfig.Port, newConfig.Port)
	log.Printf("   Log Level: %s -> %s", oldConfig.LogLevel, newConfig.LogLevel)
	log.Printf("   Metrics: %v -> %v", oldConfig.MetricsEnabled, newConfig.MetricsEnabled)
}

func (s *MicroService) GetConfig() *ServiceConfig {
	if config := s.config.Load(); config != nil {
		return config.(*ServiceConfig)
	}
	return &ServiceConfig{
		ServiceName:    "unknown",
		Port:           8080,
		LogLevel:       "info",
		MetricsEnabled: true,
		HealthCheckURL: "/health",
	}
}

func main() {
	fmt.Println("🌍 Argus - Universal Configuration Watcher")
	fmt.Println("==========================================")
	fmt.Println()

	// Create a sample service config
	config := ServiceConfig{
		ServiceName:    "user-service",
		Port:           8080,
		LogLevel:       "info",
		MetricsEnabled: true,
		HealthCheckURL: "/health",
	}

	data, _ := json.MarshalIndent(config, "", "  ")
	if err := os.WriteFile("service_config.json", data, 0600); err != nil {
		log.Printf("Warning: Failed to write config file: %v", err)
		return
	}
	defer func() {
		if err := os.Remove("service_config.json"); err != nil {
			log.Printf("Warning: Failed to remove config file: %v", err)
		}
	}()

	fmt.Printf("📝 Created service_config.json:\n%s\n\n", string(data))

	// Initialize service
	service := &MicroService{}
	service.UpdateConfig(&config)

	fmt.Println("✅ Argus Usage Scenarios:")
	fmt.Println()

	fmt.Println("🔧 1. WEB FRAMEWORKS:")
	fmt.Println("   • Gin HTTP server port, middleware settings")
	fmt.Println("   • Echo rate limiting, CORS configuration")
	fmt.Println("   • Fiber template settings, static file paths")
	fmt.Println()

	fmt.Println("🗄️  2. DATABASES:")
	fmt.Println("   • GORM connection pools, query timeouts")
	fmt.Println("   • Redis cache TTL, eviction policies")
	fmt.Println("   • MongoDB read preferences, write concerns")
	fmt.Println()

	fmt.Println("📨 3. MESSAGE BROKERS:")
	fmt.Println("   • NATS subscription subjects, queue groups")
	fmt.Println("   • RabbitMQ exchange bindings, retry policies")
	fmt.Println("   • Kafka consumer groups, batch sizes")
	fmt.Println()

	fmt.Println("🎮 4. SPECIALIZED APPS:")
	fmt.Println("   • Game servers: player limits, match timeouts")
	fmt.Println("   • IoT gateways: device polling intervals")
	fmt.Println("   • API gateways: rate limits, circuit breakers")
	fmt.Println()

	fmt.Println("☁️  5. CLOUD & DEVOPS:")
	fmt.Println("   • Kubernetes operators: reconciliation intervals")
	fmt.Println("   • CI/CD tools: pipeline configurations")
	fmt.Println("   • Monitoring tools: alert thresholds")
	fmt.Println()

	fmt.Println("💡 KEY BENEFITS:")
	fmt.Println("   ✓ OS-independent (works everywhere)")
	fmt.Println("   ✓ Zero external dependencies")
	fmt.Println("   ✓ ~40ns cache performance")
	fmt.Println("   ✓ Thread-safe atomic operations")
	fmt.Println("   ✓ Works with ANY Go application")
	fmt.Println("   ✓ Kubernetes ConfigMap compatible")
	fmt.Println("   ✓ No HTTP endpoints (secure)")
	fmt.Println()

	fmt.Println("🔍 How it works:")
	fmt.Println("   1. Watch any JSON/YAML config file")
	fmt.Println("   2. File changes trigger callbacks")
	fmt.Println("   3. Your app updates atomically")
	fmt.Println("   4. No restart needed!")
	fmt.Println()

	fmt.Printf("🎯 Current service config: %+v\n", service.GetConfig())

	fmt.Println("\n🏆 Argus: The universal config watcher for Go applications!")
}
