package badgerx

import (
	"errors"
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
