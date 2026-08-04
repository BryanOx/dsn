package txfile

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BryanOx/dsn/types"
)

// TransactionFile represents a transaction in a portable JSON file format.
// Unsigned files omit signature and intentId. Signed files include both.
type TransactionFile struct {
	Version     uint16 `json:"version"`
	ChainID     uint32 `json:"chainId"`
	Sender      string `json:"sender"`
	Nonce       uint64 `json:"nonce"`
	Payload     string `json:"payload"`
	Constraints string `json:"constraints,omitempty"`
	MaxFee      uint64 `json:"maxFee"`
	GasLimit    uint64 `json:"gasLimit"`
	Timestamp   uint64 `json:"timestamp"`
	TxType      uint8  `json:"txType"`
	Signature   string `json:"signature,omitempty"`
	IntentID    string `json:"intentId,omitempty"`
}

// ParseFile reads and parses a TransactionFile from a JSON file.
func ParseFile(path string) (*TransactionFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading tx file: %w", err)
	}
	var tf TransactionFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("parsing tx file: %w", err)
	}
	if tf.Sender == "" {
		return nil, fmt.Errorf("tx file missing 'sender' field")
	}
	return &tf, nil
}

// WriteFile writes a TransactionFile as pretty-printed JSON.
func WriteFile(tf *TransactionFile, path string) error {
	data, err := json.MarshalIndent(tf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling tx file: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// ToTransaction converts a TransactionFile to a types.Transaction for signing or submission.
func (tf *TransactionFile) ToTransaction(hasher types.Hasher) (*types.Transaction, error) {
	sender, err := types.ParseAddress(tf.Sender)
	if err != nil {
		return nil, fmt.Errorf("invalid sender address %q: %w", tf.Sender, err)
	}

	var payload []byte
	if tf.Payload != "" {
		cleanPayload := strings.TrimPrefix(tf.Payload, "0x")
		cleanPayload = strings.TrimPrefix(cleanPayload, "0X")
		payload, err = hex.DecodeString(cleanPayload)
		if err != nil {
			return nil, fmt.Errorf("invalid payload hex: %w", err)
		}
	}

	var constraints []byte
	if tf.Constraints != "" {
		cleanConstraints := strings.TrimPrefix(tf.Constraints, "0x")
		cleanConstraints = strings.TrimPrefix(cleanConstraints, "0X")
		constraints, err = hex.DecodeString(cleanConstraints)
		if err != nil {
			return nil, fmt.Errorf("invalid constraints hex: %w", err)
		}
	}

	tx := types.NewTransaction(
		tf.Version,
		tf.ChainID,
		sender,
		tf.Nonce,
		payload,
		constraints,
		tf.MaxFee,
		tf.GasLimit,
		tf.Timestamp,
	)
	tx.TxType = types.TxType(tf.TxType)

	if tf.Signature != "" {
		cleanSig := strings.TrimPrefix(tf.Signature, "0x")
		cleanSig = strings.TrimPrefix(cleanSig, "0X")
		sig, err := hex.DecodeString(cleanSig)
		if err != nil {
			return nil, fmt.Errorf("invalid signature hex: %w", err)
		}
		tx.Signature = sig
	}

	intentID, err := tx.ComputeIntentID(hasher)
	if err != nil {
		return nil, fmt.Errorf("computing intent ID: %w", err)
	}
	tx.IntentID = intentID

	return tx, nil
}

// FromTransaction converts a types.Transaction back to a TransactionFile.
// Used after signing to persist the signed result.
func FromTransaction(tx *types.Transaction) *TransactionFile {
	return &TransactionFile{
		Version:     tx.Version,
		ChainID:     tx.ChainID,
		Sender:      tx.Sender.String(),
		Nonce:       tx.Nonce,
		Payload:     "0x" + hex.EncodeToString(tx.Payload),
		Constraints: "0x" + hex.EncodeToString(tx.Constraints),
		MaxFee:      tx.MaxFee,
		GasLimit:    tx.GasLimit,
		Timestamp:   tx.Timestamp,
		TxType:      uint8(tx.TxType),
		Signature:   "0x" + hex.EncodeToString(tx.Signature),
		IntentID:    "0x" + hex.EncodeToString(tx.IntentID[:]),
	}
}

// IsSigned returns true if the transaction file contains a signature.
func (tf *TransactionFile) IsSigned() bool {
	return tf.Signature != ""
}

// AutoNumber scans the given directory for files matching "tx-*.json",
// finds the highest numeric suffix, and returns the next filename and number.
// Example: if tx-001.json and tx-002.json exist, returns ("tx-003.json", 3, nil).
func AutoNumber(dir string) (path string, num int, err error) {
	pattern := filepath.Join(dir, "tx-*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", 0, fmt.Errorf("scanning for tx files: %w", err)
	}

	maxNum := 0
	for _, m := range matches {
		base := filepath.Base(m)
		base = strings.TrimSuffix(base, ".json")
		base = strings.TrimPrefix(base, "tx-")
		if strings.HasSuffix(base, "-signed") {
			continue
		}
		n, err := strconv.Atoi(base)
		if err == nil && n > maxNum {
			maxNum = n
		}
	}

	next := maxNum + 1
	name := fmt.Sprintf("tx-%03d.json", next)
	return filepath.Join(dir, name), next, nil
}

// BuildTransferPayload builds a hex-encoded payload for a DSN transfer transaction.
// Format: recipient(20 bytes) + amount(8 bytes big-endian)
func BuildTransferPayload(toAddress string, amount uint64) (string, error) {
	addr, err := types.ParseAddress(toAddress)
	if err != nil {
		return "", fmt.Errorf("invalid recipient address: %w", err)
	}

	payload := make([]byte, 28)
	copy(payload[:20], addr.Bytes())
	amt := new(big.Int).SetUint64(amount).Bytes()
	for i := 0; i < 8; i++ {
		if i < len(amt) {
			payload[20+8-len(amt)+i] = amt[i]
		} else {
			payload[20+i] = 0
		}
	}

	return "0x" + hex.EncodeToString(payload), nil
}
