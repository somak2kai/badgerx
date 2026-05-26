package badgerx

import (
	"fmt"
	"testing"

	badger "github.com/dgraph-io/badger/v4"
)

// benchRecord is the value used across all benchmarks.
type benchRecord struct {
	ID    int
	Name  string
	Score float64
	Tags  []string
}

var benchValue = benchRecord{
	ID:    1,
	Name:  "badgerx-benchmark",
	Score: 99.5,
	Tags:  []string{"fast", "embedded", "go", "badger"},
}

// openBenchDB opens a BadgerXDb in a temp directory for benchmarking.
func openBenchDB(b *testing.B, opts ...BdOptions) *BadgerXDb {
	b.Helper()
	bdb, err := badger.Open(badger.DefaultOptions(b.TempDir()).WithLogger(nil))
	if err != nil {
		b.Fatalf("open badger: %v", err)
	}
	xdb := NewBadgerXDb(bdb, opts...)
	b.Cleanup(func() { xdb.Close() })
	return xdb
}

// runUpdateBench is the shared benchmark body for Update.
func runUpdateBench(b *testing.B, xdb *BadgerXDb) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("bench:record:%d", i))
		if err := xdb.Update(key, benchValue); err != nil {
			b.Fatal(err)
		}
	}
}

// runViewBench is the shared benchmark body for View.
// It pre-populates b.N keys then measures read performance.
func runViewBench(b *testing.B, xdb *BadgerXDb) {
	b.Helper()
	keys := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		keys[i] = []byte(fmt.Sprintf("bench:record:%d", i))
		if err := xdb.Update(keys[i], benchValue); err != nil {
			b.Fatalf("pre-populate: %v", err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var got benchRecord
		if err := xdb.View(keys[i], &got); err != nil {
			b.Fatal(err)
		}
	}
}

// -- Update benchmarks --

func BenchmarkUpdate_GobNoOp(b *testing.B) {
	runUpdateBench(b, openBenchDB(b))
}

func BenchmarkUpdate_GobSnappy(b *testing.B) {
	runUpdateBench(b, openBenchDB(b, WithCompressor(&SnappyCompressor{})))
}

func BenchmarkUpdate_GobZstd(b *testing.B) {
	zstdC, err := NewZstdCompressor()
	if err != nil {
		b.Fatal(err)
	}
	runUpdateBench(b, openBenchDB(b, WithCompressor(zstdC)))
}

func BenchmarkUpdate_JsonNoOp(b *testing.B) {
	runUpdateBench(b, openBenchDB(b, WithEncoder(&JsonEncoderDecoder{})))
}

func BenchmarkUpdate_JsonSnappy(b *testing.B) {
	runUpdateBench(b, openBenchDB(b,
		WithEncoder(&JsonEncoderDecoder{}),
		WithCompressor(&SnappyCompressor{}),
	))
}

func BenchmarkUpdate_JsonZstd(b *testing.B) {
	zstdC, err := NewZstdCompressor()
	if err != nil {
		b.Fatal(err)
	}
	runUpdateBench(b, openBenchDB(b,
		WithEncoder(&JsonEncoderDecoder{}),
		WithCompressor(zstdC),
	))
}

// -- View benchmarks --

func BenchmarkView_GobNoOp(b *testing.B) {
	runViewBench(b, openBenchDB(b))
}

func BenchmarkView_GobSnappy(b *testing.B) {
	runViewBench(b, openBenchDB(b, WithCompressor(&SnappyCompressor{})))
}

func BenchmarkView_GobZstd(b *testing.B) {
	zstdC, err := NewZstdCompressor()
	if err != nil {
		b.Fatal(err)
	}
	runViewBench(b, openBenchDB(b, WithCompressor(zstdC)))
}

func BenchmarkView_JsonNoOp(b *testing.B) {
	runViewBench(b, openBenchDB(b, WithEncoder(&JsonEncoderDecoder{})))
}

func BenchmarkView_JsonSnappy(b *testing.B) {
	runViewBench(b, openBenchDB(b,
		WithEncoder(&JsonEncoderDecoder{}),
		WithCompressor(&SnappyCompressor{}),
	))
}

func BenchmarkView_JsonZstd(b *testing.B) {
	zstdC, err := NewZstdCompressor()
	if err != nil {
		b.Fatal(err)
	}
	runViewBench(b, openBenchDB(b,
		WithEncoder(&JsonEncoderDecoder{}),
		WithCompressor(zstdC),
	))
}
