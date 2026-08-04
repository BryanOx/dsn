package screens

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/BryanOx/dsn/cmd/dsn-tui/client"
	"github.com/BryanOx/dsn/types"
	"github.com/BryanOx/dsn/wallet"
)

type sendResultMsg struct {
	intentID string
	err      error
}

// SendModel lets the user build, sign, and send a transaction.
type SendModel struct {
	client    *client.Client
	kp        *wallet.KeyPair
	toInput   textinput.Model
	amtInput  textinput.Model
	result    string
	loading   bool
	err       string
	done      bool
	confirmed bool
	nonce     uint64 // tracks current nonce for transactions
}

func NewSend(cl *client.Client, kp *wallet.KeyPair) *SendModel {
	to := textinput.New()
	to.Placeholder = "DSN1recipient..."
	to.CharLimit = 50
	to.Width = 40

	amt := textinput.New()
	amt.Placeholder = "100"
	amt.CharLimit = 20
	amt.Width = 20

	return &SendModel{
		client:   cl,
		kp:       kp,
		toInput:  to,
		amtInput: amt,
		nonce:    1, // TODO: fetch from dsn_getAccount RPC for accuracy
	}
}

func (m *SendModel) Init() {
	m.toInput.Focus()
}

func (m *SendModel) Update(msg tea.Msg) (*SendModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case sendResultMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.result = msg.intentID
			m.done = true
		}
		return m, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyEnter && !m.loading {
			if !m.confirmed {
				to := strings.TrimSpace(m.toInput.Value())
				amt := strings.TrimSpace(m.amtInput.Value())
				if to == "" || amt == "" {
					return m, nil
				}
				m.confirmed = true
				return m, nil
			}

			// Build and send
			m.loading = true
			m.err = ""
			m.done = false
			m.confirmed = false
			return m, m.sendTx()
		}

		if msg.Type == tea.KeyEsc {
			m.confirmed = false
		}
	}

	if !m.confirmed {
		if m.toInput.Focused() {
			m.toInput, cmd = m.toInput.Update(msg)
			return m, cmd
		}
		m.amtInput, cmd = m.amtInput.Update(msg)
		return m, cmd
	}

	return m, cmd
}

func (m *SendModel) sendTx() tea.Cmd {
	return func() tea.Msg {
		to := strings.TrimSpace(m.toInput.Value())
		amt := strings.TrimSpace(m.amtInput.Value())

		hasher := types.SHA256Hasher{}

		// Parse amount
		var amount uint64
		fmt.Sscanf(amt, "%d", &amount)

		// Parse recipient address from string
		recipientAddr, err := types.ParseAddress(to)
		if err != nil {
			return sendResultMsg{err: fmt.Errorf("invalid recipient address: %w", err)}
		}

		// Build payload: 20 bytes recipient address + 8 bytes amount (big-endian)
		payload := make([]byte, 28)
		copy(payload[:20], recipientAddr.Bytes()) // recipient address bytes
		binary.BigEndian.PutUint64(payload[20:28], amount)

		tx := types.NewTransaction(
			1,                         // version (uint16)
			0,                         // chainID (uint32)
			m.kp.Address(),            // sender
			m.nonce,                   // nonce (tracked, increment after send)
			payload,                   // payload: recipient(20) + amount(8)
			nil,                       // constraints (none for basic transfer)
			10,                        // maxFee (fixed for TUI)
			100000,                    // gasLimit
			uint64(time.Now().Unix()), // timestamp
		)

		intentID, err := tx.ComputeIntentID(hasher)
		if err != nil {
			return sendResultMsg{err: fmt.Errorf("compute intent: %w", err)}
		}
		tx.IntentID = intentID

		// Sign in place
		if err := m.kp.Sign(tx, hasher); err != nil {
			return sendResultMsg{err: fmt.Errorf("sign: %w", err)}
		}

		result, err := m.client.SendTransaction(tx)
		if err != nil {
			return sendResultMsg{intentID: result, err: err}
		}

		// Increment nonce after successful send
		m.nonce++

		return sendResultMsg{intentID: result, err: err}
	}
}

func (m *SendModel) View(width int) string {
	s := " Send Transaction\n\n"

	if m.kp == nil {
		s += " No wallet loaded. Use --wallet flag.\n"
		return s
	}

	if !m.confirmed {
		s += fmt.Sprintf(" From: %s\n\n", m.kp.Address().String())
		s += " To:  " + m.toInput.View() + "\n"
		s += " Amt: " + m.amtInput.View() + " DSN\n\n"
		s += " Tab to switch fields, Enter to continue\n"
		return s
	}

	if m.loading {
		s += fmt.Sprintf("\n Sending %s DSN to %s...\n",
			m.amtInput.Value(), m.toInput.Value())
		return s
	}

	if m.err != "" {
		s += fmt.Sprintf(" Error: %s\n", m.err)
		s += "\n Press Esc to try again\n"
		return s
	}

	if m.done {
		s += fmt.Sprintf(" Transaction sent!\n")
		s += fmt.Sprintf(" IntentID: %s\n", m.result)
	}

	return s
}
