package harness

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"time"
)

// AbortConfig holds configuration for abort conditions
type AbortConfig struct {
	NodeEndpoint      string
	MaxMemoryGrowthGB float64 // Max memory growth in GB
	MaxFinalityStall  time.Duration
	MaxFailureRate    float64 // Max acceptable failure rate
	DryRun            bool
}

// AbortCondition represents a specific abort condition
type AbortCondition struct {
	Name        string
	Description string
	Triggered   bool
	Details     string
}

// AbortChecker checks for abort conditions
type AbortChecker struct {
	cfg           AbortConfig
	logger        *slog.Logger
	initialMemory uint64
	lastBlockTime time.Time
	lastBlock     uint64
	conditions    []AbortCondition
}

// NewAbortChecker creates a new abort condition checker
func NewAbortChecker(cfg AbortConfig, logger *slog.Logger) *AbortChecker {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return &AbortChecker{
		cfg:           cfg,
		logger:        logger,
		initialMemory: m.Alloc,
		lastBlockTime: time.Now(),
		conditions:    make([]AbortCondition, 0),
	}
}

// Check evaluates all abort conditions
func (a *AbortChecker) Check(ctx context.Context, metrics *TestMetrics) []AbortCondition {
	a.conditions = a.conditions[:0] // Reset

	// Check memory growth
	if a.checkMemoryGrowth() {
		a.conditions = append(a.conditions, AbortCondition{
			Name:        "memory_growth",
			Description: "Memory growth exceeded threshold",
			Triggered:   true,
			Details:     fmt.Sprintf("current: %.2f GB, initial: %.2f GB", metrics.MemoryUsedGB, float64(a.initialMemory)/1e9),
		})
	}

	// Check finality stall
	if a.checkFinalityStall(metrics.CurrentBlock) {
		a.conditions = append(a.conditions, AbortCondition{
			Name:        "finality_stall",
			Description: "Finality has stalled",
			Triggered:   true,
			Details:     fmt.Sprintf("block %d stalled for %v", metrics.CurrentBlock, time.Since(a.lastBlockTime)),
		})
	}

	// Check failure rate
	if a.checkFailureRate(metrics.TotalSubmitted, metrics.TotalFailed) {
		a.conditions = append(a.conditions, AbortCondition{
			Name:        "high_failure_rate",
			Description: "Transaction failure rate too high",
			Triggered:   true,
			Details:     fmt.Sprintf("%.2f%% failure rate", metrics.FailureRate*100),
		})
	}

	// Check divergence (placeholder)
	if a.checkDivergence(ctx) {
		a.conditions = append(a.conditions, AbortCondition{
			Name:        "state_divergence",
			Description: "State has diverged from expected",
			Triggered:   true,
			Details:     "state root mismatch detected",
		})
	}

	// Update tracking
	a.lastBlock = metrics.CurrentBlock
	a.lastBlockTime = time.Now()

	return a.conditions
}

// checkMemoryGrowth checks if memory has grown too much
func (a *AbortChecker) checkMemoryGrowth() bool {
	if a.cfg.MaxMemoryGrowthGB == 0 {
		a.cfg.MaxMemoryGrowthGB = 2.0 // default 2GB
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	currentMemoryGB := float64(m.Alloc) / 1e9
	initialMemoryGB := float64(a.initialMemory) / 1e9
	growth := currentMemoryGB - initialMemoryGB

	return growth > a.cfg.MaxMemoryGrowthGB
}

// checkFinalityStall checks if finality has stalled
func (a *AbortChecker) checkFinalityStall(currentBlock uint64) bool {
	if a.cfg.MaxFinalityStall == 0 {
		a.cfg.MaxFinalityStall = 2 * time.Minute // default 2 minutes
	}

	if currentBlock == a.lastBlock {
		return time.Since(a.lastBlockTime) > a.cfg.MaxFinalityStall
	}

	return false
}

// checkFailureRate checks if failure rate is too high
func (a *AbortChecker) checkFailureRate(submitted, failed int64) bool {
	if a.cfg.MaxFailureRate == 0 {
		a.cfg.MaxFailureRate = 0.5 // default 50%
	}

	if submitted == 0 {
		return false
	}

	rate := float64(failed) / float64(submitted)
	return rate > a.cfg.MaxFailureRate
}

// checkDivergence checks for state divergence (stub)
func (a *AbortChecker) checkDivergence(ctx context.Context) bool {
	if a.cfg.DryRun {
		return false
	}

	// Stub: In real implementation, compare state roots
	// between expected and actual
	return false
}

// ShouldAbort returns true if any abort condition is triggered
func (a *AbortChecker) ShouldAbort(ctx context.Context, metrics *TestMetrics) bool {
	conditions := a.Check(ctx, metrics)
	return len(conditions) > 0
}

// GetConditions returns the current abort conditions
func (a *AbortChecker) GetConditions() []AbortCondition {
	return a.conditions
}

// LogConditions logs any triggered abort conditions
func (a *AbortChecker) LogConditions() {
	for _, c := range a.conditions {
		if c.Triggered {
			a.logger.Error("Abort condition triggered",
				"name", c.Name,
				"description", c.Description,
				"details", c.Details,
			)
		}
	}
}

// TestMetrics holds metrics for abort checking
type TestMetrics struct {
	TotalSubmitted int64
	TotalFailed    int64
	CurrentBlock   uint64
	MemoryUsedGB   float64
	FailureRate    float64
}
