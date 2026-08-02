package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// TestResults holds the final test results
type TestResults struct {
	Duration         time.Duration
	TotalSubmitted   int64
	TotalFailed      int64
	AverageTPS       float64
	FinalBlockHeight uint64
	PeakTPS          int
	ErrorCount       int
}

// Reporter collects and reports metrics
type Reporter struct {
	logger    *slog.Logger
	startTime time.Time
	// Counters
	totalSubmitted atomic.Int64
	totalFailed    atomic.Int64
	currentBlock   atomic.Uint64
	// Tracking
	submissionTimes []time.Time
	mu              sync.Mutex
	// Results
	results TestResults
}

// NewReporter creates a new metrics reporter
func NewReporter(logger *slog.Logger) *Reporter {
	return &Reporter{
		logger:          logger,
		submissionTimes: make([]time.Time, 0),
	}
}

// Start starts metrics collection
func (r *Reporter) Start(ctx context.Context) {
	r.startTime = time.Now()
	r.logger.Info("Metrics collection started")
}

// RecordSubmission records a successful transaction submission
func (r *Reporter) RecordSubmission(count int, blockHeight uint64) {
	r.totalSubmitted.Add(int64(count))
	r.currentBlock.Store(blockHeight)

	r.mu.Lock()
	r.submissionTimes = append(r.submissionTimes, time.Now())
	r.mu.Unlock()
}

// RecordFailure records a transaction failure
func (r *Reporter) RecordFailure() {
	r.totalFailed.Add(1)
}

// GetCurrentTPS calculates current TPS (last N seconds)
func (r *Reporter) GetCurrentTPS(windowSeconds int) float64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-time.Duration(windowSeconds) * time.Second)

	count := 0
	for i := len(r.submissionTimes) - 1; i >= 0; i-- {
		if r.submissionTimes[i].After(cutoff) {
			count++
		} else {
			break
		}
	}

	return float64(count) / float64(windowSeconds)
}

// GetAverageTPS calculates average TPS since start
func (r *Reporter) GetAverageTPS() float64 {
	elapsed := time.Since(r.startTime)
	if elapsed == 0 {
		return 0
	}
	return float64(r.totalSubmitted.Load()) / elapsed.Seconds()
}

// GetPeakTPS returns the peak TPS observed
func (r *Reporter) GetPeakTPS() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Calculate peak over 1-second windows
	if len(r.submissionTimes) < 2 {
		return int(r.totalSubmitted.Load())
	}

	// Simple window-based peak calculation
	windowSize := time.Second
	maxCount := 0
	currentCount := 0
	windowStart := r.submissionTimes[0]

	for _, t := range r.submissionTimes {
		if t.Sub(windowStart) < windowSize {
			currentCount++
		} else {
			if currentCount > maxCount {
				maxCount = currentCount
			}
			currentCount = 1
			windowStart = t
		}
	}

	if currentCount > maxCount {
		maxCount = currentCount
	}

	return maxCount
}

// GetResults returns the final test results
func (r *Reporter) GetResults() TestResults {
	r.results = TestResults{
		Duration:         time.Since(r.startTime),
		TotalSubmitted:   r.totalSubmitted.Load(),
		TotalFailed:      r.totalFailed.Load(),
		AverageTPS:       r.GetAverageTPS(),
		FinalBlockHeight: r.currentBlock.Load(),
		PeakTPS:          r.GetPeakTPS(),
	}
	return r.results
}

// FormatSummary formats a summary string
func (r *Reporter) FormatSummary() string {
	results := r.GetResults()
	return fmt.Sprintf(`=== Soak Test Results ===
Duration:         %v
Total Submitted:   %d
Total Failed:      %d
Success Rate:      %.2f%%
Average TPS:       %.2f
Peak TPS:          %d
Final Block:       %d
`, results.Duration, results.TotalSubmitted, results.TotalFailed,
		100*(float64(results.TotalSubmitted-results.TotalFailed)/float64(results.TotalSubmitted)),
		results.AverageTPS, results.PeakTPS, results.FinalBlockHeight)
}

// PrintSummary prints the summary to stdout
func (r *Reporter) PrintSummary() {
	fmt.Print(r.FormatSummary())
}

// Reset resets all metrics
func (r *Reporter) Reset() {
	r.totalSubmitted.Store(0)
	r.totalFailed.Store(0)
	r.currentBlock.Store(0)
	r.submissionTimes = r.submissionTimes[:0]
	r.startTime = time.Now()
}
