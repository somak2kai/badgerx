# badgerx

[![CI](https://github.com/somak2kai/badgerx/actions/workflows/ci.yml/badge.svg)](https://github.com/somak2kai/badgerx/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/somak2kai/badgerx?style=flat)](https://pkg.go.dev/github.com/somak2kai/badgerx)
[![Go Report Card](https://goreportcard.com/badge/github.com/somak2kai/badgerx)](https://goreportcard.com/report/github.com/somak2kai/badgerx)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**badgerx** is a thin, idiomatic Go wrapper around [BadgerDB](https://github.com/dgraph-io/badger) that abstracts away the repetitive encode/decode and compression boilerplate that every BadgerDB consumer ends up writing.

---

## Why badgerx?

BadgerDB is a fast, embeddable key-value store — but it speaks only `[]byte`. In practice, every application that uses it ends up writing the same scaffolding over and over:

```go
// without badgerx — repeated at every call site
func (d *MyDb) StoreUser(id string, u User) error {
    val, err := gobEncode(u)
    if err != nil {
        return err
    }
    return d.db.Update(func(txn *badger.Txn) error {
        return txn.Set([]byte("user:"+id), val)
    })
}

func (d *MyDb) LoadUser(id string) (User, error) {
    var u User
    err := d.db.View(func(txn *badger.Txn) error {
        item, err := txn.Get([]byte("user:" + id))
        if err != nil {
            return err
        }
        return item.Value(func(val []byte) error {
            return gobDecode(val, &u)
        })
    })
    return u, err
}
```

That pattern — encode, transact, decode — is repeated for every type you store. badgerx collapses it into two calls while keeping full control over *how* values are encoded and compressed through swappable strategies.

```go
// with badgerx
xdb := badgerx.NewBadgerXDb(db)

_ = xdb.Update([]byte("user:1"), u)

var got User
_ = xdb.View([]byte("user:1"), &got)
```

---

## Features

- **Pluggable encoding** — ships with `gob` and `json`. Bring your own (`msgpack`, `protobuf`, anything that serialises to `[]byte`).
- **Pluggable compression** — ships with `zstd`, `snappy`, and a no-op default. Implement the `Compressor` interface to add your own.
- **Strategy pattern** — encoder and compressor are swapped via functional options at construction time, not scattered across call sites.
- **Safe defaults** — `gob` encoding and no compression out of the box. Zero configuration required to get started.
- **No magic** — badgerx does not wrap the full BadgerDB API. It exposes `Update` and `View` for encoded access, and the underlying `badger.DB` remains accessible for everything else.

---

## Installation

```bash
go get github.com/somak2kai/badgerx
```

Requires Go 1.23 or later.

---

## Quick Start

```go
package main

import (
    "fmt"
    "log"

    badger "github.com/dgraph-io/badger/v4"
    "github.com/somak2kai/badgerx"
)

type User struct {
    Name string
    Age  int
}

func main() {
    db, err := badger.Open(badger.DefaultOptions("/tmp/mydb").WithLogger(nil))
    if err != nil {
        log.Fatal(err)
    }

    xdb := badgerx.NewBadgerXDb(db)
    defer xdb.Close()

    // store
    err = xdb.Update([]byte("user:1"), User{Name: "somak", Age: 30})
    if err != nil {
        log.Fatal(err)
    }

    // retrieve
    var u User
    err = xdb.View([]byte("user:1"), &u)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("%+v\n", u) // {Name:somak Age:30}
}
```

---

## Encoders

### GobEncoderDecoder (default)

Uses Go's built-in `encoding/gob`. Best for Go-to-Go communication where all readers and writers are Go programs.

```go
xdb := badgerx.NewBadgerXDb(db) // gob is the default
```

If your structs contain `interface{}` fields, register the concrete types once at startup:

```go
enc := &badgerx.GobEncoderDecoder{}
enc.RegisterType(ConcretePayload{})

xdb := badgerx.NewBadgerXDb(db, badgerx.WithEncoder(enc))
```

### JsonEncoderDecoder

Uses `encoding/json`. Human-readable storage, good for cross-language interoperability or when you need to inspect values directly in the database.

```go
xdb := badgerx.NewBadgerXDb(db, badgerx.WithEncoder(&badgerx.JsonEncoderDecoder{}))
```

### Custom Encoder

Implement the `Encoder` interface to use any serialisation format:

```go
type Encoder interface {
    Encode(v any) ([]byte, error)
    Decode(data []byte, v any) error
}
```

---

## Compressors

### DefaultNoOpCompressor (default)

No compression. Zero overhead. Use this when storage size is not a concern or when BadgerDB's built-in storage-level compression (zstd/snappy via `Options.Compression`) is sufficient.

### ZstdCompressor

[Zstandard](https://facebook.github.io/zstd/) compression. Excellent ratio with fast decompression. Best when storage efficiency matters.

```go
zstdC, err := badgerx.NewZstdCompressor()
if err != nil {
    log.Fatal(err)
}

xdb := badgerx.NewBadgerXDb(db, badgerx.WithCompressor(zstdC))
defer xdb.Close() // releases zstd encoder/decoder resources
```

### SnappyCompressor

[Snappy](https://github.com/google/snappy) compression. Prioritises speed over ratio. Good for latency-sensitive workloads with large values.

```go
xdb := badgerx.NewBadgerXDb(db, badgerx.WithCompressor(&badgerx.SnappyCompressor{}))
```

### Custom Compressor

Implement the `Compressor` interface to use any compression algorithm:

```go
type Compressor interface {
    Compress(data []byte) ([]byte, error)
    Decompress(data []byte) ([]byte, error)
    Close() error
}
```

---

## Combining Encoder and Compressor

Options compose freely:

```go
zstdC, _ := badgerx.NewZstdCompressor()

xdb := badgerx.NewBadgerXDb(db,
    badgerx.WithEncoder(&badgerx.JsonEncoderDecoder{}),
    badgerx.WithCompressor(zstdC),
)
defer xdb.Close()
```

---

## A note on compression layers

badgerx compression operates **per value** before the data reaches BadgerDB. This is distinct from BadgerDB's own storage-level compression (`Options.Compression`), which compresses value log blocks on disk. The two are complementary — you can use both simultaneously.

---

## Accessing the underlying BadgerDB

badgerx intentionally wraps only `Update` and `View`. For everything else — transactions, iterators, TTLs, subscriptions — use the underlying `badger.DB` directly. badgerx does not try to re-wrap the full API.

---

## Contributing

Contributions are welcome. Please open an issue before submitting a PR for significant changes. Run tests before pushing:

```bash
go test ./... -race
```

---

## License

MIT. See [LICENSE](LICENSE).
