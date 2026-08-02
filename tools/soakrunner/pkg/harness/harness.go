package harness

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dsn/dsn/tools/soakrunner/pkg/metrics"
	"github.com/dsn/dsn/tools/soakrunner/pkg/workload"
)

// HarnessConfig holds configuration for the soak harness
type HarnessConfig struct {
	NodeEndpoint string
	Duration     time.Duration
	TPS          int
	Profile      string
	LogInterval  time.Duration
	DryRun       bool
}

// Harness orchestrates the soak test
type Harness struct {
	cfg       HarnessConfig
	generator workload.Generator
	reporter  *metrics.Reporter
	logger    *slog.Logger
	// Metrics
	totalSubmitted atomic.Int64
	totalFailed    atomic.Int64
	currentBlock   atomic.Uint64
	// State
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewHarness creates a new soak harness
func NewHarness(cfg HarnessConfig, logger *slog.Logger) (*Harness, error) {
	// Create generator based on profile
	gen, err := createGenerator(cfg.Profile, cfg.TPS)
	if err != nil {
		return nil, err
	}

	reporter := metrics.NewReporter(logger)

	return &Harness{
		cfg:       cfg,
		generator: gen,
		reporter:  reporter,
		logger:    logger,
	}, nil
}

// createGenerator creates the appropriate generator based on profile
func createGenerator(profile string, tps int) (workload.Generator, error) {
	switch profile {
	case "light":
		gen, err := workload.Get("light", workload.TransferConfig{
			NumAccounts: 10,
			TPS:         10,
			ChainID:     1,
		})
		return gen, err
	case "moderate":
		gen, err := workload.Get("moderate", workload.TransferConfig{
			NumAccounts: 100,
			TPS:         tps,
			ChainID:     1,
		})
		return gen, err
	case "heavy":
		gen, err := workload.Get("heavy", workload.TransferConfig{
			NumAccounts: 500,
			TPS:         tps,
			ChainID:     1,
		})
		return gen, err
	case "burst":
		gen, err := workload.Get("burst", workload.BurstConfig{
			NumAccounts:      100,
			BaseTPS:          20,
			BurstMultiplier:  5,
			BurstDuration:    30 * time.Second,
			CooldownDuration: 120 * time.Second,
			ChainID:          1,
		})
		return gen, err
	case "wasm":
		gen, err := workload.Get("wasm", workload.WASMConfig{
			NumAccounts: 100,
			TPS:         tps,
			ChainID:     1,
		})
		return gen, err
	case "mixed":
		gen, err := workload.Get("mixed", workload.MixedConfig{
			NumAccounts:   100,
			TPS:           tps,
			ChainID:       1,
			TransferRatio: 0.7,
			WASMRatio:     0.3,
		})
		return gen, err
	default:
		gen, err := workload.Get("transfer", workload.TransferConfig{
			NumAccounts: 100,
			TPS:         tps,
			ChainID:     1,
		})
		return gen, err
	}
}

// Run starts the soak test
func (h *Harness) Run(ctx context.Context) error {
	h.ctx, h.cancel = context.WithCancel(ctx)
	defer h.cancel()

	h.logger.Info("Starting soak test",
		"profile", h.cfg.Profile,
		"duration", h.cfg.Duration,
		"tps", h.cfg.TPS,
		"endpoint", h.cfg.NodeEndpoint,
		"dryRun", h.cfg.DryRun,
	)

	if h.cfg.DryRun {
		return h.runDryRun()
	}

	return h.runReal()
}

// runDryRun simulates what would happen without actually connecting
func (h *Harness) runDryRun() error {
	h.logger.Info("DRY RUN MODE - No actual transactions will be submitted")

	ticker := time.NewTicker(h.cfg.LogInterval)
	defer ticker.Stop()

	startTime := time.Now()
	for {
		select {
		case <-h.ctx.Done():
			return h.ctx.Err()
		case <-ticker.C:
			elapsed := time.Since(startTime)
			h.logger.Info("Dry run progress",
				"elapsed", elapsed,
				"profile", h.generator.Name(),
			)

			if elapsed >= h.cfg.Duration {
				return nil
			}
		}
	}
}

// runReal runs the actual soak test
func (h *Harness) runReal() error {
	// Start metrics collection
	h.reporter.Start(h.ctx)

	// Create worker pool for transaction submission
	numWorkers := 4
	h.wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		go h.worker(i)
	}

	// Progress logging
	ticker := time.NewTicker(h.cfg.LogInterval)
	defer ticker.Stop()

	startTime := time.Now()
	lastBlock := uint64(0)

	for {
		select {
		case <-h.ctx.Done():
			h.wg.Wait()
			return h.ctx.Err()
		case <-ticker.C:
			elapsed := time.Since(startTime)
			h.logger.Info("Progress",
				"elapsed", elapsed,
				"submitted", h.totalSubmitted.Load(),
				"failed", h.totalFailed.Load(),
				"currentBlock", h.currentBlock.Load(),
			)

			// Check abort conditions
			if h.shouldAbort(lastBlock, elapsed) {
				h.logger.Warn("Abort conditions met, stopping test")
				h.cancel()
				h.wg.Wait()
				return nil
			}
			lastBlock = h.currentBlock.Load()

			// Check if duration reached
			if elapsed >= h.cfg.Duration {
				h.cancel()
				h.wg.Wait()
				return nil
			}
		}
	}
}

