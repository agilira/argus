// directory_watcher_test.go: TDD Tests for Directory Watching
//
// Tests for scanning directories for configuration files and
// watching all matching files for changes.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// TDD: WatchDirectory - Scan and watch all config files in a directory
// =============================================================================

func TestWatchDirectory_Basic(t *testing.T) {
	t.Run("watches_all_yaml_files_in_directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create multiple YAML files
		file1 := filepath.Join(tmpDir, "config1.yaml")
		file2 := filepath.Join(tmpDir, "config2.yaml")

		err := os.WriteFile(file1, []byte("key1: value1\n"), 0o600)
		if err != nil {
			t.Fatalf("failed to create file1: %v", err)
		}
		err = os.WriteFile(file2, []byte("key2: value2\n"), 0o600)
		if err != nil {
			t.Fatalf("failed to create file2: %v", err)
		}

		var mu sync.Mutex
		var receivedConfigs []DirectoryConfigUpdate

		watcher, err := WatchDirectory(tmpDir, DirectoryWatchOptions{
			Patterns: []string{"*.yaml", "*.yml"},
		}, func(update DirectoryConfigUpdate) {
			mu.Lock()
			receivedConfigs = append(receivedConfigs, update)
			mu.Unlock()
		})
		if err != nil {
			t.Fatalf("WatchDirectory failed: %v", err)
		}
		defer func() {
			if err := watcher.Close(); err != nil {
				t.Logf("watcher.Close failed: %v", err)
			}
		}()

		// Wait for initial load
		time.Sleep(200 * time.Millisecond)

		mu.Lock()
		count := len(receivedConfigs)
		mu.Unlock()

		// Should have received configs from both files
		if count < 2 {
			t.Errorf("expected at least 2 config updates, got %d", count)
		}
	})

	t.Run("detects_new_file_added_to_directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Start with one file
		file1 := filepath.Join(tmpDir, "initial.yaml")
		err := os.WriteFile(file1, []byte("initial: true\n"), 0o600)
		if err != nil {
			t.Fatalf("failed to create initial file: %v", err)
		}

		var mu sync.Mutex
		var updates []DirectoryConfigUpdate

		watcher, err := WatchDirectory(tmpDir, DirectoryWatchOptions{
			Patterns:     []string{"*.yaml"},
			PollInterval: 50 * time.Millisecond,
		}, func(update DirectoryConfigUpdate) {
			mu.Lock()
			updates = append(updates, update)
			mu.Unlock()
		})
		if err != nil {
			t.Fatalf("WatchDirectory failed: %v", err)
		}
		defer func() {
			if err := watcher.Close(); err != nil {
				t.Logf("watcher.Close failed: %v", err)
			}
		}()

		// Wait for initial load
		time.Sleep(100 * time.Millisecond)

		// Add a new file
		file2 := filepath.Join(tmpDir, "newfile.yaml")
		err = os.WriteFile(file2, []byte("new: file\n"), 0o600)
		if err != nil {
			t.Fatalf("failed to create new file: %v", err)
		}

		// Wait for detection
		time.Sleep(200 * time.Millisecond)

		mu.Lock()
		count := len(updates)
		hasNewFile := false
		for _, u := range updates {
			if filepath.Base(u.FilePath) == "newfile.yaml" {
				hasNewFile = true
			}
		}
		mu.Unlock()

		if count < 2 {
			t.Errorf("expected at least 2 updates, got %d", count)
		}
		if !hasNewFile {
			t.Error("expected to receive update for newfile.yaml")
		}
	})

	t.Run("detects_file_modification", func(t *testing.T) {
		tmpDir := t.TempDir()

		file1 := filepath.Join(tmpDir, "modifiable.yaml")
		err := os.WriteFile(file1, []byte("version: 1\n"), 0o600)
		if err != nil {
			t.Fatalf("failed to create file: %v", err)
		}

		var mu sync.Mutex
		var updates []DirectoryConfigUpdate

		watcher, err := WatchDirectory(tmpDir, DirectoryWatchOptions{
			Patterns:     []string{"*.yaml"},
			PollInterval: 50 * time.Millisecond,
		}, func(update DirectoryConfigUpdate) {
			mu.Lock()
			updates = append(updates, update)
			mu.Unlock()
		})
		if err != nil {
			t.Fatalf("WatchDirectory failed: %v", err)
		}
		defer func() {
			if err := watcher.Close(); err != nil {
				t.Logf("watcher.Close failed: %v", err)
			}
		}()

		// Wait for initial load
		time.Sleep(100 * time.Millisecond)

		// Modify the file
		err = os.WriteFile(file1, []byte("version: 2\n"), 0o600)
		if err != nil {
			t.Fatalf("failed to modify file: %v", err)
		}

		// Wait for detection
		time.Sleep(200 * time.Millisecond)

		mu.Lock()
		count := len(updates)
		mu.Unlock()

		if count < 2 {
			t.Errorf("expected at least 2 updates (initial + modification), got %d", count)
		}
	})

	t.Run("detects_file_deletion", func(t *testing.T) {
		tmpDir := t.TempDir()

		file1 := filepath.Join(tmpDir, "deletable.yaml")
		err := os.WriteFile(file1, []byte("to_delete: true\n"), 0o600)
		if err != nil {
			t.Fatalf("failed to create file: %v", err)
		}

		var mu sync.Mutex
		var updates []DirectoryConfigUpdate

		watcher, err := WatchDirectory(tmpDir, DirectoryWatchOptions{
			Patterns:     []string{"*.yaml"},
			PollInterval: 50 * time.Millisecond,
		}, func(update DirectoryConfigUpdate) {
			mu.Lock()
			updates = append(updates, update)
			mu.Unlock()
		})
		if err != nil {
			t.Fatalf("WatchDirectory failed: %v", err)
		}
		defer func() {
			if err := watcher.Close(); err != nil {
				t.Logf("watcher.Close failed: %v", err)
			}
		}()

		// Wait for initial load
		time.Sleep(100 * time.Millisecond)

		// Delete the file
		err = os.Remove(file1)
		if err != nil {
			t.Fatalf("failed to delete file: %v", err)
		}

		// Wait for detection
		time.Sleep(200 * time.Millisecond)

		mu.Lock()
		hasDelete := false
		for _, u := range updates {
			if u.IsDelete {
				hasDelete = true
			}
		}
		mu.Unlock()

		if !hasDelete {
			t.Error("expected to receive delete event")
		}
	})
}

