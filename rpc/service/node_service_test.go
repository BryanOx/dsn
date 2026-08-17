package service

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/BryanOx/dsn/node"
	"github.com/BryanOx/dsn/staking"
	"github.com/BryanOx/dsn/state"
	"github.com/BryanOx/dsn/types"
	"github.com/BryanOx/dsn/wallet"
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

func TestBlockNumberReturnsHex(t *testing.T) {
	cfg := node.DefaultConfig()
	n, err := node.New(cfg)
	if err != nil {
		t.Fatalf("node.New: %v", err)
	}
	defer n.Close()

	svc := &nodeService{node: n}
	ctx := context.Background()

	result, err := svc.BlockNumber(ctx)
	if err != nil {
		t.Fatalf("BlockNumber: %v", err)
	}

	// Fresh node starts at height 0 → "0x0"
	if result != "0x0" {
		t.Errorf("BlockNumber = %s, want 0x0", result)
	}

	// Must be hex-encoded (starts with 0x)
	if len(result) < 3 || result[:2] != "0x" {
		t.Errorf("BlockNumber = %s, want 0x-prefixed hex", result)
	}
}

func TestChainIdReturnsHex(t *testing.T) {
	cfg := node.DefaultConfig()
	cfg.ChainID = 7777
	n, err := node.New(cfg)
	if err != nil {
		t.Fatalf("node.New: %v", err)
	}
	defer n.Close()

	svc := &nodeService{node: n}
	ctx := context.Background()

	result, err := svc.ChainId(ctx)
	if err != nil {
		t.Fatalf("ChainId: %v", err)
	}

	// 7777 in hex is 1e61
	if result != "0x1e61" {
		t.Errorf("ChainId = %s, want 0x1e61", result)
	}
}

func TestSyncingSynced(t *testing.T) {
	n, err := node.New(node.DefaultConfig())
	if err != nil {
		t.Fatalf("node.New: %v", err)
	}
	defer n.Close()

	svc := &nodeService{node: n}
	ctx := context.Background()

	result, err := svc.Syncing(ctx)
	if err != nil {
		t.Fatalf("Syncing: %v", err)
	}

	// Fresh node at height 0 is synced (genesis = synced)
	if result != false {
		t.Errorf("Syncing = %v, want false (synced)", result)
	}
}

func TestSyncingSyncing(t *testing.T) {
	// DSN is a small chain — Syncing always returns false.
	// This test verifies the return type is consistent.
	n, err := node.New(node.DefaultConfig())
	if err != nil {
		t.Fatalf("node.New: %v", err)
	}
	defer n.Close()

	svc := &nodeService{node: n}
	ctx := context.Background()

	result, err := svc.Syncing(ctx)
	if err != nil {
		t.Fatalf("Syncing: %v", err)
	}

	// DSN is always synced (small chain)
	if result != false {
		t.Errorf("Syncing = %v, want false (small chain always synced)", result)
	}
}

func TestGetCodeReturnsBytecode(t *testing.T) {
	n, err := node.New(node.DefaultConfig())
	if err != nil {
		t.Fatalf("node.New: %v", err)
	}
	defer n.Close()

	svc := &nodeService{node: n}
	s := n.State()
	ctx := context.Background()

	// Create an address for testing (20-byte hex)
	addrBytes := make([]byte, 20)
	addrBytes[0] = 0x01
	addrHex := "0x" + hex.EncodeToString(addrBytes)

	// Test EOA (no code deployed) — should return "0x"
	result, err := svc.GetCode(ctx, addrHex)
	if err != nil {
		t.Fatalf("GetCode EOA: %v", err)
	}
	if result != "0x" {
		t.Errorf("GetCode EOA = %s, want 0x", result)
	}

	// Deploy code for the address
	var contractID types.Hash
	copy(contractID[:], addrBytes)
	testCode := []byte{0x00, 0x61, 0x00, 0x10, 0x01} // mock WASM bytecode
	if err := s.SetCode(contractID, testCode); err != nil {
		t.Fatalf("SetCode: %v", err)
	}

	// Test contract — should return hex-encoded bytecode
	result, err = svc.GetCode(ctx, addrHex)
	if err != nil {
		t.Fatalf("GetCode contract: %v", err)
	}
	expected := "0x" + hex.EncodeToString(testCode)
	if result != expected {
		t.Errorf("GetCode contract = %s, want %s", result, expected)
	}
}

func TestGetCodeInvalidAddress(t *testing.T) {
	n, err := node.New(node.DefaultConfig())
	if err != nil {
		t.Fatalf("node.New: %v", err)
	}
	defer n.Close()

	svc := &nodeService{node: n}
	ctx := context.Background()

	// Invalid hex address
	_, err = svc.GetCode(ctx, "not-an-address")
	if err == nil {
		t.Error("GetCode with invalid address should return error")
	}

	// Wrong length
	_, err = svc.GetCode(ctx, "0x01")
	if err == nil {
		t.Error("GetCode with short address should return error")
	}
}
