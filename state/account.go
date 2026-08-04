package state

import (
	"encoding/binary"
	"io"

	"github.com/BryanOx/dsn/types"
)

const PublicKeySize = 32

type Account struct {
	Address     types.Address
	Balance     types.Amount
	Nonce       uint64
	StorageRoot types.Hash
	CodeHash    types.Hash
	Permissions uint32
	PublicKey   [PublicKeySize]byte
}

func NewAccount(addr types.Address, publicKey [PublicKeySize]byte) *Account {
	return &Account{
		Address:     addr,
		Balance:     types.NewAmount(0),
		Nonce:       0,
		StorageRoot: types.Hash{},
		CodeHash:    types.Hash{},
		Permissions: 0,
		PublicKey:   publicKey,
	}
}

func (a *Account) AddBalance(amount types.Amount) error {
	newBal, err := a.Balance.Add(amount)
	if err != nil {
		return err
	}
	a.Balance = newBal
	return nil
}

func (a *Account) SubBalance(amount types.Amount) error {
	newBal, err := a.Balance.Sub(amount)
	if err != nil {
		return err
	}
	a.Balance = newBal
	return nil
}

func (a *Account) IncrementNonce() {
	a.Nonce++
}

func (a *Account) Encode(w io.Writer) error {
	if _, err := w.Write(a.Address.Bytes()); err != nil {
		return err
	}

	balBytes, err := a.Balance.MarshalBinary()
	if err != nil {
		return err
	}
	// Write balance as 16 bytes big-endian (128-bit)
	var balBuf [16]byte
	copy(balBuf[16-len(balBytes):], balBytes)
	if _, err := w.Write(balBuf[:]); err != nil {
		return err
	}

	if err := binary.Write(w, binary.BigEndian, a.Nonce); err != nil {
		return err
	}
	if _, err := w.Write(a.StorageRoot[:]); err != nil {
		return err
	}
	if _, err := w.Write(a.CodeHash[:]); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, a.Permissions); err != nil {
		return err
	}
	if _, err := w.Write(a.PublicKey[:]); err != nil {
		return err
	}

	return nil
}

func (a *Account) Decode(r io.Reader) error {
	addrBytes := make([]byte, 20)
	if _, err := io.ReadFull(r, addrBytes); err != nil {
		return err
	}
	addr, err := types.AddressFromBytes(addrBytes)
	if err != nil {
		return err
	}
	a.Address = addr

	var balBuf [16]byte
	if _, err := io.ReadFull(r, balBuf[:]); err != nil {
		return err
	}
	a.Balance = types.NewAmount(0)
	if err := a.Balance.UnmarshalBinary(balBuf[:]); err != nil {
		return err
	}

	if err := binary.Read(r, binary.BigEndian, &a.Nonce); err != nil {
		return err
	}
	if _, err := io.ReadFull(r, a.StorageRoot[:]); err != nil {
		return err
	}
	if _, err := io.ReadFull(r, a.CodeHash[:]); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &a.Permissions); err != nil {
		return err
	}
	if _, err := io.ReadFull(r, a.PublicKey[:]); err != nil {
		return err
	}

	return nil
}
