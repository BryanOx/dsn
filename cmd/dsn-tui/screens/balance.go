package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dsn/dsn/cmd/dsn-tui/client"
)

type balanceMsg struct {
	balance string
	err     error
}

// BalanceModel lets the user query an address balance.
type BalanceModel struct {
	client  *client.Client
	input   textinput.Model
	balance string
	addr    string
	loading bool
	err     string
	done    bool
}

func NewBalance(client *client.Client) *BalanceModel {
	ti := textinput.New()
	ti.Placeholder = "DSN1abc123..."
	ti.CharLimit = 50
	ti.Width = 40

	return &BalanceModel{
		client: client,
		input:  ti,
	}
}

func (m *BalanceModel) Init() {
	m.input.Focus()
}

func (m *BalanceModel) Update(msg tea.Msg) (*BalanceModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case balanceMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.balance = msg.balance
			m.done = true
		}
		return m, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyEnter && !m.loading {
			addr := strings.TrimSpace(m.input.Value())
			if addr == "" {
				return m, nil
			}
			m.addr = addr
			m.loading = true
			m.err = ""
			m.done = false
			return m, m.queryBalance(addr)
		}
	}

	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *BalanceModel) queryBalance(addr string) tea.Cmd {
	return func() tea.Msg {
		bal, err := m.client.GetBalance(addr)
		return balanceMsg{balance: bal, err: err}
	}
}

func (m *BalanceModel) View(width int) string {
	s := " Balance Query\n\n"
	s += m.input.View()
	s += "\n\n"

	if m.loading {
		s += " Querying balance...\n"
		return s
	}

	if m.err != "" {
		s += fmt.Sprintf(" Error: %s\n", m.err)
		return s
	}

	if m.done {
		s += fmt.Sprintf("\n Address: %s\n", m.addr)
		s += fmt.Sprintf(" Balance: %s DSN\n", m.balance)
	}

	s += "\n Enter an address and press Enter to query\n"
	return s
}
