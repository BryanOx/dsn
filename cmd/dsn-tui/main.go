package main

import (
	"flag"
	"fmt"
	"log"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	rpcURL := flag.String("rpc", "http://localhost:8545", "DSN node RPC URL")
	walletPath := flag.String("wallet", "", "Path to wallet key file (optional for read-only)")
	flag.Parse()

	m := newModel(*rpcURL, *walletPath)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(fmt.Errorf("tui error: %w", err))
	}
}
