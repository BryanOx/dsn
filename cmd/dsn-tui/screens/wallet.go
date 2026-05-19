package screens

import (
	"fmt"

	"github.com/dsn/dsn/cmd/dsn-tui/client"
	"github.com/dsn/dsn/wallet"
)

// WalletModel shows wallet information.
type WalletModel struct {
	client  *client.Client
	kp      *wallet.KeyPair
	balance string
	loading bool
	err     string
}

func NewWallet(cl *client.Client, kp *wallet.KeyPair) *WalletModel {
	m := &WalletModel{
		client: cl,
		kp:     kp,
	}
	if kp != nil {
		m.loading = true
		go func() {
			bal, err := cl.GetBalance(kp.Address().String())
			if err != nil {
				m.err = err.Error()
				m.loading = false
				return
			}
			m.balance = bal
			m.loading = false
		}()
	}
	return m
}

func (m *WalletModel) Init() {}

func (m *WalletModel) Update() {}

func (m *WalletModel) View(width int) string {
	s := " Wallet\n\n"

	if m.kp == nil {
		s += " No wallet loaded.\n"
		s += " Use --wallet <path> to load a wallet key file.\n"
		s += "\n"
		s += " To create a wallet, run:\n"
		s += "   dsn.exe -genesis\n"
		return s
	}

	s += fmt.Sprintf(" Address:    %s\n", m.kp.Address().String())
	s += fmt.Sprintf(" Public Key: %x\n", m.kp.PublicKey[:8])
	s += fmt.Sprintf(" Key File:   (loaded in memory)\n\n")

	if m.loading {
		s += " Fetching balance...\n"
	} else if m.err != "" {
		s += fmt.Sprintf(" Balance:    error (%s)\n", m.err)
	} else {
		s += fmt.Sprintf(" Balance:    %s DSN\n", m.balance)
	}

	return s
}
