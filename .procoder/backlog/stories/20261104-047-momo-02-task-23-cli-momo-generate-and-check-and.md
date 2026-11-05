# momo-02 Task 23: CLI `momo generate` and `--check`, and go generate integration

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 23: CLI `momo generate` and `--check`, and go generate integration (`.procoder/plans/momo-02-generator.md:2553`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-3. Files: `cmd/momo/main.go`, `cmd/momo/generate.go`, `cmd/momo/generate_test.go`, `cmd/momo/cmd.go`, `cmd/momo/helpers_test.go`. Contributes to S-3; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestGenerateCheck` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestGoGenerateIntegration` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- cmd/momo/{main,cmd,generate}.go: Run dispatcher (subcommand table in commands(), so migrate can be added), `momo generate [-c] [--check] [patterns]`, exit codes 0/1/2, diagnostics as file:line:col on stderr, `stale: <path>` on stdout in check mode (drift and orphans, deduplicated). Emit errors never reach Commit.
- Before implementation: `go test ./cmd/momo` FAIL (`undefined: Run`). After: ok, also under -race. Extra tests: TestGenerateCheckOrphan, TestGenerateDiagnostics (no output written), TestGenerateConfigErrors.
- Mutation: `Check: false` in the Writer options made TestGenerateCheck fail ("--check wrote to disk").
- Deviation: internal/testmodels/example (plan Task 22) does not exist yet, so the tests copy cmd/momo/testdata/example (Tenant, Document with companion, Comment) and generate first, instead of checking committed goldens. Swap the source once Task 22 lands.
- go build, go vet, staticcheck, procoder check clean.
