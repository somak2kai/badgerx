// Package badgerx extends [github.com/dgraph-io/badger/v4] with pluggable
// encoding and compression strategies, removing the need to manually serialize
// and compress values at every call site.
//
// # Design
//
// BadgerXDb wraps a badger.DB and applies an [Encoder] and a [Compressor] on
// every read and write. Both are swappable via functional options, defaulting
// to [GobEncoderDecoder] and [DefaultNoOpCompressor] when not specified.
//
// # Quick start
//
//	db, _ := badger.Open(badger.DefaultOptions("/tmp/mydb"))
//
//	// default: gob encoding, no compression
//	xdb := badgerx.NewBadgerXDb(db)
//	defer xdb.Close()
//
//	type User struct{ Name string; Age int }
//
//	_ = xdb.Update([]byte("user:1"), User{Name: "somak", Age: 30})
//
//	var u User
//	_ = xdb.View([]byte("user:1"), &u)
//
// # Encoders
//
// Two encoders are provided out of the box:
//   - [GobEncoderDecoder] — default, best for Go-to-Go communication.
//   - [JsonEncoderDecoder] — human-readable, good for interoperability.
//
// Implement the [Encoder] interface to supply your own (e.g. msgpack, protobuf).
//
// # Compressors
//
// Three compressors are provided:
//   - [DefaultNoOpCompressor] — default, no compression.
//   - [ZstdCompressor] — high compression ratio, fast decompression.
//   - [SnappyCompressor] — very fast, moderate compression ratio.
//
// Implement the [Compressor] interface to supply your own.
package badgerx

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/klauspost/compress/snappy"
	"github.com/klauspost/compress/zstd"
)

// compile-time interface checks.
var (
	_ Encoder    = (*GobEncoderDecoder)(nil)
	_ Compressor = (*ZstdCompressor)(nil)
	_ Compressor = (*SnappyCompressor)(nil)
	_ Compressor = (*DefaultNoOpCompressor)(nil)
)

// Encoder is the strategy interface for serializing and deserializing values.
//
// Encode must convert v into a byte slice suitable for storage in badger.
// Decode must reconstruct the original value from data into v, where v is
// always a non-nil pointer to the target type (e.g. *MyStruct).
//
// Implement this interface to provide a custom encoding strategy such as
// msgpack, protobuf, or any other binary format.
type Encoder interface {
	// Encode serializes v into a byte slice.
	Encode(v any) ([]byte, error)
	// Decode deserializes data into v. v must be a non-nil pointer.
	Decode(data []byte, v any) error
}

// Compressor is the strategy interface for compressing and decompressing
// encoded byte slices before they are written to or after they are read
// from badger.
//
// Implement this interface to provide a custom compression strategy.
// Close must release any resources held by the compressor.
type Compressor interface {
	// Compress compresses data and returns the compressed bytes.
	Compress(data []byte) ([]byte, error)
	// Decompress decompresses data and returns the original bytes.
	Decompress(data []byte) ([]byte, error)
	// Close releases any resources held by the compressor.
	Close() error
}

// BdOptions is a functional option for configuring a [BadgerXDb].
// Use [WithEncoder] and [WithCompressor] to create options.
type BdOptions func(*BadgerXDb)

// BadgerXDb wraps a [badger.DB] with pluggable encoding and compression.
// Values are encoded then compressed on write, and decompressed then decoded
// on read, transparently at every Update and View call.
//
// Use [NewBadgerXDb] to create an instance. Always call [BadgerXDb.Close]
// when done to release resources held by the compressor and the underlying DB.
type BadgerXDb struct {
	db         *badger.DB
	encoder    Encoder
	compressor Compressor
}

// WithEncoder returns a [BdOptions] that sets the encoder used by [BadgerXDb].
// If not specified, [GobEncoderDecoder] is used by default.
func WithEncoder(e Encoder) BdOptions {
	return func(db *BadgerXDb) {
		db.encoder = e
	}
}

// WithCompressor returns a [BdOptions] that sets the compressor used by [BadgerXDb].
// If not specified, [DefaultNoOpCompressor] is used by default (no compression).
func WithCompressor(c Compressor) BdOptions {
	return func(db *BadgerXDb) {
		db.compressor = c
	}
}

// NewBadgerXDb creates a new BadgerXDb wrapping the given [badger.DB].
// Defaults to [GobEncoderDecoder] and [DefaultNoOpCompressor] unless
// overridden via [WithEncoder] or [WithCompressor].
//
//	xdb := badgerx.NewBadgerXDb(db,
//	    badgerx.WithEncoder(&badgerx.JsonEncoderDecoder{}),
//	    badgerx.WithCompressor(zstdC),
//	)
func NewBadgerXDb(db *badger.DB, opts ...BdOptions) *BadgerXDb {
	bd := &BadgerXDb{
		db:         db,
		encoder:    &GobEncoderDecoder{},
		compressor: &DefaultNoOpCompressor{},
	}
	for _, opt := range opts {
		opt(bd)
	}
	return bd
}

// Close releases resources held by the compressor and closes the underlying
// badger DB. Always defer Close after creating a BadgerXDb:
//
//	xdb := badgerx.NewBadgerXDb(db)
//	defer xdb.Close()
//
// If both the compressor and the DB return errors on close, both are returned
// joined via [errors.Join].
func (d *BadgerXDb) Close() error {
	return errors.Join(d.compressor.Close(), d.db.Close())
}

