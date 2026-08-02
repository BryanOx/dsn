package staking

import (
	"fmt"

	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/types"
)

// TreasuryAddress is the canonical zero address used for the protocol treasury.
// All tokens minted to the treasury are held in this account.
var TreasuryAddress = types.Address{}

// CreditTreasury credits the treasury account with newly minted tokens.
// This is called during epoch issuance to increase the treasury supply.
// The treasury supply key tracks the total tokens ever minted to treasury.
func CreditTreasury(s StakingState, amount uint64) error {
	// 1. Early return for zero amount
	if amount == 0 {
		return nil
	}

	// 2. Read current treasury supply from kvstore
	currentSupply := ReadUint64(s, KeyTreasurySupply)

	// 3. Calculate new treasury supply
	newTreasurySupply := currentSupply + amount

	// 4. Store updated treasury supply
	if err := WriteUint64(s, KeyTreasurySupply, newTreasurySupply); err != nil {
		return fmt.Errorf("update treasury supply: %w", err)
	}

	// 5. Get or create treasury account
	acc, err := s.GetAccount(TreasuryAddress)
	if err != nil {
		// Account doesn't exist yet - create a new one
		acc = state.NewAccount(TreasuryAddress, [32]byte{})
	}

	// 6. Add the amount to the treasury account balance
	if err := acc.AddBalance(types.NewAmount(amount)); err != nil {
		return fmt.Errorf("add treasury balance: %w", err)
	}

	// 7. Persist the updated account
	if err := s.SetAccount(TreasuryAddress, acc); err != nil {
		return fmt.Errorf("set treasury account: %w", err)
	}

	return nil
}

// GetTreasuryBalance returns the current treasury supply (total tokens ever minted to treasury).
// This reads from the kvstore and returns 0 if the key hasn't been set yet.
func GetTreasuryBalance(s KVStore) (uint64, error) {
	return ReadUint64(s, KeyTreasurySupply), nil
}
