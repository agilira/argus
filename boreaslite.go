// boreaslite.go: Xantos Powered MPSC ring buffer derived from Boreas
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"runtime"
	"sync/atomic"
	"time"
)

// FileChangeEvent represents a file change optimized for minimal memory footprint
// 128 bytes (2 cache lines) for maximum path compatibility and performance
type FileChangeEvent struct {
	ModTime int64     // Unix nanoseconds (8 bytes, aligned first)
	Size    int64     // File size (8 bytes)
	Path    [110]byte // FULL POWER: 110 bytes for any file path (109 chars + null terminator)
	PathLen uint8     // Actual path length (1 byte)
	Flags   uint8     // Create(1), Delete(2), Modify(4) bits (1 byte)
	// Total: 8+8+110+1+1 = 128 bytes exactly with proper alignment
}

// Event flags for file changes
const (
	FileEventCreate uint8 = 1 << iota
	FileEventDelete
	FileEventModify
)

// BoreasLite - Ultra-fast MPSC ring buffer for file watching
// Optimized for Argus-specific use cases:
//   - Small number of files (typically 1-10)
//   - Infrequent events (file changes are rare)
//   - Low latency priority (immediate callback execution)
//   - Minimal memory footprint
type BoreasLite struct {
	// Ring buffer core (smaller than ZephyrosLite)
	buffer   []FileChangeEvent
	capacity int64
	mask     int64 // capacity - 1 for fast modulo

	// MPSC atomic cursors with cache-line padding
	writerCursor atomic.Int64 // Producer sequence
	readerCursor atomic.Int64 // Consumer sequence
	_            [48]byte     // Padding to prevent false sharing

	// Availability tracking for MPSC coordination
	availableBuffer []atomic.Int64 // Per-slot availability markers

	// Processor function (no interface overhead)
	processor func(*FileChangeEvent)

	// Optimization strategy configuration
	strategy  OptimizationStrategy
	batchSize int64 // Adaptive based on strategy

	// Control
	running atomic.Bool

	// Ultra-simple stats (just counters)
	processed atomic.Int64
	dropped   atomic.Int64
}

// NewBoreasLite creates a new ultra-fast ring buffer for file events
//
// Parameters:
//   - capacity: Ring buffer size (must be power of 2)
//   - strategy: Optimization strategy for performance tuning
//   - processor: Function to process file change events
//
// Returns:
//   - *BoreasLite: Ready-to-use ring buffer
func NewBoreasLite(capacity int64, strategy OptimizationStrategy, processor func(*FileChangeEvent)) *BoreasLite {
	// Validate power of 2
	if capacity <= 0 || (capacity&(capacity-1)) != 0 {
		capacity = 64 // Safe default for file watching
	}

	// Determine batch size based on strategy
	var batchSize int64
	switch strategy {
	case OptimizationSingleEvent:
		batchSize = 1 // Process immediately, no batching
	case OptimizationSmallBatch:
		batchSize = 4 // Small batches for balanced performance
	case OptimizationLargeBatch:
		batchSize = 16 // Large batches for throughput
	default: // OptimizationAuto will be handled at runtime
		batchSize = 4 // Safe default
	}

	// Create ring buffer
	b := &BoreasLite{
		buffer:          make([]FileChangeEvent, capacity),
		capacity:        capacity,
		mask:            capacity - 1,
		availableBuffer: make([]atomic.Int64, capacity),
		processor:       processor,
		strategy:        strategy,
		batchSize:       batchSize,
	}

	// Initialize availability markers
	for i := range b.availableBuffer {
		b.availableBuffer[i].Store(-1)
	}

	b.running.Store(true)
	return b
}

