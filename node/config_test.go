package node

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MempoolMaxSize != 10000 {
		t.Errorf("MempoolMaxSize = %d, want 10000", cfg.MempoolMaxSize)
	}
	if cfg.MempoolTTL != 300*time.Second {
		t.Errorf("MempoolTTL = %v, want 300s", cfg.MempoolTTL)
	}
	if cfg.ChainID != 0 {
		t.Errorf("ChainID = %d, want 0", cfg.ChainID)
	}
}

func TestConfigFromEnv(t *testing.T) {
	// Set env vars
	t.Setenv("DSN_CHAIN_ID", "1")
	t.Setenv("DSN_DATA_DIR", "/custom/data")
	t.Setenv("DSN_MEMPOOL_SIZE", "5000")
	t.Setenv("DSN_RPC_PORT", "9999")
	t.Setenv("DSN_P2P_PORT", "8888")

	cfg := ConfigFromEnv()

	if cfg.ChainID != 1 {
		t.Errorf("ChainID = %d, want 1", cfg.ChainID)
	}
	if cfg.DataDir != "/custom/data" {
		t.Errorf("DataDir = %s, want /custom/data", cfg.DataDir)
	}
	if cfg.MempoolMaxSize != 5000 {
		t.Errorf("MempoolMaxSize = %d, want 5000", cfg.MempoolMaxSize)
	}
	if cfg.RPCPort != 9999 {
		t.Errorf("RPCPort = %d, want 9999", cfg.RPCPort)
	}
	if cfg.P2PPort != 8888 {
		t.Errorf("P2PPort = %d, want 8888", cfg.P2PPort)
	}
}

func TestConfigFromEnvDefaults(t *testing.T) {
	// No env vars set — should use defaults
	cfg := ConfigFromEnv()
	defaults := DefaultConfig()

	if cfg.RPCPort != defaults.RPCPort {
		t.Errorf("RPCPort should default to %d, got %d", defaults.RPCPort, cfg.RPCPort)
	}
	if cfg.MempoolMaxSize != defaults.MempoolMaxSize {
		t.Errorf("MempoolMaxSize should default to %d", defaults.MempoolMaxSize)
	}
}

func TestDefaultConfig_Consensus(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxTxPerBlock != 100 {
		t.Errorf("MaxTxPerBlock = %d, want 100", cfg.MaxTxPerBlock)
	}
	// S9: the default timeout base must sit ABOVE the ±5s clock-skew window
	// (types.MaxTimestampDrift), so a round deadline can never be confused
	// with a stale-block rejection.
	if cfg.ProposerTimeout != 6*time.Second {
		t.Errorf("ProposerTimeout = %v, want 6s (above the ±5s skew window)", cfg.ProposerTimeout)
	}
	if cfg.MaxRound != 8 {
		t.Errorf("MaxRound = %d, want 8", cfg.MaxRound)
	}
}

func TestConfigFromEnv_Consensus(t *testing.T) {
	// Set env vars for consensus params
	t.Setenv("DSN_MAX_TX_PER_BLOCK", "200")
	t.Setenv("DSN_PROPOSER_TIMEOUT", "10s")

	cfg := ConfigFromEnv()

	if cfg.MaxTxPerBlock != 200 {
		t.Errorf("MaxTxPerBlock = %d, want 200", cfg.MaxTxPerBlock)
	}
	if cfg.ProposerTimeout != 10*time.Second {
		t.Errorf("ProposerTimeout = %v, want 10s", cfg.ProposerTimeout)
	}
}

// TestConfigFromEnv_RejectsTimeoutAtOrBelowSkewWindow is the S9 RED test. A
// consensus timeout base at or below the ±5s clock-skew window cannot
// distinguish a stalled proposer from a stale block, so configuration must
// reject it. ConfigFromEnv rejects the value by falling back to the default;
// values above the window pass through unchanged.
func TestConfigFromEnv_RejectsTimeoutAtOrBelowSkewWindow(t *testing.T) {
	for _, base := range []string{"1s", "2s", "4s", "5s"} {
		t.Run(base, func(t *testing.T) {
			t.Setenv("DSN_PROPOSER_TIMEOUT", base)
			cfg := ConfigFromEnv()
			want, _ := time.ParseDuration(base)
			if cfg.ProposerTimeout == want {
				t.Fatalf("ProposerTimeout = %v: a base at or below the ±5s skew window must be rejected", cfg.ProposerTimeout)
			}
		})
	}
}

// TestConfigFromEnv_TimeoutAboveSkewAccepted is the S9 companion: bases above
// the skew window must be accepted.
func TestConfigFromEnv_TimeoutAboveSkewAccepted(t *testing.T) {
	t.Setenv("DSN_PROPOSER_TIMEOUT", "6s")
	cfg := ConfigFromEnv()
	if cfg.ProposerTimeout != 6*time.Second {
		t.Fatalf("ProposerTimeout = %v, want 6s (above skew window)", cfg.ProposerTimeout)
	}
}

// TestConfig_MaxRoundDefaultsToEight is the S8 config-side RED test: the
// default MaxRound cap must be 8.
func TestConfig_MaxRoundDefaultsToEight(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxRound != 8 {
		t.Fatalf("MaxRound = %d, want 8", cfg.MaxRound)
	}
}

