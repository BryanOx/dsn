package indexer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BryanOx/dsn/types"
	"go.etcd.io/bbolt"
)

// newTestIndexer creates a temp BoltDB-backed indexer for testing.
// Returns the Indexer and a cleanup function.
func newTestIndexer(t *testing.T) (*Indexer, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test-indexer.db")
	db, err := bbolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := initSchema(db); err != nil {
		db.Close()
		t.Fatalf("failed to init schema: %v", err)
	}

	idx := &Indexer{
		db:      db,
		ch:      make(chan *IndexableBlock, 100),
		done:    make(chan struct{}),
		enabled: true,
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(dir)
	}

	return idx, cleanup
}

// makeTestTx creates a minimal transaction for testing.
func makeTestTx(gasLimit uint64) *types.Transaction {
	var intentID types.Hash
	intentID[0] = 0xAA
	intentID[1] = 0xBB

	var sender types.Address
	sender[0] = 0x11

	return &types.Transaction{
		Version:   1,
		ChainID:   1,
		IntentID:  intentID,
		Sender:    sender,
		Nonce:     1,
		Payload:   []byte("test-payload"),
		MaxFee:    10000,
		GasLimit:  gasLimit,
		Timestamp: 1000,
	}
}

func TestIndexerStoresRealGas(t *testing.T) {
	idx, cleanup := newTestIndexer(t)
	defer cleanup()

	tx := makeTestTx(1000) // GasLimit = 1000
	blockNumber := uint64(42)

	// Store with real gas = 500 (not GasLimit)
	err := indexTransaction(idx.db, tx, blockNumber, nil, 500, true, "", nil)
	if err != nil {
		t.Fatalf("indexTransaction failed: %v", err)
	}

	// Retrieve and verify
	receipt, err := idx.GetTransaction(tx.IntentID)
	if err != nil {
		t.Fatalf("GetTransaction failed: %v", err)
	}
	if receipt == nil {
		t.Fatal("receipt is nil")
	}

	if receipt.GasUsed != 500 {
		t.Errorf("GasUsed = %d, want 500", receipt.GasUsed)
	}
	if receipt.Status != true {
		t.Error("Status should be true")
	}
	if receipt.BlockNumber != blockNumber {
		t.Errorf("BlockNumber = %d, want %d", receipt.BlockNumber, blockNumber)
	}
}

func TestReceiptRevertedStatus(t *testing.T) {
	idx, cleanup := newTestIndexer(t)
	defer cleanup()

	tx := makeTestTx(1000)
	blockNumber := uint64(10)

	// Store with success=false (reverted)
	err := indexTransaction(idx.db, tx, blockNumber, nil, 500, false, "", nil)
	if err != nil {
		t.Fatalf("indexTransaction failed: %v", err)
	}

	receipt, err := idx.GetTransaction(tx.IntentID)
	if err != nil {
		t.Fatalf("GetTransaction failed: %v", err)
	}
	if receipt == nil {
		t.Fatal("receipt is nil")
	}

	if receipt.Status != false {
		t.Errorf("Status = %v, want false", receipt.Status)
	}
}

func TestReceiptContractAddressAndReturnData(t *testing.T) {
	idx, cleanup := newTestIndexer(t)
	defer cleanup()

	tx := makeTestTx(1000)
	blockNumber := uint64(5)
	contractAddr := "0x1234567890abcdef1234567890abcdef12345678"
	returnData := []byte("contract-call-result")

	err := indexTransaction(idx.db, tx, blockNumber, nil, 200, true, contractAddr, returnData)
	if err != nil {
		t.Fatalf("indexTransaction failed: %v", err)
	}

	receipt, err := idx.GetTransaction(tx.IntentID)
	if err != nil {
		t.Fatalf("GetTransaction failed: %v", err)
	}

	if receipt.ContractAddress != contractAddr {
		t.Errorf("ContractAddress = %q, want %q", receipt.ContractAddress, contractAddr)
	}
	if receipt.ReturnData != "contract-call-result" {
		t.Errorf("ReturnData = %q, want %q", receipt.ReturnData, "contract-call-result")
	}
}

