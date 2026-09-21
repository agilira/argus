# Argus BoreasLite: Spin, Yield, Park

## Summary

BoreasLite is a lock-free MPSC ring buffer with a single consumer goroutine.
This document describes how that consumer waits for work: it spins briefly for
latency, yields for a while to stay friendly, and then **blocks** until a
producer wakes it.

The result is a watcher that costs nothing measurable while idle and picks up
an event in tens of nanoseconds while events are flowing.

## Architecture Overview

```go
// One loop, parameterised per strategy.
func (b *BoreasLite) RunProcessor() {
    policy := spinPolicyFor(b.strategy)
    for b.running.Load() {
        if b.ProcessBatch() > 0 { /* reset, stay hot */ }
        // phase 1: spin   phase 2: Gosched   phase 3: park
    }
}
```

| Phase | Range | Behaviour |
|---|---|---|
| 1. Hot spinning | `0..spinLimit` | Pure busy-wait. Sub-microsecond pickup. |
| 2. Progressive yielding | `spinLimit..yieldLimit` | `runtime.Gosched()` every `yieldMask+1` iterations. |
| 3. Park | beyond `yieldLimit` | Block on a channel until a producer signals. |

### Thresholds by strategy

| Strategy | spinLimit | yieldLimit | yield every |
|---|---|---|---|
| `OptimizationSingleEvent` | 5000 | 10000 | 4 |
| `OptimizationSmallBatch` | 2000 | 6000 | 4 |
| `OptimizationLargeBatch` | 1000 | 4000 | 16 |
| `OptimizationAuto` (default) | 2000 | 8000 | 8 |
| `OptimizationLight` | 0 | 0 | — |

`OptimizationLight` parks on the first empty iteration; it is the strategy for
configuration files that change every few hours.

## The wakeup protocol

The consumer parks; producers wake it. The whole cost on the producer's side is
one atomic load:

```go
// producer, after publishing the event
if b.parked.Load() {
    b.signalConsumer()   // non-blocking send on a 1-slot channel
}

// consumer, before blocking
b.parked.Store(true)
if b.hasWork() { return }   // a producer that published before the flag
select {                    // was set would not have signalled us
case <-b.wake:
case <-b.stopCh:
case <-timer.C:             // backstop, 200ms
}
```

**Why the re-check is enough.** Go's `sync/atomic` operations are sequentially
consistent, so all of them are in one total order. Take a producer whose
`parked.Load()` returns false — it sends no signal. That load precedes the
consumer's `parked.Store(true)` in the total order, which precedes the
consumer's re-check of the cursors, which therefore observes the event the
producer had already published. The consumer does not park. The mirror case,
`parked.Load()` returning true, signals a 1-slot buffered channel, so a token
left for a consumer that has not blocked yet is still there when it does.

**The backstop.** A parked consumer times out after 200ms and re-checks. The
protocol above is not supposed to need it; it bounds the damage to 200ms should
the reasoning be wrong on some platform, and costs five wakeups a second.

**Shutdown.** `Stop()` closes `stopCh`, so a parked consumer returns at once
rather than waiting out the backstop; it then drains what is left in the ring.
`Stop()` is idempotent.

## Measurements

8-core Linux box, `PollInterval: time.Hour`, one watched file, zero events.
Read against the measurement floor: a test process with no watcher at all
reports ~5% of a core on this machine.

| Strategy | Idle CPU |
|---|---|
| SingleEvent | 5.0% |
| SmallBatch | 5.3% |
| LargeBatch | 5.8% |
| Light | 5.4% |
| Auto (the default) | 5.5% |
| *(no watcher at all)* | *4.9%* |

Every strategy sits at the floor: an idle watcher costs nothing measurable.

### Wakeup latency

Time from `WriteFileChange` to the consumer callback, consumer deeply parked,
20 samples per strategy:

| Strategy | min | median | max |
|---|---|---|---|
| SingleEvent | 3.6µs | 6.9µs | 10.5µs |
| SmallBatch | 5.1µs | 6.6µs | 28.6µs |
| LargeBatch | 4.7µs | 6.2µs | 17.8µs |
| Light | 4.5µs | 7.2µs | 23.8µs |
| Auto | 4.9µs | 6.7µs | 31.8µs |

### Throughput

The producer pays one atomic load to find out whether anyone is parked:

```
BenchmarkBoreasLite_SingleEvent-8        24.4 ns/op    0 allocs/op
BenchmarkBoreasLite_WriteFileEvent-8     12.6 ns/op    0 allocs/op
BenchmarkBoreasLite_MPSC-8               16.3 ns/op    0 allocs/op
```

## Observability

`Stats()` reports `idle_spins`: consumer iterations that found no work. A
parked consumer adds a handful per second — one per backstop expiry — so the
counter is a direct read on whether the consumer is blocking as it should.
`TestBoreasLite_IdleConsumerStopsSpinning` asserts on it.

## References

- [Go Memory Model](https://go.dev/ref/mem) — sequential consistency of `sync/atomic`
- [LMAX Disruptor](https://lmax-exchange.github.io/disruptor/) — the spin/yield/block wait strategies
- [Lock-Free Programming Patterns](https://www.cs.rochester.edu/~scott/papers/1996_PODC_queues.pdf)
