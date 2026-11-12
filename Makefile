# SPDX-License-Identifier: Apache-2.0
# bash with pipefail: the bench recipes pipe go test through tee, and without
# pipefail a failed or empty benchmark run would pass the gate.
SHELL := bash
.SHELLFLAGS := -eo pipefail -c
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
FUZZ_TARGETS += ./migrate/sqlite:FuzzParseDDL
FUZZ_TARGETS += ./schema:FuzzNormalize
FUZZ_TARGETS += ./vector:FuzzVectorDecode
FUZZ_TARGETS += ./vector:FuzzSparseDecode
FUZZ_TARGETS += ./driver/pgxdriver:FuzzVectorBinaryDecode
FUZZ_TARGETS += ./internal/sqlshared:FuzzParseTime
FUZZ_TARGETS += ./internal/sqlshared:FuzzScanErr
FUZZ_TARGETS += ./driver/sqlitedriver:FuzzConstraintTarget
FUZZ_TARGETS += ./driver/sqlitedriver:FuzzCheckVersion
FUZZ_TARGETS += ./driver/sqlitedriver:FuzzBuildDSN
FUZZ_TARGETS += ./driver/sqlitepure:FuzzPureConstraintTarget
FUZZ_TARGETS += ./driver/sqlitepure:FuzzPureBuildDSN
FUZZ_TARGETS += ./gen/testdata/golden/descriptors/document:FuzzColumnByName
FUZZ_TARGETS += .:FuzzEscapeLike
FUZZ_TARGETS += ./types:FuzzUUIDScan
FUZZ_TARGETS += ./types:FuzzTimeOfDayScan
FUZZ_TARGETS += ./types:FuzzDateScan
FUZZ_TARGETS += ./types:FuzzDateInfinityScan
FUZZ_TARGETS += ./types:FuzzNumericScan
FUZZ_TARGETS += ./types:FuzzJSONScan
FUZZ_TARGETS += ./gen/tags:FuzzParse
FUZZ_TARGETS += .:FuzzRawArgs
FUZZ_TARGETS += ./internal/sqlscan:FuzzScanRaw
FUZZ_TARGETS += .:FuzzAppendRaw
GOVULNCHECK = $(GO) run golang.org/x/vuln/cmd/govulncheck@v1.9.0
# Same gosec release as .github/workflows/gosec.yml. Locally it fails on any
# finding and on any type-check error, so a toolchain mismatch cannot pass silently.
GOSEC = $(GO) run github.com/securego/gosec/v2/cmd/gosec@v2.30.0
# Pinned like the others; go run pkg@version leaves go.mod untouched.
STATICCHECK = $(GO) run honnef.co/go/tools/cmd/staticcheck@v0.8.2
# x/exp is untagged, so this is a pseudo-version (commit 47ce03d25ef0, 2026-11-06).
APIDIFF = $(GO) run golang.org/x/exp/cmd/apidiff@v0.0.0-20261106192419-47ce03d25ef0
PG_URL ?= postgres://postgres:momo@127.0.0.1:5432/postgres?sslmode=disable
PG_APP_URL ?= postgres://momo_app:momo_app@127.0.0.1:5432/postgres?sslmode=disable
PGBOUNCER_URL ?= postgres://postgres:momo@127.0.0.1:6432/postgres?sslmode=disable
MYSQL84_URL ?= root:momo@tcp(127.0.0.1:3306)/
MYSQL97_URL ?= root:momo@tcp(127.0.0.1:3307)/
MARIADB_URL ?= root:momo@tcp(127.0.0.1:3308)/

.PHONY: bench-gate bench bench-baseline bench-suite-smoke bench-generate bench-hooks test-bench-module test-bench-module-integration bench-env test test-race test-compilefail test-fuzz test-integration test-integration-postgres test-integration-sqlite test-sqlitepure test-integration-mysql84 test-integration-mysql97 test-integration-mariadb bench-smoke fmt-check vet tidy-check staticcheck govulncheck gosec apidiff-gate db-up db-down protoc gen-integration

test:
	$(GO) test -count=1 ./...
	cd internal/integration && $(GO) test -count=1 ./...
	cd otelmomo && $(GO) test -count=1 ./...

test-race:
	$(GO) test -race -count=1 ./...

test-compilefail:
	$(GO) test -count=1 -run '^TestCompileFail' ./...
	$(GO) test -count=1 -run '^TestFieldPermissions$$' ./gen

