package profiling

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"
)

// MemoryStats captures memory usage statistics at a point in time.
type MemoryStats struct {
	HeapAlloc    uint64 `json:"heap_alloc"`     // Bytes allocated and not yet freed
	HeapSys      uint64 `json:"heap_sys"`       // Bytes obtained from system
	HeapIdle     uint64 `json:"heap_idle"`      // Bytes in idle spans
	HeapInuse    uint64 `json:"heap_inuse"`     // Bytes in non-idle spans
	HeapReleased uint64 `json:"heap_released"`  // Bytes released to OS
	HeapObjects  uint64 `json:"heap_objects"`   // Total number of allocated objects
	StackInuse   uint64 `json:"stack_inuse"`    // Bytes used by stack spans
	MSpanInuse   uint64 `json:"mspan_inuse"`    // Bytes used by mspan structures
	MCacheInuse  uint64 `json:"mcache_inuse"`   // Bytes used by mcache structures
	BuckHashSys  uint64 `json:"buckhash_sys"`   // Bytes used by bucket hash table
	GCSys        uint64 `json:"gc_sys"`         // Bytes used for garbage collection metadata
	NextGC       uint64 `json:"next_gc"`        // Next GC target heap size
	LastGC       uint64 `json:"last_gc"`        // Last GC time (nanoseconds since epoch)
	PauseTotalNs uint64 `json:"pause_total_ns"` // Total GC pause time
	NumGC        uint32 `json:"num_gc"`         // Number of GC cycles
	NumGoroutine int    `json:"num_goroutine"`  // Current number of goroutines
	Timestamp    int64  `json:"timestamp"`      // Unix timestamp when stats were captured
}

// MemoryDelta shows the change in memory usage between two snapshots.
type MemoryDelta struct {
	AllocDelta     int64  `json:"alloc_delta"`     // Change in heap allocation
	ObjectsDelta   int64  `json:"objects_delta"`   // Change in number of objects
	GoroutineDelta int    `json:"goroutine_delta"` // Change in goroutines
	GCDelta        uint32 `json:"gc_delta"`        // Number of GC cycles
	DurationNs     int64  `json:"duration_ns"`     // Duration between snapshots
	LeakedObjects  uint64 `json:"leaked_objects"`  // Objects that increased since baseline
}

// MemoryProfiler tracks memory usage over time.
type MemoryProfiler struct {
	mu           sync.Mutex
	baseline     *MemoryStats
	snapshots    []*MemoryStats
	maxSnapshots int
}

// NewMemoryProfiler creates a new memory profiler.
func NewMemoryProfiler() *MemoryProfiler {
	return &MemoryProfiler{
		snapshots:    make([]*MemoryStats, 0, 100),
		maxSnapshots: 100,
	}
}

// CaptureStats captures current memory statistics.
func CaptureStats() *MemoryStats {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return &MemoryStats{
		HeapAlloc:    memStats.HeapAlloc,
		HeapSys:      memStats.HeapSys,
		HeapIdle:     memStats.HeapIdle,
		HeapInuse:    memStats.HeapInuse,
		HeapReleased: memStats.HeapReleased,
		HeapObjects:  memStats.HeapObjects,
		StackInuse:   memStats.StackInuse,
		MSpanInuse:   memStats.MSpanInuse,
		MCacheInuse:  memStats.MCacheInuse,
		BuckHashSys:  memStats.BuckHashSys,
		GCSys:        memStats.GCSys,
		NextGC:       memStats.NextGC,
		LastGC:       memStats.LastGC,
		PauseTotalNs: memStats.PauseTotalNs,
		NumGC:        memStats.NumGC,
		NumGoroutine: runtime.NumGoroutine(),
		Timestamp:    time.Now().UnixNano(),
	}
}

// SetBaseline sets the baseline memory stats for comparison.
func (p *MemoryProfiler) SetBaseline(stats *MemoryStats) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.baseline = stats
}

// GetBaseline returns the baseline memory stats.
func (p *MemoryProfiler) GetBaseline() *MemoryStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.baseline == nil {
		p.baseline = CaptureStats()
	}
	return p.baseline
}

// AddSnapshot adds a memory snapshot.
func (p *MemoryProfiler) AddSnapshot(stats *MemoryStats) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.snapshots = append(p.snapshots, stats)

	// Keep only the most recent snapshots
	if len(p.snapshots) > p.maxSnapshots {
		p.snapshots = p.snapshots[1:]
	}
}

