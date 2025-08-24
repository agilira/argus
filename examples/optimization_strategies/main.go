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
	fmt.Println("=== ARGUS OPTIMIZATION STRATEGIES DEMO ===")

	// Demo 1: SingleEvent Strategy per 1-2 files (ultra-low latency)
	fmt.Println("\n1. SingleEvent Strategy (1-2 files) - Ultra-low latency")
	demoSingleEventStrategy()

	// Demo 2: SmallBatch Strategy per 3-20 files (balanced)
	fmt.Println("\n2. SmallBatch Strategy (3-20 files) - Balanced performance")
	demoSmallBatchStrategy()

	// Demo 3: LargeBatch Strategy per 20+ files (high throughput)
	fmt.Println("\n3. LargeBatch Strategy (20+ files) - High throughput")
	demoLargeBatchStrategy()

	// Demo 4: Auto Strategy (adaptive)
	fmt.Println("\n4. Auto Strategy - Adaptive optimization")
	demoAutoStrategy()

	fmt.Println("\n=== DEMO COMPLETED ===")
}

func demoSingleEventStrategy() {
	// Configurazione ottimizzata per 1-2 files con latenza ultra-bassa
	config := argus.Config{
		PollInterval:         10 * time.Millisecond, // Polling veloce
		OptimizationStrategy: argus.OptimizationSingleEvent,
		BoreasLiteCapacity:   64, // Buffer minimo
	}

	watcher := argus.New(*config.WithDefaults())
	defer watcher.Stop()

	// Crea file di test
	tempDir := createTempDir("single_event_demo")
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "app.json")
	writeFile(configFile, `{"mode": "production", "debug": false}`)

	// Callback ultra-veloce
	watcher.Watch(configFile, func(event argus.ChangeEvent) {
		fmt.Printf("  ⚡ ULTRA-FAST: Config changed: %s\n", filepath.Base(event.Path))
	})

	watcher.Start()
	time.Sleep(20 * time.Millisecond)

	// Test modifica
	writeFile(configFile, `{"mode": "production", "debug": true}`)
	time.Sleep(50 * time.Millisecond)

	fmt.Printf("  ✅ SingleEvent: Perfetto per config files critici\n")
}

func demoSmallBatchStrategy() {
	// Configurazione bilanciata per 3-20 files
	config := argus.Config{
		PollInterval:         25 * time.Millisecond,
		OptimizationStrategy: argus.OptimizationSmallBatch,
		BoreasLiteCapacity:   128, // Buffer bilanciato
	}

	watcher := argus.New(*config.WithDefaults())
	defer watcher.Stop()

	// Crea files di test
	tempDir := createTempDir("small_batch_demo")
	defer os.RemoveAll(tempDir)

	files := []string{"api.json", "db.json", "cache.json", "auth.json", "metrics.json"}
	for i, filename := range files {
		filePath := filepath.Join(tempDir, filename)
		writeFile(filePath, fmt.Sprintf(`{"service": "%s", "port": %d}`, filename[:len(filename)-5], 8000+i))

		watcher.Watch(filePath, func(event argus.ChangeEvent) {
			fmt.Printf("  📦 BATCH: Service config updated: %s\n", filepath.Base(event.Path))
		})
	}

	watcher.Start()
	time.Sleep(30 * time.Millisecond)

	// Test modifica multipla
	for i, filename := range files {
		filePath := filepath.Join(tempDir, filename)
		writeFile(filePath, fmt.Sprintf(`{"service": "%s", "port": %d, "updated": true}`, filename[:len(filename)-5], 8000+i))
	}
	time.Sleep(100 * time.Millisecond)

	fmt.Printf("  ✅ SmallBatch: Perfetto per microservizi\n")
}

func demoLargeBatchStrategy() {
	// Configurazione ad alta throughput per 20+ files
	config := argus.Config{
		PollInterval:         50 * time.Millisecond,
		OptimizationStrategy: argus.OptimizationLargeBatch,
		BoreasLiteCapacity:   256, // Buffer grande
	}

	watcher := argus.New(*config.WithDefaults())
	defer watcher.Stop()

	// Crea molti files di test
	tempDir := createTempDir("large_batch_demo")
	defer os.RemoveAll(tempDir)

	fileCount := 30
	for i := 0; i < fileCount; i++ {
		filename := fmt.Sprintf("service_%02d.json", i)
		filePath := filepath.Join(tempDir, filename)
		writeFile(filePath, fmt.Sprintf(`{"id": %d, "status": "active"}`, i))

		watcher.Watch(filePath, func(event argus.ChangeEvent) {
			fmt.Printf("  🚀 BULK: Bulk service updated: %s\n", filepath.Base(event.Path))
		})
	}

	watcher.Start()
	time.Sleep(60 * time.Millisecond)

	// Test modifica bulk
	for i := 0; i < fileCount; i++ {
		filename := fmt.Sprintf("service_%02d.json", i)
		filePath := filepath.Join(tempDir, filename)
		writeFile(filePath, fmt.Sprintf(`{"id": %d, "status": "updated", "batch": true}`, i))
	}
	time.Sleep(200 * time.Millisecond)

	fmt.Printf("  ✅ LargeBatch: Perfetto per orchestratori container\n")
}

func demoAutoStrategy() {
	// Configurazione adaptive che si adatta automaticamente
	config := argus.Config{
		PollInterval:         20 * time.Millisecond,
		OptimizationStrategy: argus.OptimizationAuto, // Auto-adaptive!
		// BoreasLiteCapacity lasciato a 0 per auto-sizing
	}

	watcher := argus.New(*config.WithDefaults())
	defer watcher.Stop()

	tempDir := createTempDir("auto_demo")
	defer os.RemoveAll(tempDir)

	watcher.Start()

	// Fase 1: Inizia con 1 file (dovrebbe usare SingleEvent)
	fmt.Printf("  🧠 AUTO: Fase 1 - Single file (auto-SingleEvent)\n")
	configFile := filepath.Join(tempDir, "main.json")
	writeFile(configFile, `{"files": 1}`)
	watcher.Watch(configFile, func(event argus.ChangeEvent) {
		fmt.Printf("    Auto-adapted: %s\n", filepath.Base(event.Path))
	})

	writeFile(configFile, `{"files": 1, "updated": true}`)
	time.Sleep(50 * time.Millisecond)

	// Fase 2: Aggiungi più files (dovrebbe adattarsi a SmallBatch)
	fmt.Printf("  🧠 AUTO: Fase 2 - Multiple files (auto-SmallBatch)\n")
	for i := 1; i <= 10; i++ {
		filename := fmt.Sprintf("service_%d.json", i)
		filePath := filepath.Join(tempDir, filename)
		writeFile(filePath, fmt.Sprintf(`{"service": %d}`, i))
		watcher.Watch(filePath, func(event argus.ChangeEvent) {
			fmt.Printf("    Auto-adapted: %s\n", filepath.Base(event.Path))
		})
	}

	// Test modifica
	for i := 1; i <= 10; i++ {
		filename := fmt.Sprintf("service_%d.json", i)
		filePath := filepath.Join(tempDir, filename)
		writeFile(filePath, fmt.Sprintf(`{"service": %d, "auto": true}`, i))
	}
	time.Sleep(100 * time.Millisecond)

	fmt.Printf("  ✅ Auto: Si adatta automaticamente al carico!\n")
}

// Helper functions
func createTempDir(prefix string) string {
	tempDir, err := os.MkdirTemp("", prefix)
	if err != nil {
		log.Fatal(err)
	}
	return tempDir
}

func writeFile(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		log.Fatal(err)
	}
}
