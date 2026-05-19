package state

import (
	"testing"

	"github.com/dsn/dsn/types"
)

func TestSMT_New(t *testing.T) {
	hasher := types.SHA256Hasher{}
	smt := NewSMT(hasher)
	if smt == nil {
		t.Fatal("NewSMT returned nil")
	}
	var emptyHash types.Hash
	if smt.Root() != emptyHash {
		t.Error("new SMT should have zero root")
	}
}

func TestSMT_InsertAndGet(t *testing.T) {
	hasher := types.SHA256Hasher{}
	smt := NewSMT(hasher)

	key := []byte("account:abc123")
	value := []byte(`{"balance":100}`)

	err := smt.Insert(key, value)
	if err != nil {
		t.Fatal(err)
	}

	got, ok := smt.Get(key)
	if !ok {
		t.Fatal("key not found after insert")
	}
	if string(got) != string(value) {
		t.Errorf("got %s, want %s", got, value)
	}
}

func TestSMT_InsertTwice_UpdatesValue(t *testing.T) {
	hasher := types.SHA256Hasher{}
	smt := NewSMT(hasher)

	key := []byte("mykey")
	err := smt.Insert(key, []byte("v1"))
	if err != nil {
		t.Fatal(err)
	}

	err = smt.Insert(key, []byte("v2"))
	if err != nil {
		t.Fatal(err)
	}

	got, _ := smt.Get(key)
	if string(got) != "v2" {
		t.Errorf("got %s, want v2", got)
	}
}

func TestSMT_GetNonExistent(t *testing.T) {
	hasher := types.SHA256Hasher{}
	smt := NewSMT(hasher)

	_, ok := smt.Get([]byte("nonexistent"))
	if ok {
		t.Error("should not find nonexistent key")
	}
}

func TestSMT_Delete(t *testing.T) {
	hasher := types.SHA256Hasher{}
	smt := NewSMT(hasher)

	key := []byte("todelete")
	smt.Insert(key, []byte("value"))

	smt.Delete(key)

	_, ok := smt.Get(key)
	if ok {
		t.Error("key should be deleted")
	}
}

func TestSMT_RootChangesOnInsert(t *testing.T) {
	hasher := types.SHA256Hasher{}
	smt := NewSMT(hasher)

	root0 := smt.Root()

	smt.Insert([]byte("a"), []byte("1"))
	root1 := smt.Root()

	if root0 == root1 {
		t.Error("root should change after insert")
	}
}

func TestSMT_RootDeterminism(t *testing.T) {
	hasher := types.SHA256Hasher{}
	smt1 := NewSMT(hasher)
	smt2 := NewSMT(hasher)

	smt1.Insert([]byte("alice"), []byte("100"))
	smt1.Insert([]byte("bob"), []byte("200"))

	smt2.Insert([]byte("alice"), []byte("100"))
	smt2.Insert([]byte("bob"), []byte("200"))

	if smt1.Root() != smt2.Root() {
		t.Error("same inserts should produce same root")
	}
}

func TestSMT_DifferentInserts_DifferentRoots(t *testing.T) {
	hasher := types.SHA256Hasher{}
	smt1 := NewSMT(hasher)
	smt2 := NewSMT(hasher)

	smt1.Insert([]byte("alice"), []byte("100"))
	smt2.Insert([]byte("alice"), []byte("999"))

	if smt1.Root() == smt2.Root() {
		t.Error("different values should produce different roots")
	}
}

func TestSMT_EmptyRoot(t *testing.T) {
	hasher := types.SHA256Hasher{}
	smt := NewSMT(hasher)

	var expected types.Hash
	if smt.Root() != expected {
		t.Error("empty SMT should have zero root")
	}
}

func TestSMT_MultipleKeys(t *testing.T) {
	hasher := types.SHA256Hasher{}
	smt := NewSMT(hasher)

	keys := [][]byte{[]byte("k1"), []byte("k2"), []byte("k3"), []byte("k4"), []byte("k5")}
	for i, k := range keys {
		smt.Insert(k, []byte{byte(i)})
	}

	for i, k := range keys {
		v, ok := smt.Get(k)
		if !ok {
			t.Errorf("key %s not found", k)
			continue
		}
		if len(v) != 1 || v[0] != byte(i) {
			t.Errorf("key %s: got %v, want %v", k, v, []byte{byte(i)})
		}
	}
}