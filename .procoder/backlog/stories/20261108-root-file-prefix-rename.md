# Rename root files to the prefix taxonomy

Status: open
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

After every move has landed, rename the remaining root files to the taxonomy of section 5 of `.procoder/research/package-split.md` with `git mv` only, no content change, in one commit: `errors.go` to `err.go`, `builder.go` to `build.go`, `onconn.go` to `db_onconn.go`, `txsetup.go` to `db_txsetup.go`, `relation.go` to `rel.go`, `assoc.go` to `rel_assoc.go`, `loader.go` to `rel_loader.go`, `joinload.go` to `rel_join.go`, `relpred.go` to `rel_pred.go`, `counts.go` to `rel_counts.go`, `graph.go` to `rel_graph.go`, `pipeline.go` to `rel_pipeline.go`, `scalars.go` to `raw_scalars.go`, `writeguard.go` to `write_guard.go`, `returning.go` to `write_returning.go`, and the rest of the table. `Tx` is split out of `db.go` into `tx.go` in the same commit (a pure move of declarations, still one package). Test files follow their subject. Then the ratchet list is deleted and the file-prefix guard becomes strict. Doing this last means no in-flight branch edits a file whose name just changed.

## Acceptance criteria

- [ ] the rename commit shows only renames at 100 percent similarity in a rename-aware diff stat, except the `tx.go` split which shows moved declarations and no edits (output attached)
- [ ] `internal/repocheck/testdata/prefix_exceptions.txt` is deleted and `TestRootFilePrefixes` passes with an empty list
- [ ] `CONTRIBUTING.md`, `docs/ARCHITECTURE.md` and any doc that cites a root file name by path is updated; a search for the old names in `*.md` finds none
- [ ] all unit, race, compile-fail and integration jobs are green; golden test data and `//go:generate` directives are unaffected

## Evidence
