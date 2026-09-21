// boreaslite_idle_test.go: the consumer loop must not burn CPU while idle.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"testing"
	"time"
)

// idleStrategies is every strategy RunProcessor dispatches on.
var idleStrategies = []struct {
	name string
	s    OptimizationStrategy
}{
	{"SingleEvent", OptimizationSingleEvent},
	{"SmallBatch", OptimizationSmallBatch},
	{"LargeBatch", OptimizationLargeBatch},
	{"Light", OptimizationLight},
	{"Auto", OptimizationAuto},
}

// TestBoreasLite_IdleConsumerStopsSpinning is the regression test for the idle
// spin: with no events at all, the consumer must exhaust its spin budget once
// and then block, not cycle through it forever.
//
// idle_spins counts consumer iterations that found no work. A parked consumer
// adds a handful per second (the backstop); a spinning one adds millions.
func TestBoreasLite_IdleConsumerStopsSpinning(t *testing.T) {
	const budget = 1000 // generous: the spinning implementation does ~10^6

	for _, tc := range idleStrategies {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBoreasLite(64, tc.s, func(*FileChangeEvent) {})
			defer b.Stop()
			go b.RunProcessor()

			// Let the processor burn its initial spin budget and settle.
			time.Sleep(300 * time.Millisecond)

			stats := b.Stats()
			before, ok := stats["idle_spins"]
			if !ok {
				t.Fatalf("Stats() has no idle_spins counter: %v", stats)
			}

			time.Sleep(400 * time.Millisecond)
			after := b.Stats()["idle_spins"]

			if got := after - before; got > budget {
				t.Errorf("idle consumer spun %d times in 400ms (budget %d): the loop is busy-waiting",
					got, budget)
			}
		})
	}
}

// TestBoreasLite_ParkedConsumerWakesOnWrite: parking must not cost latency.
// The bound is half the park backstop, so only the producer's signal can make
// this pass — and it is loose enough for a loaded CI runner (the measured
// wakeup is ~7us).
func TestBoreasLite_ParkedConsumerWakesOnWrite(t *testing.T) {
	for _, tc := range idleStrategies {
		t.Run(tc.name, func(t *testing.T) {
			got := make(chan time.Time, 1)
			b := NewBoreasLite(64, tc.s, func(*FileChangeEvent) {
				select {
				case got <- time.Now():
				default:
				}
			})
			defer b.Stop()
			go b.RunProcessor()

			// Deeply parked: past every spin and yield phase.
			time.Sleep(300 * time.Millisecond)

			start := time.Now()
			if !b.WriteFileChange("/tmp/idle-test.json", time.Now(), 42, false, false, true) {
				t.Fatal("WriteFileChange: buffer refused the event")
			}

			select {
			case at := <-got:
				if d := at.Sub(start); d > 100*time.Millisecond {
					t.Errorf("parked consumer took %v to pick up an event", d)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("parked consumer never picked up the event")
			}
		})
	}
}

// TestBoreasLite_StopWakesParkedConsumer: Stop must return the processor
// goroutine promptly even when it is blocked waiting for an event.
func TestBoreasLite_StopWakesParkedConsumer(t *testing.T) {
	for _, tc := range idleStrategies {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBoreasLite(64, tc.s, func(*FileChangeEvent) {})
			done := make(chan struct{})
			go func() {
				b.RunProcessor()
				close(done)
			}()

			time.Sleep(300 * time.Millisecond)

			start := time.Now()
			b.Stop()
			select {
			case <-done:
				if d := time.Since(start); d > 100*time.Millisecond {
					t.Errorf("RunProcessor took %v to return after Stop", d)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("RunProcessor did not return after Stop")
			}

			b.Stop() // idempotent
		})
	}
}

// TestBoreasLite_ParkedConsumerDrainsBurst: a burst written while the consumer
// is parked must arrive complete, not just its first event.
func TestBoreasLite_ParkedConsumerDrainsBurst(t *testing.T) {
	const events = 32

	for _, tc := range idleStrategies {
		t.Run(tc.name, func(t *testing.T) {
			seen := make(chan struct{}, events)
			b := NewBoreasLite(64, tc.s, func(*FileChangeEvent) { seen <- struct{}{} })
			defer b.Stop()
			go b.RunProcessor()

			time.Sleep(300 * time.Millisecond)

			for i := 0; i < events; i++ {
				if !b.WriteFileChange("/tmp/burst.json", time.Now(), int64(i), false, false, true) {
					t.Fatalf("WriteFileChange: buffer refused event %d", i)
				}
			}

			deadline := time.After(2 * time.Second)
			for i := 0; i < events; i++ {
				select {
				case <-seen:
				case <-deadline:
					t.Fatalf("only %d of %d events were delivered", i, events)
				}
			}
		})
	}
}
