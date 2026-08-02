package screens

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dsn/dsn/cmd/dsn-tui/client"
	"github.com/dsn/dsn/internal/txfile"
	"github.com/dsn/dsn/types"
	"github.com/dsn/dsn/wallet"
)

type txFilesState int

const (
	stateListing txFilesState = iota
	stateViewing
	stateSignConfirm
	stateSigning
	stateSigned
	stateSendConfirm
	stateSending
	stateSent
	stateError
)

type txFileEntry struct {
	path   string
	signed bool
	info   *txfile.TransactionFile
}

type TxFilesModel struct {
	client *client.Client
	kp     *wallet.KeyPair

	state  txFilesState
	files  []txFileEntry
	cursor int
	err    string
	result string

	viewing      *txFileEntry
	confirmInput textinput.Model
}

func NewTxFiles(cl *client.Client, kp *wallet.KeyPair) *TxFilesModel {
	return &TxFilesModel{
		client:       cl,
		kp:           kp,
		state:        stateListing,
		files:        []txFileEntry{},
		cursor:       0,
		confirmInput: textinput.New(),
	}
}

func (m *TxFilesModel) Init() {
	m.refresh()
}

func (m *TxFilesModel) refresh() {
	m.files = nil
	m.err = ""

	matches, err := filepath.Glob("transactions/tx-*.json")
	if err != nil {
		m.err = fmt.Sprintf("Error scanning tx files: %v", err)
		return
	}

	sort.Strings(matches)

	for _, path := range matches {
		tf, err := txfile.ParseFile(path)
		if err != nil {
			continue
		}
		m.files = append(m.files, txFileEntry{
			path:   path,
			signed: tf.IsSigned(),
			info:   tf,
		})
	}
}

func (m *TxFilesModel) Update(msg tea.Msg) (*TxFilesModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.state {
		case stateListing:
			return m.handleListingKey(msg)
		case stateViewing:
			return m.handleViewingKey(msg)
		case stateSignConfirm:
			return m.handleSignConfirmKey(msg)
		case stateSendConfirm:
			return m.handleSendConfirmKey(msg)
		case stateSigned, stateSent:
			return m.handleDoneKey(msg)
		case stateSending:
			return m, nil
		case stateError:
			if msg.Type == tea.KeyEsc || msg.Type == tea.KeyEnter {
				m.state = stateListing
				m.refresh()
			}
			return m, nil
		}
	}
	return m, nil
}

func (m *TxFilesModel) handleListingKey(msg tea.KeyMsg) (*TxFilesModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown:
		if m.cursor < len(m.files)-1 {
			m.cursor++
		}
	case tea.KeyEnter:
		if len(m.files) == 0 {
			return m, nil
		}
		m.viewing = &m.files[m.cursor]
		m.state = stateViewing
	case tea.KeyRunes:
		if msg.String() == "r" {
			m.refresh()
		}
	}
	return m, nil
}

func (m *TxFilesModel) handleViewingKey(msg tea.KeyMsg) (*TxFilesModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.state = stateListing
		m.viewing = nil
	case tea.KeyEnter:
		if m.viewing == nil {
			return m, nil
		}
		if !m.viewing.signed && m.kp != nil {
			m.state = stateSignConfirm
		} else if m.viewing.signed {
			m.state = stateSendConfirm
		}
	}
	return m, nil
}

func (m *TxFilesModel) handleSignConfirmKey(msg tea.KeyMsg) (*TxFilesModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.state = stateViewing
	case tea.KeyEnter:
		return m, m.doSign()
	}
	return m, nil
}

func (m *TxFilesModel) handleSendConfirmKey(msg tea.KeyMsg) (*TxFilesModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.state = stateViewing
	case tea.KeyEnter:
		return m, m.doSend()
	}
	return m, nil
}

func (m *TxFilesModel) handleDoneKey(msg tea.KeyMsg) (*TxFilesModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyEnter:
		m.state = stateListing
		m.refresh()
	}
	return m, nil
}

func (m *TxFilesModel) doSign() tea.Cmd {
	return func() tea.Msg {
		if m.viewing == nil || m.kp == nil {
			return nil
		}

		tf, err := txfile.ParseFile(m.viewing.path)
		if err != nil {
			m.err = fmt.Sprintf("Error reading tx file: %v", err)
			m.state = stateError
			return nil
		}

		if tf.IsSigned() {
			m.err = "Transaction is already signed"
			m.state = stateError
			return nil
		}

		hasher := types.SHA256Hasher{}
		tx, err := tf.ToTransaction(hasher)
		if err != nil {
			m.err = fmt.Sprintf("Error converting tx: %v", err)
			m.state = stateError
			return nil
		}

		if err := m.kp.Sign(tx, hasher); err != nil {
			m.err = fmt.Sprintf("Error signing: %v", err)
			m.state = stateError
			return nil
		}

		signedTF := txfile.FromTransaction(tx)
		ext := filepath.Ext(m.viewing.path)
		base := strings.TrimSuffix(m.viewing.path, ext)
		outPath := base + "-signed" + ext

		if err := txfile.WriteFile(signedTF, outPath); err != nil {
			m.err = fmt.Sprintf("Error writing signed file: %v", err)
			m.state = stateError
			return nil
		}

		m.result = outPath
		m.state = stateSigned
		return nil
	}
}