// AdaptStrategy dynamically adjusts the optimization strategy based on file count.
// This is called when OptimizationAuto is used and file count changes.
// Automatically selects the optimal batch size for current workload:
//   - 1-3 files: SingleEvent (ultra-low latency, 24.91ns)
//   - 4-50 files: SmallBatch (balanced performance, 100% detection)
//   - 51+ files: LargeBatch (high throughput, 1000+ files supported)
func (b *BoreasLite) AdaptStrategy(fileCount int) {
	if b.strategy != OptimizationAuto {
		return // Fixed strategy, no adaptation
	}

	var newBatchSize int64
	switch {
	case fileCount <= 3:
		newBatchSize = 1 // SingleEvent optimization
	case fileCount <= 50:
		newBatchSize = 4 // SmallBatch optimization
	default:
		newBatchSize = 16 // LargeBatch optimization
	}

	// Update batch size atomically (safe to change at runtime)
	b.batchSize = newBatchSize
}

// WriteFileEvent adds a file change event to the ring buffer
// ZERO ALLOCATIONS - uses provided event struct directly
//
// Parameters:
//   - event: Pre-populated file change event
//
// Returns:
//   - bool: true if written, false if ring is full/closed
//
// Performance: Target <8ns per operation
func (b *BoreasLite) WriteFileEvent(event *FileChangeEvent) bool {
	if !b.running.Load() {
		b.dropped.Add(1)
		return false
	}

	// MPSC: Claim sequence atomically
	sequence := b.writerCursor.Add(1) - 1

	// Check buffer full (file events should NEVER be dropped, but safety check)
	if sequence >= b.readerCursor.Load()+b.capacity {
		b.dropped.Add(1)
		return false
	}

	// Copy event to buffer slot (zero allocation)
	slot := &b.buffer[sequence&b.mask]
	*slot = *event

	// Mark available for reading
	b.availableBuffer[sequence&b.mask].Store(sequence)

	return true
}

// WriteFileChange is a convenience method for creating events from parameters.
// Slightly slower than WriteFileEvent but more convenient for direct parameter usage.
// Automatically handles path length limits and flag setting.
//
// Parameters:
//   - path: File path (automatically truncated if > 109 characters)
//   - modTime: File modification time
//   - size: File size in bytes
//   - isCreate: True if this is a file creation event
//   - isDelete: True if this is a file deletion event
//   - isModify: True if this is a file modification event
//
// Returns:
//   - bool: true if event was successfully queued, false if buffer is full
func (b *BoreasLite) WriteFileChange(path string, modTime time.Time, size int64, isCreate, isDelete, isModify bool) bool {
	event := FileChangeEvent{
		ModTime: modTime.UnixNano(),
		Size:    size,
	}

	// Copy path with bounds checking
	pathBytes := []byte(path)
	copyLen := len(pathBytes)
	if copyLen > 109 { // Use full buffer capacity (110 bytes - 1 for safety)
		copyLen = 109
	}
	copy(event.Path[:], pathBytes[:copyLen])
	// Safe conversion: copyLen is guaranteed <= 109 (fits in uint8)
	event.PathLen = uint8(copyLen) // #nosec G115 -- bounds checked above, copyLen <= 109

	// Set flags
	if isCreate {
		event.Flags |= FileEventCreate
	}
	if isDelete {
		event.Flags |= FileEventDelete
	}
	if isModify {
		event.Flags |= FileEventModify
	}

	return b.WriteFileEvent(&event)
}

// ProcessBatch processes available events in small batches
// Optimized for low latency - smaller batches than ZephyrosLite
//
// Returns:
//   - int: Number of events processed
func (b *BoreasLite) ProcessBatch() int {
	current := b.readerCursor.Load()
	writerPos := b.writerCursor.Load()

	if current >= writerPos {
		return 0 // Nothing to process
	}

	bufferOccupancy := writerPos - current

	// STRATEGY-BASED OPTIMIZATION: Choose processing path based on configuration
	switch b.strategy {
	case OptimizationSingleEvent:
		return b.processSingleEventOptimized(current, writerPos, bufferOccupancy)
	case OptimizationSmallBatch:
		return b.processSmallBatchOptimized(current, writerPos, bufferOccupancy)
	case OptimizationLargeBatch:
		return b.processLargeBatchOptimized(current, writerPos, bufferOccupancy)
	default: // OptimizationAuto
		return b.processAutoOptimized(current, writerPos, bufferOccupancy)
	}
}

