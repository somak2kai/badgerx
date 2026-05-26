# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

---

## [0.1.0] - 2026-05-26

### Added

- `BadgerXDb` — core wrapper around `badger.DB` with pluggable encoding and compression via functional options.
- `Encoder` interface — strategy interface for serialization. Implement to provide custom encoding (msgpack, protobuf, etc.).
- `Compressor` interface — strategy interface for compression. Implement to provide custom compression algorithms.
- `GobEncoderDecoder` — default encoder using `encoding/gob`. Includes `RegisterType` for structs with `interface{}` fields.
- `JsonEncoderDecoder` — encoder using `encoding/json` for human-readable or cross-language storage.
- `DefaultNoOpCompressor` — default compressor with no compression overhead.
- `ZstdCompressor` — compressor using Zstandard via `github.com/klauspost/compress/zstd`. Reuses encoder/decoder instances across calls for performance.
- `SnappyCompressor` — compressor using Snappy via `github.com/klauspost/compress/snappy`. Optimised for speed.
- `NewBadgerXDb` — constructor with functional options pattern. Defaults to `GobEncoderDecoder` and `DefaultNoOpCompressor`.
- `NewZstdCompressor` — constructor for `ZstdCompressor` that initialises reusable zstd encoder and decoder.
- `BadgerXDb.Update` — encodes and compresses a value then writes it to badger under the given key.
- `BadgerXDb.View` — reads, decompresses, and decodes a value from badger into the given pointer.
- `BadgerXDb.Close` — closes the compressor and the underlying badger DB, surfacing both errors via `errors.Join`.
- CI pipeline via GitHub Actions testing against Go 1.22, 1.23, and 1.24 with race detection enabled.

[Unreleased]: https://github.com/somak2kai/badgerx/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/somak2kai/badgerx/releases/tag/v0.1.0
