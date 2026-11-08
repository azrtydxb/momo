# Generate the capability matrix and the type-mapping table with go generate

Status: open
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

`docs/capabilities.md` (326 lines) and `docs/TYPE_MAPPING.md` (169 lines) are hand-edited tables that restate facts already in code: the `dialect.Dialect` capability flags and `suite.CapabilitiesOf`, and the table in `gen/typemap`. They are where 'SQLite and MySQL are not usable' is copied by hand eight times. Add `internal/gendocs` with a `//go:generate` directive that writes the dialect status table (usable or not, driver package, `MaxParams`, each boolean flag of `dialect.Dialect`) into `docs/capabilities.md` between `<!-- generated:capabilities -->` markers, and the Go-to-column-type table from `gen/typemap` into `docs/TYPE_MAPPING.md` between `<!-- generated:typemap -->` markers, one column per dialect. The status line in README, llms.txt, ai-guide, supported-versions and migrations that says which databases are usable is replaced by a one-line include of the same generated text (marker blocks, same generator). A CI step runs `go generate ./internal/gendocs/...` and a clean-tree check, so a code change without regenerated docs fails. The 'usable' flag is derived from whether a driver package exists under `driver/` for the dialect, so adding a SQLite driver flips every page without a doc edit. This story reuses the generated-section pattern and does not touch the llms-full generation of momo-13 story 256.

## Acceptance criteria

- [ ] `go generate ./internal/gendocs/...` is deterministic: running it twice yields no diff (CI step output)
- [ ] the CI drift check fails on a throwaway branch where a `dialect.Dialect` flag is flipped without regenerating (red evidence run id)
- [ ] the hand-written dialect-status prose is removed from `README.md`, `llms.txt`, `docs/ai-guide.md`, `docs/supported-versions.md`, `docs/migrations.md` and `docs/capabilities.md`, replaced by generated blocks; a search for 'not usable' outside generated blocks finds nothing
- [ ] the type table in `docs/TYPE_MAPPING.md` matches `gen/typemap` for every Go type listed, with `unsupported` where a dialect has no mapping (a test compares the doc block to the generator output)
- [ ] dependency version rows in `docs/supported-versions.md` come from `go.mod` (a test fails when the row and `go.mod` differ)
- [ ] the generator imports no package that needs cgo or a database; it runs on the lint job's Go setup

## Evidence
