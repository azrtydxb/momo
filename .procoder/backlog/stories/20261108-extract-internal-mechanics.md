# Extract internal mechanics: sqlscan, redact, keyenc, chunk

Status: done
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

Move the four pieces of non-API mechanics out of the root package into `internal/` (section 4 of `.procoder/research/package-split.md`), so the root file list shrinks and the mechanics get their own tests. `internal/sqlscan` takes the raw-SQL tokenizer from `raw_args.go` (`scanRaw`, `rawTok`, `rawTokKind`, `scanDollarParam`, `isIdentByte`, `isIdentStart`, `indexFrom`) with its fuzz test `FuzzScanRaw`; `internal/redact` takes `maskArgs` from `db.go` (the exported constant `momo.RedactedArg` stays and is defined from the internal one); `internal/keyenc` takes the key-tuple encoding from `keys.go`; `internal/chunk` takes the pure bind-limit arithmetic (`keysPerStatement`, `maxCompositeTuples`, and the split step of `chunkSize` and `runChunks`). Nothing exported changes. The Makefile `FUZZ_TARGETS` entry for the scanner changes to the new package path in the same commit, because `TestFuzzTargetsListed` fails otherwise.

## Acceptance criteria

- [x] none of the four internal packages imports `github.com/azrtydxb/momo` (architecture test green; the packages take `dialect.Dialect` and plain Go types)
- [x] `raw_args.go` keeps only `NamedArg`, `Named`, `In`, `Ident` and the builder-side rendering; `keys.go` is deleted; `maskArgs` is gone from `db.go` (file listing in the PR)
- [x] `internal/sqlscan` carries the existing scanner unit tests and `FuzzScanRaw`, and the Makefile `FUZZ_TARGETS` lists `./internal/sqlscan` (`TestFuzzTargetsListed` green)
- [x] `internal/redact` has a unit test that masked arguments are copies (mutating the returned `[]byte` does not change the driver slice) and that sensitive indexes out of range are ignored, carried over from `sensitive_args_internal_test.go`
- [x] `internal/chunk` has table tests for the composite tuple cap and the SQLite cap with `MaxParams` set, carried over from `options_maxparams_limits_test.go`
- [x] all existing root tests pass unchanged except import and package-name edits; no exported identifier is added or removed (`apidiff` empty)

## Evidence

Red-first does not apply: this is a pure move of existing code with no behaviour change; the tests that pin the behaviour already existed and the moved ones were carried over.

ci-agent run 42494595487 on b625fd0 (all jobs green: unit, integration postgres, integration-mysql mariadb/mysql84/mysql97, fuzz-smoke, bench-module, lint).

- Import graph: `TestImportGraph` in internal/repocheck passed (`ok internal/repocheck`) with the four packages added to its allow-list; the packages import only the standard library and `dialect`.
- File listing: keys.go deleted; `maskArgs` removed from db.go; raw_args.go keeps NamedArg, Named, In, Ident and builder rendering.
- sqlscan: `ok internal/sqlscan` in unit, race and fuzz-smoke (`make test-fuzz` runs `./internal/sqlscan:FuzzScanRaw`); tests TestScanTokens, TestScanUnterminated, TestIndexFrom, FuzzScanRaw; TestFuzzTargetsListed green in unit.
- redact: `ok internal/redact`; TestMaskReturnsCopies, TestMaskIgnoresOutOfRangeIndexes, TestMaskReplacesSensitivePositions.
- chunk: `ok internal/chunk`; TestKeysPerStatementPerDialect, TestKeysPerStatementReservedAndLimits (composite cap, SQLite cap, MaxParams), TestEachSplitsInOrder, TestEachEdges.
- keyenc: `ok internal/keyenc`; TestString, TestStringNormalization.
- Root tests green unchanged apart from import and call-site edits; no exported identifier added or removed (RedactedArg keeps its value, now `redact.Arg`).

Review follow-up: the review's fixes are on head e3b5e16, and ci-agent run 42575189091 passed all eight jobs on it. They fold `raw_render_seed_internal_test.go` into `raw_args_internal_test.go`, and add the root fuzz target `FuzzAppendRaw` to FUZZ_TARGETS. They rename the shadowing locals `chunk` to `part`, add the four internal-leaf lines in `packages.txt`, and drop the deleted files from the prefix-guard lists.
