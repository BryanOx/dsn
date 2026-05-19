package leakdetect

import (
	"runtime"
	"sync"
	"testing"
)

// Detector tracks goroutine count and memory usage to detect resource leaks.
type Detector struct {
	initialGoroutines int
	threshold         int
	mu                 sync.Mutex
}

// New creates a new Detector that captures the current goroutine count.
// Default threshold is 0 - any goroutines created after detection are considered a leak.
func New() *Detector {
	return NewWithThreshold(0)
}

// NewWithThreshold creates a new Detector with a custom threshold.
// Threshold is the maximum allowed increase in goroutine count.
func NewWithThreshold(threshold int) *Detector {
	return &Detector{
		initialGoroutines: runtime.NumGoroutine(),
		threshold:         threshold,
	}
}

// Check compares the current goroutine count to the initial count.
// If the difference exceeds the threshold, it fails the test.
func (d *Detector) Check(t testing.TB) {
	current := d.Snapshot()
	diff := current - d.initialGoroutines
	if diff > d.threshold {
		t.Fatalf("goroutine leak detected: initial=%d, current=%d, diff=%d (threshold=%d)",
			d.initialGoroutines, current, diff, d.threshold)
	}
}

// Snapshot returns the current goroutine count.
func (d *Detector) Snapshot() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return runtime.NumGoroutine()
}

// Diff returns the difference between current and initial goroutine count.
func (d *Detector) Diff() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return runtime.NumGoroutine() - d.initialGoroutines
}

// SetThreshold updates the leak detection threshold.
func (d *Detector) SetThreshold(threshold int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.threshold = threshold
}

// Threshold returns the current threshold value.
func (d *Detector) Threshold() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.threshold
}

// MemSnapshot captures current memory statistics.
func (d *Detector) MemSnapshot() runtime.MemStats {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats
}

// MemDiff returns the difference in heap allocation (in bytes) between two MemStats.
func (d *Detector) MemDiff(before, after runtime.MemStats) uint64 {
	if after.HeapAlloc >= before.HeapAlloc {
		return after.HeapAlloc - before.HeapAlloc
	}
	// Handle case where GC caused allocation to decrease
	return 0
}

// AssertMemGrowth fails the test if memory growth exceeds the limit (in bytes).
func (d *Detector) AssertMemGrowth(t testing.TB, before, after runtime.MemStats, limit uint64) {
	growth := d.MemDiff(before, after)
	if growth > limit {
		t.Fatalf("excessive memory growth: %d bytes (limit: %d bytes)", growth, limit)
	}
}

// Reset re-captures the current goroutine count as the new baseline.
func (d *Detector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.initialGoroutines = runtime.NumGoroutine()
}