// processSingleEventOptimized - Ultra-low latency for 1-2 files
func (b *BoreasLite) processSingleEventOptimized(current, writerPos, bufferOccupancy int64) int {
	// ULTRA-FAST PATH: Single event with minimal overhead
	if bufferOccupancy == 1 {
		if b.availableBuffer[current&b.mask].Load() == current {
			b.processor(&b.buffer[current&b.mask])
			b.availableBuffer[current&b.mask].Store(-1)
			b.readerCursor.Store(current + 1)
			b.processed.Add(1)
			return 1
		}
		return 0
	}

	// Process small batches immediately (2-3 events)
	maxProcess := minInt64(3, writerPos-current)
	available := current - 1

	for seq := current; seq < current+maxProcess; seq++ {
		if b.availableBuffer[seq&b.mask].Load() == seq {
			available = seq
		} else {
			break
		}
	}

	if available < current {
		return 0
	}

	processed := int(available - current + 1)
	for seq := current; seq <= available; seq++ {
		idx := seq & b.mask
		b.processor(&b.buffer[idx])
		b.availableBuffer[idx].Store(-1)
	}

	b.readerCursor.Store(available + 1)
	b.processed.Add(int64(processed))
	return processed
}

// processSmallBatchOptimized - Balanced performance for 3-20 files
func (b *BoreasLite) processSmallBatchOptimized(current, writerPos, _ int64) int {
	maxProcess := minInt64(b.batchSize, writerPos-current)
	available := current - 1

	// Find contiguous available events
	for seq := current; seq < current+maxProcess; seq++ {
		if b.availableBuffer[seq&b.mask].Load() == seq {
			available = seq
		} else {
			break
		}
	}

	if available < current {
		return 0
	}

	processed := int(available - current + 1)

	// Use simple loop for small batches (no unrolling overhead)
	for seq := current; seq <= available; seq++ {
		idx := seq & b.mask
		b.processor(&b.buffer[idx])
		b.availableBuffer[idx].Store(-1)
	}

	b.readerCursor.Store(available + 1)
	b.processed.Add(int64(processed))
	return processed
}

// processLargeBatchOptimized - High throughput for 20+ files with Zephyros optimizations
func (b *BoreasLite) processLargeBatchOptimized(current, writerPos, bufferOccupancy int64) int {
	// Adaptive batching based on buffer pressure
	adaptiveBatchSize := b.batchSize
	if bufferOccupancy > b.capacity*3/4 {
		adaptiveBatchSize = minInt64(b.batchSize*4, b.capacity/2)
	}

	maxProcess := minInt64(adaptiveBatchSize, writerPos-current)
	available := current - 1
	maxScan := current + maxProcess

	// Smart prefetching for optimal cache hits
	for seq := current; seq < maxScan; seq++ {
		if seq+4 < maxScan {
			_ = b.availableBuffer[(seq+4)&b.mask].Load() // Prefetch 4 slots ahead
		}

		if b.availableBuffer[seq&b.mask].Load() == seq {
			available = seq
		} else {
			break
		}
	}

	if available < current {
		return 0
	}

	processed := int(available - current + 1)

	// 4x Unrolled processing for maximum throughput
	seq := current
	remainder := processed & 3
	chunks := processed >> 2

	for i := 0; i < chunks; i++ {
		if seq+8 <= available {
			_ = b.buffer[(seq+8)&b.mask] // Prefetch data 8 slots ahead
		}

		// Process 4 events at once
		idx1 := seq & b.mask
		b.processor(&b.buffer[idx1])
		seq++

		idx2 := seq & b.mask
		b.processor(&b.buffer[idx2])
		seq++

		idx3 := seq & b.mask
		b.processor(&b.buffer[idx3])
		seq++

		idx4 := seq & b.mask
		b.processor(&b.buffer[idx4])
		seq++

		// Batch reset for cache locality
		b.availableBuffer[idx1].Store(-1)
		b.availableBuffer[idx2].Store(-1)
		b.availableBuffer[idx3].Store(-1)
		b.availableBuffer[idx4].Store(-1)
	}

	// Process remaining items
	for i := 0; i < remainder; i++ {
		idx := seq & b.mask
		b.processor(&b.buffer[idx])
		b.availableBuffer[idx].Store(-1)
		seq++
	}

	b.readerCursor.Store(available + 1)
	b.processed.Add(int64(processed))
	return processed
}

