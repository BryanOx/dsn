package main

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the soak runner
type Config struct {
	NodeEndpoint string        `yaml:"nodeEndpoint"`
	Profile      string        `yaml:"profile"`
	Duration     time.Duration `yaml:"duration"`
	TPS          int           `yaml:"tps"`
	NumAccounts  int           `yaml:"numAccounts"`
	LogInterval  time.Duration `yaml:"logInterval"`
	DryRun       bool          `yaml:"dryRun"`
	Profiles     ProfileConfig `yaml:"profiles"`
	Burst        BurstConfig   `yaml:"burst"`
	Mixed        MixedConfig   `yaml:"mixed"`
}

// ProfileConfig holds profile-specific configurations
type ProfileConfig struct {
	Light    ProfileSettings `yaml:"light"`
	Moderate ProfileSettings `yaml:"moderate"`
	Heavy    ProfileSettings `yaml:"heavy"`
	Burst    ProfileSettings `yaml:"burst"`
	WASM     ProfileSettings `yaml:"wasm"`
	Mixed    ProfileSettings `yaml:"mixed"`
}

// ProfileSettings for a specific profile
type ProfileSettings struct {
	TPS         int `yaml:"tps"`
	NumAccounts int `yaml:"numAccounts"`
}

// BurstConfig for burst workload
type BurstConfig struct {
	BurstMultiplier  int           `yaml:"burstMultiplier"`
	BurstDuration    time.Duration `yaml:"burstDuration"`
	CooldownDuration time.Duration `yaml:"cooldownDuration"`
}

// MixedConfig for mixed workload
type MixedConfig struct {
	TransferRatio float64 `yaml:"transferRatio"`
	WASMRatio     float64 `yaml:"wasmRatio"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply profile settings if specified
	if cfg.Profile != "" {
		if err := cfg.applyProfile(); err != nil {
			return nil, err
		}
	}

	// Set defaults for unset values
	if cfg.NodeEndpoint == "" {
		cfg.NodeEndpoint = "http://localhost:8080"
	}
	if cfg.Duration == 0 {
		cfg.Duration = time.Hour
	}
	if cfg.TPS == 0 {
		cfg.TPS = 100
	}
	if cfg.NumAccounts == 0 {
		cfg.NumAccounts = 100
	}
	if cfg.LogInterval == 0 {
		cfg.LogInterval = 10 * time.Second
	}

	return &cfg, nil
}

// applyProfile applies the selected profile settings
func (c *Config) applyProfile() error {
	switch c.Profile {
	case "light":
		c.TPS = c.Profiles.Light.TPS
		c.NumAccounts = c.Profiles.Light.NumAccounts
	case "moderate":
		c.TPS = c.Profiles.Moderate.TPS
		c.NumAccounts = c.Profiles.Moderate.NumAccounts
	case "heavy":
		c.TPS = c.Profiles.Heavy.TPS
		c.NumAccounts = c.Profiles.Heavy.NumAccounts
	case "burst":
		c.TPS = c.Profiles.Burst.TPS
		c.NumAccounts = c.Profiles.Burst.NumAccounts
	case "wasm":
		c.TPS = c.Profiles.WASM.TPS
		c.NumAccounts = c.Profiles.WASM.NumAccounts
	case "mixed":
		c.TPS = c.Profiles.Mixed.TPS
		c.NumAccounts = c.Profiles.Mixed.NumAccounts
	default:
		return fmt.Errorf("unknown profile: %s", c.Profile)
	}
	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.NodeEndpoint == "" {
		return fmt.Errorf("nodeEndpoint is required")
	}
	if c.TPS <= 0 {
		return fmt.Errorf("tps must be positive")
	}
	if c.NumAccounts <= 0 {
		return fmt.Errorf("numAccounts must be positive")
	}
	if c.Duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}
	return nil
}

// GetBurstConfig returns the burst configuration
func (c *Config) GetBurstConfig() BurstConfig {
	if c.Burst.BurstMultiplier == 0 {
		return BurstConfig{
			BurstMultiplier:  5,
			BurstDuration:    30 * time.Second,
			CooldownDuration: 120 * time.Second,
		}
	}
	return c.Burst
}

// GetMixedConfig returns the mixed workload configuration
func (c *Config) GetMixedConfig() MixedConfig {
	if c.Mixed.TransferRatio == 0 {
		return MixedConfig{
			TransferRatio: 0.7,
			WASMRatio:     0.3,
		}
	}
	return c.Mixed
}
