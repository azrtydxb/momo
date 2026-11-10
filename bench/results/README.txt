Layout of bench/results
=======================

make bench writes one directory per run, named <UTC timestamp>-<goos>-<goarch>,
for example 20261108T093000Z-linux-amd64. It holds:

  env.json        the machine, toolchain, VCS revision and the pinned module
                  versions, plus the PostgreSQL and pgvector versions when
                  BENCH_PG_URL is set (go run ./cmd/envcapture)
  versions.txt    go list -m all of the bench module
  codesize.json   generated-code size and compile cost of bench/models/momo
  raw.txt         go test -bench output, BENCH_COUNT repetitions (default 10)
  summary.txt     benchstat over raw.txt
  vector-encoding.txt
                  BenchmarkVectorEncoding of driver/pgxdriver: text against
                  binary pgx codecs for vector, halfvec, sparsevec and bit
                  (written when BENCH_PG_URL is set)
  benchstat.err   benchstat diagnostics

This file describes the layout only. The numbers and their interpretation live
in docs/PERFORMANCE.md.
