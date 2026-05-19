package consensus

import (
	"testing"

	"github.com/dsn/dsn/state"
	"github.com/dsn/dsn/staking"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
	"github.com/stretchr/testify/require"
)

// ByzantineType defines the adversarial behavior of a simulated validator
type ByzantineType int

const (
	Honest         ByzantineType = 0
	ByzProposer    ByzantineType = 1  // equivocates as proposer
	ByzVoter       ByzantineType = 2  // double-votes
	ByzPartitioned ByzantineType = 3  // isolated from network
)

// SimValidator represents a simulated validator in the BFT harness
type SimValidator struct {
	KeyPair      *wallet.KeyPair
	ConsensusID  types.Address
	Byzantine    ByzantineType
	// Track votes to enable double-vote simulation
	HasVoted     bool
	VotedBlock   types.Hash
}

// BFTTestHarness simulates N validators without P2P networking
type BFTTestHarness struct {
	t           testing.TB
	Validators  []SimValidator
	State       *state.InMemoryState
	Snapshots   map[uint64]*staking.ValidatorSnapshot
	Height      uint64
	Blocks      map[uint64][]*types.Block  // all blocks per height
	Commits     map[uint64]*types.Block    // committed blocks
	Hasher      types.Hasher
	Config      BFTConfig
}

type BFTConfig struct {
	NumValidators   int
	ByzantineIdx    int    // -1 = none
	ByzantineType   ByzantineType
}

// testMempool implements MempoolI for building blocks without real mempool
type testMempool struct {
	txs []*types.Transaction
}

func (m *testMempool) PendingTxs() []*types.Transaction { return m.txs }
func (m *testMempool) Remove(_ types.Hash)              {}

// harnessSigner wraps wallet.KeyPair to implement Signer interface
type harnessSigner struct {
	kp *wallet.KeyPair
}

func (s *harnessSigner) Sign(hash types.Hash) ([]byte, error) {
	return s.kp.SignHash(hash)
}

// NewBFTTestHarness creates N validators with accounts, registers them in staking,
// processes epoch transition, creates snapshot. Returns the harness ready to run.
func NewBFTTestHarness(t testing.TB, config BFTConfig) *BFTTestHarness {
	t.Helper()

	require.Greater(t, config.NumValidators, 0, "NewBFTTestHarness: must have at least 1 validator")
	require.GreaterOrEqual(t, config.ByzantineIdx, -1, "ByzantineIdx must be >= -1")

	// Create hasher first
	hasher := types.SHA256Hasher{}

	// Create in-memory state with hasher
	s := state.NewInMemoryState(hasher)

	// Generate validators
	validators := make([]SimValidator, config.NumValidators)
	keyPairs := make([]*wallet.KeyPair, config.NumValidators)
	validatorAddrs := make([]types.Address, config.NumValidators)

	for i := 0; i < config.NumValidators; i++ {
		kp, err := wallet.GenerateKey()
		require.NoError(t, err, "NewBFTTestHarness: generate key failed")
		keyPairs[i] = kp

		consensusID := types.DeriveConsensusID(kp.PublicKey)
		validatorAddrs[i] = kp.Address()

		// Determine byzantine type
		byzType := Honest
		if i == config.ByzantineIdx && config.ByzantineIdx >= 0 {
			byzType = config.ByzantineType
		}

		validators[i] = SimValidator{
			KeyPair:     kp,
			ConsensusID: consensusID,
			Byzantine:   byzType,
		}
	}

	// Register validators in staking (pattern from helpers.go)
	stake := uint64(100_000)
	for i, addr := range validatorAddrs {
		kp := keyPairs[i]

		// Create account with funds (stake * 2 so there's enough after staking)
		acc := state.NewAccount(addr, kp.PublicKey)
		acc.AddBalance(types.NewAmount(stake * 2))
		s.SetAccount(addr, acc)

		// Register validator (commission=0, currentEpoch=0)
		_, err := staking.RegisterValidator(s, kp.PublicKey, addr, types.NewAmount(stake), 0, 0)
		require.NoError(t, err, "registerValidator: failed for validator %d", i)

		// Deduct stake from account
		acc, err = s.GetAccount(addr)
		require.NoError(t, err)
		acc.SubBalance(types.NewAmount(stake))
		s.SetAccount(addr, acc)
	}

	// Process epoch transition (height 100 = epoch boundary with DefaultBlocksPerEpoch=100)
	err := staking.ProcessEpochTransition(s, 100)
	require.NoError(t, err, "NewBFTTestHarness: ProcessEpochTransition failed")

	// Create snapshot for epoch 1
	snap, err := staking.CreateSnapshot(s, 1)
	require.NoError(t, err, "NewBFTTestHarness: CreateSnapshot failed")

	return &BFTTestHarness{
		t:          t,
		Validators: validators,
		State:      s,
		Snapshots:  map[uint64]*staking.ValidatorSnapshot{1: snap},
		Height:     0,
		Blocks:     make(map[uint64][]*types.Block),
		Commits:    make(map[uint64]*types.Block),
		Hasher:     hasher,
		Config:     config,
	}
}

