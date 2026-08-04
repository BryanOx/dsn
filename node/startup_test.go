package node

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BryanOx/dsn/genesis"
	"github.com/BryanOx/dsn/staking"
	"github.com/BryanOx/dsn/types"
	"github.com/stretchr/testify/require"
)

// TestStartup_PersistsGenesisState verifies that a fresh node boot persists the
// genesis accounts and kvstore (not just the SMT root) to disk, so a restart
// recovers the same genesis state instead of re-initializing an empty one.
func TestStartup_PersistsGenesisState(t *testing.T) {
	genFile := writeGenesisFile(t, t.TempDir())

	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.GenesisFile = genFile
	cfg.MetricsPort = 0
	cfg.RPCPort = 0

	// First boot: genesis state must be initialized AND persisted.
	n1, err := New(cfg)
	require.NoError(t, err)

	require.NoError(t, n1.Start(context.Background()))

	root := n1.persistent.GetStateRoot()
	require.NotEqual(t, types.Hash{}, root, "genesis state root should be persisted")
	require.Equal(t, n1.State().GetStateRoot(), root)

	// Treasury account must be present with its genesis balance.
	treasury := mustAddress(t, "0000000000000000000000000000000000000000")
	acc, err := n1.State().GetAccount(treasury)
	require.NoError(t, err)
	require.Equal(t, 0, acc.Balance.Cmp(types.NewAmount(5000000)), "treasury balance from genesis")

	// Economic kvstore written by genesis init must be on disk.
	require.Equal(t, uint64(8000000), staking.ReadUint64(n1.State(), staking.KeyTotalSupply))

	require.NoError(t, n1.Close())

	// Second boot on the same DataDir must recover the persisted genesis state.
	n2, err := New(cfg)
	require.NoError(t, err)
	defer n2.Close()

	require.NoError(t, n2.Start(context.Background()))

	require.Equal(t, root, n2.persistent.GetStateRoot(), "state root should survive restart")

	acc, err = n2.State().GetAccount(treasury)
	require.NoError(t, err)
	require.Equal(t, 0, acc.Balance.Cmp(types.NewAmount(5000000)), "treasury balance should be recovered after restart")
	require.Equal(t, uint64(8000000), staking.ReadUint64(n2.State(), staking.KeyTotalSupply), "kvstore should be recovered after restart")
}

// writeGenesisFile writes a valid genesis document and returns its path.
func writeGenesisFile(t *testing.T, dir string) string {
	t.Helper()

	doc := &genesis.GenesisDoc{
		GenesisVersion: 1,
		GenesisTime:    time.Now().UTC().Truncate(time.Second),
		ChainID:        "test-persist-genesis",
		InitialHeight:  1,
		ConsensusParams: genesis.ConsensusParams{
			MaxTxPerBlock:    100,
			MaxBytesPerBlock: 1048576,
			MaxGasPerBlock:   10000000,
		},
		EpochParams: genesis.EpochParams{
			BlocksPerEpoch:        100,
			UnstakeCooldownEpochs: 7,
			MaxValidators:         100,
			MinimumStake:          1000000,
		},
		InflationParams: genesis.InflationParams{
			Enabled:    false,
			AnnualRate: "0",
		},
		InitialValidators: []genesis.ValidatorEntry{
			{
				Address:      "0123456789abcdef0123456789abcdef01234567",
				PubKey:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				ConsensusKey: "0123456789abcdef0123456789abcdef01234567",
				Stake:        10000000,
				Commission:   "1000",
			},
		},
		InitialBalances: []genesis.BalanceEntry{
			{Address: "0000000000000000000000000000000000000001", Amount: 1000000},
			{Address: "0000000000000000000000000000000000000002", Amount: 2000000},
		},
		Treasury: genesis.TreasuryEntry{
			Address:        "0000000000000000000000000000000000000000",
			InitialBalance: 5000000,
		},
	}

	data, err := genesis.MarshalGenesis(doc)
	require.NoError(t, err)

	path := filepath.Join(dir, "genesis.json")
	require.NoError(t, os.WriteFile(path, data, 0600))
	return path
}

// mustAddress decodes a hex string into a types.Address.
func mustAddress(t *testing.T, s string) types.Address {
	t.Helper()

	b, err := hex.DecodeString(s)
	require.NoError(t, err)

	addr, err := types.AddressFromBytes(b)
	require.NoError(t, err)
	return addr
}
