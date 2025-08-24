// config_test.go: Testing Argus Configuration
//
// Copyright (c) 2025 AGILira
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"testing"
)

func TestSleepStrategy(t *testing.T) {
	t.Run("new_sleep_strategy", func(t *testing.T) {
		strategy := NewSleepStrategy()
		if strategy == nil {
			t.Error("NewSleepStrategy should not return nil")
		}
	})

	t.Run("wait_method", func(t *testing.T) {
		strategy := NewSleepStrategy()
		// Wait should not panic and should complete quickly
		strategy.Wait()
		// Test passes if no panic occurs
	})

	t.Run("reset_method", func(t *testing.T) {
		strategy := NewSleepStrategy()
		// Reset should not panic
		strategy.Reset()
		// Test passes if no panic occurs
	})

	t.Run("idle_strategy_interface", func(t *testing.T) {
		var idleStrategy IdleStrategy = NewSleepStrategy()

		// Test that it implements the interface correctly
		idleStrategy.Wait()
		idleStrategy.Reset()

		// Both should complete without error
	})
}
