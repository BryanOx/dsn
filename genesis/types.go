// Package genesis provides canonical genesis file loading, validation, and state initialization.
package genesis

import (
	"encoding/json"
	"time"
)

// GenesisDoc represents the canonical genesis configuration for a DSN network.
type GenesisDoc struct {
	GenesisTime     time.Time         `json:"genesis_time"`
	ChainID         string            `json:"chain_id"`
	InitialHeight   uint64            `json:"initial_height"`
	ConsensusParams ConsensusParams   `json:"consensus_params"`
	EpochParams     EpochParams       `json:"epoch_params"`
	InflationParams InflationParams   `json:"inflation_params"`
	InitialValidators []ValidatorEntry `json:"initial_validators"`
	InitialBalances  []BalanceEntry   `json:"initial_balances"`
	Treasury        TreasuryEntry     `json:"treasury"`
}

// ConsensusParams defines block-level constraints enforced by the consensus engine.
type ConsensusParams struct {
	MaxTxPerBlock   uint64 `json:"max_tx_per_block"`
	MaxBytesPerBlock uint64 `json:"max_bytes_per_block"`
	MaxGasPerBlock  uint64 `json:"max_gas_per_block"`
}

// EpochParams defines epoch-level configuration.
type EpochParams struct {
	BlocksPerEpoch       uint64 `json:"blocks_per_epoch"`
	UnstakeCooldownEpochs uint64 `json:"unstake_cooldown_epochs"`
	MaxValidators        uint64 `json:"max_validators"`
	MinimumStake         uint64 `json:"minimum_stake"`
}

// InflationParams defines token minting and inflation behavior.
type InflationParams struct {
	Enabled    bool   `json:"enabled"`
	AnnualRate string `json:"annual_rate,omitempty"`
	MintPerBlock uint64 `json:"mint_per_block,omitempty"`
}

// ValidatorEntry defines an initial validator in the genesis file.
type ValidatorEntry struct {
	Address      string `json:"address"`
	PubKey       string `json:"pub_key"`
	ConsensusKey string `json:"consensus_key"`
	Stake        uint64 `json:"stake"`
	Commission   string `json:"commission"`
}

// BalanceEntry defines an initial account balance in the genesis file.
type BalanceEntry struct {
	Address string `json:"address"`
	Amount  uint64 `json:"amount"`
}

// TreasuryEntry defines the treasury account.
type TreasuryEntry struct {
	Address        string `json:"address"`
	InitialBalance uint64 `json:"initial_balance"`
}

// MarshalGenesis marshals a GenesisDoc to canonical JSON bytes.
func MarshalGenesis(doc *GenesisDoc) ([]byte, error) {
	return json.Marshal(doc)
}

// UnmarshalGenesis parses canonical JSON bytes into a GenesisDoc.
func UnmarshalGenesis(data []byte) (*GenesisDoc, error) {
	var doc GenesisDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}