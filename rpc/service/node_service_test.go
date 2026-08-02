package service

import (
	"context"
	"testing"

	"github.com/dsn/dsn/node"
	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
)

// TestGetSupply verifies that GetSupply reports economic supply from
// KeyTotalSupply rather than summing account balances, so tokens held at
// the burn address are not double-counted and Staked reflects total bonded.
func TestGetSupply(t *testing.T) {
	n, err := node.New(node.DefaultConfig())
	if err != nil {
		t.Fatalf("node.New: %v", err)
	}
	defer n.Close()

	svc := &nodeService{node: n}
	s := n.State()
	ctx := context.Background()

	// Seed economic total supply.
	if err := staking.WriteUint64(s, staking.KeyTotalSupply, 10_000_000); err != nil {
		t.Fatalf("write total supply: %v", err)
	}

	// A burn-address account with a large balance must not affect Total
	// or Circulating.
	burnAcc := state.NewAccount(staking.BurnAddress, [32]byte{})
	if err := burnAcc.AddBalance(types.NewAmount(999_999_999)); err != nil {
		t.Fatalf("credit burn address: %v", err)
	}
	if err := s.SetAccount(staking.BurnAddress, burnAcc); err != nil {
		t.Fatalf("set burn account: %v", err)
	}

	// Without validators, Staked is 0 and Circulating equals Total.
	res, err := svc.GetSupply(ctx)
	if err != nil {
		t.Fatalf("GetSupply: %v", err)
	}
	if res.Total != "10000000" {
		t.Errorf("Total = %s, want 10000000", res.Total)
	}
	if res.Circulating != "10000000" {
		t.Errorf("Circulating = %s, want 10000000", res.Circulating)
	}
	if res.Staked != "0" {
		t.Errorf("Staked = %s, want 0", res.Staked)
	}

	// Register a validator: RegisterValidator adds its stake to total
	// bonded, so Staked must reflect it.
	kp, err := wallet.GenerateKey()
	if err != nil {
		t.Fatalf("wallet.GenerateKey: %v", err)
	}
	if _, err := staking.RegisterValidator(s, kp.PublicKey, kp.Address(), types.NewAmount(100000), 0, 0); err != nil {
		t.Fatalf("RegisterValidator: %v", err)
	}

	res, err = svc.GetSupply(ctx)
	if err != nil {
		t.Fatalf("GetSupply: %v", err)
	}
	if res.Total != "10000000" {
		t.Errorf("Total = %s, want 10000000", res.Total)
	}
	if res.Staked != "100000" {
		t.Errorf("Staked = %s, want 100000", res.Staked)
	}
	if res.Circulating != "9900000" {
		t.Errorf("Circulating = %s, want 9900000", res.Circulating)
	}
}
