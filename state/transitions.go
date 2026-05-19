package state

import (
	"fmt"

	"github.com/dsn/dsn/types"
)

// Transfer moves `amount` DSN from `from` to `to`.
// Self-transfers are no-ops.
func Transfer(db StateDB, from, to types.Address, amount types.Amount, hasher types.Hasher) error {
	if from == to {
		return nil
	}

	sender, err := db.GetAccount(from)
	if err != nil {
		return fmt.Errorf("sender: %w", err)
	}

	receiver, err := db.GetAccount(to)
	if err != nil {
		// Auto-create receiver account
		receiver = NewAccount(to, [PublicKeySize]byte{})
	}

	if sender.Balance.Cmp(amount) < 0 {
		return fmt.Errorf("insufficient balance: have %s, need %s", sender.Balance, amount)
	}

	if err := sender.SubBalance(amount); err != nil {
		return err
	}
	if err := receiver.AddBalance(amount); err != nil {
		return err
	}

	if err := db.SetAccount(from, sender); err != nil {
		return err
	}
	if err := db.SetAccount(to, receiver); err != nil {
		return err
	}

	return nil
}

// Mint creates new DSN at an address (genesis only).
func Mint(db StateDB, addr types.Address, amount types.Amount, hasher types.Hasher) error {
	acc, err := db.GetAccount(addr)
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}

	if err := acc.AddBalance(amount); err != nil {
		return err
	}

	return db.SetAccount(addr, acc)
}

// Burn destroys DSN at an address.
func Burn(db StateDB, addr types.Address, amount types.Amount, hasher types.Hasher) error {
	acc, err := db.GetAccount(addr)
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}

	if err := acc.SubBalance(amount); err != nil {
		return err
	}

	return db.SetAccount(addr, acc)
}