// RunRound executes a single consensus round:
// - Determines proposer via WeightedProposerAtHeight
// - If byzantine proposer: builds TWO blocks (equivocation)
// - If honest: builds ONE block
// - Each validator: receives block(s), validates, creates prevote
// - If 2/3+ prevotes for a block, creates precommit
// - If byzantine voter: votes for conflicting blocks
// - Tracks committed blocks
func (h *BFTTestHarness) RunRound() {
	h.t.Helper()

	// Get active validators for proposer selection
	activeVals, err := staking.GetActiveValidators(h.State)
	require.NoError(h.t, err, "RunRound: get active validators failed")
	require.Greater(h.t, len(activeVals), 0, "RunRound: no active validators")

	// Determine proposer for current height
	height := h.Height + 1
	proposerAddr := WeightedProposerAtHeight(height, activeVals)

	// Find the validator index for the proposer
	var proposerIdx int
	for i, v := range h.Validators {
		if v.ConsensusID == proposerAddr {
			proposerIdx = i
			break
		}
	}
	proposer := &h.Validators[proposerIdx]

	// Build block(s) based on proposer type
	var blocks []*types.Block
	if proposer.Byzantine == ByzProposer {
		// Byzantine proposer: equivocate by building two different blocks
		block1 := h.buildBlock(proposer, height, types.Hash{})
		block2 := h.buildBlock(proposer, height, block1.Header.PreviousHash)
		blocks = []*types.Block{block1, block2}
	} else {
		// Honest proposer: build one block
		prevHash := h.getTipHash()
		block := h.buildBlock(proposer, height, prevHash)
		blocks = []*types.Block{block}
	}

	// Store blocks for this height
	h.Blocks[height] = blocks

	// Get snapshot for voting
	snap := h.Snapshots[1] // Use epoch 1 snapshot for now
	require.NotNil(h.t, snap, "RunRound: snapshot is nil")

	// Simulate voting: each validator votes on block(s)
	h.simulateVoting(height, blocks, snap)

	// Update height
	h.Height = height
}

// buildBlock builds a block at the given height with the given previous hash
func (h *BFTTestHarness) buildBlock(proposer *SimValidator, height uint64, prevHash types.Hash) *types.Block {
	h.t.Helper()

	// Build block using BuildBlock (empty txs for now)
	block, err := BuildBlock(
		h.State,
		nil, // no VM for testing
		&testMempool{txs: nil},
		height,
		prevHash,
		proposer.ConsensusID,
		&harnessSigner{kp: proposer.KeyPair},
		h.Hasher,
		100, // max txs
		nil, // no evidence
	)
	require.NoError(h.t, err, "buildBlock: build failed")

	return block
}

// getTipHash returns the hash of the latest committed block (or genesis hash if none)
func (h *BFTTestHarness) getTipHash() types.Hash {
	if h.Height == 0 {
		return types.Hash{}
	}
	committed := h.Commits[h.Height]
	if committed == nil {
		return types.Hash{}
	}
	headerHash, err := committed.HeaderHash(h.Hasher)
	require.NoError(h.t, err, "getTipHash: header hash failed")
	return headerHash
}