// worker is a goroutine that generates and submits transactions
func (h *Harness) worker(id int) {
	defer h.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			// Generate transactions
			txs, err := h.generator.Generate(h.ctx, h.currentBlock.Load())
			if err != nil {
				h.logger.Error("Generate error", "worker", id, "error", err)
				continue
			}

			// Submit transactions (stub for now)
			for _, tx := range txs {
				if err := h.submitTransaction(tx); err != nil {
					h.totalFailed.Add(1)
				} else {
					h.totalSubmitted.Add(1)
				}
			}

			// Update metrics
			h.reporter.RecordSubmission(len(txs), h.currentBlock.Load())
		}
	}
}

// submitTransaction submits a transaction to the node (stub)
func (h *Harness) submitTransaction(tx workload.Transaction) error {
	// Stub: In real implementation, this would connect to the node
	// via RPC/grpc and submit the transaction
	if h.cfg.DryRun {
		return nil
	}

	// TODO: Implement actual RPC submission
	// For now, simulate successful submission
	return nil
}

// shouldAbort checks if abort conditions are met
func (h *Harness) shouldAbort(lastBlock uint64, elapsed time.Duration) bool {
	currentBlock := h.currentBlock.Load()

	// Check for stalled block production
	if lastBlock > 0 && currentBlock == lastBlock && elapsed > 2*time.Minute {
		h.logger.Warn("Block production stalled")
		return true
	}

	// Check for high failure rate
	if h.totalSubmitted.Load() > 100 {
		failureRate := float64(h.totalFailed.Load()) / float64(h.totalSubmitted.Load())
		if failureRate > 0.5 {
			h.logger.Warn("High failure rate", "rate", failureRate)
			return true
		}
	}

	return false
}

// GetResults returns the test results
func (h *Harness) GetResults() metrics.TestResults {
	return h.reporter.GetResults()
}

// PrintSummary prints the final summary
func (h *Harness) PrintSummary() {
	results := h.GetResults()
	// If duration is unrealistic (before start time), use configured duration
	if results.Duration < 0 || results.Duration.Hours() > 24000 {
		results.Duration = h.cfg.Duration
	}
	fmt.Println("\n=== Soak Test Summary ===")
	fmt.Printf("Profile: %s\n", h.cfg.Profile)
	fmt.Printf("Duration: %v\n", results.Duration)
	fmt.Printf("Total Submitted: %d\n", results.TotalSubmitted)
	fmt.Printf("Total Failed: %d\n", results.TotalFailed)
	fmt.Printf("Average TPS: %.2f\n", results.AverageTPS)
	fmt.Printf("Final Block Height: %d\n", results.FinalBlockHeight)
	fmt.Println("=========================")
}
