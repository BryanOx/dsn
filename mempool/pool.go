package mempool

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/state"
	"go.etcd.io/bbolt"
)

// StateDB subset needed by mempool
type BalanceChecker interface {
	GetAccount(addr types.Address) (*state.Account, error)
}

// pendingTxsBucket is the BoltDB bucket name for persistent pending transactions
const pendingTxsBucket = "pending_txs"

type pendingTx struct {
	tx        *types.Transaction
	submitted time.Time
}

// Mempool holds pending transactions before block inclusion.
type Mempool struct {
	mu        sync.RWMutex
	txs       map[types.Hash]*pendingTx
	index     *txIndex
	state     BalanceChecker
	maxSize   int
	ttl       time.Duration
	nextNonce map[types.Address]uint64 // tracks local nonce per sender
	// BoltDB persistence
	db         *bbolt.DB
	bucketName string
}

// New creates a new mempool.
func New(maxSize int, ttl time.Duration, state BalanceChecker) *Mempool {
	return &Mempool{
		txs:       make(map[types.Hash]*pendingTx),
		index:     newTxIndex(),
		state:     state,
		maxSize:   maxSize,
		ttl:       ttl,
		nextNonce: make(map[types.Address]uint64),
	}
}

// NewWithPersistence creates a new mempool with BoltDB persistence.
// The db parameter should be a reference to the node's persistent BoltDB instance.
// On Submit, transactions are persisted to BoltDB. On Remove, they are deleted.
// LoadPersistent can be used to recover transactions after a node restart.
func NewWithPersistence(maxSize int, ttl time.Duration, state BalanceChecker, db *bbolt.DB) *Mempool {
	mp := New(maxSize, ttl, state)
	mp.db = db
	mp.bucketName = pendingTxsBucket

	// Ensure the bucket exists
	if db != nil {
		db.Update(func(tx *bbolt.Tx) error {
			_, err := tx.CreateBucketIfNotExists([]byte(pendingTxsBucket))
			return err
		})
	}

	return mp
}

// Submit validates and adds a transaction to the pool.
func (mp *Mempool) Submit(tx *types.Transaction) error {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if tx == nil {
		return types.ErrInvalidEncoding
	}

	// Basic validation
	if err := tx.Validate(); err != nil {
		return err
	}

	// Validate contract transactions
	if tx.TxType == types.TxTypeDeployContract {
		// Decode and validate deploy contract tx
		contractTx, err := tx.DeployContract()
		if err != nil {
			return fmt.Errorf("invalid deploy contract tx: %w", err)
		}
		if err := contractTx.Validate(); err != nil {
			return err
		}
	} else if tx.TxType == types.TxTypeCallContract {
		// Decode and validate call contract tx
		contractTx, err := tx.CallContract()
		if err != nil {
			return fmt.Errorf("invalid call contract tx: %w", err)
		}
		if err := contractTx.Validate(); err != nil {
			return err
		}
	} else if tx.TxType == types.TxTypeValidatorRegistration {
		// Validate validator registration tx
		regMsg, err := tx.ValidatorRegistration()
		if err != nil {
			return fmt.Errorf("invalid validator registration tx: %w", err)
		}
		if regMsg == nil {
			return fmt.Errorf("validator registration message is nil")
		}
		// Validate stake amount (minimum 100,000 DSN)
		if regMsg.Stake < 100000 {
			return fmt.Errorf("%w: minimum stake is 100000", ErrInsufficientStake)
		}
		// Validate public key length
		if len(regMsg.PubKey) != 32 {
			return fmt.Errorf("invalid public key length: %d", len(regMsg.PubKey))
		}
		// Validate commission rate (0-10000 basis points)
		if regMsg.Commission > 10000 {
			return fmt.Errorf("commission rate exceeds maximum: %d > 10000", regMsg.Commission)
		}
	}

	// Check capacity
	if len(mp.txs) >= mp.maxSize {
		return ErrMempoolFull
	}

	// Check duplicate
	if _, exists := mp.txs[tx.IntentID]; exists {
		return ErrAlreadyInPool
	}

	// Validate against current state
	// Use local nonce tracking to allow multiple pending txs from same sender
	expectedNonce := mp.nextNonce[tx.Sender]
	if expectedNonce == 0 {
		// First tx from this sender: check against state
		acc, err := mp.state.GetAccount(tx.Sender)
		if err != nil {
			return fmt.Errorf("sender account not found: %w", err)
		}

		// Verify signature based on tx type
		if tx.TxType == types.TxTypeValidatorRegistration {
			// For validator registration, verify against pubkey in the message
			regMsg, _ := tx.ValidatorRegistration()
			if regMsg != nil && len(regMsg.PubKey) == 32 {
				pubKey := ed25519.PublicKey(regMsg.PubKey)
				if !ed25519.Verify(pubKey, tx.IntentID[:], tx.Signature) {
					return types.ErrInvalidSignature
				}
			}
		} else {
			// For standard transactions, verify against stored public key
			if !ed25519.Verify(acc.PublicKey[:], tx.IntentID[:], tx.Signature) {
				return types.ErrInvalidSignature
			}
		}

		expectedNonce = acc.Nonce

		// Check balance covers required amount
		required := types.NewAmount(tx.MaxFee)
		if tx.TxType == types.TxTypeValidatorRegistration {
			// For validator registration, also check stake
			regMsg, _ := tx.ValidatorRegistration()
			if regMsg != nil {
				stakeAmt := types.NewAmount(regMsg.Stake)
				required, _ = required.Add(stakeAmt)
			}
		}
		if acc.Balance.Cmp(required) < 0 {
			if tx.TxType == types.TxTypeValidatorRegistration {
				return fmt.Errorf("%w: need stake + fee", ErrInsufficientStake)
			}
			return types.ErrMaxFeeExceeded
		}
	}

	// Nonce must be expectedNonce + 1
	if tx.Nonce != expectedNonce+1 {
		return types.ErrNonceMismatch
	}

	// Add to pool
	mp.txs[tx.IntentID] = &pendingTx{
		tx:        tx,
		submitted: time.Now(),
	}
	mp.index.Insert(tx)

	// Update local nonce tracking
	mp.nextNonce[tx.Sender] = tx.Nonce

	// Persist to BoltDB if persistence is enabled
	if mp.db != nil && mp.bucketName != "" {
		var buf bytes.Buffer
		if err := tx.Encode(&buf); err != nil {
			// Rollback in-memory state if persistence fails
			delete(mp.txs, tx.IntentID)
			mp.index.Remove(tx.IntentID)
			return fmt.Errorf("failed to encode tx for persistence: %w", err)
		}
		txID := tx.IntentID // capture before shadowing
		if err := mp.db.Update(func(boltTx *bbolt.Tx) error {
			return boltTx.Bucket([]byte(mp.bucketName)).Put(txID[:], buf.Bytes())
		}); err != nil {
			// Rollback in-memory state if persistence fails
			delete(mp.txs, tx.IntentID)
			mp.index.Remove(tx.IntentID)
			return fmt.Errorf("failed to persist tx: %w", err)
		}
	}

	return nil
}

