package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dsn/dsn/sdk"
)

func main() {
	rpcURL := flag.String("rpc", "http://localhost:8545", "DSN RPC URL")
	contractAddr := flag.String("contract", "", "Contract address to monitor")
	pollInterval := flag.Duration("interval", 2*time.Second, "Poll interval")
	limit := flag.Uint64("limit", 100, "Events per poll")
	flag.Parse()

	if *contractAddr == "" {
		log.Fatal("--contract flag is required (e.g., --contract 0x1234...)")
	}

	client := sdk.New(*rpcURL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	fmt.Printf("Monitoring contract %s at %s\n", *contractAddr, *rpcURL)
	fmt.Printf("Poll interval: %s, Limit: %d\n", *pollInterval, *limit)
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()

	var offset uint64

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(*pollInterval):
		}

		filter := sdk.EventFilter{
			Contract: *contractAddr,
			FromBlock: 0,
			Offset:    offset,
			Limit:     *limit,
		}

		events, err := client.GetEvents(ctx, filter)
		if err != nil {
			log.Printf("Error fetching events: %v", err)
			continue
		}

		if len(events) == 0 {
			continue
		}

		for _, event := range events {
			fmt.Printf("[Block %d] Tx: %s\n", event.BlockNumber, event.TxHash)
			fmt.Printf("  Contract: %s\n", event.Contract)
			fmt.Printf("  Topics: %v\n", event.Topics)
			if event.Data != "" {
				data, _ := hex.DecodeString(event.Data)
				fmt.Printf("  Data (%d bytes): %x\n", len(data), data)
			}
			fmt.Println()
		}

		// Update offset for next poll
		offset += uint64(len(events))
		if uint64(len(events)) < *limit {
			// No more events to fetch
			offset = 0
		}
	}
}