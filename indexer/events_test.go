package indexer

import (
	"fmt"
	"testing"

	"github.com/BryanOx/dsn/types"
	"go.etcd.io/bbolt"
)

func makeEvent(contractID types.Hash, topic string, blockHeight uint64, txIndex uint32) *types.Event {
	return &types.Event{
		ContractID:  contractID,
		Topic:       topic,
		Data:        []byte("event-data"),
		BlockHeight: blockHeight,
		TxIndex:     txIndex,
	}
}

func TestEventIndexerFilterByContractAndTopic(t *testing.T) {
	idx, cleanup := newTestIndexer(t)
	defer cleanup()

	var c1, c2 types.Hash
	c1[0] = 0x01
	c2[0] = 0x02

	// Index events from C1 and C2 with overlapping topic "transfer"
	for i := uint64(1); i <= 5; i++ {
		event := makeEvent(c1, "transfer", i, 0)
		if err := indexEvent(idx.db, event); err != nil {
			t.Fatalf("indexEvent C1 failed: %v", err)
		}
	}
	for i := uint64(1); i <= 5; i++ {
		event := makeEvent(c2, "transfer", i, 0)
		if err := indexEvent(idx.db, event); err != nil {
			t.Fatalf("indexEvent C2 failed: %v", err)
		}
	}

	// Filter: contract=C1, topic="transfer"
	filter := EventFilter{
		Contract: &c1,
		Topic0:   "transfer",
	}
	results, err := idx.GetEvents(filter)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}

	if len(results) != 5 {
		t.Fatalf("got %d events, want 5 (C1 only)", len(results))
	}

	for _, ed := range results {
		if ed.Event.ContractID != c1 {
			t.Errorf("unexpected contract ID: %x, want C1", ed.Event.ContractID)
		}
		if ed.Event.Topic != "transfer" {
			t.Errorf("unexpected topic: %q, want transfer", ed.Event.Topic)
		}
	}
}

func TestEventIndexerEmptyFilterReturnsAll(t *testing.T) {
	idx, cleanup := newTestIndexer(t)
	defer cleanup()

	var c1 types.Hash
	c1[0] = 0x01

	// Index 100 events from different blocks
	for i := uint64(0); i < 100; i++ {
		event := makeEvent(c1, "log", i, 0)
		if err := indexEvent(idx.db, event); err != nil {
			t.Fatalf("indexEvent failed at block %d: %v", i, err)
		}
	}

	// Empty filter — should return all
	filter := EventFilter{}
	results, err := idx.GetEvents(filter)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}

	if len(results) != 100 {
		t.Errorf("got %d events, want 100 (empty filter returns all)", len(results))
	}
}

func TestEventIndexerFilterByTopicOnly(t *testing.T) {
	idx, cleanup := newTestIndexer(t)
	defer cleanup()

	var c1 types.Hash
	c1[0] = 0x01

	// Index events with different topics
	topics := []string{"transfer", "approval", "mint"}
	for i, topic := range topics {
		for j := uint64(1); j <= 3; j++ {
			event := makeEvent(c1, topic, j, uint32(i))
			if err := indexEvent(idx.db, event); err != nil {
				t.Fatalf("indexEvent failed: %v", err)
			}
		}
	}

	// Filter by topic only (no contract)
	filter := EventFilter{
		Topic0: "transfer",
	}
	results, err := idx.GetEvents(filter)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("got %d events, want 3 (transfer topic only)", len(results))
	}
}

func TestEventIndexerFilterByBlockRange(t *testing.T) {
	idx, cleanup := newTestIndexer(t)
	defer cleanup()

	var c1 types.Hash
	c1[0] = 0x01

	// Index 10 events across blocks 1-10
	for i := uint64(1); i <= 10; i++ {
		event := makeEvent(c1, "log", i, 0)
		if err := indexEvent(idx.db, event); err != nil {
			t.Fatalf("indexEvent failed: %v", err)
		}
	}

	// Filter: blocks 3-7
	filter := EventFilter{
		FromBlock: 3,
		ToBlock:   7,
	}
	results, err := idx.GetEvents(filter)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}

	if len(results) != 5 {
		t.Errorf("got %d events, want 5 (blocks 3-7)", len(results))
	}
}

