package workload

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"math/big"
	"time"

	"github.com/dsn/dsn/types"
)

// TransferConfig holds configuration for transfer workload
type TransferConfig struct {
	NumAccounts int
	TPS         int
	ChainID     uint32
	MaxFee      uint64
	GasLimit    uint64
}

// transferGenerator generates random transfer transactions
type transferGenerator struct {
	cfg      TransferConfig
	accounts []types.Address
	nonceMap map[[20]byte]uint64
}

// NewTransferGenerator creates a new transfer workload generator
func NewTransferGenerator(cfg interface{}) Generator {
	c, ok := cfg.(TransferConfig)
	if !ok {
		c = TransferConfig{
			NumAccounts: 100,
			TPS:         100,
			ChainID:     1,
			MaxFee:      1000,
			GasLimit:    21000,
		}
	}
	g := &transferGenerator{
		cfg:      c,
		accounts: make([]types.Address, c.NumAccounts),
		nonceMap: make(map[[20]byte]uint64),
	}
	// Initialize accounts with random addresses
	for i := 0; i < c.NumAccounts; i++ {
		var addr types.Address
		rand.Read(addr[:])
		g.accounts[i] = addr
	}
	return g
}

func init() {
	Register("transfer", NewTransferGenerator)
	Register("light", NewTransferGenerator)
	Register("moderate", NewTransferGenerator)
	Register("heavy", NewTransferGenerator)
}

// Name returns the name of the generator
func (g *transferGenerator) Name() string {
	return "transfer"
}

// Generate creates random transfer transactions
func (g *transferGenerator) Generate(ctx context.Context, blockHeight uint64) ([]Transaction, error) {
	txs := make([]Transaction, 0, g.cfg.TPS)
	accounts := g.accounts

	for i := 0; i < g.cfg.TPS; i++ {
		// Select random sender and recipient (different from sender)
		senderIdx := randInt(len(accounts))
		recipientIdx := senderIdx
		for recipientIdx == senderIdx {
			recipientIdx = randInt(len(accounts))
		}

		sender := accounts[senderIdx]
		recipient := accounts[recipientIdx]

		// Get and increment nonce for sender
		var senderKey [20]byte
		copy(senderKey[:], sender[:])
		nonce := g.nonceMap[senderKey]
		g.nonceMap[senderKey] = nonce + 1

		// Random amount between 1 and 10000
		amount := uint64(randInt(10000)) + 1

		txs = append(txs, Transaction{
			Type:      "transfer",
			Sender:    sender[:],
			Recipient: recipient[:],
			Amount:    amount,
			Nonce:     nonce,
			MaxFee:    g.cfg.MaxFee,
			GasLimit:  g.cfg.GasLimit,
		})
	}

	return txs, nil
}

// randInt returns a random integer in [0, max)
func randInt(max int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		// Fallback to non-cryptographic random
		return iSum() % max
	}
	return int(n.Int64())
}

// iSum is a simple fallback for random number generation
var seed = uint32(12345)

func iSum() int {
	seed = seed*1103515245 + 12345
	return int(seed >> 16)
}

// SerializeTransferTx serializes a transfer transaction to bytes
func SerializeTransferTx(tx *Transaction) ([]byte, error) {
	// Simple binary format: sender(20) + recipient(20) + amount(8) + nonce(8) + maxFee(8) + gasLimit(8)
	buf := make([]byte, 72)
	copy(buf[0:20], tx.Sender)
	copy(buf[20:40], tx.Recipient)
	binary.BigEndian.PutUint64(buf[40:48], tx.Amount)
	binary.BigEndian.PutUint64(buf[48:56], tx.Nonce)
	binary.BigEndian.PutUint64(buf[56:64], tx.MaxFee)
	binary.BigEndian.PutUint64(buf[64:72], tx.GasLimit)
	return buf, nil
}

// ConvertToTypesTransaction converts our Transaction to types.Transaction
func ConvertToTypesTransaction(tx Transaction, chainID uint32) *types.Transaction {
	senderAddr, _ := types.AddressFromBytes(tx.Sender)
	recipientAddr, _ := types.AddressFromBytes(tx.Recipient)
	payload := types.EncodeTransferPayload(recipientAddr, tx.Amount)
	return types.NewTransaction(
		1,
		chainID,
		senderAddr,
		tx.Nonce,
		payload,
		nil,
		tx.MaxFee,
		tx.GasLimit,
		uint64(time.Now().Unix()),
	)
}