// GetSnapshots returns all memory snapshots.
func (p *MemoryProfiler) GetSnapshots() []*MemoryStats {
	p.mu.Lock()
	defer p.mu.Unlock()

	snapshots := make([]*MemoryStats, len(p.snapshots))
	copy(snapshots, p.snapshots)
	return snapshots
}

// CalculateDelta calculates the memory delta between baseline and current stats.
func (p *MemoryProfiler) CalculateDelta(current *MemoryStats) *MemoryDelta {
	baseline := p.GetBaseline()

	return &MemoryDelta{
		AllocDelta:     int64(current.HeapAlloc) - int64(baseline.HeapAlloc),
		ObjectsDelta:   int64(current.HeapObjects) - int64(baseline.HeapObjects),
		GoroutineDelta: current.NumGoroutine - baseline.NumGoroutine,
		GCDelta:        current.NumGC - baseline.NumGC,
		DurationNs:     current.Timestamp - baseline.Timestamp,
	}
}

// DetectPotentialLeaks identifies potential memory leaks by comparing with baseline.
func (p *MemoryProfiler) DetectPotentialLeaks(current *MemoryStats) []string {
	baseline := p.GetBaseline()
	var leaks []string

	// Check if heap allocation has grown significantly
	allocGrowth := float64(current.HeapAlloc-baseline.HeapAlloc) / float64(baseline.HeapAlloc)
	if allocGrowth > 0.5 && current.HeapAlloc > 1024*1024 { // 50% growth and > 1MB
		leaks = append(leaks, fmt.Sprintf("Heap allocation grew by %.1f%% (from %d to %d bytes)",
			allocGrowth*100, baseline.HeapAlloc, current.HeapAlloc))
	}

	// Check if number of objects has grown significantly
	objectGrowth := float64(current.HeapObjects-baseline.HeapObjects) / float64(baseline.HeapObjects)
	if objectGrowth > 0.3 && current.HeapObjects > 1000 { // 30% growth and > 1000 objects
		leaks = append(leaks, fmt.Sprintf("Object count grew by %.1f%% (from %d to %d)",
			objectGrowth*100, baseline.HeapObjects, current.HeapObjects))
	}

	// Check for goroutine leaks
	if current.NumGoroutine > baseline.NumGoroutine*2 && current.NumGoroutine > 100 {
		leaks = append(leaks, fmt.Sprintf("Goroutine count doubled: %d -> %d",
			baseline.NumGoroutine, current.NumGoroutine))
	}

	return leaks
}

// ProfileOperation profiles a function's memory usage.
func (p *MemoryProfiler) ProfileOperation(fn func()) *MemoryDelta {
	before := CaptureStats()
	p.SetBaseline(before)

	fn()

	after := CaptureStats()
	p.AddSnapshot(after)

	return p.CalculateDelta(after)
}

// ForceGC triggers garbage collection and returns stats after GC.
func ForceGC() *MemoryStats {
	runtime.GC()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	stats := CaptureStats()
	return stats
}

// ProfileWithMemory profiles a function with periodic memory sampling.
func ProfileWithMemory(ctx context.Context, interval time.Duration, fn func(ctx context.Context) error) (*MemoryProfiler, error) {
	profiler := NewMemoryProfiler()
	profiler.SetBaseline(CaptureStats())

	// Start background sampling
	done := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				profiler.AddSnapshot(CaptureStats())
			case <-done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	// Run the function
	err := fn(ctx)

	// Signal completion and wait for sampler
	close(done)
	wg.Wait()

	// Add final snapshot
	profiler.AddSnapshot(CaptureStats())

	return profiler, err
}

// FormatBytes formats a byte count as human-readable string.
func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// PrintStats prints memory stats in a human-readable format.
func PrintStats(stats *MemoryStats) {
	fmt.Printf("Memory Statistics:\n")
	fmt.Printf("  Heap Alloc:    %s\n", FormatBytes(stats.HeapAlloc))
	fmt.Printf("  Heap Sys:      %s\n", FormatBytes(stats.HeapSys))
	fmt.Printf("  Heap Inuse:    %s\n", FormatBytes(stats.HeapInuse))
	fmt.Printf("  Heap Objects:  %d\n", stats.HeapObjects)
	fmt.Printf("  Goroutines:    %d\n", stats.NumGoroutine)
	fmt.Printf("  GC Cycles:     %d\n", stats.NumGC)
	if stats.LastGC > 0 {
		fmt.Printf("  Last GC:       %s\n", time.Unix(0, int64(stats.LastGC)))
	}
}
