# Argus Ring Buffer Performance Benchmarks

This directory contains isolated performance benchmarks for the BoreasLite ring buffer implementation used in Argus. The benchmarks are separated from the main test suite to provide accurate performance measurements without interference from intensive unit tests.

## Benchmark Results

All benchmarks were executed on AMD Ryzen 5 7520U.

### Single Event Processing (Optimized Path)
```
BenchmarkBoreasLite_SingleEvent-8    47011258    25.63 ns/op    39.02 Mops/sec    0 B/op    0 allocs/op
```
- **Latency**: 25.63 nanoseconds per operation
- **Throughput**: 39.02 million operations per second
- **Memory**: Zero allocations in hot path

### Write Operations
```
BenchmarkBoreasLite_WriteFileEvent-8    67764067    53.20 ns/op    18.80 Mops/sec    0 B/op    0 allocs/op
```
- **Latency**: 53.20 nanoseconds per operation
- **Throughput**: 18.80 million operations per second
- **Memory**: Zero allocations

### Multi-Producer Single Consumer (MPSC)
```
BenchmarkBoreasLite_MPSC-8    31231618    34.77 ns/op    28.76 Mops/sec    0 B/op    0 allocs/op
```
- **Latency**: 34.77 nanoseconds per operation under concurrent load
- **Throughput**: 28.76 million operations per second
- **Scalability**: Performance maintained across multiple producers

### Comparison with Go Channels

#### BoreasLite
```
BenchmarkBoreasLite_vsChannels/BoreasLite-8    23317405    45.72 ns/op    21.87 Mops/sec    0 B/op    0 allocs/op
```

#### Go Channels
```
BenchmarkBoreasLite_vsChannels/GoChannels-8    18621222    61.43 ns/op    16.28 Mops/sec    0 B/op    0 allocs/op
```

#### Performance Delta
- **BoreasLite**: 21.87 million ops/sec
- **Go Channels**: 16.28 million ops/sec
- **Improvement**: 34.3% faster than native Go channels

### High Throughput Sustained Load
```
BenchmarkBoreasLite_HighThroughput-8    27012850    53.88 ns/op    18.56 Mops/sec    0 B/op    0 allocs/op
```
- **Sustained throughput**: 18.56 million operations per second
- **Buffer size**: 8192 events
- **Strategy**: Large batch optimization

## Technical Implementation

### Ring Buffer Architecture
- **Type**: Multiple Producer Single Consumer (MPSC)
- **Synchronization**: Lock-free atomic operations
- **Memory Layout**: Cache-line aligned, power-of-2 sizing
- **Event Size**: 128 bytes (2 cache lines)

### Optimization Strategies
1. **SingleEvent**: Ultra-low latency for 1-2 files (25ns)
2. **SmallBatch**: Balanced performance for 3-20 files
3. **LargeBatch**: High throughput for 20+ files with 4x unrolling

### Memory Characteristics
- **Zero allocations** in all hot paths
- **Fixed memory footprint**: 8KB + (128 bytes × buffer_size)
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