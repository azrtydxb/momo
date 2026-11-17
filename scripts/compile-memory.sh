#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Compile-memory measurement for packages that call momo's typed builders
# (issue #217). Run from the repository root by
# .github/workflows/compile-memory.yml, after the fixture is generated:
#
#   go run ./internal/compilebench/gen -out internal/compilebench/work ...
#   go run ./cmd/momo generate ./internal/compilebench/work/models/...
#
# Subcommands (all read OUT, the directory for logs and profiles):
#
#   measure  compile the caller package three times, forcing a recompile of that
#            one package each time (a unique -gcflags value), under
#            /usr/bin/time -v: default, -l (no inlining) and -c=1 (no parallel
#            backend). Writes the peak-RSS table and fails when the default
#            peak exceeds PEAK_RSS_BUDGET_KB.
#   profile  build cmd/compile from GOROOT source (the shipped compile has no
#            symbols), compile the caller package with it through -toolexec and
#            -memprofile, and print the symbolised pprof tables.
#   nm       list the instantiated generic functions of the archives measure
#            wrote, grouped by generic function (internal/compilebench/nmreport).
#
# Nothing here belongs in a developer loop: the numbers are for CI runners.
set -euo pipefail

: "${OUT:?OUT must name the output directory}"
PKG=github.com/azrtydxb/momo/internal/compilebench/work/caller
FIXTURE=${FIXTURE:-internal/compilebench/work}
SUMMARY=${GITHUB_STEP_SUMMARY:-/dev/null}
mkdir -p "$OUT/bin"

# fence prints the first lines of a file as a code block.
fence() { # title file [lines]
	echo "#### $1"
	echo '```'
	head -n "${3:-40}" "$2"
	echo '```'
}

peak_kb() { awk -F': ' '/Maximum resident set size/ {print $2}' "$1"; }
wall() { awk -F': ' '/Elapsed \(wall clock\) time/ {print $2}' "$1"; }

# build compiles the caller package with flags for that package only. Every
# variant has a different -gcflags value, so Go recompiles the package and
# reuses the cached dependencies. The archive is kept for the nm report.
build() { # name flags
	/usr/bin/time -v -o "$OUT/time-$1.txt" \
		go build -gcflags="$PKG=$2" -o "$OUT/bin/caller-$1.a" "$PKG"
}

measure() {
	: "${PEAK_RSS_BUDGET_KB:?PEAK_RSS_BUDGET_KB must be set}"
	build default "-trimpath=x"
	build noinline "-l -trimpath=x"
	build c1 "-c=1 -trimpath=x"

	local lines files
	lines=$(cat "$FIXTURE"/caller/*.go | wc -l)
	files=$(find "$FIXTURE/caller" -name '*.go' | wc -l)
	{
		echo "### Compiler peak memory of the typed-query fixture"
		echo
		echo "$(go version); fixture: ${MODELS:-?} models, ${QUERIES:-?} queries, $lines caller lines in $files files."
		echo
		echo "| variant | gcflags | peak RSS (MiB) | wall |"
		echo "| --- | --- | ---: | ---: |"
		for v in default:"" noinline:"-l" c1:"-c=1"; do
			name=${v%%:*}
			kb=$(peak_kb "$OUT/time-$name.txt")
			echo "| $name | \`${v#*:}\` | $((kb / 1024)) | $(wall "$OUT/time-$name.txt") |"
		done
		echo
		echo "Budget for the default build: $((PEAK_RSS_BUDGET_KB / 1024)) MiB (PEAK_RSS_BUDGET_KB in .github/workflows/compile-memory.yml)."
	} | tee "$OUT/measure.md" >>"$SUMMARY"

	local got
	got=$(peak_kb "$OUT/time-default.txt")
	if [ "$got" -gt "$PEAK_RSS_BUDGET_KB" ]; then
		echo "::error::compiler peak RSS ${got} KB exceeds the budget of ${PEAK_RSS_BUDGET_KB} KB"
		exit 1
	fi
}

profile() {
	local compile="$OUT/bin/compile" wrapper="$OUT/bin/toolexec.sh"
	go build -o "$compile" cmd/compile

	# The wrapper swaps in the symbolised compiler only for the compile that
	# carries -memprofile, and passes every other tool (and the -V=full probe
	# that keys the build cache) through untouched.
	cat >"$wrapper" <<'EOF'
#!/bin/sh
tool=$1
shift
case "$(basename "$tool")" in
compile)
	case "$*" in
	*-memprofile=*) exec "$SYMBOLISED_COMPILE" "$@" ;;
	esac
	;;
esac
exec "$tool" "$@"
EOF
	chmod +x "$wrapper"
	export SYMBOLISED_COMPILE="$compile"

	/usr/bin/time -v -o "$OUT/time-profile.txt" \
		go build -toolexec "$wrapper" \
		-gcflags="$PKG=-memprofile=$OUT/mem.prof -memprofilerate=${MEMPROFILERATE:-32768} -trimpath=y" \
		-o /dev/null "$PKG"

	local idx
	for idx in alloc_space inuse_space; do
		go tool pprof -top -nodecount=40 -sample_index="$idx" "$compile" "$OUT/mem.prof" >"$OUT/pprof-$idx-flat.txt"
		go tool pprof -top -cum -nodecount=40 -sample_index="$idx" "$compile" "$OUT/mem.prof" >"$OUT/pprof-$idx-cum.txt"
	done
	{
		echo "### Symbolised compiler memory profile"
		echo
		echo "Heap profile written at compiler exit (after a GC), so inuse_space is what survives to the end and alloc_space is the cumulative allocation. Peak RSS of the profiled build: $(($(peak_kb "$OUT/time-profile.txt") / 1024)) MiB."
		echo
		fence "alloc_space, cumulative" "$OUT/pprof-alloc_space-cum.txt" 30
		fence "alloc_space, flat" "$OUT/pprof-alloc_space-flat.txt" 25
		fence "inuse_space, flat" "$OUT/pprof-inuse_space-flat.txt" 20
	} >>"$SUMMARY"
}

nm() {
	local v
	for v in default noinline; do
		go tool nm -size "$OUT/bin/caller-$v.a" >"$OUT/nm-$v.txt"
		go run ./internal/compilebench/nmreport -top 30 -title "Instantiated generic functions, $v build" <"$OUT/nm-$v.txt" >"$OUT/nm-$v.md"
		cat "$OUT/nm-$v.md" >>"$SUMMARY"
	done
}

case "${1:-}" in
measure) measure ;;
profile) profile ;;
nm) nm ;;
*)
	echo "usage: OUT=dir $0 measure|profile|nm" >&2
	exit 2
	;;
esac
