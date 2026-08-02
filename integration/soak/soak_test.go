//go:build integration

package soak

import (
	"os"
	"testing"
	"time"

	"github.com/dsn/dsn/integration"
	"github.com/dsn/dsn/types"
	"github.com/stretchr/testify/require"
)

// getSoakDuration parses SOAK_DURATION env var to determine which tests to run.
// Empty = skip all, "1m" = light only, "24h" = 24h test, "72h" = 72h test.
func getSoakDuration() time.Duration {
	dur := os.Getenv("SOAK_DURATION")
	if dur == "" {
		return 0
	}
	d, err := time.ParseDuration(dur)
	if err != nil {
		return 0
	}
	return d
}

// TestSoak24h runs a 24-hour soak test.
// Disabled by default — use SOAK_DURATION=24h env var to enable.
func TestSoak24h(t *testing.T) {
	dur := getSoakDuration()
	if dur == 0 {
		t.Skip("Skipping 24h soak test — set SOAK_DURATION=24h to enable")
	}
	if dur != 24*time.Hour {
		t.Skip("Skipping 24h soak — SOAK_DURATION does not match 24h")
	}

	// Create 3-node cluster
	nodes, kps := integration.NewMultiNodeNetwork(t, 3)
	defer func() {
		for _, n := range nodes {
			n.Close()
		}
	}()

	// Fund accounts
	fundAllAccounts(t, nodes, kps, 10_000_000)

	// Create hasher for transaction signing
	hasher := types.SHA256Hasher{}

	// Run 24h soak with moderate TPS
	t.Log("Starting 24-hour soak test with 100 TPS")
	startTime := time.Now()
	targetTPS := 100

	for round := 0; ; round++ {
		// Check duration
		if time.Since(startTime) >= dur {
			break
		}

		// Generate and submit transactions
		txs := generateSoakTransactions(t, nodes, kps, &hasher, round, targetTPS)
		for _, n := range nodes {
			integration.MineBlockWithTxs(t, n, kps, txs)
		}

		// Verify convergence every 10 blocks
		if round%10 == 0 {
			integration.CompareStateRoots(t, nodes)
			t.Logf("24h soak: round %d, elapsed %v", round, time.Since(startTime))
		}

		// Rate limit
		time.Sleep(100 * time.Millisecond)
	}

	t.Log("24h soak completed — all checks passed")
}

// TestSoak72h runs a 72-hour soak test.
// Disabled by default — use SOAK_DURATION=72h env var to enable.
func TestSoak72h(t *testing.T) {
	dur := getSoakDuration()
	if dur == 0 {
		t.Skip("Skipping 72h soak test — set SOAK_DURATION=72h to enable")
	}
	if dur != 72*time.Hour {
		t.Skip("Skipping 72h soak — SOAK_DURATION does not match 72h")
	}

	// Create 3-node cluster
	nodes, kps := integration.NewMultiNodeNetwork(t, 3)
	defer func() {
		for _, n := range nodes {
			n.Close()
		}
	}()

	fundAllAccounts(t, nodes, kps, 50_000_000)

	hasher := types.SHA256Hasher{}

	// Run 72h soak with heavy TPS
	t.Log("Starting 72-hour soak test with 500 TPS")
	startTime := time.Now()
	targetTPS := 500

	for round := 0; ; round++ {
		if time.Since(startTime) >= dur {
			break
		}

		txs := generateSoakTransactions(t, nodes, kps, &hasher, round, targetTPS)
		for _, n := range nodes {
			integration.MineBlockWithTxs(t, n, kps, txs)
		}

		if round%20 == 0 {
			integration.CompareStateRoots(t, nodes)
			t.Logf("72h soak: round %d, elapsed %v", round, time.Since(startTime))
		}

		time.Sleep(200 * time.Millisecond)
	}

	t.Log("72h soak completed — all checks passed")
}

// TestSoak_Light runs a quick 1-minute light soak for CI.
// Uses SOAK_DURATION=1m to enable.
func TestSoak_Light(t *testing.T) {
	dur := getSoakDuration()
	if dur == 0 {
		t.Skip("Skipping light soak test — set SOAK_DURATION=1m to enable")
	}
	if dur > 10*time.Minute {
		t.Skip("Skipping light soak — SOAK_DURATION too long")
	}

	// Create 3-node cluster
	nodes, kps := integration.NewMultiNodeNetwork(t, 3)
	defer func() {
		for _, n := range nodes {
			n.Close()
		}
	}()

	fundAllAccounts(t, nodes, kps, 100_000)

	hasher := types.SHA256Hasher{}
	t.Logf("Starting light soak test for %v", dur)
	startTime := time.Now()
	targetTPS := 10
	round := 0

	for time.Since(startTime) < dur {
		txs := generateSoakTransactions(t, nodes, kps, &hasher, round, targetTPS)
		for _, n := range nodes {
			integration.MineBlockWithTxs(t, n, kps, txs)
		}

		if round%5 == 0 {
			integration.CompareStateRoots(t, nodes)
		}

		round++
		time.Sleep(100 * time.Millisecond)
	}

	t.Logf("Light soak completed: %d rounds, all checks passed", round)
}

