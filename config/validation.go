package config

import "fmt"

// ValidateConfig validates the configuration and returns descriptive errors.
// Port validation: must be in range 1024-65535.
// MaxPeers: must be greater than 0.
// DataDir: must be non-empty if persistence is needed.
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return ErrConfigValidationFailed.Wrap(fmt.Errorf("configuration is nil"))
	}

	// Validate P2P config
	if cfg.P2P.Port != 0 {
		if cfg.P2P.Port < 1024 || cfg.P2P.Port > 65535 {
			return &ConfigError{
				Code:    ErrCodeInvalidPort,
				Message: fmt.Sprintf("P2P port %d is out of valid range (1024-65535)", cfg.P2P.Port),
			}
		}
	}

	if cfg.P2P.MaxPeers <= 0 {
		return &ConfigError{
			Code:    ErrCodeInvalidMaxPeers,
			Message: fmt.Sprintf("P2P max peers %d must be greater than 0", cfg.P2P.MaxPeers),
		}
	}

	// Validate RPC config
	if cfg.RPC.Port < 1024 || cfg.RPC.Port > 65535 {
		return &ConfigError{
			Code:    ErrCodeInvalidPort,
			Message: fmt.Sprintf("RPC port %d is out of valid range (1024-65535)", cfg.RPC.Port),
		}
	}

	// Validate Metrics config
	if cfg.Metrics.Port != 0 {
		if cfg.Metrics.Port < 1024 || cfg.Metrics.Port > 65535 {
			return &ConfigError{
				Code:    ErrCodeInvalidPort,
				Message: fmt.Sprintf("Metrics port %d is out of valid range (1024-65535)", cfg.Metrics.Port),
			}
		}
	}

	// Validate Storage config - DataDir is optional, but if set must be non-empty
	// However, if persistence is needed (indexer, snapshots), DataDir should be set
	if cfg.Chain.IndexerEnabled && cfg.Storage.DataDir == "" {
		return &ConfigError{
			Code:    ErrCodeDataDirRequired,
			Message: "Indexer is enabled but DataDir is not set",
		}
	}

	// Validate Snapshot config
	if cfg.Snapshot.Enable && cfg.Storage.DataDir == "" && cfg.Snapshot.OutputDir == "" {
		// This is a warning but not an error - snapshots will just be disabled
	}

	// Validate Chain config
	if cfg.Chain.MempoolMaxSize <= 0 {
		return &ConfigError{
			Code:    ErrCodeInvalidMaxPeers,
			Message: fmt.Sprintf("Mempool max size %d must be greater than 0", cfg.Chain.MempoolMaxSize),
		}
	}

	if cfg.Chain.MaxTxPerBlock <= 0 {
		return &ConfigError{
			Code:    ErrCodeInvalidMaxPeers,
			Message: fmt.Sprintf("Max transactions per block %d must be greater than 0", cfg.Chain.MaxTxPerBlock),
		}
	}

	// Validate Validator config
	if cfg.Validator.CommissionRate > 10000 {
		return &ConfigError{
			Code:    ErrCodeValidationFailed,
			Message: fmt.Sprintf("Commission rate %d exceeds maximum (10000 = 100%%)", cfg.Validator.CommissionRate),
		}
	}

	return nil
}
