package ws

// SubscriptionHandler provides methods to be called by the indexer/node
// to broadcast events to WebSocket subscribers.
type SubscriptionHandler struct {
	hub *Hub
}

// NewSubscriptionHandler creates a new subscription handler.
func NewSubscriptionHandler(hub *Hub) *SubscriptionHandler {
	return &SubscriptionHandler{hub: hub}
}

// BroadcastNewHead broadcasts a new block header event.
func (h *SubscriptionHandler) BroadcastNewHead(block interface{}) {
	h.hub.Broadcast("newHeads", block)
}

// BroadcastLog broadcasts a log/event matching subscription filters.
func (h *SubscriptionHandler) BroadcastLog(log interface{}) {
	h.hub.Broadcast("logs", log)
}

// BroadcastPendingTransaction broadcasts a new pending transaction.
func (h *SubscriptionHandler) BroadcastPendingTransaction(txHash interface{}) {
	h.hub.Broadcast("pendingTransactions", txHash)
}

// LogFilter represents a filter for logs subscription.
type LogFilter struct {
	Contract string
	Topics   []string
}

// Matches checks if a log matches the filter.
func (f *LogFilter) Matches(contract string, topics []string) bool {
	// If no contract filter, match all
	if f.Contract != "" && f.Contract != contract {
		return false
	}

	// If no topics filter, match all
	if len(f.Topics) > 0 {
		// Check if any of the filter topics match the log topics
		for _, ft := range f.Topics {
			found := false
			for _, t := range topics {
				if ft == t {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	return true
}

// ClientSubscription holds a client's subscription with optional filter.
type ClientSubscription struct {
	Client *Client
	Filter interface{}
}