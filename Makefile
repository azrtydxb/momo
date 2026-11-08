# SPDX-License-Identifier: Apache-2.0
SHELL := /bin/sh
GO ?= go
# An execution count, not a duration: a time-based -fuzztime puts a deadline on
# the fuzz coordinator, and when it fires while a worker call is in flight the
# run fails with "context deadline exceeded" and no crasher (see FuzzBuilder on
# CI run 42400921170). Counts never hit that path.
FUZZTIME ?= 20000x
# One fuzz target per line, so parallel branches add targets without conflicts.
FUZZ_TARGETS  = .:FuzzBuilder
FUZZ_TARGETS += ./schema:FuzzSchemaSort
FUZZ_TARGETS += ./schema:FuzzNormalizeType
FUZZ_TARGETS += ./types:FuzzHstore
FUZZ_TARGETS += ./migrate:FuzzSplitStatements
FUZZ_TARGETS += ./schema:FuzzNormalize
FUZZ_TARGETS += ./vector:FuzzVectorDecode
FUZZ_TARGETS += ./vector:FuzzSparseDecode
FUZZ_TARGETS += ./driver/pgxdriver:FuzzVectorBinaryDecode
FUZZ_TARGETS += ./gen/testdata/golden/descriptors/document:FuzzColumnByName
FUZZ_TARGETS += .:FuzzEscapeLike
FUZZ_TARGETS += ./types:FuzzUUIDScan
FUZZ_TARGETS += ./types:FuzzTimeOfDayScan
FUZZ_TARGETS += ./types:FuzzDateScan
FUZZ_TARGETS += ./types:FuzzJSONScan
FUZZ_TARGETS += ./gen/tags:FuzzParse
FUZZ_TARGETS += .:FuzzRawArgs
FUZZ_TARGETS += .:FuzzScanRaw
GOVULNCHECK = go run golang.org/x/vuln/cmd/govulncheck@v1.9.0
# Same gosec release as .github/workflows/gosec.yml. Locally it fails on any
# finding and on any type-check error, so a toolchain mismatch cannot pass silently.
GOSEC = go run github.com/securego/gosec/v2/cmd/gosec@v2.30.0
# Pinned like the others; go run pkg@version leaves go.mod untouched.
STATICCHECK = go run honnef.co/go/tools/cmd/staticcheck@v0.8.2
PG_URL ?= postgres://postgres:momo@127.0.0.1:5432/postgres?sslmode=disable
PG_APP_URL ?= postgres://momo_app:momo_app@127.0.0.1:5432/postgres?sslmode=disable
PGBOUNCER_URL ?= postgres://postgres:momo@127.0.0.1:6432/postgres?sslmode=disable
MYSQL84_URL ?= root:momo@tcp(127.0.0.1:3306)/
MYSQL97_URL ?= root:momo@tcp(127.0.0.1:3307)/
MARIADB_URL ?= root:momo@tcp(127.0.0.1:3308)/

.PHONY: bench-gate bench bench-baseline bench-suite-smoke bench-generate bench-hooks test-bench-module test-bench-module-integration bench-env test test-race test-compilefail test-fuzz test-integration test-integration-postgres test-integration-mysql84 test-integration-mysql97 test-integration-mariadb bench-smoke fmt-check vet tidy-check staticcheck govulncheck gosec db-up db-down

test:
	$(GO) test -count=1 ./...
	cd internal/integration && $(GO) test -count=1 ./...

test-race:
	$(GO) test -race -count=1 ./...

test-compilefail:
	$(GO) test -count=1 -run '^TestCompileFail' .
	$(GO) test -count=1 -run '^TestFieldPermissions$$' ./gen

