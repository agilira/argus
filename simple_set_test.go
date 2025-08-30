// simple_set_test.go: Test semplice per il metodo Set
package argus

import (
	"testing"
)

func TestSimpleSet(t *testing.T) {
	config := NewConfigManager("test").
		StringFlag("host", "default", "Host")

	// Non parsare nulla, usa solo defaults
	err := config.Parse([]string{})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Valore di default
	if config.GetString("host") != "default" {
		t.Errorf("Expected default 'default', got %q", config.GetString("host"))
	}

	// Set esplicito
	config.Set("host", "explicit")

	// Dovrebbe avere precedenza
	if config.GetString("host") != "explicit" {
		t.Errorf("Expected explicit 'explicit', got %q", config.GetString("host"))
	}
}

func TestSetWithFlag(t *testing.T) {
	config := NewConfigManager("test").
		StringFlag("host", "default", "Host")

	// Parse con flag
	err := config.Parse([]string{"--host=from-flag"})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Valore dalla flag
	if config.GetString("host") != "from-flag" {
		t.Errorf("Expected from flag 'from-flag', got %q", config.GetString("host"))
	}

	// Set esplicito dovrebbe avere precedenza
	config.Set("host", "explicit")

	if config.GetString("host") != "explicit" {
		t.Errorf("Expected explicit 'explicit', got %q", config.GetString("host"))
	}
}
