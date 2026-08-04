package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/BryanOx/dsn/cmd/dsn-tui/client"
	"github.com/BryanOx/dsn/cmd/dsn-tui/screens"
	"github.com/BryanOx/dsn/internal/passphrase"
	"github.com/BryanOx/dsn/wallet"
)

// rootModel is the top-level Bubbletea model.
type rootModel struct {
	client    *client.Client
	kp        *wallet.KeyPair
	activeTab int
	width     int
	errMsg    string

	dashboard *screens.DashboardModel
	balance   *screens.BalanceModel
	send      *screens.SendModel
	txFiles   *screens.TxFilesModel
	pending   *screens.PendingModel
	walletScr *screens.WalletModel

	help help.Model
}

func newModel(rpcURL string, walletPath string) *rootModel {
	cl := client.New(rpcURL)

	var kp *wallet.KeyPair
	if walletPath != "" {
		// D11: prompt once at startup via the passphrase provider.
		// Non-TTY environments use a nil provider — legacy plaintext wallets
		// load directly; encrypted keystores fail gracefully (wallet stays nil).
		var provider wallet.PassphraseFunc
		if passphrase.IsTTY() {
			provider = func() (string, error) {
				return passphrase.PromptTwice("wallet passphrase")
			}
		}
		var err error
		kp, err = wallet.LoadKeyFile(walletPath, provider)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not load wallet %s: %v\n", walletPath, err)
			kp = nil
		}
	}

	m := &rootModel{
		client:    cl,
		kp:        kp,
		activeTab: 0,
		help:      help.New(),
	}

	m.dashboard = screens.NewDashboard(cl)
	m.balance = screens.NewBalance(cl)
	m.send = screens.NewSend(cl, kp)
	m.txFiles = screens.NewTxFiles(cl, kp)
	m.pending = screens.NewPending(cl)
	m.walletScr = screens.NewWallet(cl, kp)

	// Init dashboard on startup
	m.dashboard.Init()
	m.pending.Init()
	if kp != nil {
		m.dashboard.SetWalletAddr(kp.Address().String())
	}

	return m
}

func (m *rootModel) Init() tea.Cmd {
	m.balance.Init()
	return nil
}

func (m *rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.help.Width = msg.Width
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c", "q"))):
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("tab", "right"))):
			m.activeTab = (m.activeTab + 1) % len(allTabs)
			m.onTabChange()
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab", "left"))):
			m.activeTab = (m.activeTab - 1 + len(allTabs)) % len(allTabs)
			m.onTabChange()
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			m.refreshCurrent()
			return m, nil
		}
	}

	// Route messages to active screen
	switch m.activeTab {
	case 0: // Dashboard
		return m, cmd
	case 1: // Balance
		updated, balanceCmd := m.balance.Update(msg)
		m.balance = updated
		cmd = balanceCmd
	case 2: // Send
		updated, sendCmd := m.send.Update(msg)
		m.send = updated
		cmd = sendCmd
	case 3: // Tx Files
		updated, txCmd := m.txFiles.Update(msg)
		m.txFiles = updated
		cmd = txCmd
	case 4: // Pending
		return m, cmd
	case 5: // Wallet
		return m, cmd
	}

	return m, cmd
}

func (m *rootModel) onTabChange() {
	switch m.activeTab {
	case 0:
		m.dashboard.Init()
		if m.kp != nil {
			m.dashboard.SetWalletAddr(m.kp.Address().String())
		}
	case 1:
		m.balance.Init()
	case 3:
		m.txFiles.Init()
	case 4:
		m.pending.Init()
	}
}

func (m *rootModel) refreshCurrent() {
	switch m.activeTab {
	case 0:
		m.dashboard.Update()
	case 4:
		m.pending.Update()
	}
}

func (m *rootModel) View() string {
	var screen string

	switch m.activeTab {
	case 0:
		screen = m.dashboard.View(m.width)
	case 1:
		screen = m.balance.View(m.width)
	case 2:
		screen = m.send.View(m.width)
	case 3:
		screen = m.txFiles.View(m.width)
	case 4:
		screen = m.pending.View(m.width)
	case 5:
		screen = m.walletScr.View(m.width)
	}

	tabs := renderTabs(m.activeTab)
	help := helpStyle.Render("  tab/→/← nav • r refresh • q quit")

	return appStyle.Render(
		headerStyle.Render(" DSN — Deterministic Settlement Network ") + "\n" +
			tabs + "\n\n" +
			screen + "\n" +
			help,
	)
}
