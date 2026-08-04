package node

import (
	"github.com/BryanOx/dsn/consensus"
	"github.com/BryanOx/dsn/types"
)

// decodeSyncedBlock decodes a block served by a peer during block sync. The
// wire block list carries EncodeBlockMessage frames with the leading message
// type byte stripped (see network.encodeBlockAsWireFromBlock); DecodeBlockMessage
// requires the type byte, so it is re-added here — the single choke point that
// keeps the block-sync engine free of gossip wire-format knowledge (D2).
func decodeSyncedBlock(data []byte) (*types.Block, error) {
	frame := make([]byte, 0, len(data)+1)
	frame = append(frame, consensus.BlockMessageType)
	frame = append(frame, data...)
	return consensus.DecodeBlockMessage(frame)
}

// loadParentHeader returns the parent header and expected previous hash used
// to validate a block at the given height: the on-disk header of height-1 when
// available, otherwise the node's current tip hash — falling back to the zero
// hash when neither exists (e.g. the first block served after genesis).
func (n *Node) loadParentHeader(block *types.Block) (*types.BlockHeader, types.Hash) {
	expectedPrevHash := n.GetTipHash()
	if n.persistent != nil && block.Header.Height > 0 {
		if parent, err := consensus.LoadBlockHeader(n.persistent, block.Header.Height-1); err == nil {
			expectedPrevHash, _ = parent.HeaderHash(n.hasher)
		} else {
			expectedPrevHash = types.ZeroHash
		}
	}
	return &types.BlockHeader{
		Height:       block.Header.Height - 1,
		PreviousHash: block.Header.PreviousHash,
	}, expectedPrevHash
}

// applySyncedBlock is the BlockSyncEngine block handler
// (func([]byte) (bool, error)): it validates and applies a block served by a
// peer. It mirrors the live path (ValidateBlock → applyAcceptedBlock) so a
// synced block is applied exactly as a gossiped one; returning (false, err)
// stops the serving window and penalizes the serving peer. Heights at or below
// the finalized height are skipped silently — reported as accepted with no
// error so the serving peer is never penalized for replaying an
// already-finalized range (D4/S1).
func (n *Node) applySyncedBlock(data []byte) (bool, error) {
	block, err := decodeSyncedBlock(data)
	if err != nil {
		return false, err
	}
	if block.Header.Height <= n.finalizedHeight {
		return true, nil
	}
	parent, prev := n.loadParentHeader(block)
	snapID := n.state.Snapshot()
	// Sync variant: blocks being replayed were produced seconds ago and the
	// ±5s live-gossip freshness window must not reject them; every
	// chain-relative check (epoch, proposer, validator set, hashes, commit
	// proof, re-executed state root) still applies.
	if err := consensus.ValidateBlockForSync(block, parent, prev, n.state, n.hasher, n.vm, n.cfg.BlockTimeSec); err != nil {
		_ = n.state.RevertToSnapshot(snapID)
		return false, err
	}
	n.applyAcceptedBlock(block)
	return true, nil
}
