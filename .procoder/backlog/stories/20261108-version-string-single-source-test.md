# Version strings: one source, any other vX.Y.Z fails

Status: done
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

`TestDocsNameManifestVersion` already keeps lines marked `x-release-please-version` equal to `.release-please-manifest.json` and pins the `extra-files` list. It does not catch an unmarked version in prose, which is how `v0.1.0-dev` and a pgx `v5.11.0` against a `go.mod` of `v5.12.0` got into the docs. Extend `internal/repocheck/docversion_test.go`: scan `README.md`, `llms.txt`, `docs/*.md` and `examples/**/README.md` for `v?X.Y.Z` strings. Allowed: a line carrying the release-please marker; a line inside a marked block; a line in a section titled with 'Upgrading', 'Since', 'Changelog' or 'History' (a historical statement); a Go version equal to the `go.mod` floor; a dependency version that equals the one in `go.mod`; a third-party version listed in a small `internal/repocheck/testdata/version_allow.txt` with a reason per line (PostgreSQL, pgvector, PgBouncer image tags). Anything else fails with file, line and the allowed forms. The pgx, yaml and tools rows in `docs/supported-versions.md` are generated from `go.mod` by the table generator in story generated-capability-matrix-and-type-table, or marked until then.

## Acceptance criteria

- [x] an unmarked `v0.9.9` added to `docs/README.md` fails `TestDocsVersionStrings` with the file and line (red evidence on CI from a throwaway branch)
- [x] a marked line and a line under an 'Upgrading' heading with `v0.1.0-dev` pass
- [x] a dependency row that disagrees with `go.mod` fails; the current `docs/supported-versions.md` pgx row `v5.11.0` against `go.mod` `v5.12.0` is the red case, fixed in the same commit
- [x] `vector/doc_test.go` no longer asserts the literal `v0.1.6`; the sqlite-vec pin is read from one constant, and `docs/vectors.md` gets it from that constant through the allow file or a marker
- [x] the release-please `extra-files` list is checked against the set of docs that carry a marker, so a new doc with a marker but no listing fails
- [x] `TestDocsNameManifestVersion` and `TestDocsNameGoFloor` still pass unchanged

## Evidence

- Red, ci-agent run 42494461721 on throwaway branch `worktree-agent-a6d787996597eb319-red` (headSha 71078e3, deleted afterwards): the test and allow file plus an unmarked `v0.9.9` appended to `docs/README.md`, the pgx row of `docs/supported-versions.md` set to `v5.11.0`, and the untouched `v0.1.7` in `docs/vectors.md`. `TestDocsVersionStrings` failed with `docs/README.md:53 names 0.9.9`, `docs/supported-versions.md:31 names 5.11.0` and `docs/vectors.md:395 names 0.1.7`, each with the allowed forms, in the integration jobs.
- Criterion 1: the red run above. Criterion 3: the same run, pgx row v5.11.0 against go.mod v5.12.0; on this base the row already read v5.12.0, so no docs fix was needed, and `TestDocsVersionRules` pins both the agreeing and the disagreeing row.
- Criterion 2: `TestDocsVersionRules` cases "marked line", "upgrading heading" and "subheading stays historical"; on the real docs, `docs/migrations.md` `v0.1.0-dev` under "Upgrading: generated file header" passes. Green run 42498772264 (headSha 3163f19f16555d3c73c302b1fe1039f55ee33f09), all jobs success.
- Criterion 4: `vector/doc_test.go` asserts `sqlite-vec` instead of `v0.1.6`; the pin is read by `TestDocsVersionStrings` from `SQLiteVecVersion` in `driver/sqlitedriver/vec.go` (parsed as text, so no cgo import). `docs/vectors.md` v0.1.6 passes through that constant; the `v0.1.7` example was reworded to "a later patch release of the same minor". Green run 42498772264.
- Criterion 5: `TestDocsVersionStrings` fails any scanned doc that carries a marker but is missing from `extra-files`; it passes on the current 11 listed files in run 42498772264. The negative case is the `TestDocsVersionRules` subtest "marker file missing from extra-files fails" (via `unlistedCarriers`).
- Criterion 6: `docversion_test.go` is unchanged; both tests pass in run 42498772264.
