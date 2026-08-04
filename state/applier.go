package state

import (
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/BryanOx/dsn/types"
)

// ApplyTransaction validates and applies a single transaction to the state.
// It verifies signature, nonce, balance, then applies the transaction.
// Returns a receipt hash (the transaction's IntentID) and any error.
func ApplyTransaction(db *InMemoryState, tx *types.Transaction, hasher types.Hasher, height uint64) (types.Hash, error) {
	// 1. Basic transaction validation
	if err := tx.Validate(); err != nil {
		return types.Hash{}, fmt.Errorf("tx validation: %w", err)
	}

	// 2. Get sender account
	sender, err := db.GetAccount(tx.Sender)
	if err != nil {
		return types.Hash{}, fmt.Errorf("sender: %w", err)
	}

	// 3. Verify signature against stored public key
	if !ed25519.Verify(sender.PublicKey[:], tx.IntentID[:], tx.Signature) {
		return types.Hash{}, fmt.Errorf("invalid signature")
	}

	// 4. Verify nonce (expected nonce = account nonce + 1)
	if tx.Nonce != sender.Nonce+1 {
		return types.Hash{}, fmt.Errorf("nonce mismatch: got %d, expected %d", tx.Nonce, sender.Nonce+1)
	}

	// 5. Verify sender has enough balance for fees
	feeAmount := types.NewAmount(tx.MaxFee)
	if sender.Balance.Cmp(feeAmount) < 0 {
		return types.Hash{}, fmt.Errorf("insufficient balance for fee: have %s, need %s", sender.Balance, feeAmount)
	}

	// 6. Process transfer if this is a standard transaction
	if tx.TxType == types.TxTypeStandard || tx.TxType == 0 {
		if len(tx.Payload) >= 28 {
			// Parse recipient address (first 20 bytes)
			recipientAddr, err := types.AddressFromBytes(tx.Payload[:20])
			if err != nil {
				return types.Hash{}, fmt.Errorf("invalid recipient address: %w", err)
			}

			// Parse amount (next 8 bytes, big-endian uint64)
			transferAmount := binary.BigEndian.Uint64(tx.Payload[20:28])
			transferAmt := types.NewAmount(transferAmount)

			// Verify sender has enough for fee + transfer
			totalCost, err := feeAmount.Add(transferAmt)
			if err != nil {
				return types.Hash{}, fmt.Errorf("total cost overflow: %w", err)
			}
			if sender.Balance.Cmp(totalCost) < 0 {
				return types.Hash{}, fmt.Errorf("insufficient balance for transfer + fee: have %s, need %s", sender.Balance, totalCost)
			}

			// Get or create recipient account
			recipient, err := db.GetAccount(recipientAddr)
			if err != nil {
				// Create new account for recipient
				recipient = NewAccount(recipientAddr, [32]byte{})
			}

			// Credit recipient
			if err := recipient.AddBalance(transferAmt); err != nil {
				return types.Hash{}, fmt.Errorf("failed to credit recipient: %w", err)
			}

			// Deduct transfer amount from sender
			if err := sender.SubBalance(transferAmt); err != nil {
				return types.Hash{}, err
			}

			// Save recipient
			if err := db.SetAccount(recipientAddr, recipient); err != nil {
				return types.Hash{}, err
			}
		}
	} else if tx.TxType == types.TxTypeValidatorRegistration {
		// Process validator registration
		regMsg, err := tx.ValidatorRegistration()
		if err != nil {
			return types.Hash{}, fmt.Errorf("invalid validator registration: %w", err)
		}
		if regMsg == nil {
			return types.Hash{}, fmt.Errorf("validator registration message is nil")
		}

		// Verify the sender address matches the registration address
		if regMsg.Address != tx.Sender {
			return types.Hash{}, fmt.Errorf("sender address does not match registration address")
		}

		// Verify the public key length
		if len(regMsg.PubKey) != 32 {
			return types.Hash{}, fmt.Errorf("invalid public key length")
		}

		// Verify stake amount meets minimum (100,000 DSN)
		const minValidatorStake = 100000
		if regMsg.Stake < minValidatorStake {
			return types.Hash{}, fmt.Errorf("stake amount %d below minimum %d", regMsg.Stake, minValidatorStake)
		}

		// Verify commission rate (0-10000 basis points = 0-100%)
		if regMsg.Commission > 10000 {
			return types.Hash{}, fmt.Errorf("commission rate %d exceeds max 10000", regMsg.Commission)
		}

		// Check sender has enough balance for stake + fee
		stakeAmt := types.NewAmount(regMsg.Stake)
		totalCost, err := feeAmount.Add(stakeAmt)
		if err != nil {
			return types.Hash{}, fmt.Errorf("total cost overflow: %w", err)
		}
		if sender.Balance.Cmp(totalCost) < 0 {
			return types.Hash{}, fmt.Errorf("insufficient balance for stake + fee: have %s, need %s", sender.Balance, totalCost)
		}

		// Deduct stake from sender (fee will be deducted later)
		if err := sender.SubBalance(stakeAmt); err != nil {
			return types.Hash{}, fmt.Errorf("deduct stake: %w", err)
		}

		// Derive consensus ID from public key (SHA256(pubkey)[:20])
		consensusID := types.DeriveConsensusIDFromBytes(regMsg.PubKey)

		// Store pending validator using the same pattern as staking package
		// Key format: validator/<consensusID hex>
		validatorKey := "validator/" + hex.EncodeToString(consensusID[:])

		// Check if validator already exists
		if _, exists := db.GetBytes(validatorKey); exists {
			return types.Hash{}, fmt.Errorf("validator already registered")
		}

		// Encode validator data: pubkey(32) + operatorAddr(20) + stake(16) + commission(2) + status(1) + activationEpoch(8)
		// Using a simple binary encoding compatible with staking.Registry
		validatorData := make([]byte, 0, 79)
		validatorData = append(validatorData, regMsg.PubKey...) // 32 bytes
		validatorData = append(validatorData, tx.Sender[:]...)  // 20 bytes
		stakeBytes, _ := stakeAmt.MarshalBinary()
		validatorData = append(validatorData, stakeBytes...) // up to 16 bytes
		// Pad stake to 16 bytes
		if len(stakeBytes) < 16 {
			validatorData = append(validatorData, make([]byte, 16-len(stakeBytes))...)
		}
		// Commission (2 bytes)
		var commissionBytes [2]byte
		binary.BigEndian.PutUint16(commissionBytes[:], uint16(regMsg.Commission))
		validatorData = append(validatorData, commissionBytes[:]...)
		// Status (1 byte) - 0 = pending
		validatorData = append(validatorData, 0)
		// ActivationEpoch (8 bytes) - current epoch + 1
		currentEpoch := getCurrentEpoch(db)
		activationEpoch := currentEpoch + 1
		var epochBytes [8]byte
		binary.BigEndian.PutUint64(epochBytes[:], activationEpoch)
		validatorData = append(validatorData, epochBytes[:]...)

		// Store validator
		if err := db.SetBytes(validatorKey, validatorData); err != nil {
			return types.Hash{}, fmt.Errorf("store validator: %w", err)
		}

		// Also store pubkey index: validator/pubkey/<pubkey hex> -> consensusID
		pubkeyIndexKey := "validator/pubkey/" + hex.EncodeToString(regMsg.PubKey)
		if err := db.SetBytes(pubkeyIndexKey, consensusID[:]); err != nil {
			return types.Hash{}, fmt.Errorf("store pubkey index: %w", err)
		}

		// Also store operator index: validator/operator/<operator address> -> consensusID
		operatorIndexKey := "validator/operator/" + tx.Sender.String()
		if err := db.SetBytes(operatorIndexKey, consensusID[:]); err != nil {
			return types.Hash{}, fmt.Errorf("store operator index: %w", err)
		}

		// Increment validator count
		countKey := "validator/count"
		countBytes, _ := db.GetBytes(countKey)
		var count uint64 = 1
		if countBytes != nil {
			count = binary.BigEndian.Uint64(countBytes) + 1
		}
		var countBuf [8]byte
		binary.BigEndian.PutUint64(countBuf[:], count)
		if err := db.SetBytes(countKey, countBuf[:]); err != nil {
			return types.Hash{}, fmt.Errorf("store validator count: %w", err)
		}

		// Update total bonded stake
		totalBondedKey := "staking/total_bonded"
		totalBondedBytes, _ := db.GetBytes(totalBondedKey)
		totalBonded := types.NewAmount(0)
		if totalBondedBytes != nil {
			totalBonded.UnmarshalBinary(totalBondedBytes)
		}
		newTotalBonded, _ := totalBonded.Add(stakeAmt)
		newTotalBondedBytes, _ := newTotalBonded.MarshalBinary()
		if err := db.SetBytes(totalBondedKey, newTotalBondedBytes); err != nil {
			return types.Hash{}, fmt.Errorf("update total bonded: %w", err)
		}

		// Note: We don't save sender here - the regular flow will deduct fee and save
		// The stake was already deducted from sender.Balance above
	}

	// 7. Deduct fee from sender
	if err := sender.SubBalance(feeAmount); err != nil {
		return types.Hash{}, err
	}

	// 8. Increment nonce
	sender.IncrementNonce()

	// 9. Save updated sender
	if err := db.SetAccount(tx.Sender, sender); err != nil {
		return types.Hash{}, err
	}

	// Return the transaction's IntentID as the receipt
	return tx.IntentID, nil
}

// getCurrentEpoch retrieves the current epoch from state (defaults to 0).
func getCurrentEpoch(db *InMemoryState) uint64 {
	epochBytes, _ := db.GetBytes("epoch/current")
	if epochBytes == nil {
		return 0
	}
	return binary.BigEndian.Uint64(epochBytes)
}
