# Rename root files to the prefix taxonomy

Status: done
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

After every move has landed, rename the remaining root files to the taxonomy of section 5 of `.procoder/research/package-split.md` with `git mv` only, no content change, in one commit: `errors.go` to `err.go`, `builder.go` to `build.go`, `onconn.go` to `db_onconn.go`, `txsetup.go` to `db_txsetup.go`, `relation.go` to `rel.go`, `assoc.go` to `rel_assoc.go`, `loader.go` to `rel_loader.go`, `joinload.go` to `rel_join.go`, `relpred.go` to `rel_pred.go`, `counts.go` to `rel_counts.go`, `graph.go` to `rel_graph.go`, `pipeline.go` to `rel_pipeline.go`, `scalars.go` to `raw_scalars.go`, `writeguard.go` to `write_guard.go`, `returning.go` to `write_returning.go`, and the rest of the table. `Tx` is split out of `db.go` into `tx.go` in the same commit (a pure move of declarations, still one package). Test files follow their subject. Then the ratchet list is deleted and the file-prefix guard becomes strict. Doing this last means no in-flight branch edits a file whose name just changed.

## Acceptance criteria

- [x] the rename commit shows only renames at 100 percent similarity in a rename-aware diff stat, except the `tx.go` split which shows moved declarations and no edits (output attached)
- [x] `internal/repocheck/testdata/prefix_exceptions.txt` is deleted and `TestRootFilePrefixes` passes with an empty list
- [x] `CONTRIBUTING.md`, `docs/ARCHITECTURE.md` and any doc that cites a root file name by path is updated; a search for the old names in `*.md` finds none
- [x] all unit, race, compile-fail and integration jobs are green; golden test data and `//go:generate` directives are unaffected

## Evidence

- Criterion 1: rename commit ca99878: `git show --stat -M --format= ca99878` reports `214 files changed, 0 insertions(+), 0 deletions(-)`, and `git diff -M100% --name-status 79a15ed ca99878` lists 214 entries, all `R`. The first pass missed one file, renamed in the follow-up commit e6a47ed (one more R100: `vector_parity_integration_test.go => col_vector_parity_integration_test.go`). The `Tx` split is the separate commit 56a2bdc right after the renames (`db.go | 133 ---`, `tx.go | 145 +++`); the only added lines in `tx.go` that are not removed from `db.go` are the licence header, `package`, and the `import` block. `strconv` left the `db.go` imports because only `Tx.Tx` used it. `DB.Begin`, `DB.AdoptTx` and `DB.Tx` stay in `db.go` (they are `DB` methods); the `Tx` type, its methods, `inTx` and `spExec` moved.
- Criterion 2: commit 8bc6f08 deletes `prefix_exceptions.txt` and `prefix_frozen.txt`; `TestRootFilePrefixes` now takes only the file list (strict). Deviation: the old rule "a test file needs an existing subject file" cannot hold for many test files that have no single subject (a subject only has four suffix forms), so the strict rule is: a test file's name without `_test` starts with an allowed prefix. `TestRootFilePrefixRules` covers foreign prefixes for sources and tests and the old names.
- Criterion 3: a search of every tracked file outside `.procoder/` and `CHANGELOG.md` for the old file name of each of the 214 renamed files finds none left. Updated: `CONTRIBUTING.md` (section "Root package file names"), `docs/vectors.md`, `docs/supported-versions.md`, and the comments in `rel_loader.go`, `col_type_mapping_test.go` and `expr_comparison_shared_test.go` (that last file also had a runtime `f == "errors.go"` exclusion, now `err.go`). `docs/ARCHITECTURE.md` cited no root file. No `//go:generate`, golden path, Makefile or workflow named a renamed file.
- Criterion 4: ci-agent run 42663209856 on headSha e6a47edabd81e58549b4e16e56bae314598d8c4f, all 8 jobs success (unit, lint, apidiff-docs, fuzz-smoke, bench-module, integration postgres, integration-mysql mysql97, mysql84, mariadb). The first run 42660591997 on 8bc6f08 failed only on `TestRootFilePrefixes` because one rename was missed. Compile-fail runs inside unit. Race runs only in ci.yml on sprint-6 and is confirmed there after the merge.