func TestWatchDirectory_Patterns(t *testing.T) {
	t.Run("filters_by_pattern", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create files with different extensions
		yamlFile := filepath.Join(tmpDir, "config.yaml")
		jsonFile := filepath.Join(tmpDir, "config.json")
		txtFile := filepath.Join(tmpDir, "readme.txt")

		_ = os.WriteFile(yamlFile, []byte("yaml: true\n"), 0o600)
		_ = os.WriteFile(jsonFile, []byte(`{"json": true}`), 0o600)
		_ = os.WriteFile(txtFile, []byte("not a config"), 0o600)

		var mu sync.Mutex
		var updates []DirectoryConfigUpdate

		// Only watch YAML files
		watcher, err := WatchDirectory(tmpDir, DirectoryWatchOptions{
			Patterns: []string{"*.yaml"},
		}, func(update DirectoryConfigUpdate) {
			mu.Lock()
			updates = append(updates, update)
			mu.Unlock()
		})
		if err != nil {
			t.Fatalf("WatchDirectory failed: %v", err)
		}
		defer func() {
			if err := watcher.Close(); err != nil {
				t.Logf("watcher.Close failed: %v", err)
			}
		}()

		time.Sleep(200 * time.Millisecond)

		mu.Lock()
		count := len(updates)
		mu.Unlock()

		// Should only have yaml file
		if count != 1 {
			t.Errorf("expected 1 update (only yaml), got %d", count)
		}
	})

	t.Run("supports_multiple_patterns", func(t *testing.T) {
		tmpDir := t.TempDir()

		yamlFile := filepath.Join(tmpDir, "config.yaml")
		jsonFile := filepath.Join(tmpDir, "config.json")
		tomlFile := filepath.Join(tmpDir, "config.toml")

		_ = os.WriteFile(yamlFile, []byte("yaml: true\n"), 0o600)
		_ = os.WriteFile(jsonFile, []byte(`{"json": true}`), 0o600)
		_ = os.WriteFile(tomlFile, []byte("[toml]\nvalue = true"), 0o600)

		var mu sync.Mutex
		var updates []DirectoryConfigUpdate

		// Watch YAML and JSON, but not TOML
		watcher, err := WatchDirectory(tmpDir, DirectoryWatchOptions{
			Patterns: []string{"*.yaml", "*.json"},
		}, func(update DirectoryConfigUpdate) {
			mu.Lock()
			updates = append(updates, update)
			mu.Unlock()
		})
		if err != nil {
			t.Fatalf("WatchDirectory failed: %v", err)
		}
		defer func() {
			if err := watcher.Close(); err != nil {
				t.Logf("watcher.Close failed: %v", err)
			}
		}()

		time.Sleep(200 * time.Millisecond)

		mu.Lock()
		count := len(updates)
		mu.Unlock()

		// Should have yaml and json, not toml
		if count != 2 {
			t.Errorf("expected 2 updates (yaml + json), got %d", count)
		}
	})
}