// TestSoak_Burst tests burst pattern workload.
func TestSoak_Burst(t *testing.T) {
	// Always run with short mode check
	if testing.Short() {
		t.Skip("Skipping burst soak in short mode")
	}

	// Create 3-node cluster
	nodes, kps := integration.NewMultiNodeNetwork(t, 3)
	defer func() {
		for _, n := range nodes {
			n.Close()
		}
	}()

	fundAllAccounts(t, nodes, kps, 500_000)

	hasher := types.SHA256Hasher{}
	t.Log("Starting burst soak test")

	// Burst pattern: high TPS bursts with cooldown periods
	burstCount := 5
	burstDuration := 10
	cooldownDuration := 20

	for burst := 0; burst < burstCount; burst++ {
		// High TPS burst
		t.Logf("Burst %d/%d: high TPS phase", burst+1, burstCount)
		for round := 0; round < burstDuration; round++ {
			txs := generateSoakTransactions(t, nodes, kps, &hasher, burst*100+round, 100)
			for _, n := range nodes {
				integration.MineBlockWithTxs(t, n, kps, txs)
			}
		}

		integration.CompareStateRoots(t, nodes)

		// Cooldown period with low TPS
		t.Logf("Burst %d/%d: cooldown phase", burst+1, burstCount)
		for round := 0; round < cooldownDuration; round++ {
			txs := generateSoakTransactions(t, nodes, kps, &hasher, burst*100+burstDuration+round, 5)
			for _, n := range nodes {
				integration.MineBlockWithTxs(t, n, kps, txs)
			}
		}

		integration.CompareStateRoots(t, nodes)
	}

	t.Log("Burst soak completed — all checks passed")
}

// TestSoak_Mixed tests mixed workload (transfers + contract calls).
func TestSoak_Mixed(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping mixed soak in short mode")
	}

	// Create 3-node cluster
	nodes, kps := integration.NewMultiNodeNetwork(t, 3)
	defer func() {
		for _, n := range nodes {
			n.Close()
		}
	}()

	fundAllAccounts(t, nodes, kps, 1_000_000)

	hasher := types.SHA256Hasher{}
	t.Log("Starting mixed workload soak test")

	// Mixed pattern: 70% transfers, 30% contract calls
	rounds := 30
	for round := 0; round < rounds; round++ {
		// Track how many transactions each sender has sent IN THIS ROUND
		txCountThisRound := make(map[types.Address]uint64)
		for _, kp := range kps {
			txCountThisRound[kp.Address()] = 0
		}

		// Read current nonces from state BEFORE generating this round's transactions
		currentNonces := make(map[types.Address]uint64)
		for _, kp := range kps {
			acc, err := nodes[0].State().GetAccount(kp.Address())
			if err != nil {
				currentNonces[kp.Address()] = 0
			} else {
				currentNonces[kp.Address()] = acc.Nonce
			}
		}

		// Transfer transactions (70%)
		var txs []*types.Transaction
		transferCount := 7
		for i := 0; i < transferCount; i++ {
			senderIdx := (round + i) % len(kps)
			kp := kps[senderIdx]
			addr := kp.Address()
			// State expects tx.Nonce == account.Nonce + 1 + txIndexThisRound
			nonce := currentNonces[addr] + 1 + txCountThisRound[addr]
			txCountThisRound[addr]++
			tx := types.NewTransaction(
				1, 0, addr, nonce,
				types.EncodeTransferPayload(addr, 0), nil, 100, 1000,
				uint64(time.Now().Unix()),
			)
			require.NoError(t, kp.Sign(tx, &hasher))
			txs = append(txs, tx)
		}

		// Contract-like transactions (30%) - using payload as "contract call"
		contractCount := 3
		for i := 0; i < contractCount; i++ {
			senderIdx := (round + i) % len(kps)
			kp := kps[senderIdx]
			addr := kp.Address()
			// State expects tx.Nonce == account.Nonce + 1 + txIndexThisRound
			nonce := currentNonces[addr] + 1 + txCountThisRound[addr]
			txCountThisRound[addr]++
			tx := types.NewTransaction(
				1, 0, addr, nonce,
				types.EncodeTransferPayload(addr, 0), nil, 200, 2000,
				uint64(time.Now().Unix()),
			)
			require.NoError(t, kp.Sign(tx, &hasher))
			txs = append(txs, tx)
		}

		// Mine on all nodes
		for _, n := range nodes {
			integration.MineBlockWithTxs(t, n, kps, txs)
		}

		// Verify every 5 rounds
		if round%5 == 0 {
			integration.CompareStateRoots(t, nodes)
			t.Logf("Mixed soak: round %d/%d", round+1, rounds)
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Log("Mixed soak completed — all checks passed")
}