// PendingTxs returns non-expired transactions sorted by (fee DESC, timestamp ASC, intent_id ASC).
func (mp *Mempool) PendingTxs() []*types.Transaction {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	mp.expire()

	all := mp.index.Sorted()
	result := make([]*types.Transaction, 0, len(all))
	for _, tx := range all {
		if pt, ok := mp.txs[tx.IntentID]; ok {
			if time.Since(pt.submitted) <= mp.ttl {
				result = append(result, tx)
			}
		}
	}
	return result
}

// Remove deletes a transaction from the pool by IntentID.
func (mp *Mempool) Remove(intentID types.Hash) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	delete(mp.txs, intentID)
	mp.index.Remove(intentID)

	// Delete from BoltDB if persistence is enabled
	if mp.db != nil && mp.bucketName != "" {
		_ = mp.db.Update(func(tx *bbolt.Tx) error {
			return tx.Bucket([]byte(mp.bucketName)).Delete(intentID[:])
		})
	}
}

// Count returns the number of transactions in the pool.
func (mp *Mempool) Count() int {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	return len(mp.txs)
}

// Get returns a transaction by IntentID, or nil if not found.
func (mp *Mempool) Get(intentID types.Hash) *types.Transaction {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	if pt, ok := mp.txs[intentID]; ok {
		return pt.tx
	}
	return nil
}

// LoadPersistent loads all persisted transactions from BoltDB and returns them.
// The returned transactions can be re-submitted to the mempool for re-validation
// and re-broadcast to peers. Returns an empty slice if no persisted txs exist
// or if persistence is not enabled.
func (mp *Mempool) LoadPersistent() ([]*types.Transaction, error) {
	if mp.db == nil || mp.bucketName == "" {
		return []*types.Transaction{}, nil
	}

	var txs []*types.Transaction
	err := mp.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(mp.bucketName))
		if b == nil {
			return nil
		}

		return b.ForEach(func(k, v []byte) error {
			tx := &types.Transaction{}
			if err := tx.Decode(bytes.NewReader(v)); err != nil {
				// Skip corrupt entries
				return nil
			}
			txs = append(txs, tx)
			return nil
		})
	})

	if err != nil {
		return nil, fmt.Errorf("failed to load persisted txs: %w", err)
	}

	return txs, nil
}

// expire removes transactions past their TTL.
func (mp *Mempool) expire() {
	for id, pt := range mp.txs {
		if time.Since(pt.submitted) > mp.ttl {
			delete(mp.txs, id)
			mp.index.Remove(id)
		}
	}
}

// txIndex maintains a sorted list of transactions.
type txIndex struct {
	entries []*types.Transaction
}

func newTxIndex() *txIndex {
	return &txIndex{
		entries: make([]*types.Transaction, 0),
	}
}

func (idx *txIndex) Insert(tx *types.Transaction) {
	idx.entries = append(idx.entries, tx)
}

func (idx *txIndex) Remove(intentID types.Hash) {
	for i, e := range idx.entries {
		if e.IntentID == intentID {
			idx.entries = append(idx.entries[:i], idx.entries[i+1:]...)
			return
		}
	}
}

func (idx *txIndex) Sorted() []*types.Transaction {
	sorted := make([]*types.Transaction, len(idx.entries))
	copy(sorted, idx.entries)

	sort.Slice(sorted, func(i, j int) bool {
		// Higher fee first
		if sorted[i].MaxFee != sorted[j].MaxFee {
			return sorted[i].MaxFee > sorted[j].MaxFee
		}
		// Same fee: older timestamp first
		if sorted[i].Timestamp != sorted[j].Timestamp {
			return sorted[i].Timestamp < sorted[j].Timestamp
		}
		// Same fee & timestamp: lexicographic intent_id
		return string(sorted[i].IntentID[:]) < string(sorted[j].IntentID[:])
	})

	return sorted
}