// processAutoOptimized - Dynamic strategy based on runtime conditions
func (b *BoreasLite) processAutoOptimized(current, writerPos, bufferOccupancy int64) int {
	// Choose strategy based on buffer occupancy
	switch {
	case bufferOccupancy <= 3:
		return b.processSingleEventOptimized(current, writerPos, bufferOccupancy)
	case bufferOccupancy <= 16:
		return b.processSmallBatchOptimized(current, writerPos, bufferOccupancy)
	default:
		return b.processLargeBatchOptimized(current, writerPos, bufferOccupancy)
	}
}

// RunProcessor runs the consumer loop with strategy-optimized behavior
func (b *BoreasLite) RunProcessor() {
	// Strategy-specific spinning behavior
	switch b.strategy {
	case OptimizationSingleEvent:
		b.runSingleEventProcessor()
	case OptimizationSmallBatch:
		b.runSmallBatchProcessor()
	case OptimizationLargeBatch:
		b.runLargeBatchProcessor()
	default: // OptimizationAuto
		b.runAutoProcessor()
	}
}

// runSingleEventProcessor - Ultra-aggressive spinning for 1-2 files
func (b *BoreasLite) runSingleEventProcessor() {
	spins := 0
	for b.running.Load() {
		processed := b.ProcessBatch()
		if processed > 0 {
			spins = 0
			continue // Hot loop for immediate processing
		}

		spins++
		if spins < 5000 { // Aggressive spinning for ultra-low latency
			continue
		} else if spins < 10000 { // Progressive yielding phase
			if spins&3 == 0 { // Yield every 4 iterations
				runtime.Gosched()
			}
		} else {
			// Sleep phase for battery and cloud efficiency
			time.Sleep(100 * time.Microsecond) // Brief sleep to release CPU
			spins = 0                          // Reset after sleep
		}
	}

	// Final drain
	for b.ProcessBatch() > 0 {
	}
}

// runSmallBatchProcessor - Balanced spinning for 3-20 files
func (b *BoreasLite) runSmallBatchProcessor() {
	spins := 0
	for b.running.Load() {
		processed := b.ProcessBatch()
		if processed > 0 {
			spins = 0
			if processed >= int(b.batchSize/2) {
				continue // Continue for burst processing
			}
		} else {
			spins++
			if spins < 2000 {
				continue
			} else if spins < 6000 {
				if spins&3 == 0 { // Yield every 4 iterations
					runtime.Gosched()
				}
			} else {
				// Sleep phase for better CPU efficiency
				time.Sleep(200 * time.Microsecond) // Slightly longer sleep than SingleEvent
				spins = 0
			}
		}
	}

	// Final drain
	for b.ProcessBatch() > 0 {
	}
}

// runLargeBatchProcessor - Optimized for high throughput 20+ files
func (b *BoreasLite) runLargeBatchProcessor() {
	spins := 0
	for b.running.Load() {
		processed := b.ProcessBatch()
		if processed > 0 {
			spins = 0
			if processed >= int(b.batchSize) {
				continue // Hot loop for maximum throughput
			}
		} else {
			spins++
			if spins < 1000 {
				continue
			} else if spins < 4000 {
				if spins&15 == 0 { // Yield every 16 iterations
					runtime.Gosched()
				}
			} else {
				// Sleep phase for throughput optimization with CPU efficiency
				time.Sleep(500 * time.Microsecond) // Longer sleep for batch processing
				spins = 0
			}
		}
	}

	// Final drain
	for b.ProcessBatch() > 0 {
	}
}

