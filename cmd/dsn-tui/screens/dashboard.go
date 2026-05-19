package screens

import (
	"fmt"

	"github.com/dsn/dsn/cmd/dsn-tui/client"
)

// DashboardModel shows node status overview.
type DashboardModel struct {
	client    *client.Client
	stateRoot string
	balance   string
	account   *client.AccountResult
	loading   bool
	err       string
}

func NewDashboard(client *client.Client) *DashboardModel {
	return &DashboardModel{
		client:  client,
		loading: true,
	}
}

func (m *DashboardModel) Init() {
	m.loading = true
	m.err = ""
	go func() {
		root, err := m.client.GetStateRoot()
		if err != nil {
			m.err = err.Error()
			m.loading = false
			return
		}
		m.stateRoot = root
		m.loading = false
	}()
}

func (m *DashboardModel) Update() {
	m.Init()
}

func (m *DashboardModel) SetWalletAddr(addr string) {
	if addr == "" {
		return
	}
	go func() {
		acc, err := m.client.GetAccount(addr)
		if err != nil {
			return
		}
		m.account = acc
		m.balance = acc.Balance
	}()
}

func (m *DashboardModel) View(width int) string {
	s := " Node Status\n\n"

	if m.loading {
		s += " Loading...\n"
		return s
	}

	if m.err != "" {
		s += fmt.Sprintf(" Error: %s\n", m.err)
		return s
	}

	s += fmt.Sprintf(" State Root: %s\n", truncate(m.stateRoot, 16))

	if m.account != nil {
		s += fmt.Sprintf(" Wallet:     %s\n", truncate(m.account.Address, 16))
		s += fmt.Sprintf(" Balance:    %s DSN\n", m.balance)
		s += fmt.Sprintf(" Nonce:      %d\n", m.account.Nonce)
	} else {
		s += " Wallet:     not loaded (use --wallet flag)\n"
	}

	s += "\n"
	s += fmt.Sprintf(" RPC: connected\n")

	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