func TestGetEventsByBlock(t *testing.T) {
	idx, cleanup := newTestIndexer(t)
	defer cleanup()

	var c1 types.Hash
	c1[0] = 0x01

	// Index 3 events at block 5
	for i := uint32(0); i < 3; i++ {
		event := makeEvent(c1, "transfer", 5, i)
		if err := indexEvent(idx.db, event); err != nil {
			t.Fatalf("indexEvent failed: %v", err)
		}
	}

	// Index 2 events at block 6
	for i := uint32(0); i < 2; i++ {
		event := makeEvent(c1, "transfer", 6, i)
		if err := indexEvent(idx.db, event); err != nil {
			t.Fatalf("indexEvent failed: %v", err)
		}
	}

	results, err := idx.GetEventsByBlock(5)
	if err != nil {
		t.Fatalf("GetEventsByBlock failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("got %d events, want 3 (block 5)", len(results))
	}

	for _, ed := range results {
		if ed.BlockNumber != 5 {
			t.Errorf("event at block %d, want 5", ed.BlockNumber)
		}
	}
}

func TestEventIndexerManyContractsSameTopic(t *testing.T) {
	idx, cleanup := newTestIndexer(t)
	defer cleanup()

	// Create 10 different contracts
	contracts := make([]types.Hash, 10)
	for i := range contracts {
		contracts[i][0] = byte(i + 1)
	}

	// Index 2 events per contract
	for ci, c := range contracts {
		for j := uint64(1); j <= 2; j++ {
			event := makeEvent(c, "transfer", j, uint32(ci))
			if err := indexEvent(idx.db, event); err != nil {
				t.Fatalf("indexEvent failed: %v", err)
			}
		}
	}

	// Filter for contract[3] only
	filter := EventFilter{
		Contract: &contracts[3],
		Topic0:   "transfer",
	}
	results, err := idx.GetEvents(filter)
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("got %d events, want 2 (contract[3] only)", len(results))
	}

	// Total events in DB should be 20
	allResults, err := idx.GetEvents(EventFilter{})
	if err != nil {
		t.Fatalf("GetEvents (all) failed: %v", err)
	}
	if len(allResults) != 20 {
		t.Errorf("total events = %d, want 20", len(allResults))
	}
}

// BenchmarkEventIndexer is a simple benchmark for the event indexer.
func BenchmarkEventIndexer(b *testing.B) {
	idx, cleanup := newTestIndexerB(b)
	defer cleanup()

	var c1 types.Hash
	c1[0] = 0x01

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		blockNum := uint64(i % 1000)
		event := &types.Event{
			ContractID:  c1,
			Topic:       fmt.Sprintf("topic-%d", i%5),
			Data:        []byte("data"),
			BlockHeight: blockNum,
			TxIndex:     0,
		}
		if err := indexEvent(idx.db, event); err != nil {
			b.Fatalf("indexEvent failed: %v", err)
		}
	}
}

func newTestIndexerB(b *testing.B) (*Indexer, func()) {
	b.Helper()
	dir := b.TempDir()
	dbPath := dir + "/test-indexer.db"
	db, err := bbolt.Open(dbPath, 0600, nil)
	if err != nil {
		b.Fatalf("failed to open test db: %v", err)
	}

	if err := initSchema(db); err != nil {
		db.Close()
		b.Fatalf("failed to init schema: %v", err)
	}

	idx := &Indexer{
		db:      db,
		ch:      make(chan *IndexableBlock, 100),
		done:    make(chan struct{}),
		enabled: true,
	}

	return idx, func() {
		db.Close()
	}
}