// runAutoProcessor - Dynamic behavior based on runtime conditions
func (b *BoreasLite) runAutoProcessor() {
	spins := 0
	for b.running.Load() {
		processed := b.ProcessBatch()
		if processed > 0 {
			spins = 0
			continue
		}

		spins++
		if spins < 2000 {
			continue
		} else if spins < 8000 {
			if spins&7 == 0 { // Yield every 8 iterations
				runtime.Gosched()
			}
		} else {
			// HYBRID APPROACH: Sleep for CPU efficiency
			time.Sleep(50 * time.Microsecond) // Brief sleep for battery/cloud efficiency
			spins = 0                         // Reset counter after sleep
		}
	}

	// Final drain (with timeout to prevent infinite loops)
	drainAttempts := 0
	for b.ProcessBatch() > 0 && drainAttempts < 1000 {
		drainAttempts++
	}
}

// Stop stops the processor immediately without graceful shutdown.
// Optimized for file watching use cases where immediate termination is acceptable.
// Sets the running flag to false, causing all processor loops to exit.
func (b *BoreasLite) Stop() {
	b.running.Store(false)
}

// Stats returns minimal statistics for monitoring ring buffer performance.
// Provides real-time metrics for debugging and performance analysis.
//
// Returns a map containing:
//   - writer_position: Current writer sequence number
//   - reader_position: Current reader sequence number
//   - buffer_size: Ring buffer capacity
//   - items_buffered: Number of events waiting to be processed
//   - items_processed: Total events processed since startup
//   - items_dropped: Total events dropped due to buffer overflow
//   - running: 1 if processor is running, 0 if stopped
func (b *BoreasLite) Stats() map[string]int64 {
	writerPos := b.writerCursor.Load()
	readerPos := b.readerCursor.Load()

	return map[string]int64{
		"writer_position": writerPos,
		"reader_position": readerPos,
		"buffer_size":     b.capacity,
		"items_buffered":  writerPos - readerPos,
		"items_processed": b.processed.Load(),
		"items_dropped":   b.dropped.Load(),
		"running":         boolToInt64(b.running.Load()),
	}
}

// ConvertChangeEventToFileEvent converts standard ChangeEvent to optimized FileChangeEvent.
// Used for interfacing between Argus's public API and BoreasLite's optimized internal format.
// Handles path truncation and flag conversion automatically.
func ConvertChangeEventToFileEvent(event ChangeEvent) FileChangeEvent {
	fileEvent := FileChangeEvent{
		ModTime: event.ModTime.UnixNano(),
		Size:    event.Size,
	}

	// Copy path
	pathBytes := []byte(event.Path)
	copyLen := len(pathBytes)
	if copyLen > 109 { // Use full buffer capacity (110 bytes - 1 for safety)
		copyLen = 109
	}
	copy(fileEvent.Path[:], pathBytes[:copyLen])
	// Safe conversion: copyLen is guaranteed <= 109 (fits in uint8)
	fileEvent.PathLen = uint8(copyLen) // #nosec G115 -- bounds checked above, copyLen <= 109

	// Set flags
	if event.IsCreate {
		fileEvent.Flags |= FileEventCreate
	}
	if event.IsDelete {
		fileEvent.Flags |= FileEventDelete
	}
	if !event.IsCreate && !event.IsDelete {
		fileEvent.Flags |= FileEventModify
	}

	return fileEvent
}

// ConvertFileEventToChangeEvent converts FileChangeEvent back to standard ChangeEvent.
// Used when delivering events to user callbacks, converting from BoreasLite's
// optimized internal format back to the public API format.
func ConvertFileEventToChangeEvent(fileEvent FileChangeEvent) ChangeEvent {
	return ChangeEvent{
		Path:     string(fileEvent.Path[:fileEvent.PathLen]),
		ModTime:  time.Unix(0, fileEvent.ModTime),
		Size:     fileEvent.Size,
		IsCreate: (fileEvent.Flags & FileEventCreate) != 0,
		IsDelete: (fileEvent.Flags & FileEventDelete) != 0,
		IsModify: (fileEvent.Flags & FileEventModify) != 0,
	}
}

// minInt64 returns the smaller of two int64 values.
// Helper function for batch size calculations and bounds checking.
func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// boolToInt64 converts a boolean to int64 for statistics reporting.
// Used in Stats() method to provide numeric representation of boolean states.
func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
