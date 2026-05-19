package vm

import (
	"testing"

	"github.com/dsn/dsn/types"
	"github.com/stretchr/testify/require"
)

func TestEventLog_Empty(t *testing.T) {
	el := NewEventLog()
	require.Equal(t, 0, el.Len())
	require.Empty(t, el.Events())
}

func TestEventLog_Emit(t *testing.T) {
	el := NewEventLog()
	contractID := types.Hash{1, 2, 3}
	el.Emit(contractID, "test_topic", []byte("hello"), 0, 100)

	require.Equal(t, 1, el.Len())
	events := el.Events()
	require.Len(t, events, 1)
	require.Equal(t, contractID, events[0].ContractID)
	require.Equal(t, "test_topic", events[0].Topic)
	require.Equal(t, []byte("hello"), events[0].Data)
	require.Equal(t, uint32(0), events[0].TxIndex)
	require.Equal(t, uint64(100), events[0].BlockHeight)
}

func TestEventLog_Order(t *testing.T) {
	el := NewEventLog()
	contractID := types.Hash{1}
	el.Emit(contractID, "first", []byte("a"), 0, 100)
	el.Emit(contractID, "second", []byte("b"), 1, 100)
	el.Emit(contractID, "third", []byte("c"), 2, 100)

	events := el.Events()
	require.Len(t, events, 3)
	require.Equal(t, "first", events[0].Topic)
	require.Equal(t, "second", events[1].Topic)
	require.Equal(t, "third", events[2].Topic)
	require.Equal(t, uint32(0), events[0].TxIndex)
	require.Equal(t, uint32(1), events[1].TxIndex)
	require.Equal(t, uint32(2), events[2].TxIndex)
}

func TestEventLog_Multiple(t *testing.T) {
	el := NewEventLog()
	cid1 := types.Hash{1}
	cid2 := types.Hash{2}

	el.Emit(cid1, "topic1", []byte("data1"), 0, 100)
	el.Emit(cid2, "topic2", []byte("data2"), 1, 100)

	events := el.Events()
	require.Len(t, events, 2)
	require.Equal(t, cid1, events[0].ContractID)
	require.Equal(t, cid2, events[1].ContractID)
	require.Equal(t, uint32(0), events[0].TxIndex)
	require.Equal(t, uint32(1), events[1].TxIndex)
}