test-fuzz:
	@for t in $(FUZZ_TARGETS); do \
		pkg=$${t%%:*}; fn=$${t##*:}; \
		$(GO) test -run='^$$' -fuzz="^$$fn$$" -fuzztime=$(FUZZTIME) $$pkg || exit 1; \
	done

test-integration: test-integration-postgres

test-integration-postgres:
	MOMO_REQUIRE_INTEGRATION=1 MOMO_INTEGRATION_DIALECTS=postgres MOMO_TEST_POSTGRES_URL='$(PG_URL)' MOMO_TEST_POSTGRES_APP_URL='$(PG_APP_URL)' MOMO_TEST_PGBOUNCER_URL='$(PGBOUNCER_URL)' $(GO) test -tags integration -race -count=1 ./...

test-integration-mysql84:
	MOMO_REQUIRE_INTEGRATION=1 MOMO_INTEGRATION_DIALECTS=mysql84 MOMO_TEST_MYSQL84_URL='$(MYSQL84_URL)' $(GO) test -tags integration -race -count=1 ./...

test-integration-mysql97:
	MOMO_REQUIRE_INTEGRATION=1 MOMO_INTEGRATION_DIALECTS=mysql97 MOMO_TEST_MYSQL97_URL='$(MYSQL97_URL)' $(GO) test -tags integration -race -count=1 ./...

test-integration-mariadb:
	MOMO_REQUIRE_INTEGRATION=1 MOMO_INTEGRATION_DIALECTS=mariadb MOMO_TEST_MARIADB_URL='$(MARIADB_URL)' $(GO) test -tags integration -race -count=1 ./...

# The bench module is separate (GORM, Ent and Bun stay out of the library's
# import graph), so ./... above never reaches it.
test-bench-module:
	cd bench && $(GO) vet ./... && $(GO) vet -tags integration ./... && $(GO) test -count=1 ./... && $(GO) mod tidy -diff

test-bench-module-integration:
	cd bench && BENCH_PG_URL='$(PG_URL)' $(GO) test -p 1 -count=1 -tags integration ./...

# The hook-overhead benchmark needs a database, so it only compiles with the
# integration tag and bench-smoke skips it. The figure is the budget recorded
# in docs/PERFORMANCE.md.
bench-hooks:
	MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL='$(PG_URL)' $(GO) test -run='^$$' -tags integration -bench='^BenchmarkSelectNoHooks$$' -benchmem -benchtime=200x .

# Regenerates the Momo metadata and the Ent client for the bench module.
bench-generate:
	cd bench && $(GO) run github.com/azrtydxb/momo/cmd/momo generate && $(GO) run entgo.io/ent/cmd/ent generate --target ./adapters/ent/entgen ./adapters/ent/schema

# Full run on the dedicated machine: BENCH_COUNT repetitions (default 10), results
# under bench/results. Needs BENCH_PG_URL.
bench:
	./bench/scripts/run.sh full

# One iteration of every benchmark, for CI.
bench-suite-smoke:
	BENCH_PG_URL='$(PG_URL)' ./bench/scripts/run.sh smoke

# Rewrites bench/baseline/allocs.txt from the allocation-gate benchmarks.
bench-baseline:
	./bench/scripts/run.sh baseline

# The allocation gate CI runs on pull requests (.github/workflows/bench-gate.yml),
# runnable against any PostgreSQL: allocs/op versus bench/baseline/allocs.txt.
BENCHSTAT = go run golang.org/x/perf/cmd/benchstat@v0.0.0-20261009192801-be2c69fb417e
bench-gate:
	@set -e; out=$$(mktemp -d); \
	(cd bench && BENCH_PG_URL='$(PG_URL)' $(GO) test -tags integration -run='^$$' -bench='BenchmarkAllocs_' -benchmem -count=6 ./... | tee $$out/new.txt); \
	$(BENCHSTAT) -filter '.unit:allocs/op' bench/baseline/allocs.txt $$out/new.txt | tee $$out/delta.txt; \
	(cd bench && $(GO) run ./cmd/benchgate -max-percent 5 $$out/delta.txt)

bench-env:
	cd bench && $(GO) run ./cmd/envcapture

bench-smoke:
	$(GO) test -run='^$$' -bench=. -benchtime=1x ./...

fmt-check:
	@out=$$(git ls-files -z '*.go' | xargs -0 gofmt -l); if [ -n "$$out" ]; then echo "$$out"; exit 1; fi

vet:
	$(GO) vet ./...
	$(GO) vet -tags integration ./...
	cd internal/integration && $(GO) vet ./...

tidy-check:
	$(GO) mod tidy -diff
	cd internal/integration && $(GO) mod tidy -diff

staticcheck:
	$(STATICCHECK) ./...

govulncheck:
	$(GOVULNCHECK) ./...

gosec:
	$(GOSEC) -quiet -exclude-dir=testdata -exclude-dir=.claude ./...

db-up:
	docker compose -f internal/testdb/compose.yaml up -d --wait

db-down:
	docker compose -f internal/testdb/compose.yaml down -v
