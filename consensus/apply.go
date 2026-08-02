package consensus

import (
	"fmt"

	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/vm"
)

// TxExecution captures the outcome of applying a single transaction.
type TxExecution struct {
	// Receipt is the transaction's receipt hash.
	Receipt types.Hash
	// Fee is the amount charged to the sender and included in the block fee
	// summary.
	Fee uint64
	// Events holds the events emitted by a successful contract execution.
	// Events from reverted or failed executions are discarded (T8-4).
	Events []types.Event
}

// ApplyTransaction executes a single transaction against the in-memory state.
// It is the single execution path shared by block building, block validation,
// and block replay so every path applies identical state transitions, nonce
// updates, gas charges, and fee accounting.
//
// Standard and validator-registration transactions are applied via
// state.ApplyTransaction and charged the full MaxFee.
// Contract transactions (deploy/call) are executed via the VM against the
// block context carried by header and charged only the gas actually used
// (T8-3: unused gas is refunded to the sender).
func ApplyTransaction(s *state.InMemoryState, tx *types.Transaction, header *types.BlockHeader, vm *vm.VM, hasher types.Hasher, txIndex uint32) (TxExecution, error) {
	var exec TxExecution

	if tx.TxType == types.TxTypeDeployContract || tx.TxType == types.TxTypeCallContract {
		if vm == nil {
			return exec, fmt.Errorf("vm required for contract transaction %x", tx.IntentID)
		}

		var contractTx interface{}
		var err error
		kind := "call"
		if tx.TxType == types.TxTypeDeployContract {
			kind = "deploy"
			contractTx, err = tx.DeployContract()
			if err != nil {
				return exec, fmt.Errorf("decode deploy contract: %w", err)
			}
		} else {
			contractTx, err = tx.CallContract()
			if err != nil {
				return exec, fmt.Errorf("decode call contract: %w", err)
			}
		}

		execResult, err := vm.Execute(contractTx, s, header.Height, header.Timestamp, txIndex)
		if err != nil {
			return exec, fmt.Errorf("vm execute %s: %w", kind, err)
		}
		// T8-3: Gas refund on success - charge only the gas actually used.
		// Unused gas (GasLimit - GasUsed) is refunded to the sender.
		// T8-5/F4: Never charge more than MaxFee, even if execution consumed
		// more gas than the sender declared.
		charged := execResult.GasUsed
		if charged > tx.MaxFee {
			charged = tx.MaxFee
		}
		if err := chargeGas(s, tx.Sender, charged); err != nil {
			return exec, fmt.Errorf("charge gas: %w", err)
		}
		// T8-4: Discard events on tx failure - only collect events from
		// successful txs.
		if !execResult.Reverted {
			exec.Events = execResult.Events
		}
		exec.Receipt = tx.IntentID
		exec.Fee = charged
		return exec, nil
	}

	// Standard and validator-registration transactions.
	receipt, err := state.ApplyTransaction(s, tx, hasher, header.Height)
	if err != nil {
		return exec, err
	}
	exec.Receipt = receipt
	exec.Fee = tx.MaxFee
	return exec, nil
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