// Update encodes value using the configured [Encoder], optionally compresses
// it using the configured [Compressor], and stores the result under key.
//
// The same encoder and compressor must be active when reading the value back
// via [BadgerXDb.View].
func (d *BadgerXDb) Update(key []byte, value any) error {
	if d.encoder == nil {
		return fmt.Errorf("no encoder found, unable to save data")
	}
	val, err := d.encoder.Encode(value)
	if err != nil {
		return fmt.Errorf("unable to encode data: %w", err)
	}
	val, err = d.compressor.Compress(val)
	if err != nil {
		return fmt.Errorf("unable to compress data: %w", err)
	}
	return d.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, val)
	})
}

// View retrieves the value stored under key, decompresses it, and decodes it
// into v. v must be a non-nil pointer to the same type that was passed to
// [BadgerXDb.Update].
//
// Returns [badger.ErrKeyNotFound] if the key does not exist.
//
//	var u User
//	if err := xdb.View([]byte("user:1"), &u); errors.Is(err, badger.ErrKeyNotFound) {
//	    // key not found
//	}
func (d *BadgerXDb) View(key []byte, v any) error {
	return d.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			val, err := d.compressor.Decompress(val)
			if err != nil {
				return err
			}
			return d.encoder.Decode(val, v)
		})
	})
}

// GobEncoderDecoder implements [Encoder] using the standard [encoding/gob] package.
// It is the default encoder used by [BadgerXDb] when no encoder is specified.
//
// For structs that contain interface{} fields, call [GobEncoderDecoder.RegisterType]
// once at startup for each concrete type that may appear in those fields.
type GobEncoderDecoder struct{}

// RegisterType registers a concrete type with gob so it can be correctly
// encoded and decoded when stored inside an interface{} field.
// This is only required when your structs contain interface{} fields.
// Call once at application startup — not on every read or write.
//
//	enc := &badgerx.GobEncoderDecoder{}
//	enc.RegisterType(MyPayload{})
//	enc.RegisterType(AnotherPayload{})
//
//	xdb := badgerx.NewBadgerXDb(db, badgerx.WithEncoder(enc))
func (g *GobEncoderDecoder) RegisterType(v any) {
	gob.Register(v)
}

// Encode serializes v into gob-encoded bytes.
func (g *GobEncoderDecoder) Encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decode deserializes gob-encoded data into v. v must be a non-nil pointer.
func (g *GobEncoderDecoder) Decode(data []byte, v any) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	return dec.Decode(v)
}

// JsonEncoderDecoder implements [Encoder] using the standard [encoding/json] package.
// Prefer this over [GobEncoderDecoder] when human-readable storage or
// cross-language interoperability is required.
// Values must be JSON-serializable (exported fields, json struct tags recommended).
type JsonEncoderDecoder struct{}

// Encode serializes v into JSON-encoded bytes.
func (j *JsonEncoderDecoder) Encode(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Decode deserializes JSON-encoded data into v. v must be a non-nil pointer.
func (j *JsonEncoderDecoder) Decode(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// DefaultNoOpCompressor implements [Compressor] with no compression.
// It is the default compressor used by [BadgerXDb] when no compressor is specified.
// Use [WithCompressor] to swap in [ZstdCompressor] or [SnappyCompressor]
// when storage efficiency matters.
type DefaultNoOpCompressor struct{}

// Compress returns data unmodified.
func (z *DefaultNoOpCompressor) Compress(data []byte) ([]byte, error) { return data, nil }

// Decompress returns data unmodified.
func (z *DefaultNoOpCompressor) Decompress(data []byte) ([]byte, error) { return data, nil }

// Close is a no-op for DefaultNoOpCompressor.
func (z *DefaultNoOpCompressor) Close() error { return nil }

// ZstdCompressor implements [Compressor] using the Zstandard algorithm.
// It offers an excellent compression ratio with fast decompression, making it
// well-suited for workloads where storage size matters more than write speed.
//
// The encoder and decoder are created once and reused across calls — safe for
// concurrent use. Use [NewZstdCompressor] to create an instance.
type ZstdCompressor struct {
	reader *zstd.Decoder
	writer *zstd.Encoder
}

// NewZstdCompressor creates a [ZstdCompressor] initialising reusable zstd
// encoder and decoder instances. The returned compressor is safe for concurrent use.
// Call [ZstdCompressor.Close] when done to release resources.
func NewZstdCompressor() (*ZstdCompressor, error) {
	writer, err := zstd.NewWriter(nil)
	if err != nil {
		return nil, fmt.Errorf("zstd writer init: %w", err)
	}
	reader, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("zstd reader init: %w", err)
	}
	return &ZstdCompressor{writer: writer, reader: reader}, nil
}

// Compress compresses data using the Zstandard algorithm.
func (z *ZstdCompressor) Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	return z.writer.EncodeAll(data, make([]byte, 0, len(data))), nil
}

// Decompress decompresses zstd-compressed data.
func (z *ZstdCompressor) Decompress(data []byte) ([]byte, error) {
	return z.reader.DecodeAll(data, nil)
}

// Close releases resources held by the zstd encoder and decoder.
func (z *ZstdCompressor) Close() error {
	z.reader.Close()
	return z.writer.Close()
}

// SnappyCompressor implements [Compressor] using the Snappy algorithm.
// It prioritises speed over compression ratio, making it a good fit for
// latency-sensitive workloads with large values where some size reduction
// is still desirable.
type SnappyCompressor struct{}

// Compress compresses data using the Snappy algorithm.
func (s *SnappyCompressor) Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	return snappy.Encode(nil, data), nil
}

// Decompress decompresses snappy-compressed data.
func (s *SnappyCompressor) Decompress(data []byte) ([]byte, error) {
	return snappy.Decode(nil, data)
}

// Close is a no-op for SnappyCompressor.
func (s *SnappyCompressor) Close() error { return nil }
