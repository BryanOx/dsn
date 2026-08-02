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
	blockTimestamp := uint64(time.Now().Unix())

	for i, tx := range txList {
		txIndex := uint32(i)
		// Check if this is a contract transaction
		if tx.TxType == types.TxTypeDeployContract && vm != nil {
			// Decode contract tx from payload
			contractTx, err := tx.DeployContract()
			if err != nil {
				return nil, fmt.Errorf("decode deploy contract: %w", err)
			}
			// Execute via VM with real block context
			execResult, err := vm.Execute(contractTx, s, height, blockTimestamp, txIndex)
			if err != nil {
				return nil, fmt.Errorf("vm execute deploy: %w", err)
			}
			// T8-3: Gas refund on success - charge only the gas actually used
			// Unused gas (GasLimit - GasUsed) is refunded to sender
			if err := chargeGas(s, tx.Sender, execResult.GasUsed); err != nil {
				return nil, fmt.Errorf("charge gas: %w", err)
			}
			// T8-4: Discard events on tx failure - only collect events from successful txs
			if !execResult.Reverted {
				blockEvents = append(blockEvents, execResult.Events...)
			}
			totalFees += tx.MaxFee
			receipts = append(receipts, tx.IntentID)
		} else if tx.TxType == types.TxTypeCallContract && vm != nil {
			// Decode contract tx from payload
			contractTx, err := tx.CallContract()
			if err != nil {
				return nil, fmt.Errorf("decode call contract: %w", err)
			}
			// Execute via VM with real block context
			execResult, err := vm.Execute(contractTx, s, height, blockTimestamp, txIndex)
			if err != nil {
				return nil, fmt.Errorf("vm execute call: %w", err)
			}
			// T8-3: Gas refund on success - charge only the gas actually used
			if err := chargeGas(s, tx.Sender, execResult.GasUsed); err != nil {
				return nil, fmt.Errorf("charge gas: %w", err)
			}
			// T8-4: Discard events on tx failure - only collect events from successful txs
			if !execResult.Reverted {
				blockEvents = append(blockEvents, execResult.Events...)
			}
			totalFees += tx.MaxFee
			receipts = append(receipts, tx.IntentID)
		} else {
			// Standard transaction - apply via state
			receipt, err := state.ApplyTransaction(s, &tx, hasher, height)
			if err != nil {
				return nil, err
			}
			totalFees += tx.MaxFee
			receipts = append(receipts, receipt)
		}
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

	// 8. Build block with new fields
	txRoot := computeTxRoot(txList, hasher)
	receiptRoot := computeHashesRoot(receipts, hasher)

	block := &types.Block{
		Header: types.BlockHeader{
			Version:          1,
			Height:           height,
			PreviousHash:     prevHash,
			StateRoot:        stateRoot,
			TxRoot:           txRoot,
			ReceiptRoot:      receiptRoot,
			ValidatorRoot:    validatorRoot,
			ValidatorSetHash: valSetHash,
			Epoch:            epoch,
			Timestamp:        uint64(time.Now().Unix()),
			Proposer:         proposer,
			EventsRoot:       types.ComputeEventsRoot(blockEvents),
		},
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

// chargeGas deducts the gas cost from the sender's account balance.
func chargeGas(s *state.InMemoryState, sender types.Address, gasUsed uint64) error {
	// Gas price is 1 token per gas unit (simple model)
	gasCost := types.NewAmount(gasUsed)

	acc, err := s.GetAccount(sender)
	if err != nil {
		return fmt.Errorf("get sender account: %w", err)
	}

	if acc.Balance.Cmp(gasCost) < 0 {
		return fmt.Errorf("insufficient balance for gas: have %s, need %s", acc.Balance, gasCost)
	}

	if err := acc.SubBalance(gasCost); err != nil {
		return err
	}

	return s.SetAccount(sender, acc)
}