func TestWatchDirectory_Subdirectories(t *testing.T) {
	t.Run("recursive_watches_subdirectories", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create subdirectory structure
		subDir := filepath.Join(tmpDir, "subdir")
		err := os.MkdirAll(subDir, 0o755)
		if err != nil {
			t.Fatalf("failed to create subdir: %v", err)
		}

		rootFile := filepath.Join(tmpDir, "root.yaml")
		subFile := filepath.Join(subDir, "sub.yaml")

		_ = os.WriteFile(rootFile, []byte("level: root\n"), 0o600)
		_ = os.WriteFile(subFile, []byte("level: sub\n"), 0o600)

		var mu sync.Mutex
		var updates []DirectoryConfigUpdate

		watcher, err := WatchDirectory(tmpDir, DirectoryWatchOptions{
			Patterns:  []string{"*.yaml"},
			Recursive: true,
		}, func(update DirectoryConfigUpdate) {
			mu.Lock()
			updates = append(updates, update)
			mu.Unlock()
		})
		if err != nil {
			t.Fatalf("WatchDirectory failed: %v", err)
		}
		defer func() {
			if err := watcher.Close(); err != nil {
				t.Logf("watcher.Close failed: %v", err)
			}
		}()

		time.Sleep(200 * time.Millisecond)

		mu.Lock()
		count := len(updates)
		mu.Unlock()

		// Should have both root and sub file
		if count != 2 {
			t.Errorf("expected 2 updates (root + sub), got %d", count)
		}
	})

	t.Run("non_recursive_ignores_subdirectories", func(t *testing.T) {
		tmpDir := t.TempDir()

		subDir := filepath.Join(tmpDir, "subdir")
		_ = os.MkdirAll(subDir, 0o755)

		rootFile := filepath.Join(tmpDir, "root.yaml")
		subFile := filepath.Join(subDir, "sub.yaml")

		_ = os.WriteFile(rootFile, []byte("level: root\n"), 0o600)
		_ = os.WriteFile(subFile, []byte("level: sub\n"), 0o600)

		var mu sync.Mutex
		var updates []DirectoryConfigUpdate

		watcher, err := WatchDirectory(tmpDir, DirectoryWatchOptions{
			Patterns:  []string{"*.yaml"},
			Recursive: false, // Explicitly non-recursive
		}, func(update DirectoryConfigUpdate) {
			mu.Lock()
			updates = append(updates, update)
			mu.Unlock()
		})
		if err != nil {
			t.Fatalf("WatchDirectory failed: %v", err)
		}
		defer func() {
			if err := watcher.Close(); err != nil {
				t.Logf("watcher.Close failed: %v", err)
			}
		}()

		time.Sleep(200 * time.Millisecond)

		mu.Lock()
		count := len(updates)
		mu.Unlock()

		// Should only have root file
		if count != 1 {
			t.Errorf("expected 1 update (only root), got %d", count)
		}
	})
}