func (m *TxFilesModel) doSend() tea.Cmd {
	return func() tea.Msg {
		if m.viewing == nil {
			return nil
		}

		tf, err := txfile.ParseFile(m.viewing.path)
		if err != nil {
			m.err = fmt.Sprintf("Error reading tx file: %v", err)
			m.state = stateError
			return nil
		}

		if !tf.IsSigned() {
			m.err = "Transaction is not signed"
			m.state = stateError
			return nil
		}

		hasher := types.SHA256Hasher{}
		tx, err := tf.ToTransaction(hasher)
		if err != nil {
			m.err = fmt.Sprintf("Error converting tx: %v", err)
			m.state = stateError
			return nil
		}

		result, err := m.client.SendTransaction(tx)
		if err != nil {
			m.err = fmt.Sprintf("Error sending: %v", err)
			m.state = stateError
			return nil
		}

		m.result = result
		m.state = stateSent
		return nil
	}
}

func (m *TxFilesModel) View(width int) string {
	switch m.state {
	case stateListing:
		return m.viewListing()
	case stateViewing:
		return m.viewDetails()
	case stateSignConfirm:
		return m.viewSignConfirm()
	case stateSigning:
		return " Signing transaction...\n"
	case stateSigned:
		return fmt.Sprintf(" Transaction signed!\n\n Signed file: %s\n\n Press Enter to continue\n", m.result)
	case stateSendConfirm:
		return m.viewSendConfirm()
	case stateSending:
		return " Sending transaction...\n"
	case stateSent:
		return fmt.Sprintf(" Transaction sent!\n\n Result: %s\n\n Press Enter to continue\n", m.result)
	case stateError:
		return fmt.Sprintf(" Error: %s\n\n Press Enter to continue\n", m.err)
	}
	return ""
}

func (m *TxFilesModel) viewListing() string {
	s := " Transaction Files\n\n"

	if m.err != "" {
		s += fmt.Sprintf(" Error: %s\n\n", m.err)
	}

	if len(m.files) == 0 {
		s += " No tx files found in transactions/ directory.\n"
		s += " Use 'dsn tx create' in the CLI to create one.\n"
		s += "\n r — refresh\n"
		return s
	}

	for i, f := range m.files {
		cursor := "  "
		if i == m.cursor {
			cursor = " >"
		}
		status := "unsigned"
		if f.signed {
			status = "SIGNED"
		}
		sender := "?"
		if f.info != nil && f.info.Sender != "" {
			sender = f.info.Sender
			if len(sender) > 10 {
				sender = sender[:10] + "…"
			}
		}
		s += fmt.Sprintf(" %s %s [%s] %s\n", cursor, f.path, status, sender)
	}

	s += "\n ↑/↓ navigate • Enter view • r refresh\n"
	return s
}

func (m *TxFilesModel) viewDetails() string {
	if m.viewing == nil {
		return ""
	}

	tf := m.viewing.info
	s := fmt.Sprintf(" Transaction: %s\n\n", m.viewing.path)

	s += fmt.Sprintf(" Version:     %d\n", tf.Version)
	s += fmt.Sprintf(" ChainID:     %d\n", tf.ChainID)
	s += fmt.Sprintf(" Sender:      %s\n", tf.Sender)
	s += fmt.Sprintf(" Nonce:       %d\n", tf.Nonce)
	s += fmt.Sprintf(" Payload:     %s\n", tf.Payload)
	s += fmt.Sprintf(" MaxFee:      %d\n", tf.MaxFee)
	s += fmt.Sprintf(" GasLimit:    %d\n", tf.GasLimit)
	s += fmt.Sprintf(" Timestamp:   %d\n", tf.Timestamp)
	s += fmt.Sprintf(" TxType:      %d\n", tf.TxType)

	if tf.Signature != "" {
		sig := tf.Signature
		if len(sig) > 20 {
			sig = sig[:20] + "…"
		}
		s += fmt.Sprintf(" Signature:   %s\n", sig)
		id := tf.IntentID
		if len(id) > 20 {
			id = id[:20] + "…"
		}
		s += fmt.Sprintf(" IntentID:    %s\n", id)
	}

	s += "\n"
	if !m.viewing.signed && m.kp != nil {
		s += " Enter — Sign this transaction\n"
	} else if m.viewing.signed {
		s += " Enter — Send this transaction\n"
	} else if !m.viewing.signed && m.kp == nil {
		s += " No wallet loaded. Use --wallet flag to sign.\n"
	}
	s += " Esc — Back to list\n"

	return s
}

func (m *TxFilesModel) viewSignConfirm() string {
	return fmt.Sprintf(" Sign transaction %s?\n\n Enter to confirm • Esc to cancel\n", m.viewing.path)
}

func (m *TxFilesModel) viewSendConfirm() string {
	return fmt.Sprintf(" Send transaction %s?\n\n Enter to confirm • Esc to cancel\n", m.viewing.path)
}
