package config

import (
	"testing"
)

// TestValidateConfig_InvalidPorts tests that ValidateConfig rejects
// configurations with invalid port numbers.
func TestValidateConfig_InvalidPorts(t *testing.T) {
	tests := []struct {
		name        string
		port        int
		setPort     func(*Config, int)
		expectError bool
	}{
		{
			name:        "P2P port 0",
			port:        0,
			setPort:     func(c *Config, p int) { c.P2P.Port = p },
			expectError: false, // 0 means disabled, which is valid
		},
		{
			name:        "P2P port 80",
			port:        80,
			setPort:     func(c *Config, p int) { c.P2P.Port = p },
			expectError: true, // below 1024
		},
		{
			name:        "P2P port 1023",
			port:        1023,
			setPort:     func(c *Config, p int) { c.P2P.Port = p },
			expectError: true, // below 1024
		},
		{
			name:        "P2P port 1024",
			port:        1024,
			setPort:     func(c *Config, p int) { c.P2P.Port = p },
			expectError: false, // minimum valid
		},
		{
			name:        "P2P port 65535",
			port:        65535,
			setPort:     func(c *Config, p int) { c.P2P.Port = p },
			expectError: false, // maximum valid
		},
		{
			name:        "P2P port 65536",
			port:        65536,
			setPort:     func(c *Config, p int) { c.P2P.Port = p },
			expectError: true, // above 65535
		},
		{
			name:        "RPC port 80",
			port:        80,
			setPort:     func(c *Config, p int) { c.RPC.Port = p },
			expectError: true, // below 1024
		},
		{
			name:        "RPC port 0",
			port:        0,
			setPort:     func(c *Config, p int) { c.RPC.Port = p },
			expectError: true, // RPC must be > 0
		},
		{
			name:        "Metrics port 0",
			port:        0,
			setPort:     func(c *Config, p int) { c.Metrics.Port = p },
			expectError: false, // 0 means disabled
		},
		{
			name:        "Metrics port 80",
			port:        80,
			setPort:     func(c *Config, p int) { c.Metrics.Port = p },
			expectError: true, // below 1024
		},
		{
			name:        "Metrics port 65536",
			port:        65536,
			setPort:     func(c *Config, p int) { c.Metrics.Port = p },
			expectError: true, // above 65535
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.setPort(&cfg, tt.port)

			err := ValidateConfig(&cfg)

			if tt.expectError && err == nil {
				t.Errorf("%s: expected error but got none", tt.name)
			}

			if !tt.expectError && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.name, err)
			}
		})
	}
}

// TestValidateConfig_MissingRequired tests that ValidateConfig returns
// errors for missing required fields.
func TestValidateConfig_MissingRequired(t *testing.T) {
	tests := []struct {
		name        string
		modify      func(*Config)
		expectError bool
	}{
		{
			name: "empty DataDir with Indexer enabled",
			modify: func(c *Config) {
				c.Chain.IndexerEnabled = true
				c.Storage.DataDir = ""
			},
			expectError: true,
		},
		{
			name: "MaxPeers = 0",
			modify: func(c *Config) {
				c.P2P.MaxPeers = 0
			},
			expectError: true,
		},
		{
			name: "MaxPeers negative",
			modify: func(c *Config) {
				c.P2P.MaxPeers = -1
			},
			expectError: true,
		},
		{
			name: "MempoolMaxSize = 0",
			modify: func(c *Config) {
				c.Chain.MempoolMaxSize = 0
			},
			expectError: true,
		},
		{
			name: "MempoolMaxSize negative",
			modify: func(c *Config) {
				c.Chain.MempoolMaxSize = -1
			},
			expectError: true,
		},
		{
			name: "MaxTxPerBlock = 0",
			modify: func(c *Config) {
				c.Chain.MaxTxPerBlock = 0
			},
			expectError: true,
		},
		{
			name: "MaxTxPerBlock negative",
			modify: func(c *Config) {
				c.Chain.MaxTxPerBlock = -1
			},
			expectError: true,
		},
		{
			name: "CommissionRate > 10000",
			modify: func(c *Config) {
				c.Validator.CommissionRate = 10001
			},
			expectError: true,
		},
		{
			name: "Valid config",
			modify: func(c *Config) {
				// No modification
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			// Ensure defaults don't trigger errors
			cfg.P2P.Port = 3000  // Set valid P2P port
			cfg.RPC.Port = 8545  // Set valid RPC port

			tt.modify(&cfg)

			err := ValidateConfig(&cfg)

			if tt.expectError && err == nil {
				t.Errorf("%s: expected error but got none", tt.name)
			}

			if !tt.expectError && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.name, err)
			}
		})
	}
}

// TestValidateConfig_NilConfig tests that ValidateConfig handles nil config.
func TestValidateConfig_NilConfig(t *testing.T) {
	err := ValidateConfig(nil)
	if err == nil {
		t.Error("expected error for nil config")
	}
}

// TestValidateConfig_PortRangeBoundaries tests port validation at boundaries.
func TestValidateConfig_PortRangeBoundaries(t *testing.T) {
	// Test P2P port boundaries
	tests := []struct {
		port    int
		valid   bool
		service string
	}{
		{1023, false, "P2P"},
		{1024, true, "P2P"},
		{65535, true, "P2P"},
		{65536, false, "P2P"},
		{80, false, "RPC"},
		{443, false, "RPC"},
		{8080, true, "RPC"},
		{80, false, "Metrics"},
		{8080, true, "Metrics"},
	}

	for _, tt := range tests {
		cfg := DefaultConfig()

		switch tt.service {
		case "P2P":
			cfg.P2P.Port = tt.port
		case "RPC":
			cfg.RPC.Port = tt.port
		case "Metrics":
			cfg.Metrics.Port = tt.port
		}

		err := ValidateConfig(&cfg)

		if tt.valid && err != nil {
			t.Errorf("port %d (%s) should be valid but got error: %v", tt.port, tt.service, err)
		}

		if !tt.valid && err == nil {
			t.Errorf("port %d (%s) should be invalid but got no error", tt.port, tt.service)
		}
	}
}

// TestValidateConfig_AllValidPorts tests that valid port ranges all pass.
func TestValidateConfig_AllValidPorts(t *testing.T) {
	cfg := DefaultConfig()
	cfg.P2P.Port = 3000
	cfg.RPC.Port = 8545
	cfg.Metrics.Port = 9464
	cfg.P2P.MaxPeers = 50
	cfg.Chain.MempoolMaxSize = 10000
	cfg.Chain.MaxTxPerBlock = 100
	cfg.Validator.CommissionRate = 1000

	err := ValidateConfig(&cfg)
	if err != nil {
		t.Errorf("expected valid config but got error: %v", err)
	}
}

// TestValidateConfig_SnapshotWithNoDataDir tests config validation when
// snapshots are enabled but DataDir is empty (warning scenario).
func TestValidateConfig_SnapshotWithNoDataDir(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Snapshot.Enable = true
	cfg.Storage.DataDir = ""
	cfg.Snapshot.OutputDir = ""

	// This should not fail - snapshots just won't work without DataDir
	err := ValidateConfig(&cfg)
	if err != nil {
		t.Errorf("expected no error but got: %v", err)
	}
}