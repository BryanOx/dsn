package harness

import (
	"context"
	"fmt"
	"log/slog"
)

// VerifierConfig holds configuration for state verification
type VerifierConfig struct {
	NodeEndpoint       string
	VerificationBlocks int // How often to verify (every N blocks)
	DryRun             bool
}

// VerificationResult holds the result of state verification
type VerificationResult struct {
	BlockHeight    uint64
	StateRootValid bool
	MempoolDrained bool
	FinalityOK     bool
	Errors         []string
}

// Verifier performs continuous state verification
type Verifier struct {
	cfg      VerifierConfig
	logger   *slog.Logger
	lastHash []byte
}

// NewVerifier creates a new state verifier
func NewVerifier(cfg VerifierConfig, logger *slog.Logger) *Verifier {
	return &Verifier{
		cfg:    cfg,
		logger: logger,
	}
}

// Verify performs state verification at the given block height
func (v *Verifier) Verify(ctx context.Context, blockHeight uint64) (*VerificationResult, error) {
	if v.cfg.DryRun {
		return &VerificationResult{
			BlockHeight:    blockHeight,
			StateRootValid: true,
			MempoolDrained: true,
			FinalityOK:     true,
			Errors:         nil,
		}, nil
	}

	result := &VerificationResult{
		BlockHeight: blockHeight,
	}

	// Verify state root consistency
	if err := v.verifyStateRoot(ctx, blockHeight); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("state root: %v", err))
		result.StateRootValid = false
	} else {
		result.StateRootValid = true
	}

	// Verify mempool drain (mempool should be empty or low)
	if err := v.verifyMempool(ctx); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("mempool: %v", err))
		result.MempoolDrained = false
	} else {
		result.MempoolDrained = true
	}

	// Verify finality (no forks, blocks are finalized)
	if err := v.verifyFinality(ctx, blockHeight); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("finality: %v", err))
		result.FinalityOK = false
	} else {
		result.FinalityOK = true
	}

	return result, nil
}

// verifyStateRoot checks state root consistency
func (v *Verifier) verifyStateRoot(ctx context.Context, blockHeight uint64) error {
	// Stub: In real implementation, query node for state root
	// and compare against expected value
	// For now, simulate success
	return nil
}

// verifyMempool checks if mempool is drained
func (v *Verifier) verifyMempool(ctx context.Context) error {
	// Stub: In real implementation, query node's mempool size
	// Should be near zero or below threshold
	// For now, simulate success
	return nil
}

// verifyFinality checks for fork detection and finality
func (v *Verifier) verifyFinality(ctx context.Context, blockHeight uint64) error {
	// Stub: In real implementation:
	// 1. Query last N blocks' hashes
	// 2. Verify they're in canonical chain
	// 3. Check no competing forks exist
	// For now, simulate success
	return nil
}

// VerifyBlockRange verifies a range of blocks
func (v *Verifier) VerifyBlockRange(ctx context.Context, startHeight, endHeight uint64) ([]*VerificationResult, error) {
	results := make([]*VerificationResult, 0, endHeight-startHeight+1)

	for height := startHeight; height <= endHeight; height++ {
		result, err := v.Verify(ctx, height)
		if err != nil {
			return nil, fmt.Errorf("verify block %d: %w", height, err)
		}
		results = append(results, result)
	}

	return results, nil
}

// ShouldVerify determines if verification should run at this block height
func (v *Verifier) ShouldVerify(blockHeight uint64) bool {
	if v.cfg.VerificationBlocks == 0 {
		v.cfg.VerificationBlocks = 10 // default
	}
	return blockHeight%uint64(v.cfg.VerificationBlocks) == 0
}

// LogResult logs the verification result
func (v *Verifier) LogResult(result *VerificationResult) {
	if len(result.Errors) > 0 {
		v.logger.Warn("Verification issues found",
			"block", result.BlockHeight,
			"stateRootValid", result.StateRootValid,
			"mempoolDrained", result.MempoolDrained,
			"finalityOK", result.FinalityOK,
			"errors", result.Errors,
		)
	} else {
		v.logger.Debug("Verification passed",
			"block", result.BlockHeight,
		)
	}
}
