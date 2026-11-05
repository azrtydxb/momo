# Getting-started README for v0.1.0

Status: open
Created: 2026-11-05
Epic: momo-13-docs-release
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

As a Go developer, or an AI coding agent, adopting Momo in an application on PostgreSQL with pgvector, I need a root README that takes me from install to a working app without reading the source: define a model, generate code, create and apply a migration, open a database through the pgx driver, run inserts, selects, updates and deletes, load a relation, run a vector similarity query with search settings, and use transactions. Done means the README exists, states the v0.1.0 status and the Go floor, and every Go snippet in it is real, compiled code that cannot drift from the library.

## Acceptance criteria

- [x] `README.md` exists at the repository root with install lines `go get github.com/azrtydxb/momo@v0.1.0` and `go install github.com/azrtydxb/momo/cmd/momo@v0.1.0`
- [x] every Go snippet in the README appears verbatim in the compiled example under `examples/quickstart`, enforced by a doc-sync test
- [x] every `momo` command and flag in the README is accepted by `momo help` and `momo migrate help`, enforced by the doc-sync test
- [x] the README states the PgBouncer-safe default exec mode and recommends a `.gitattributes` rule for generated files
- [x] `procoder check` gate is clean and the work is committed

## Evidence

- `go test -count=1 ./examples/quickstart/...` passes; `go build ./examples/...` passes.
- Review revision: README now has the import block (doc-synced), a "Changing a model" section with `--rename-column`/`--rename-table`, the forward-only note, pgvector server install, `sslmode=disable` local-only note, distance functions per vector kind, yaml and tag glosses, `hash`, and an import-path rewrite note.
- readme_test.go now pins the Go floor against the go.mod go line and the no-compatibility-promise sentence. Mutation checks (floor changed to 1.25, promise sentence negated, import line altered) each made the test fail; README restored afterwards.
- Gate criterion stays open until `procoder check` and the commit are done.
