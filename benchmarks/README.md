# Argus Ring Buffer Performance Benchmarks

This directory contains isolated performance benchmarks for the BoreasLite ring buffer implementation used in Argus. The benchmarks are separated from the main test suite to provide accurate performance measurements without interference from intensive unit tests.

## Benchmark Results

All benchmarks were executed on an 8-core Linux box, Go 1.25.

### Single Event Processing (Optimized Path)
```
BenchmarkBoreasLite_SingleEvent-8    49122489    24.90 ns/op    40.17 Mops/sec    0 B/op    0 allocs/op
```
- **Latency**: 24.90 nanoseconds to write and process one event
- **Throughput**: 40.17 million operations per second
- **Memory**: Zero allocations in hot path

## Technical Implementation

### Ring Buffer Architecture
- **Type**: Multiple Producer Single Consumer (MPSC)
- **Synchronization**: Lock-free atomic operations
- **Memory Layout**: Cache-line aligned, power-of-2 sizing
- **Event Size**: 128 bytes (2 cache lines)

### Optimization Strategies
1. **SingleEvent**: batch of 1, longest hot-spin window, for 1-2 files
2. **SmallBatch**: batch of 4, for 3-20 files
3. **LargeBatch**: batch of 16 with 4x unrolling, for 20+ files
4. **Light**: batch of 1, no spinning at all, for files that change rarely

Every strategy writes and processes an event in ~25 ns while events are
flowing, and parks when they stop; the first event after an idle period pays a
~7 us wakeup.

### Memory Characteristics
- **Zero allocations** in all hot paths
- **Fixed memory footprint**: 136 bytes × buffer_size (the event slot plus its availability marker)
- **Cache efficiency**: Power-of-2 ring buffer with atomic sequence numbers

## Running Benchmarks

Execute all benchmarks:
```bash
go test -bench="BenchmarkBoreasLite.*" -run=^$ -benchmem
```

Execute specific benchmark:
```bash
go test -bench=BenchmarkBoreasLite_SingleEvent -run=^$ -benchmem
```

Execute with multiple iterations:
```bash
go test -bench="BenchmarkBoreasLite.*" -run=^$ -benchmem -count=3
```

## Dependencies

- `github.com/agilira/argus`: Main library (via replace directive)
- `github.com/agilira/go-timecache`: High-performance timestamp caching

## Notes

- Benchmarks use minimal processing functions to isolate ring buffer performance
- All measurements include complete write-to-process cycles where applicable
- MPSC benchmarks use GOMAXPROCS concurrent producers
- Results represent sustainable performance under continuous load