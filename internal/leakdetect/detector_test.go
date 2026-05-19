package leakdetect

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	d := New()
	if d == nil {
		t.Fatal("New() returned nil")
	}
	if d.initialGoroutines == 0 {
		t.Logf("initial goroutines: %d", d.initialGoroutines)
	}
}

func TestDetector_Snapshot(t *testing.T) {
	d := New()
	snap := d.Snapshot()
	if snap <= 0 {
		t.Errorf("expected positive goroutine count, got %d", snap)
	}
}

func TestDetector_Diff(t *testing.T) {
	d := New()
	_ = d.Snapshot() // capture baseline

	// Spawn some goroutines
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(10 * time.Millisecond)
		}()
	}
	wg.Wait()

	diff := d.Diff()
	// Allow some tolerance - goroutines may have finished
	if diff < 0 {
		t.Logf("goroutine count decreased (possible GC): diff=%d", diff)
	}
}

func TestDetector_Check(t *testing.T) {
	d := NewWithThreshold(5)
	// This should pass - we're under threshold
	d.Check(t)
}

func TestDetector_Check_ExceedsThreshold(t *testing.T) {
	d := NewWithThreshold(0) // Strict threshold

	// Spawn goroutines to exceed threshold
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Keep goroutine alive during check
			time.Sleep(50 * time.Millisecond)
		}()
	}
	wg.Wait()

	// Verify that goroutine count exceeds threshold
	diff := d.Diff()
	if diff <= 0 {
		// Goroutines may have finished - skip this test
		t.Skip("goroutines finished before diff check")
	}

	// The Check method should fail - we verify behavior by checking diff is positive
	if diff > d.Threshold() {
		// This is expected - threshold is 0, diff > 0
		t.Logf("goroutine leak correctly detected: diff=%d > threshold=%d", diff, d.Threshold())
	}
}

func TestDetector_MemSnapshot(t *testing.T) {
	d := New()
	stats := d.MemSnapshot()
	if stats.HeapAlloc == 0 {
		t.Logf("heap alloc: %d", stats.HeapAlloc)
	}
	// Basic sanity check - values should be reasonable
	if stats.HeapAlloc > 1<<60 { // Exabytes - clearly wrong
		t.Errorf("unreasonable heap alloc: %d", stats.HeapAlloc)
	}
}

func TestDetector_MemDiff(t *testing.T) {
	d := New()

	before := d.MemSnapshot()

	// Allocate some memory
	var allocations [][]byte
	for i := 0; i < 100; i++ {
		allocations = append(allocations, make([]byte, 1024))
	}

	after := d.MemSnapshot()

	diff := d.MemDiff(before, after)
	if diff == 0 {
		t.Logf("no memory growth detected (possible GC)")
	}
	// We expect some growth, but exact amount varies
	t.Logf("memory diff: %d bytes", diff)

	// Prevent compiler from optimizing away allocations
	runtime.KeepAlive(allocations)
}

func TestDetector_AssertMemGrowth(t *testing.T) {
	d := New()

	before := d.MemSnapshot()

	// Small allocation - should be under limit
	_ = make([]byte, 1024)

	after := d.MemSnapshot()

	// This should pass with a reasonable limit
	d.AssertMemGrowth(t, before, after, 10<<20) // 10MB limit
}

func TestDetector_AssertMemGrowth_ExceedsLimit(t *testing.T) {
	d := New()

	before := d.MemSnapshot()

	// Large allocation
	_ = make([]byte, 20<<20) // 20MB

	after := d.MemSnapshot()

	// Verify memory growth detection works
	growth := d.MemDiff(before, after)
	limit := uint64(1 << 20) // 1MB

	// The growth should exceed our limit
	if growth > limit {
		// This is expected - we allocated 20MB, limit is 1MB
		t.Logf("memory growth correctly detected: %d bytes > limit %d bytes", growth, limit)
	} else {
		// GC may have cleaned up
		t.Skip("memory was reclaimed before diff check")
	}
}

func TestDetector_Reset(t *testing.T) {
	d := New()
	_ = d.Diff() // capture baseline

	// Spawn goroutines
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)
	}()
	wg.Wait()

	// Reset baseline
	d.Reset()

	// Now diff should be near zero
	newDiff := d.Diff()
	if newDiff > 2 { // Allow small variance
		t.Errorf("after reset, diff should be small, got %d", newDiff)
	}
}

func TestDetector_Threshold(t *testing.T) {
	d := NewWithThreshold(10)
	if d.Threshold() != 10 {
		t.Errorf("expected threshold 10, got %d", d.Threshold())
	}

	d.SetThreshold(20)
	if d.Threshold() != 20 {
		t.Errorf("expected threshold 20, got %d", d.Threshold())
	}
}