func TestWatchDirectory_Security(t *testing.T) {
	t.Run("rejects_path_traversal", func(t *testing.T) {
		_, err := WatchDirectory("../../../etc", DirectoryWatchOptions{
			Patterns: []string{"*"},
		}, func(update DirectoryConfigUpdate) {})

		if err == nil {
			t.Error("expected error for path traversal attack")
		}
	})

	t.Run("validates_directory_exists", func(t *testing.T) {
		_, err := WatchDirectory("/nonexistent/path/to/nowhere", DirectoryWatchOptions{
			Patterns: []string{"*.yaml"},
		}, func(update DirectoryConfigUpdate) {})

		if err == nil {
			t.Error("expected error for nonexistent directory")
		}
	})

	t.Run("rejects_file_not_directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "notadir.txt")
		_ = os.WriteFile(filePath, []byte("content"), 0o600)

		_, err := WatchDirectory(filePath, DirectoryWatchOptions{
			Patterns: []string{"*.yaml"},
		}, func(update DirectoryConfigUpdate) {})

		if err == nil {
			t.Error("expected error when path is a file, not directory")
		}
	})
}

func TestWatchDirectory_MergedConfig(t *testing.T) {
	t.Run("provides_merged_config_from_all_files", func(t *testing.T) {
		tmpDir := t.TempDir()

		file1 := filepath.Join(tmpDir, "01-base.yaml")
		file2 := filepath.Join(tmpDir, "02-override.yaml")

		_ = os.WriteFile(file1, []byte("base_key: base_value\nshared: from_base\n"), 0o600)
		_ = os.WriteFile(file2, []byte("override_key: override_value\nshared: from_override\n"), 0o600)

		var mu sync.Mutex
		var mergedConfigs []map[string]interface{}

		watcher, err := WatchDirectoryMerged(tmpDir, DirectoryWatchOptions{
			Patterns: []string{"*.yaml"},
		}, func(merged map[string]interface{}, files []string) {
			mu.Lock()
			mergedConfigs = append(mergedConfigs, merged)
			mu.Unlock()
		})
		if err != nil {
			t.Fatalf("WatchDirectoryMerged failed: %v", err)
		}
		defer func() {
			if err := watcher.Close(); err != nil {
				t.Logf("watcher.Close failed: %v", err)
			}
		}()

		time.Sleep(200 * time.Millisecond)

		mu.Lock()
		if len(mergedConfigs) == 0 {
			mu.Unlock()
			t.Fatal("expected at least one merged config")
		}
		merged := mergedConfigs[len(mergedConfigs)-1]
		mu.Unlock()

		// Should have keys from both files
		if merged["base_key"] != "base_value" {
			t.Errorf("expected base_key=base_value, got %v", merged["base_key"])
		}
		if merged["override_key"] != "override_value" {
			t.Errorf("expected override_key=override_value, got %v", merged["override_key"])
		}
		// Later file (02-) should override earlier file (01-)
		if merged["shared"] != "from_override" {
			t.Errorf("expected shared=from_override (alphabetical order), got %v", merged["shared"])
		}
	})
}

// =============================================================================
// BENCHMARK
// =============================================================================

func BenchmarkWatchDirectory_Scan(b *testing.B) {
	tmpDir := b.TempDir()

	// Create 100 files
	for i := 0; i < 100; i++ {
		filePath := filepath.Join(tmpDir, "config"+string(rune('a'+i%26))+".yaml")
		_ = os.WriteFile(filePath, []byte("key: value\n"), 0o600)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		watcher, err := WatchDirectory(tmpDir, DirectoryWatchOptions{
			Patterns: []string{"*.yaml"},
		}, func(update DirectoryConfigUpdate) {})
		if err != nil {
			b.Fatal(err)
		}
		if err := watcher.Close(); err != nil {
			b.Logf("watcher.Close failed: %v", err)
		}
	}
}
