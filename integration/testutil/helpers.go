//go:build integration
// +build integration

package testutil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"testing"

	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
)

// BuildTransferTx builds a signed transfer transaction
func BuildTransferTx(sender types.Address, nonce uint64, to types.Address, amount uint64, kp *wallet.KeyPair, contractID types.Hash) *types.Transaction {
	// Build calldata: 20 bytes address + 8 bytes amount = 28 bytes
	calldata := make([]byte, 28)
	copy(calldata[:20], to[:])
	binary.BigEndian.PutUint64(calldata[20:28], amount)

	tx := &types.Transaction{
		Sender:   sender,
		Nonce:    nonce,
		TxType:   types.TxTypeCallContract,
		GasLimit: 1000000,
		MaxFee:   1000000,
	}

	callTx := &types.CallContractTx{
		ContractID: contractID,
		Sender:     sender,
		Entrypoint: "transfer",
		Calldata:   calldata,
		GasLimit:   tx.GasLimit,
	}

	var buf bytes.Buffer
	callTx.Encode(&buf)
	tx.Payload = buf.Bytes()
	kp.Sign(tx, types.SHA256Hasher{})
	return tx
}

// BuildDeployTokenTx builds a signed deploy transaction for the token contract
func BuildDeployTokenTx(sender types.Address, nonce uint64, totalSupply uint64, kp *wallet.KeyPair, wasmCode []byte) *types.Transaction {
	codeHash := sha256.Sum256(wasmCode)
	calldata := make([]byte, 8)
	binary.BigEndian.PutUint64(calldata, totalSupply)

	tx := &types.Transaction{
		Sender:   sender,
		Nonce:    nonce,
		TxType:   types.TxTypeDeployContract,
		GasLimit: 5000000,
		MaxFee:   1000000,
	}

	deployTx := &types.DeployContractTx{
		Sender:   sender,
		Nonce:    nonce,
		WasmCode: wasmCode,
		CodeHash: codeHash,
		MaxFee:   tx.MaxFee,
		GasLimit: tx.GasLimit,
	}

	var buf bytes.Buffer
	deployTx.Encode(&buf)
	tx.Payload = buf.Bytes()
	kp.Sign(tx, types.SHA256Hasher{})
	return tx
}

// LoadTokenWasm loads the token contract WASM binary
func LoadTokenWasm(t testing.TB) []byte {
	t.Helper()
	// Try to load from testdata
	code, err := os.ReadFile("integration/testdata/token.wasm")
	if err != nil {
		t.Fatalf("LoadTokenWasm: %v (compile with: GOOS=wasip1 GOARCH=wasm go build -o integration/testdata/token.wasm ./examples/token/)", err)
	}
	return code
}

// SetupTestAccounts sets up initial accounts with balance for testing
func SetupTestAccounts(tn *TestNode, addresses []types.Address, balance uint64) {
	for _, addr := range addresses {
		var pubKey [32]byte
		copy(pubKey[:], addr[:])
		acc := state.NewAccount(addr, pubKey)
		acc.AddBalance(types.NewAmount(balance))
		tn.Node.State().SetAccount(addr, acc)
	}
}
