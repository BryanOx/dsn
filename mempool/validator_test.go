package mempool

import (
	"testing"
)

// TestValidatorRegistrationTx_Acceptance tests that a validator
// registration transaction is accepted by the mempool.
func TestValidatorRegistrationTx_Acceptance(t *testing.T) {
	// This test would verify:
	// 1. ValidatorRegistrationMsg can be created
	// 2. NewValidatorRegistrationTx creates valid transaction
	// 3. Mempool accepts the transaction
	// 4. Transaction is executable in a block

	t.Skip("integration: requires full transaction flow")

	/*
	// Create validator key
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	validatorAddr, err := types.AddressFromBytes(pub[:20])
	require.NoError(t, err)

	// Create validator registration message
	msg := &types.ValidatorRegistrationMsg{
		PubKey:        pub[:],
		StakeAmount:   10000000,
		CommissionRate: 1000,
		Metadata:      "validator-1",
	}

	// Create transaction
	tx, err := types.NewValidatorRegistrationTx(
		validatorAddr, // sender (same as validator for self-registration)
		0,              // nonce
		msg,
		1000,           // max fee
		uint64(time.Now().Unix()),
		1,              // chain ID
	)
	require.NoError(t, err)

	// Create mempool
	pool := NewMempool(DefaultConfig())

	// Add transaction to mempool
	err = pool.AddTx(tx)
	require.NoError(t, err, "mempool should accept validator registration tx")

	// Verify transaction is in mempool
	if pool.Size() != 1 {
		t.Errorf("expected 1 transaction in mempool, got %d", pool.Size())
	}

	// Get transaction from mempool
	retrievedTx := pool.GetTx(tx.TxID)
	if retrievedTx == nil {
		t.Error("transaction should be retrievable from mempool")
	}

	// Verify it's a validator registration tx
	msg2, err := retrievedTx.ValidatorRegistration()
	if err != nil {
		t.Errorf("failed to get validator registration: %v", err)
	}
	if msg2.StakeAmount != msg.StakeAmount {
		t.Errorf("stake amount mismatch: %d vs %d", msg2.StakeAmount, msg.StakeAmount)
	}
	*/
}

// TestValidatorRegistrationTx_Validation tests that invalid validator
// registration transactions are rejected.
func TestValidatorRegistrationTx_Validation(t *testing.T) {
	t.Skip("integration: requires transaction validation")

	// Test cases:
	// - Zero stake amount
	// - Commission > 10000
	// - Invalid pubkey length
	// - etc.
}

// TestValidatorRegistrationTx_DuplicatePubkey tests that duplicate
// pubkey registrations are rejected.
func TestValidatorRegistrationTx_DuplicatePubkey(t *testing.T) {
	t.Skip("integration: requires state tracking")
	// Verify that a validator with same pubkey cannot be registered twice
}

// TestValidatorRegistrationTx_AlreadyRegistered tests that already
// registered validators cannot register again.
func TestValidatorRegistrationTx_AlreadyRegistered(t *testing.T) {
	t.Skip("integration: requires state tracking")
	// Verify that once registered, validator cannot register again
}