// TestConfigFromEnv_MaxRound is the S8 config-side RED test: DSN_MAX_ROUND must
// override the default cap.
func TestConfigFromEnv_MaxRound(t *testing.T) {
	t.Setenv("DSN_MAX_ROUND", "2")
	cfg := ConfigFromEnv()
	if cfg.MaxRound != 2 {
		t.Fatalf("MaxRound = %d, want 2 from DSN_MAX_ROUND", cfg.MaxRound)
	}
}

// TestConfigFromEnv_MaxRoundZeroRejected is the S8 lower-bound test: a
// configured MaxRound of 0 would pin the pending round at 0 forever (no view
// change possible), so DSN_MAX_ROUND=0 must be rejected — the default cap stays
// in effect.
func TestConfigFromEnv_MaxRoundZeroRejected(t *testing.T) {
	t.Setenv("DSN_MAX_ROUND", "0")
	cfg := ConfigFromEnv()
	if cfg.MaxRound == 0 {
		t.Fatal("MaxRound = 0: DSN_MAX_ROUND=0 must be rejected (it would pin round 0 forever)")
	}
	if want := DefaultConfig().MaxRound; cfg.MaxRound != want {
		t.Fatalf("MaxRound = %d, want default %d when DSN_MAX_ROUND=0 is rejected", cfg.MaxRound, want)
	}
}

func TestConfigFromEnv_InvalidConsensus(t *testing.T) {
	// Invalid values should be ignored, defaults used
	t.Setenv("DSN_MAX_TX_PER_BLOCK", "-1")
	t.Setenv("DSN_PROPOSER_TIMEOUT", "0")

	cfg := ConfigFromEnv()

	// Invalid values should fall back to defaults
	if cfg.MaxTxPerBlock == -1 {
		t.Error("MaxTxPerBlock should not be -1")
	}
}

func TestDefaultConfig_Snapshot(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.SnapshotInterval != 10 {
		t.Errorf("SnapshotInterval = %d, want 10", cfg.SnapshotInterval)
	}
	if cfg.FastSyncEnabled != true {
		t.Errorf("FastSyncEnabled = %v, want true", cfg.FastSyncEnabled)
	}
	if cfg.TrustedCheckpointHeight != 0 {
		t.Errorf("TrustedCheckpointHeight = %d, want 0", cfg.TrustedCheckpointHeight)
	}
	if cfg.TrustedCheckpointHash != "" {
		t.Errorf("TrustedCheckpointHash = %s, want empty string", cfg.TrustedCheckpointHash)
	}
}

func TestConfigFromEnv_Snapshot(t *testing.T) {
	// Set env vars for snapshot/fast-sync params
	t.Setenv("DSN_SNAPSHOT_INTERVAL", "25")
	t.Setenv("DSN_FAST_SYNC", "true")
	t.Setenv("DSN_TRUSTED_HEIGHT", "5000")
	t.Setenv("DSN_TRUSTED_HASH", "abcd1234")

	cfg := ConfigFromEnv()

	if cfg.SnapshotInterval != 25 {
		t.Errorf("SnapshotInterval = %d, want 25", cfg.SnapshotInterval)
	}
	if cfg.FastSyncEnabled != true {
		t.Errorf("FastSyncEnabled = %v, want true", cfg.FastSyncEnabled)
	}
	if cfg.TrustedCheckpointHeight != 5000 {
		t.Errorf("TrustedCheckpointHeight = %d, want 5000", cfg.TrustedCheckpointHeight)
	}
	if cfg.TrustedCheckpointHash != "abcd1234" {
		t.Errorf("TrustedCheckpointHash = %s, want abcd1234", cfg.TrustedCheckpointHash)
	}
}

func TestConfigFromEnv_SnapshotDefaults(t *testing.T) {
	// No env vars set — should use defaults
	cfg := ConfigFromEnv()
	defaults := DefaultConfig()

	if cfg.SnapshotInterval != defaults.SnapshotInterval {
		t.Errorf("SnapshotInterval should default to %d, got %d", defaults.SnapshotInterval, cfg.SnapshotInterval)
	}
	if cfg.FastSyncEnabled != defaults.FastSyncEnabled {
		t.Errorf("FastSyncEnabled should default to %v, got %v", defaults.FastSyncEnabled, cfg.FastSyncEnabled)
	}
	if cfg.TrustedCheckpointHeight != defaults.TrustedCheckpointHeight {
		t.Errorf("TrustedCheckpointHeight should default to %d, got %d", defaults.TrustedCheckpointHeight, cfg.TrustedCheckpointHeight)
	}
	if cfg.TrustedCheckpointHash != defaults.TrustedCheckpointHash {
		t.Errorf("TrustedCheckpointHash should default to %s, got %s", defaults.TrustedCheckpointHash, cfg.TrustedCheckpointHash)
	}
}

// TestConfigFromEnv_BootstrapPeersDedupes verifies that duplicate addresses in
// DSN_BOOTSTRAP_PEERS are collapsed, preserving order.
func TestConfigFromEnv_BootstrapPeersDedupes(t *testing.T) {
	t.Setenv("DSN_BOOTSTRAP_PEERS", "node0:26656,node1:26656,node0:26656")

	cfg := ConfigFromEnv()

	want := []string{"node0:26656", "node1:26656"}
	if len(cfg.BootstrapPeers) != len(want) {
		t.Fatalf("BootstrapPeers = %v, want %v", cfg.BootstrapPeers, want)
	}
	for i := range want {
		if cfg.BootstrapPeers[i] != want[i] {
			t.Errorf("BootstrapPeers = %v, want %v", cfg.BootstrapPeers, want)
		}
	}
}
