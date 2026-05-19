package screens

import (
	"fmt"

	"github.com/dsn/dsn/cmd/dsn-tui/client"
	"github.com/dsn/dsn/types"
)

// PendingModel shows pending mempool transactions.
type PendingModel struct {
	client   *client.Client
	txs      []types.Transaction
	loading  bool
	err      string
}

func NewPending(cl *client.Client) *PendingModel {
	return &PendingModel{
		client:  cl,
		loading: true,
	}
}

func (m *PendingModel) Init() {
	m.loading = true
	m.err = ""
	go func() {
		txs, err := m.client.GetPendingTxs()
		if err != nil {
			m.err = err.Error()
			m.loading = false
			return
		}
		m.txs = txs
		m.loading = false
	}()
}

func (m *PendingModel) Update() {
	m.Init()
}

func (m *PendingModel) View(width int) string {
	s := " Pending Transactions\n\n"

	if m.loading {
		s += " Loading...\n"
		return s
	}

	if m.err != "" {
		s += fmt.Sprintf(" Error: %s\n", m.err)
		return s
	}

	if len(m.txs) == 0 {
		s += " No pending transactions.\n"
		return s
	}

	s += fmt.Sprintf(" %d transaction(s) in mempool\n\n", len(m.txs))

	for i, tx := range m.txs {
		s += fmt.Sprintf(" %d. IntentID: %x\n", i+1, tx.IntentID[:8])
		s += fmt.Sprintf("    From:     %s\n", tx.Sender.String())
		s += fmt.Sprintf("    MaxFee:   %d\n", tx.MaxFee)
		s += fmt.Sprintf("    GasLimit: %d\n", tx.GasLimit)
		s += fmt.Sprintf("    Nonce:    %d\n", tx.Nonce)
		s += "\n"
	}

	return s
}