func TestIndexBlockWithTxResults(t *testing.T) {
	idx, cleanup := newTestIndexer(t)
	defer cleanup()

	tx1 := makeTestTx(1000)
	tx2 := makeTestTx(2000)
	// Differentiate IntentIDs
	tx2.IntentID[2] = 0xCC

	var header types.BlockHeader
	header.Height = 1

	ib := &IndexableBlock{
		Number: 1,
		Header: &header,
		Txns:   []*types.Transaction{tx1, tx2},
		TxResults: map[types.Hash]*TxResult{
			tx1.IntentID: {
				GasUsed: 300,
				Success: true,
			},
			tx2.IntentID: {
				GasUsed:         150,
				Success:         false,
				ContractAddress: "0xdeadbeef",
				ReturnData:      []byte("revert-reason"),
			},
		},
	}

	if err := idx.indexBlock(ib); err != nil {
		t.Fatalf("indexBlock failed: %v", err)
	}

	// Check tx1
	r1, err := idx.GetTransaction(tx1.IntentID)
	if err != nil {
		t.Fatalf("GetTransaction tx1 failed: %v", err)
	}
	if r1.GasUsed != 300 {
		t.Errorf("tx1 GasUsed = %d, want 300", r1.GasUsed)
	}
	if r1.Status != true {
		t.Error("tx1 Status should be true")
	}

	// Check tx2
	r2, err := idx.GetTransaction(tx2.IntentID)
	if err != nil {
		t.Fatalf("GetTransaction tx2 failed: %v", err)
	}
	if r2.GasUsed != 150 {
		t.Errorf("tx2 GasUsed = %d, want 150", r2.GasUsed)
	}
	if r2.Status != false {
		t.Error("tx2 Status should be false")
	}
	if r2.ContractAddress != "0xdeadbeef" {
		t.Errorf("tx2 ContractAddress = %q, want 0xdeadbeef", r2.ContractAddress)
	}
}

func TestIndexBlockBackwardCompatibleDefaults(t *testing.T) {
	idx, cleanup := newTestIndexer(t)
	defer cleanup()

	tx := makeTestTx(1000)

	var header types.BlockHeader
	header.Height = 1

	// No TxResults — should fall back to GasLimit and success=true
	ib := &IndexableBlock{
		Number: 1,
		Header: &header,
		Txns:   []*types.Transaction{tx},
	}

	if err := idx.indexBlock(ib); err != nil {
		t.Fatalf("indexBlock failed: %v", err)
	}

	receipt, err := idx.GetTransaction(tx.IntentID)
	if err != nil {
		t.Fatalf("GetTransaction failed: %v", err)
	}

	if receipt.GasUsed != tx.GasLimit {
		t.Errorf("GasUsed = %d, want %d (GasLimit fallback)", receipt.GasUsed, tx.GasLimit)
	}
	if receipt.Status != true {
		t.Error("Status should default to true")
	}
}

func TestGetTransactionsByBlock(t *testing.T) {
	idx, cleanup := newTestIndexer(t)
	defer cleanup()

	tx1 := makeTestTx(1000)
	tx2 := makeTestTx(2000)
	tx2.IntentID[2] = 0xCC

	var header types.BlockHeader
	header.Height = 7

	ib := &IndexableBlock{
		Number: 7,
		Header: &header,
		Txns:   []*types.Transaction{tx1, tx2},
		TxResults: map[types.Hash]*TxResult{
			tx1.IntentID: {GasUsed: 100, Success: true},
			tx2.IntentID: {GasUsed: 200, Success: true},
		},
	}

	if err := idx.indexBlock(ib); err != nil {
		t.Fatalf("indexBlock failed: %v", err)
	}

	receipts, err := idx.GetTransactionsByBlock(7)
	if err != nil {
		t.Fatalf("GetTransactionsByBlock failed: %v", err)
	}
	if len(receipts) != 2 {
		t.Fatalf("got %d receipts, want 2", len(receipts))
	}
}
