package workload

import (
	"context"
	"time"
)

// BurstConfig holds configuration for burst workload
type BurstConfig struct {
	NumAccounts      int
	BaseTPS          int
	BurstMultiplier  int
	BurstDuration    time.Duration
	CooldownDuration time.Duration
	ChainID          uint32
	MaxFee           uint64
	GasLimit         uint64
}

// burstGenerator generates burst traffic pattern
type burstGenerator struct {
	cfg             BurstConfig
	transferGen     Generator
	currentTPS      int
	inBurst         bool
	burstStartTime  time.Time
	cooldownEndTime time.Time
}

// NewBurstGenerator creates a new burst workload generator
func NewBurstGenerator(cfg interface{}) Generator {
	c, ok := cfg.(BurstConfig)
	if !ok {
		c = BurstConfig{
			NumAccounts:      100,
			BaseTPS:          20,
			BurstMultiplier:  5,
			BurstDuration:    30 * time.Second,
			CooldownDuration: 120 * time.Second,
			ChainID:          1,
			MaxFee:           10000,
			GasLimit:         50000,
		}
	}

	transferCfg := TransferConfig{
		NumAccounts: c.NumAccounts,
		TPS:         c.BaseTPS,
		ChainID:     c.ChainID,
		MaxFee:      c.MaxFee,
		GasLimit:    c.GasLimit,
	}

	return &burstGenerator{
		cfg:         c,
		transferGen: NewTransferGenerator(transferCfg),
		currentTPS:  c.BaseTPS,
		inBurst:     false,
	}
}

func init() {
	Register("burst", NewBurstGenerator)
}

// Name returns the name of the generator
func (g *burstGenerator) Name() string {
	return "burst"
}

// Generate creates transactions based on current burst state
func (g *burstGenerator) Generate(ctx context.Context, blockHeight uint64) ([]Transaction, error) {
	now := time.Now()

	// Check if we should be in burst mode
	if g.inBurst && now.After(g.burstStartTime.Add(g.cfg.BurstDuration)) {
		// End of burst, start cooldown
		g.inBurst = false
		g.cooldownEndTime = now.Add(g.cfg.CooldownDuration)
		g.currentTPS = g.cfg.BaseTPS
	} else if !g.inBurst && now.After(g.cooldownEndTime) {
		// End of cooldown, start burst
		g.inBurst = true
		g.burstStartTime = now
		g.currentTPS = g.cfg.BaseTPS * g.cfg.BurstMultiplier
	}

	// Generate transactions at current TPS rate
	// Note: actual rate limiting is handled by the harness
	txs, err := g.transferGen.Generate(ctx, blockHeight)
	if err != nil {
		return nil, err
	}

	// Return only the number of transactions for current TPS
	if len(txs) > g.currentTPS {
		txs = txs[:g.currentTPS]
	}

	return txs, nil
}

// GetCurrentTPS returns the current target TPS
func (g *burstGenerator) GetCurrentTPS() int {
	return g.currentTPS
}

// IsInBurst returns whether the generator is currently in burst mode
func (g *burstGenerator) IsInBurst() bool {
	return g.inBurst
}

// Reset resets the burst state
func (g *burstGenerator) Reset() {
	g.inBurst = false
	g.currentTPS = g.cfg.BaseTPS
	g.burstStartTime = time.Time{}
	g.cooldownEndTime = time.Time{}
}
