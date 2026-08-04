package vm

import (
	"sync"

	"github.com/BryanOx/dsn/types"
)

// EventLog collects deterministic events during contract execution.
type EventLog struct {
	mu     sync.Mutex
	events []types.Event
}

// NewEventLog creates a new empty event log.
func NewEventLog() *EventLog {
	return &EventLog{
		events: make([]types.Event, 0),
	}
}

// Emit appends a deterministic event to the log.
func (el *EventLog) Emit(contractID types.Hash, topic string, data []byte, txIndex uint32, blockHeight uint64) {
	el.mu.Lock()
	defer el.mu.Unlock()

	el.events = append(el.events, types.Event{
		ContractID:  contractID,
		Topic:       topic,
		Data:        data,
		TxIndex:     txIndex,
		BlockHeight: blockHeight,
	})
}

// Events returns all emitted events in order (thread-safe copy).
func (el *EventLog) Events() []types.Event {
	el.mu.Lock()
	defer el.mu.Unlock()

	result := make([]types.Event, len(el.events))
	copy(result, el.events)
	return result
}

// Len returns the number of events.
func (el *EventLog) Len() int {
	el.mu.Lock()
	defer el.mu.Unlock()
	return len(el.events)
}
