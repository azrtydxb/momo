# momo-07 Task 8: pgx binary codecs for all four vector types

Status: open
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each
Carried: 002-generator-from-go-structs-with-its-type-prerequisites — Criterion [S-50] unmet: BenchmarkVectorEncoding results are not recorded under bench/results (the directory does not exist; plan assigns it to momo-12). Binary codecs themselves exist in driver/pgxdriver/vector_codec.go with TestVectorBinaryCodec.

## Description

Implements plan momo-07-vectors Task 8: pgx binary codecs for all four vector types (`.procoder/plans/momo-07-vectors.md:1339`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-50. Files: `driver/pgxdriver/vector_codec.go`, `driver/pgxdriver/vector_codec_test.go`, `driver/pgxdriver/vector_bench_test.go`, `driver/pgxdriver/pgxdriver.go`.

## Acceptance criteria

- [x] `TestVectorBinaryCodec` exists as written in the plan, failed before the implementation, and passes now
- [x] `BenchmarkVectorEncoding` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-50] `TestVectorBinaryCodec` round-trips all four types in binary mode and matches text-mode results; `BenchmarkVectorEncoding` results recorded under bench/results; fails if binary and text decode differently.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- RED: `go vet ./driver/pgxdriver` before implementation: `undefined: VectorFormat`, `undefined: ConfigureVectors`, `undefined: WithVectorFormat`. GREEN: `go test -tags integration -race -count=1 ./driver/pgxdriver -run Vector` ok; TestVectorBinaryCodec passes in 6 subtests (text and binary x exec, describe, cache statement), all equal to the input and to each other.
- Wire bytes: TestVectorBinaryWireMatchesPgvector compares the encoder with the live pgvector `vector_send`, `halfvec_send`, `sparsevec_send` and `bit_send`, and decodes those bytes back. TestVectorBinarySQLComparison checks `$1::vector = '[1,2.5,-3]'::vector` (and the other three types) with binary params. TestVectorCopyFrom covers COPY.
- Mutation: writing sparsevec indices 1-based (`uint32(i)+1`) failed the wire test, the binary round trips, COPY, the SQL comparison and the fuzz seed; reverted.
- Fuzz: `FuzzVectorBinaryDecode` 20s, 850k execs, no crash; added to Makefile FUZZ_TARGETS.
- Benchmark (-benchtime=2s, ns/op text vs binary): vector-1536 626M/533M, halfvec-3072 924M/847M, sparsevec-1000 558M/404M, bit-1024 314M/392M (per 1000 inserts). The plan's benchmark uses the default QueryExecModeExec, which always sends text params (pgx Build with no statement description), so "binary" there measures codec-planned text, not binary wire. Real binary needs WithExecMode(DescribeExec/CacheStatement). Results are not written under bench/results: the plan assigns that to momo-12.
- Sprint 7: bench/results now exists (momo-12). `bench/scripts/run.sh` (full and smoke, not baseline) runs `BenchmarkVectorEncoding` of driver/pgxdriver when BENCH_PG_URL is set and writes it to `<run>/vector-encoding.txt` in the results directory; `TestBenchResultsLayout` requires that file when BENCH_PG_URL is set, and docs/PERFORMANCE.md plus bench/results/README.txt describe it. As docs/PERFORMANCE.md says, run directories are produced by `make bench` on a dedicated machine and are not committed, so recording means the run writes them there. CI: ci-agent run 42784116091, headSha 2eda85d2041b0d0f6ec5200f96facc8aeddc8f96, conclusion success; the bench-module job runs `TestBenchResultsLayout`. Whether that job sets BENCH_PG_URL, and so produces vector-encoding.txt, was not inspected.
- Gate: `procoder check` 0 blocking; `make staticcheck` clean; `go build ./...` and `go vet ./...` pass. Commit 3e58cbb.

