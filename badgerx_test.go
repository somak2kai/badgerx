package badgerx

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	badger "github.com/dgraph-io/badger/v4"
)

// testRecord is a plain struct used across round-trip tests.
type testRecord struct {
	Name  string
	Score int
	Tags  []string
}

// testRecordWithPayload has an interface{} field to exercise gob.RegisterType.
type testRecordWithPayload struct {
	Name    string
	Payload any
}

type innerPayload struct {
	Value string
}

// openTestDB opens a badger DB in a temp directory and closes it at test end.
func openTestDB(t *testing.T) *badger.DB {
	t.Helper()
	db, err := badger.Open(badger.DefaultOptions(t.TempDir()).WithLogger(nil))
	if err != nil {
		t.Fatalf("open badger: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestUpdateView_Combinations verifies all encoder/compressor combinations
// correctly round-trip a value through Update and View.
func TestUpdateView_Combinations(t *testing.T) {
	zstdC, err := NewZstdCompressor()
	if err != nil {
		t.Fatalf("zstd init: %v", err)
	}

	combos := []struct {
		name string
		opts []BdOptions
	}{
		{"gob+noop", nil},
		{"gob+snappy", []BdOptions{WithCompressor(&SnappyCompressor{})}},
		{"gob+zstd", []BdOptions{WithCompressor(zstdC)}},
		{"json+noop", []BdOptions{WithEncoder(&JsonEncoderDecoder{})}},
		{"json+snappy", []BdOptions{WithEncoder(&JsonEncoderDecoder{}), WithCompressor(&SnappyCompressor{})}},
		{"json+zstd", []BdOptions{WithEncoder(&JsonEncoderDecoder{}), WithCompressor(zstdC)}},
	}

	want := testRecord{Name: "badgerx", Score: 42, Tags: []string{"fast", "embedded", "go"}}
	key := []byte("test:record")

	for _, c := range combos {
		t.Run(c.name, func(t *testing.T) {
			db := NewBadgerXDb(openTestDB(t), c.opts...)

			if err := db.Update(key, want); err != nil {
				t.Fatalf("Update: %v", err)
			}

			var got testRecord
			if err := db.View(key, &got); err != nil {
				t.Fatalf("View: %v", err)
			}

			if !reflect.DeepEqual(got, want) {
				t.Errorf("got %+v, want %+v", got, want)
			}
		})
	}
}

// TestView_MissingKey verifies that viewing a non-existent key returns ErrKeyNotFound.
func TestView_MissingKey(t *testing.T) {
	db := NewBadgerXDb(openTestDB(t))

	var got testRecord
	err := db.View([]byte("nonexistent"), &got)
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
	if !errors.Is(err, badger.ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
}

// TestUpdate_NilEncoder verifies that Update returns an error when no encoder is set.
func TestUpdate_NilEncoder(t *testing.T) {
	db := &BadgerXDb{db: openTestDB(t), compressor: &DefaultNoOpCompressor{}}

	err := db.Update([]byte("key"), testRecord{Name: "test"})
	if err == nil {
		t.Fatal("expected error for nil encoder, got nil")
	}
}

// TestIterateView_Basic verifies that all records stored under a prefix are
// returned in order and correctly decoded into fresh variables each iteration.
func TestIterateView_Basic(t *testing.T) {
	db := NewBadgerXDb(openTestDB(t))

	want := []testRecord{
		{Name: "alice", Score: 1, Tags: []string{"a"}},
		{Name: "bob", Score: 2, Tags: []string{"b"}},
		{Name: "carol", Score: 3, Tags: []string{"c"}},
	}

	for i, r := range want {
		key := []byte(fmt.Sprintf("user:%d", i))
		if err := db.Update(key, r); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}

	var got []testRecord
	err := db.IterateView([]byte("user:"), badger.DefaultIteratorOptions, func(decode DecodeFunc) error {
		var r testRecord
		if err := decode(&r); err != nil {
			return err
		}
		got = append(got, r)
		return nil
	})
	if err != nil {
		t.Fatalf("IterateView: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// TestIterateView_PrefixIsolation verifies that only keys matching the given
// prefix are returned, not keys stored under a different prefix.
func TestIterateView_PrefixIsolation(t *testing.T) {
	db := NewBadgerXDb(openTestDB(t))

	_ = db.Update([]byte("user:1"), testRecord{Name: "alice"})
	_ = db.Update([]byte("user:2"), testRecord{Name: "bob"})
	_ = db.Update([]byte("score:1"), testRecord{Name: "should-not-appear"})

	var got []testRecord
	err := db.IterateView([]byte("user:"), badger.DefaultIteratorOptions, func(decode DecodeFunc) error {
		var r testRecord
		if err := decode(&r); err != nil {
			return err
		}
		got = append(got, r)
		return nil
	})
	if err != nil {
		t.Fatalf("IterateView: %v", err)
	}

	if len(got) != 2 {
		t.Errorf("expected 2 results, got %d", len(got))
	}
	for _, r := range got {
		if r.Name == "should-not-appear" {
			t.Error("non-matching prefix key appeared in results")
		}
	}
}

// TestIterateView_NoMatch verifies that iterating a prefix with no matching
// keys calls fn zero times and returns nil.
func TestIterateView_NoMatch(t *testing.T) {
	db := NewBadgerXDb(openTestDB(t))

	_ = db.Update([]byte("user:1"), testRecord{Name: "alice"})

	count := 0
	err := db.IterateView([]byte("score:"), badger.DefaultIteratorOptions, func(decode DecodeFunc) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("IterateView: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 calls, got %d", count)
	}
}

// TestIterateView_CallbackError verifies that a non-nil error returned from fn
// stops iteration and is surfaced as the return value of IterateView.
func TestIterateView_CallbackError(t *testing.T) {
	db := NewBadgerXDb(openTestDB(t))

	_ = db.Update([]byte("user:1"), testRecord{Name: "alice"})
	_ = db.Update([]byte("user:2"), testRecord{Name: "bob"})

	sentinel := fmt.Errorf("stop here")
	count := 0
	err := db.IterateView([]byte("user:"), badger.DefaultIteratorOptions, func(decode DecodeFunc) error {
		count++
		return sentinel
	})

	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
	if count != 1 {
		t.Errorf("expected fn called once before stopping, got %d", count)
	}
}

// TestIterateView_FreshVariablePerIteration verifies that each call to fn
// produces an independent value — not a shared pointer overwritten each loop.
func TestIterateView_FreshVariablePerIteration(t *testing.T) {
	db := NewBadgerXDb(openTestDB(t))

	_ = db.Update([]byte("user:1"), testRecord{Name: "alice", Score: 1})
	_ = db.Update([]byte("user:2"), testRecord{Name: "bob", Score: 2})

	var ptrs []*testRecord
	err := db.IterateView([]byte("user:"), badger.DefaultIteratorOptions, func(decode DecodeFunc) error {
		var r testRecord
		if err := decode(&r); err != nil {
			return err
		}
		ptrs = append(ptrs, &r) // store pointer to local var
		return nil
	})
	if err != nil {
		t.Fatalf("IterateView: %v", err)
	}

	if len(ptrs) != 2 {
		t.Fatalf("expected 2 results, got %d", len(ptrs))
	}
	// if v were shared both pointers would hold the last decoded value
	if ptrs[0].Name == ptrs[1].Name {
		t.Errorf("both pointers hold the same value — variable was shared across iterations")
	}
}

// TestGobRegisterType verifies that RegisterType allows encoding/decoding
// structs that contain interface{} fields.
func TestGobRegisterType(t *testing.T) {
	enc := &GobEncoderDecoder{}
	enc.RegisterType(innerPayload{})

	db := NewBadgerXDb(openTestDB(t), WithEncoder(enc))
	key := []byte("test:payload")

	want := testRecordWithPayload{Name: "badgerx", Payload: innerPayload{Value: "hello"}}

	if err := db.Update(key, want); err != nil {
		t.Fatalf("Update: %v", err)
	}

	var got testRecordWithPayload
	if err := db.View(key, &got); err != nil {
		t.Fatalf("View: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
