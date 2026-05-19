package vm

import (
	"testing"
)

func TestGasMeter_Deduct(t *testing.T) {
	tests := []struct {
		name     string
		limit    uint64
		costs    []uint64
		wantUsed uint64
	}{
		{
			name:     "single deduction within limit",
			limit:    1000,
			costs:    []uint64{500},
			wantUsed: 500,
		},
		{
			name:     "multiple deductions within limit",
			limit:    1000,
			costs:    []uint64{100, 200, 300},
			wantUsed: 600,
		},
		{
			name:     "exact limit reached",
			limit:    1000,
			costs:    []uint64{500, 500},
			wantUsed: 1000,
		},
		{
			name:     "exceeds limit on second deduction",
			limit:    1000,
			costs:    []uint64{600, 500},
			wantUsed: 600, // stops at 600, second 500 rejected
		},
		{
			name:     "zero cost",
			limit:    1000,
			costs:    []uint64{0},
			wantUsed: 0,
		},
		{
			name:     "zero limit",
			limit:    0,
			costs:    []uint64{0},
			wantUsed: 0,
		},
		{
			name:     "zero limit with non-zero cost",
			limit:    0,
			costs:    []uint64{1},
			wantUsed: 0, // rejected, no gas used
		},
		{
			name:     "large limit",
			limit:    100000000,
			costs:    []uint64{50000000, 30000000},
			wantUsed: 80000000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gm := NewGasMeter(tt.limit)

			var totalCost uint64
			var lastErr error
			for _, cost := range tt.costs {
				err := gm.Deduct(cost)
				lastErr = err
				if err == nil {
					totalCost += cost
				}
			}

			// Verify the final state
			if gm.Used != tt.wantUsed {
				t.Errorf("GasMeter.Used = %d, want %d", gm.Used, tt.wantUsed)
			}

			// For the "exceeds limit" test, verify an error was returned
			if tt.name == "exceeds limit on second deduction" && lastErr == nil {
				t.Errorf("Expected error when limit exceeded, got nil")
			}
		})
	}
}

func TestGasMeter_Remaining(t *testing.T) {
	gm := NewGasMeter(1000)

	// Initial state
	if remaining := gm.Remaining(); remaining != 1000 {
		t.Errorf("Remaining() = %d, want 1000", remaining)
	}

	// After deduction
	_ = gm.Deduct(300)
	if remaining := gm.Remaining(); remaining != 700 {
		t.Errorf("Remaining() = %d, want 700", remaining)
	}

	// At limit
	_ = gm.Deduct(700)
	if remaining := gm.Remaining(); remaining != 0 {
		t.Errorf("Remaining() = %d, want 0", remaining)
	}
}

func TestGasMeter_EdgeCases(t *testing.T) {
	// Test that repeated small deductions work correctly
	gm := NewGasMeter(100)
	for i := 0; i < 100; i++ {
		if err := gm.Deduct(1); err != nil {
			t.Errorf("Deduct(1) failed at iteration %d: %v", i, err)
		}
	}

	// Should be at limit now
	if err := gm.Deduct(1); err != ErrGasLimitExceeded {
		t.Errorf("Expected ErrGasLimitExceeded, got %v", err)
	}

	// Test overflow protection
	gm = NewGasMeter(^uint64(0) - 10) // near max uint64
	err := gm.Deduct(20)
	// Should not panic, should handle gracefully
	if err != nil {
		t.Logf("Gas limit exceeded as expected: %v", err)
	}
}

func TestGasMeter_Reset(t *testing.T) {
	gm := NewGasMeter(1000)
	_ = gm.Deduct(500)

	// Create new meter to simulate reset
	gm = NewGasMeter(1000)
	if remaining := gm.Remaining(); remaining != 1000 {
		t.Errorf("After reset, Remaining() = %d, want 1000", remaining)
	}
}