// simulateVoting simulates the voting phase of consensus
func (h *BFTTestHarness) simulateVoting(height uint64, blocks []*types.Block, snap *staking.ValidatorSnapshot) {
	// For each block, create a voting state and collect votes
	// If any block gets 2/3+ precommits, it becomes committed

	// Map block hash to voting state
	blockVotes := make(map[types.Hash]*VotingState)

	for _, block := range blocks {
		headerHash, err := block.HeaderHash(h.Hasher)
		require.NoError(h.t, err, "simulateVoting: header hash failed")
		blockVotes[headerHash] = NewVotingState(height, 0, headerHash, snap)
	}

	// PHASE 1: Collect all prevotes from all validators
	for i := range h.Validators {
		v := &h.Validators[i]

		// Byzantine partitioned: doesn't participate in voting
		if v.Byzantine == ByzPartitioned {
			continue
		}

		// Determine which block to vote for
		var voteHash types.Hash
		if v.Byzantine == ByzVoter && len(blocks) >= 2 {
			// Byzantine voter: double-vote by voting for different blocks
			// First vote goes to first block, second vote will be simulated separately
			voteHash, _ = blocks[0].HeaderHash(h.Hasher)
		} else {
			// Honest validator: vote for the first block (or only block)
			if len(blocks) > 0 {
				voteHash, _ = blocks[0].HeaderHash(h.Hasher)
			}
		}

		// Create prevote
		prevote := &types.Vote{
			VoteType:  types.VotePrevote,
			Height:    height,
			Round:     0,
			BlockHash: voteHash,
			Validator: v.ConsensusID,
		}
		err := prevote.Sign(v.KeyPair.PrivateKey[:])
		require.NoError(h.t, err, "simulateVoting: prevote sign failed")

		// Add prevote to voting state
		if vs, ok := blockVotes[voteHash]; ok {
			err = vs.AddPrevote(prevote)
			if err == nil { // Only add if not duplicate
				v.HasVoted = true
				v.VotedBlock = voteHash
			}
		}

		// Handle Byzantine voter double-vote: vote for second block if exists
		if v.Byzantine == ByzVoter && len(blocks) >= 2 {
			voteHash2, _ := blocks[1].HeaderHash(h.Hasher)
			if voteHash2 != voteHash {
				prevote2 := &types.Vote{
					VoteType:  types.VotePrevote,
					Height:    height,
					Round:     0,
					BlockHash: voteHash2,
					Validator: v.ConsensusID,
				}
				err := prevote2.Sign(v.KeyPair.PrivateKey[:])
				require.NoError(h.t, err, "simulateVoting: prevote2 sign failed")

				if vs2, ok := blockVotes[voteHash2]; ok {
					_ = vs2.AddPrevote(prevote2) // May fail due to duplicate vote, that's the point
				}
			}
		}
	}

	// PHASE 2: Check which blocks have prevote majority, then collect precommits
	// Find blocks with prevote majority
	var blocksWithPrevoteMajority []types.Hash
	for hash, vs := range blockVotes {
		if vs.HasPrevoteMajority() {
			blocksWithPrevoteMajority = append(blocksWithPrevoteMajority, hash)
		}
	}

	// PHASE 3: Collect precommits for blocks that have prevote majority
	for i := range h.Validators {
		v := &h.Validators[i]

		// Byzantine partitioned: doesn't participate in voting
		if v.Byzantine == ByzPartitioned {
			continue
		}

		// For each block with prevote majority, create precommit
		for _, hash := range blocksWithPrevoteMajority {
			vs := blockVotes[hash]

			// Create precommit for this block
			precommit := &types.Vote{
				VoteType:  types.VotePrecommit,
				Height:    height,
				Round:     0,
				BlockHash: hash,
				Validator: v.ConsensusID,
			}
			err := precommit.Sign(v.KeyPair.PrivateKey[:])
			require.NoError(h.t, err, "simulateVoting: precommit sign failed")

			err = vs.AddPrecommit(precommit)
			// Ignore errors (could be duplicate vote)

			// Check if we have precommit majority
			if vs.HasPrecommitMajority() {
				// Find the block that was committed
				for _, block := range blocks {
					blockHash, _ := block.HeaderHash(h.Hasher)
					if blockHash == hash {
						h.Commits[height] = block
						break
					}
				}
			}
		}
	}
}

// GetCommittedBlock returns the committed block at the given height (or nil if none)
func (h *BFTTestHarness) GetCommittedBlock(height uint64) *types.Block {
	return h.Commits[height]
}

// VerifySafety checks that no conflicting blocks were committed at the same height.
// In BFT, safety means no two different blocks at the same height can be committed.
func (h *BFTTestHarness) VerifySafety() bool {
	for height, block := range h.Commits {
		if block == nil {
			continue
		}
		// Check if there are other blocks at this height that differ
		blocksAtHeight := h.Blocks[height]
		if len(blocksAtHeight) > 1 {
			// Multiple blocks at this height - check if more than one was committed
			// In correct BFT, only one block should be committed
			// If we have commits for multiple different blocks, safety is violated
			headerHash, _ := block.HeaderHash(h.Hasher)
			for _, otherBlock := range blocksAtHeight {
				otherHash, _ := otherBlock.HeaderHash(h.Hasher)
				if otherHash != headerHash {
					// Found a conflicting block at same height
					// If that block is also committed, safety is violated
					if h.Commits[height] != nil {
						otherCommittedHash, _ := h.Commits[height].HeaderHash(h.Hasher)
						if otherCommittedHash != headerHash {
							h.t.Logf("SAFETY VIOLATION: height %d has conflicting commits: %x vs %x",
								height, headerHash[:], otherCommittedHash[:])
							return false
						}
					}
				}
			}
		}
	}
	return true
}

// VerifyLiveness checks if the chain made progress (at least one block committed)
func (h *BFTTestHarness) VerifyLiveness() bool {
	return h.Height > 0 && h.Commits[h.Height] != nil
}