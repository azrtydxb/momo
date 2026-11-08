# Extract internal mechanics: sqlscan, redact, keyenc, chunk

Status: open
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: -

## Description

Move the four pieces of non-API mechanics out of the root package into `internal/` (section 4 of `.procoder/research/package-split.md`), so the root file list shrinks and the mechanics get their own tests. `internal/sqlscan` takes the raw-SQL tokenizer from `raw_args.go` (`scanRaw`, `rawTok`, `rawTokKind`, `scanDollarParam`, `isIdentByte`, `isIdentStart`, `indexFrom`) with its fuzz test `FuzzScanRaw`; `internal/redact` takes `maskArgs` from `db.go` (the exported constant `momo.RedactedArg` stays and is defined from the internal one); `internal/keyenc` takes the key-tuple encoding from `keys.go`; `internal/chunk` takes the pure bind-limit arithmetic (`keysPerStatement`, `maxCompositeTuples`, and the split step of `chunkSize` and `runChunks`). Nothing exported changes. The Makefile `FUZZ_TARGETS` entry for the scanner changes to the new package path in the same commit, because `TestFuzzTargetsListed` fails otherwise.

## Acceptance criteria

- [ ] none of the four internal packages imports `github.com/azrtydxb/momo` (architecture test green; the packages take `dialect.Dialect` and plain Go types)
- [ ] `raw_args.go` keeps only `NamedArg`, `Named`, `In`, `Ident` and the builder-side rendering; `keys.go` is deleted; `maskArgs` is gone from `db.go` (file listing in the PR)
- [ ] `internal/sqlscan` carries the existing scanner unit tests and `FuzzScanRaw`, and the Makefile `FUZZ_TARGETS` lists `./internal/sqlscan` (`TestFuzzTargetsListed` green)
- [ ] `internal/redact` has a unit test that masked arguments are copies (mutating the returned `[]byte` does not change the driver slice) and that sensitive indexes out of range are ignored, carried over from `sensitive_args_internal_test.go`
- [ ] `internal/chunk` has table tests for the composite tuple cap and the SQLite cap with `MaxParams` set, carried over from `options_maxparams_limits_test.go`
- [ ] all existing root tests pass unchanged except import and package-name edits; no exported identifier is added or removed (`apidiff` empty)

## Evidence
