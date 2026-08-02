package sdk

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
)

// TxBuilder is a fluent builder for transactions.
type TxBuilder struct {
	client    *Client
	from      string
	to        string
	nonce     uint64
	value     *big.Int
	data      []byte
	gasLimit  uint64
	gasPrice  uint64
	maxFee    uint64
	timestamp uint64
	chainID   uint32
	version   uint16
}

// NewTxBuilder creates a new transaction builder.
func (c *Client) NewTxBuilder() *TxBuilder {
	return &TxBuilder{
		client:    c,
		gasLimit:  21000,
		gasPrice:  1,
		timestamp: uint64(time.Now().Unix()),
		chainID:   1,
		version:   1,
	}
}

// From sets the sender address.
func (b *TxBuilder) From(addr string) *TxBuilder {
	b.from = addr
	return b
}

// To sets the recipient address.
func (b *TxBuilder) To(addr string) *TxBuilder {
	b.to = addr
	return b
}

// Nonce sets the transaction nonce.
func (b *TxBuilder) Nonce(nonce uint64) *TxBuilder {
	b.nonce = nonce
	return b
}

// Value sets the transaction value in wei.
func (b *TxBuilder) Value(value *big.Int) *TxBuilder {
	b.value = value
	return b
}

// Data sets the transaction data (hex string or bytes).
func (b *TxBuilder) Data(data string) *TxBuilder {
	// Try to parse as hex
	if d, err := hex.DecodeString(data); err == nil {
		b.data = d
	} else {
		b.data = []byte(data)
	}
	return b
}

// DataBytes sets the transaction data as raw bytes.
func (b *TxBuilder) DataBytes(data []byte) *TxBuilder {
	b.data = data
	return b
}

// GasLimit sets the gas limit.
func (b *TxBuilder) GasLimit(gasLimit uint64) *TxBuilder {
	b.gasLimit = gasLimit
	return b
}

// GasPrice sets the gas price.
func (b *TxBuilder) GasPrice(gasPrice uint64) *TxBuilder {
	b.gasPrice = gasPrice
	return b
}

// MaxFee sets the max fee.
func (b *TxBuilder) MaxFee(maxFee uint64) *TxBuilder {
	b.maxFee = maxFee
	return b
}

// ChainID sets the chain ID.
func (b *TxBuilder) ChainID(chainID uint32) *TxBuilder {
	b.chainID = chainID
	return b
}

// Build builds an unsigned transaction.
func (b *TxBuilder) Build() (*types.Transaction, error) {
	// Validate required fields
	if b.from == "" {
		return nil, ErrInvalidParams
	}

	// Parse sender address
	sender, err := types.ParseAddress(b.from)
	if err != nil {
		return nil, fmt.Errorf("invalid sender address: %w", err)
	}

	// Handle recipient (optional for contract deployment)
	var recipient types.Address
	if b.to != "" {
		recipient, err = types.ParseAddress(b.to)
		if err != nil {
			return nil, fmt.Errorf("invalid recipient address: %w", err)
		}
	}

	// Calculate max fee if not set
	if b.maxFee == 0 {
		b.maxFee = b.gasLimit * b.gasPrice
	}

	// Parse value
	value := big.NewInt(0)
	if b.value != nil {
		value = b.value
	}

	// Build payload
	var payload []byte

	// Standard transfer: encode as (recipient, value, data)
	// For contract interactions, use custom encoding
	payload = encodeTransferPayload(recipient, value, b.data)

	tx := types.NewTransaction(
		b.version,
		b.chainID,
		sender,
		b.nonce,
		payload,
		nil, // constraints
		b.maxFee,
		b.gasLimit,
		b.timestamp,
	)

	return tx, nil
}

// SignAndBuild builds and signs a transaction with a keypair.
func (b *TxBuilder) SignAndBuild(key *wallet.KeyPair) (*types.Transaction, error) {
	tx, err := b.Build()
	if err != nil {
		return nil, err
	}

	// Sign the transaction
	if err := key.Sign(tx, types.SHA256Hasher{}); err != nil {
		return nil, fmt.Errorf("sign transaction: %w", err)
	}

	return tx, nil
}

// BuildAndSend builds, signs, and sends a transaction.
func (b *TxBuilder) BuildAndSend(key *wallet.KeyPair) (string, error) {
	tx, err := b.SignAndBuild(key)
	if err != nil {
		return "", err
	}

	// Convert to SDK Transaction for sending
	sdkTx := transactionToSDK(tx)

	return b.client.SendTransaction(context.Background(), &sdkTx)
}

