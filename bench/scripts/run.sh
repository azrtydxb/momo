#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Runs the benchmark suite and writes one results directory:
#   <BENCH_RESULTS_DIR>/<UTC timestamp>-<goos>-<goarch>/
#     env.json      machine, toolchain and pinned module versions
#     versions.txt  go list -m all of the bench module
#     raw.txt       go test -bench output, BENCH_COUNT repetitions
#     summary.txt   benchstat over raw.txt
#
# Usage: run.sh full|smoke|baseline
#   full      BENCH_COUNT repetitions (default 10), for the dedicated machine
#   smoke     one iteration of each benchmark, for CI
#   baseline  allocation-gate benchmarks only, written to bench/baseline/allocs.txt
#
# BENCH_PG_URL points at the PostgreSQL server; without it the database
# benchmarks skip. The run never writes outside BENCH_RESULTS_DIR (and the
# baseline file in baseline mode).
set -euo pipefail

BENCHSTAT="golang.org/x/perf/cmd/benchstat@v0.0.0-20261009192801-be2c69fb417e"

mode="${1:-smoke}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
benchdir="$(cd "$here/.." && pwd)"
results="${BENCH_RESULTS_DIR:-$benchdir/results}"

case "$mode" in
full)
	count="${BENCH_COUNT:-10}"
	benchtime=()
	pattern="."
	;;
smoke)
	count=1
	benchtime=(-benchtime 1x)
	pattern="."
	;;
baseline)
	count=6
	benchtime=()
	pattern="BenchmarkAllocs_"
	;;
*)
	echo "usage: run.sh full|smoke|baseline" >&2
	exit 2
	;;
esac

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
cd "$benchdir"
goos="$(go env GOOS)"
goarch="$(go env GOARCH)"
run="$results/$stamp-$goos-$goarch"
mkdir -p "$run"

go run ./cmd/envcapture >"$run/env.json"
go list -m all >"$run/versions.txt"
go run ./cmd/codesize >"$run/codesize.json"

go test -tags integration -run '^$' -bench "$pattern" -benchmem -count "$count" ${benchtime[@]+"${benchtime[@]}"} ./... | tee "$run/raw.txt"

if ! go run "$BENCHSTAT" "$run/raw.txt" >"$run/summary.txt" 2>"$run/benchstat.err"; then
	echo "benchstat reported no comparable data; see benchstat.err" >"$run/summary.txt"
fi

if [ "$mode" = baseline ]; then
	mkdir -p "$benchdir/baseline"
	cp "$run/raw.txt" "$benchdir/baseline/allocs.txt"
fi
echo "results in $run"