test-fuzz:
	@for t in $(FUZZ_TARGETS); do \
		pkg=$${t%%:*}; fn=$${t##*:}; \
		$(GO) test -run='^$$' -fuzz="^$$fn$$" -fuzztime=$(FUZZTIME) $$pkg || exit 1; \
	done

test-integration: test-integration-postgres test-integration-sqlite

test-integration-postgres:
	MOMO_REQUIRE_INTEGRATION=1 MOMO_INTEGRATION_DIALECTS=postgres MOMO_TEST_POSTGRES_URL='$(PG_URL)' MOMO_TEST_POSTGRES_APP_URL='$(PG_APP_URL)' MOMO_TEST_PGBOUNCER_URL='$(PGBOUNCER_URL)' $(GO) test -tags integration -race -count=1 -timeout 25m ./...
	cd otelmomo && MOMO_REQUIRE_INTEGRATION=1 MOMO_INTEGRATION_DIALECTS=postgres MOMO_TEST_POSTGRES_URL='$(PG_URL)' MOMO_TEST_POSTGRES_APP_URL='$(PG_APP_URL)' $(GO) test -count=1 ./...
	cd internal/compat/gormfixture && MOMO_REQUIRE_INTEGRATION=1 MOMO_INTEGRATION_DIALECTS=postgres MOMO_TEST_POSTGRES_URL='$(PG_URL)' MOMO_TEST_POSTGRES_APP_URL='$(PG_APP_URL)' $(GO) test -tags integration -count=1 ./...
	cd internal/compat/bunfixture && MOMO_REQUIRE_INTEGRATION=1 MOMO_INTEGRATION_DIALECTS=postgres MOMO_TEST_POSTGRES_URL='$(PG_URL)' MOMO_TEST_POSTGRES_APP_URL='$(PG_APP_URL)' $(GO) test -tags integration -count=1 ./...
	cd internal/compat/entfixture && MOMO_REQUIRE_INTEGRATION=1 MOMO_INTEGRATION_DIALECTS=postgres MOMO_TEST_POSTGRES_URL='$(PG_URL)' MOMO_TEST_POSTGRES_APP_URL='$(PG_APP_URL)' $(GO) test -tags integration -count=1 ./...
	$(MAKE) protoc
	cd internal/integration && MOMO_REQUIRE_INTEGRATION=1 MOMO_INTEGRATION_DIALECTS=postgres MOMO_TEST_POSTGRES_URL='$(PG_URL)' PROTOC=$(CURDIR)/internal/integration/protoc-dist/bin/protoc $(GO) test -race -count=1 -timeout 25m ./...

test-integration-mysql84:
	MOMO_REQUIRE_INTEGRATION=1 MOMO_INTEGRATION_DIALECTS=mysql84 MOMO_TEST_MYSQL84_URL='$(MYSQL84_URL)' $(GO) test -tags integration -race -count=1 -timeout 25m ./...

test-integration-mysql97:
	MOMO_REQUIRE_INTEGRATION=1 MOMO_INTEGRATION_DIALECTS=mysql97 MOMO_TEST_MYSQL97_URL='$(MYSQL97_URL)' $(GO) test -tags integration -race -count=1 -timeout 25m ./...

test-integration-mariadb:
	MOMO_REQUIRE_INTEGRATION=1 MOMO_INTEGRATION_DIALECTS=mariadb MOMO_TEST_MARIADB_URL='$(MARIADB_URL)' $(GO) test -tags integration -race -count=1 -timeout 25m ./...

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

# Regenerates the Momo metadata and the Ent client for the bench module. The Ent
# header carries the SPDX line the repository check requires on every Go file.
bench-generate:
	cd bench && $(GO) run github.com/azrtydxb/momo/cmd/momo generate && $(GO) run entgo.io/ent/cmd/ent generate --header "$$(printf '// SPDX-License-Identifier: Apache-2.0\n\n// Code generated by ent, DO NOT EDIT.')" --target ./adapters/ent/entgen ./adapters/ent/schema

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

# The allocation gate CI runs in the bench-module job of ci.yml and ci-agent.yml,
# runnable against any PostgreSQL: allocs/op versus bench/baseline/allocs.txt.
BENCHSTAT = $(GO) run golang.org/x/perf/cmd/benchstat@v0.0.0-20261009192801-be2c69fb417e
bench-gate:
	out=$$(mktemp -d); \
	(cd bench && BENCH_PG_URL='$(PG_URL)' $(GO) test -run='^$$' -tags integration -bench='BenchmarkAllocs_' -benchmem -count=6 ./... | tee $$out/new.txt); \
	$(BENCHSTAT) -filter '.unit:allocs/op' bench/baseline/allocs.txt $$out/new.txt | tee $$out/delta.txt; \
	(cd bench && $(GO) run ./cmd/benchgate -max-percent 5 -baseline baseline/allocs.txt -new $$out/new.txt $$out/delta.txt)

bench-env:
	cd bench && $(GO) run ./cmd/envcapture

# SQLite needs no server: no URL is set, so testopen.Each skips PostgreSQL and
# runs every dialect-neutral test on SQLite. mattn/go-sqlite3 needs cgo.
test-integration-sqlite:
	CGO_ENABLED=1 $(GO) test -tags integration -race -count=1 -timeout 25m ./...

# S-89: the pure-Go SQLite adapter builds and passes its suite with cgo off.
# -race needs cgo, so the race run of this package is in test-integration.
test-sqlitepure:
	CGO_ENABLED=0 $(GO) test -count=1 -tags=integration ./driver/sqlitepure/... ./internal/sqlshared/...

bench-smoke:
	$(GO) test -run='^$$' -bench=. -benchtime=1x ./...

fmt-check:
	@out=$$(git ls-files -z '*.go' | xargs -0 gofmt -l); if [ -n "$$out" ]; then echo "$$out"; exit 1; fi

vet:
	$(GO) vet ./...
	$(GO) vet -tags integration ./...
	cd bench && $(GO) vet ./... && $(GO) vet -tags integration ./...
	cd internal/integration && $(GO) vet ./...
	cd otelmomo && $(GO) vet ./...
	cd internal/compat/gormfixture && $(GO) vet -tags integration ./...
	cd internal/compat/bunfixture && $(GO) vet -tags integration ./...
	cd internal/compat/entfixture && $(GO) vet -tags integration ./...

tidy-check:
	$(GO) mod tidy -diff
	cd bench && $(GO) mod tidy -diff
	cd internal/integration && $(GO) mod tidy -diff
	cd otelmomo && $(GO) mod tidy -diff
	cd internal/compat/gormfixture && $(GO) mod tidy -diff
	cd internal/compat/bunfixture && $(GO) mod tidy -diff
	cd internal/compat/entfixture && $(GO) mod tidy -diff

# gen-check regenerates everything `go generate` writes in the root module (the
# generated blocks of the docs from internal/gendocs, and any later generator)
# and fails when that changes or adds a file, so a code change without its
# regenerated docs cannot merge. It covers every //go:generate directive, not
# a fixed list of files. The diff is printed for the log.
.PHONY: gen-check
gen-check:
	$(GO) generate ./...
	@out=$$(git status --porcelain); if [ -n "$$out" ]; then echo "go generate changed these files; run it and commit the result:"; echo "$$out"; git --no-pager diff; exit 1; fi

# The nested modules are separate go.mod trees, so ./... above never reaches
# them. staticcheck skips files marked "Code generated ... DO NOT EDIT", so the
# Ent client in entfixture needs no exclusion there.
staticcheck:
	$(STATICCHECK) ./...
	cd bench && $(STATICCHECK) ./...
	cd internal/integration && $(STATICCHECK) ./...
	cd otelmomo && $(STATICCHECK) ./...
	cd internal/compat/gormfixture && $(STATICCHECK) -tags integration ./...
	cd internal/compat/bunfixture && $(STATICCHECK) -tags integration ./...
	cd internal/compat/entfixture && $(STATICCHECK) -tags integration ./...

# govulncheck reports only vulnerable symbols the module can reach, so the
# fixtures (which import GORM, Bun and Ent) are scanned like any other module.
govulncheck:
	$(GOVULNCHECK) ./...
	cd bench && $(GOVULNCHECK) ./...
	cd internal/integration && $(GOVULNCHECK) ./...
	cd otelmomo && $(GOVULNCHECK) ./...
	cd internal/compat/gormfixture && $(GOVULNCHECK) -tags integration ./...
	cd internal/compat/bunfixture && $(GOVULNCHECK) -tags integration ./...
	cd internal/compat/entfixture && $(GOVULNCHECK) -tags integration ./...

# Same exclusions as .github/workflows/gosec.yml. Inside entfixture the
# generated Ent client is the directory "ent".
gosec:
	$(GOSEC) -quiet -exclude-dir=testdata -exclude-dir=.claude -exclude-dir=internal/compat/entfixture/ent -exclude-dir=internal/integration/grpc/example/pb ./...
	cd bench && $(GOSEC) -quiet -exclude-dir=testdata ./...
	cd internal/integration && $(GOSEC) -quiet -exclude-dir=testdata -exclude-dir=grpc/example/pb ./...
	cd internal/compat/gormfixture && $(GOSEC) -quiet -exclude-dir=testdata -tags=integration ./...
	cd internal/compat/bunfixture && $(GOSEC) -quiet -exclude-dir=testdata -tags=integration ./...
	cd internal/compat/entfixture && $(GOSEC) -quiet -exclude-dir=testdata -exclude-dir=ent -tags=integration ./...

db-up:
	docker compose -f internal/testdb/compose.yaml up -d --wait

db-down:
	docker compose -f internal/testdb/compose.yaml down -v

# Fails when the exported API changed and no docs changed; see CONTRIBUTING.md.
apidiff-gate:
	APIDIFF='$(APIDIFF)' bash scripts/apidiff-docs-gate.sh

# protoc installs the protoc release pinned in internal/integration/tools.lock,
# after checking its SHA-256, into internal/integration/protoc-dist.
protoc:
	bash scripts/fetch-protoc.sh

# gen-integration regenerates the committed example output of the ecosystem
# generators (GraphQL, gRPC, OpenAPI) under internal/integration.
gen-integration: protoc
	bash scripts/gen-integration.sh
