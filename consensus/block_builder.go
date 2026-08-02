package consensus

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/vm"
)

// Signer signs block headers.
type Signer interface {
	Sign(hash types.Hash) ([]byte, error)
}

// StateDB is the subset of state needed for block building.
type StateDB interface {
	GetAccount(addr types.Address) (*state.Account, error)
	SetAccount(addr types.Address, acc *state.Account) error
	Commit() (types.Hash, error)
}

// MempoolI is the subset of mempool needed for block building.
type MempoolI interface {
	PendingTxs() []*types.Transaction
	Remove(intentID types.Hash)
}

// BuildBlock constructs a block from pending mempool transactions.
// The proposer is passed externally since it's determined by consensus.
// The validator set is derived from state via BeginBlock.
// Evidence is processed before BeginBlock so slashing is reflected in epoch transitions.
// The vm parameter is optional - if provided, contract transactions will be executed by the VM.
func BuildBlock(s *state.InMemoryState, vm *vm.VM, mp MempoolI, height uint64, prevHash types.Hash,
	proposer types.Address, signer Signer, hasher types.Hasher, maxTxs int,
	evidence []types.Evidence, blockTimeSec uint64) (*types.Block, error) {

	// 0. Process evidence BEFORE BeginBlock so slashing affects epoch transitions
	for _, ev := range evidence {
		if _, err := ProcessEvidence(s, ev, height); err != nil {
			return nil, fmt.Errorf("process evidence: %w", err)
		}
	}

	// 1. BeginBlock — system transitions
	epoch, valSetHash, err := BeginBlock(s, height, blockTimeSec)
	if err != nil {
		return nil, fmt.Errorf("begin block: %w", err)
	}

	// 2. Get and sort user txs
	txs := mp.PendingTxs()
	if len(txs) > maxTxs {
		txs = txs[:maxTxs]
	}

	// Convert []*types.Transaction to []types.Transaction
	txList := make([]types.Transaction, len(txs))
	for i, tx := range txs {
		txList[i] = *tx
	}

	// Sort by (sender, nonce) for deterministic execution order.
	// This ensures nonces are applied in correct sequence per sender,
	// regardless of fee-based ordering in the mempool.
	sort.Slice(txList, func(i, j int) bool {
		if txList[i].Sender != txList[j].Sender {
			return string(txList[i].Sender[:]) < string(txList[j].Sender[:])
		}
		return txList[i].Nonce < txList[j].Nonce
	})

	// 3. Execute user txs
	var totalFees uint64
	var receipts []types.Hash
	var blockEvents []types.Event

	// Capture block timestamp at start of tx processing for contract execution
	// (F5: the header carries this same single value).
	blockTimestamp := uint64(time.Now().Unix())

	// Build the header incrementally: the fields needed for tx execution are
	// known before the transactions run, the derived roots are filled in after
	// the state commit.
	header := types.BlockHeader{
		Version:          1,
		Height:           height,
		PreviousHash:     prevHash,
		Epoch:            epoch,
		ValidatorSetHash: valSetHash,
		Proposer:         proposer,
		Timestamp:        blockTimestamp,
	}

	for i, tx := range txList {
		exec, err := ApplyTransaction(s, &tx, &header, vm, hasher, uint32(i))
		if err != nil {
			return nil, err
		}
		totalFees += exec.Fee
		receipts = append(receipts, exec.Receipt)
		blockEvents = append(blockEvents, exec.Events...)
	}

	// 4. Create partial block for FinalizeBlock (only FeeSummary needed)
	partialBlock := &types.Block{
		FeeSummary: types.NewFeeSummary(totalFees),
	}

	// 5. FinalizeBlock - distribute fees
	if err := FinalizeBlock(s, partialBlock); err != nil {
		return nil, fmt.Errorf("finalize block: %w", err)
	}

	// 6. Commit state
	stateRoot, err := s.Commit()
	if err != nil {
		return nil, err
	}

	// 7. Derive active validator set for this epoch from STAKING STATE
	activeVals, err := staking.GetActiveValidators(s)
	if err != nil {
		return nil, fmt.Errorf("get active validators: %w", err)
	}
	consensusIDs := make([]types.Address, len(activeVals))
	for i, v := range activeVals {
		consensusIDs[i] = v.ConsensusID
	}
	validatorRoot := ValidatorRoot(consensusIDs)

	// 8. Complete the header with the derived roots and build the block
	header.StateRoot = stateRoot
	header.TxRoot = computeTxRoot(txList, hasher)
	header.ReceiptRoot = computeHashesRoot(receipts, hasher)
	header.ValidatorRoot = validatorRoot
	header.EventsRoot = types.ComputeEventsRoot(blockEvents)

	block := &types.Block{
		Header:       header,
		Transactions: txList,
		FeeSummary:   types.NewFeeSummary(totalFees),
		Evidence:     evidence,
		Events:       blockEvents,
	}

	headerHash, err := block.HeaderHash(hasher)
	if err != nil {
		return nil, err
	}

	sig, err := signer.Sign(headerHash)
	if err != nil {
		return nil, err
	}
	block.Signature = sig

	return block, nil
}

func computeTxRoot(txs []types.Transaction, hasher types.Hasher) types.Hash {
	if len(txs) == 0 {
		return types.Hash{}
	}
	var buf bytes.Buffer
	for _, tx := range txs {
		var txBuf bytes.Buffer
		tx.Encode(&txBuf)
		h, _ := hasher.Hash(txBuf.Bytes())
		buf.Write(h[:])
	}
	h, _ := hasher.Hash(buf.Bytes())
	return h
}

func computeHashesRoot(hashes []types.Hash, hasher types.Hasher) types.Hash {
	if len(hashes) == 0 {
		return types.Hash{}
	}
	var buf bytes.Buffer
	for _, h := range hashes {
		buf.Write(h[:])
	}
	h, _ := hasher.Hash(buf.Bytes())
	return h
}
