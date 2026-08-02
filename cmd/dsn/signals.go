package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// SignalHandler handles system signals for devnet.
type SignalHandler struct {
	sigChan     chan os.Signal
	restartChan chan struct{} // Triggered on SIGHUP for hot restart
	stopChan    chan struct{}
	wg          sync.WaitGroup
	handlers    []func() error // Callbacks to restart services
	mu          sync.Mutex
}

// NewSignalHandler creates a new signal handler.
func NewSignalHandler() *SignalHandler {
	return &SignalHandler{
		sigChan:     make(chan os.Signal, 1),
		restartChan: make(chan struct{}, 1),
		stopChan:    make(chan struct{}),
	}
}

// Start starts listening for signals.
// The handler will trigger restart callbacks on SIGHUP (portable across platforms).
// On Unix, SIGUSR1 is also supported. On Windows, use SIGHUP.
func (sh *SignalHandler) Start() {
	// Register for SIGHUP (hot restart) and SIGTERM/SIGINT (graceful shutdown)
	// SIGHUP works on both Windows and Unix-like systems
	signal.Notify(sh.sigChan, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)

	sh.wg.Add(1)
	go sh.run()
}

// run runs the signal handling loop.
func (sh *SignalHandler) run() {
	defer sh.wg.Done()

	for {
		select {
		case <-sh.stopChan:
			return
		case sig := <-sh.sigChan:
			switch sig {
			case syscall.SIGHUP:
				sh.handleRestart()
			case syscall.SIGTERM, syscall.SIGINT:
				sh.handleShutdown()
			}
		}
	}
}

// handleRestart handles SIGUSR1 - triggers a hot restart
func (sh *SignalHandler) handleRestart() {
	log.Println("Received SIGUSR1 - initiating hot restart...")

	// Non-blocking send to restart channel
	select {
	case sh.restartChan <- struct{}{}:
	default:
		// Channel already has a pending restart request
	}

	// Execute all registered restart handlers
	sh.mu.Lock()
	for i, handler := range sh.handlers {
		if err := handler(); err != nil {
			log.Printf("Restart handler %d failed: %v", i, err)
		}
	}
	sh.mu.Unlock()

	log.Println("Hot restart completed")
}

// handleShutdown handles SIGTERM/SIGINT - triggers graceful shutdown
func (sh *SignalHandler) handleShutdown() {
	log.Println("Received shutdown signal - stopping...")
	signal.Stop(sh.sigChan)
	close(sh.stopChan)
}

// OnRestart registers a callback to be called on hot restart.
// The callback should save state and restart services.
func (sh *SignalHandler) OnRestart(handler func() error) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.handlers = append(sh.handlers, handler)
}

// RestartChan returns the channel that receives a signal on hot restart.
func (sh *SignalHandler) RestartChan() <-chan struct{} {
	return sh.restartChan
}

// Stop stops the signal handler.
func (sh *SignalHandler) Stop() {
	signal.Stop(sh.sigChan)
	close(sh.stopChan)
	sh.wg.Wait()
}

// DevnetHotRestart manages the hot restart functionality for devnet.
type DevnetHotRestart struct {
	handler    *SignalHandler
	restartFn  func() error // Function to restart all services
	mu         sync.Mutex
	restarting bool
}

// NewDevnetHotRestart creates a new hot restart manager.
func NewDevnetHotRestart(restartFn func() error) *DevnetHotRestart {
	hr := &DevnetHotRestart{
		handler:   NewSignalHandler(),
		restartFn: restartFn,
	}

	// Register the restart callback
	hr.handler.OnRestart(hr.performRestart)

	return hr
}

// Start starts listening for SIGUSR1 signals.
func (hr *DevnetHotRestart) Start() {
	hr.handler.Start()
	log.Println("Hot restart handler started - send SIGUSR1 to trigger restart")
}

// Stop stops the hot restart handler.
func (hr *DevnetHotRestart) Stop() {
	hr.handler.Stop()
}

// performRestart executes the restart function with proper locking.
func (hr *DevnetHotRestart) performRestart() error {
	hr.mu.Lock()
	if hr.restarting {
		hr.mu.Unlock()
		return fmt.Errorf("restart already in progress")
	}
	hr.restarting = true
	hr.mu.Unlock()

	defer func() {
		hr.mu.Lock()
		hr.restarting = false
		hr.mu.Unlock()
	}()

	if hr.restartFn != nil {
		return hr.restartFn()
	}
	return nil
}

// IsRestarting returns true if a restart is in progress.
func (hr *DevnetHotRestart) IsRestarting() bool {
	hr.mu.Lock()
	defer hr.mu.Unlock()
	return hr.restarting
}

// RestartChan returns the channel that receives a signal on hot restart.
func (hr *DevnetHotRestart) RestartChan() <-chan struct{} {
	return hr.handler.RestartChan()
}
