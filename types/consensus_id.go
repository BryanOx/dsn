package types

import "crypto/sha256"

// ConsensusIDLength is the byte length of a ConsensusID (same as Address).
const ConsensusIDLength = 20

// DeriveConsensusID computes the canonical validator consensus identity
// from an Ed25519 public key.
//
// consensus_id = SHA256(consensus_pubkey)[:20]
func DeriveConsensusID(pubKey [32]byte) Address {
	hash := sha256.Sum256(pubKey[:])
	var id Address
	copy(id[:], hash[:20])
	return id
}

// DeriveConsensusIDFromBytes computes the canonical validator consensus identity
// from a byte slice public key.
func DeriveConsensusIDFromBytes(pubKey []byte) Address {
	hash := sha256.Sum256(pubKey)
	var id Address
	copy(id[:], hash[:20])
	return id
}