// encodeTransferPayload encodes recipient, value, and data into payload format.
func encodeTransferPayload(recipient types.Address, value *big.Int, data []byte) []byte {
	// Simple encoding: recipient (20 bytes) + value (big int) + data length + data
	payload := make([]byte, 0, 32+len(data))

	// Pad value to 32 bytes (big-endian)
	valueBytes := value.Bytes()
	valuePadded := make([]byte, 32)
	copy(valuePadded[32-len(valueBytes):], valueBytes)

	payload = append(payload, recipient[:]...)
	payload = append(payload, valuePadded...)

	// Add data length and data
	payload = append(payload, byte(len(data)))
	payload = append(payload, data...)

	return payload
}

// transactionToSDK converts types.Transaction to SDK Transaction.
func transactionToSDK(tx *types.Transaction) Transaction {
	sender := tx.Sender.String()

	var recipient string
	if len(tx.Payload) >= 20 {
		var addr types.Address
		copy(addr[:], tx.Payload[:20])
		recipient = addr.String()
	}

	return Transaction{
		Hash:      hex.EncodeToString(tx.IntentID[:]),
		Sender:    sender,
		Recipient: recipient,
		IntentID:  hex.EncodeToString(tx.IntentID[:]),
		Nonce:     tx.Nonce,
		Value:     new(big.Int).SetBytes(tx.Payload).String(),
		MaxFee:    fmt.Sprintf("%d", tx.MaxFee),
		GasLimit:  tx.GasLimit,
		Data:      hex.EncodeToString(tx.Payload),
	}
}

// MarshalJSON implements custom JSON marshaling for Transaction.
func (t *Transaction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Hash      string   `json:"hash"`
		Sender    string   `json:"sender"`
		Recipient string   `json:"recipient"`
		IntentID  string   `json:"intentId"`
		Nonce     uint64   `json:"nonce"`
		Value     string   `json:"value"`
		MaxFee    string   `json:"maxFee"`
		GasLimit  uint64   `json:"gasLimit"`
		Data      string   `json:"data"`
		Receipt   *Receipt `json:"receipt,omitempty"`
	}{
		Hash:      t.Hash,
		Sender:    t.Sender,
		Recipient: t.Recipient,
		IntentID:  t.IntentID,
		Nonce:     t.Nonce,
		Value:     t.Value,
		MaxFee:    t.MaxFee,
		GasLimit:  t.GasLimit,
		Data:      t.Data,
		Receipt:   t.Receipt,
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for Transaction.
func (t *Transaction) UnmarshalJSON(data []byte) error {
	var tmp struct {
		Hash      string   `json:"hash"`
		Sender    string   `json:"sender"`
		Recipient string   `json:"recipient"`
		IntentID  string   `json:"intentId"`
		Nonce     uint64   `json:"nonce"`
		Value     string   `json:"value"`
		MaxFee    string   `json:"maxFee"`
		GasLimit  uint64   `json:"gasLimit"`
		Data      string   `json:"data"`
		Receipt   *Receipt `json:"receipt,omitempty"`
	}

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	t.Hash = tmp.Hash
	t.Sender = tmp.Sender
	t.Recipient = tmp.Recipient
	t.IntentID = tmp.IntentID
	t.Nonce = tmp.Nonce
	t.Value = tmp.Value
	t.MaxFee = tmp.MaxFee
	t.GasLimit = tmp.GasLimit
	t.Data = tmp.Data
	t.Receipt = tmp.Receipt

	return nil
}

// SignTransaction signs an unsigned transaction with a keypair.
func SignTransaction(tx *types.Transaction, key *wallet.KeyPair) error {
	return key.Sign(tx, types.SHA256Hasher{})
}

// TransactionToHex converts a signed transaction to hex string.
func TransactionToHex(tx *types.Transaction) (string, error) {
	// Encode transaction to bytes
	var buf bytes.Buffer
	if err := tx.Encode(&buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(buf.Bytes()), nil
}

// ParseTransaction parses a hex string to a transaction.
func ParseTransaction(hexStr string) (*types.Transaction, error) {
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}

	tx := &types.Transaction{}
	if err := tx.Decode(bytes.NewReader(data)); err != nil {
		return nil, err
	}

	return tx, nil
}

// NewKeyPairFromSeed creates a keypair from a seed (for deterministic key derivation).
func NewKeyPairFromSeed(seed []byte) (*wallet.KeyPair, error) {
	if len(seed) != 32 {
		return nil, fmt.Errorf("seed must be 32 bytes")
	}

	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)

	kp := &wallet.KeyPair{}
	copy(kp.PublicKey[:], pub)
	copy(kp.PrivateKey[:], priv)

	return kp, nil
}

// GetTransactionCount retrieves the nonce for an address.
func (c *Client) GetTransactionCount(ctx context.Context, address string) (uint64, error) {
	account, err := c.GetAccount(ctx, address)
	if err != nil {
		return 0, err
	}

	return account.Nonce, nil
}

// SendSignedTransaction sends a pre-signed transaction.
func (c *Client) SendSignedTransaction(ctx context.Context, txHex string) (string, error) {
	return c.SendRawTransaction(ctx, txHex)
}
