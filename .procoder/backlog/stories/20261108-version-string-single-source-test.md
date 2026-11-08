# Version strings: one source, any other vX.Y.Z fails

Status: open
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

`TestDocsNameManifestVersion` already keeps lines marked `x-release-please-version` equal to `.release-please-manifest.json` and pins the `extra-files` list. It does not catch an unmarked version in prose, which is how `v0.1.0-dev` and a pgx `v5.11.0` against a `go.mod` of `v5.12.0` got into the docs. Extend `internal/repocheck/docversion_test.go`: scan `README.md`, `llms.txt`, `docs/*.md` and `examples/**/README.md` for `v?X.Y.Z` strings. Allowed: a line carrying the release-please marker; a line inside a marked block; a line in a section titled with 'Upgrading', 'Since', 'Changelog' or 'History' (a historical statement); a Go version equal to the `go.mod` floor; a dependency version that equals the one in `go.mod`; a third-party version listed in a small `internal/repocheck/testdata/version_allow.txt` with a reason per line (PostgreSQL, pgvector, PgBouncer image tags). Anything else fails with file, line and the allowed forms. The pgx, yaml and tools rows in `docs/supported-versions.md` are generated from `go.mod` by the table generator in story generated-capability-matrix-and-type-table, or marked until then.

## Acceptance criteria

- [ ] an unmarked `v0.9.9` added to `docs/README.md` fails `TestDocsVersionStrings` with the file and line (red evidence on CI from a throwaway branch)
- [ ] a marked line and a line under an 'Upgrading' heading with `v0.1.0-dev` pass
- [ ] a dependency row that disagrees with `go.mod` fails; the current `docs/supported-versions.md` pgx row `v5.11.0` against `go.mod` `v5.12.0` is the red case, fixed in the same commit
- [ ] `vector/doc_test.go` no longer asserts the literal `v0.1.6`; the sqlite-vec pin is read from one constant, and `docs/vectors.md` gets it from that constant through the allow file or a marker
- [ ] the release-please `extra-files` list is checked against the set of docs that carry a marker, so a new doc with a marker but no listing fails
- [ ] `TestDocsNameManifestVersion` and `TestDocsNameGoFloor` still pass unchanged

## Evidence
