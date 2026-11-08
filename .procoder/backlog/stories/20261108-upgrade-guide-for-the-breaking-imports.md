# Migration guide docs/upgrading.md for the breaking imports

Status: open
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

The split breaks every user who calls a moved identifier (section 3 of `.procoder/research/package-split.md`). Create `docs/upgrading.md` with a section 'Upgrading to 0.4.0' that has one table of old identifier, new import path and new identifier for every move, a short rewrite recipe, and the reason (methods live with their type; subpackages import `momo`, so no alias is possible). The table is checked, not trusted: each move story appends its pairs to `internal/repocheck/testdata/moves/<package>.txt`, and a repocheck test reads every file in that directory and fails if a pair is missing from `docs/upgrading.md`, or if `docs/upgrading.md` lists a pair no `moves` file has. The skeleton and the test land first, so each move story fails its own PR until it adds its rows. The rewrite recipe gives a `gofmt -r` rule or `sed` line per package that handles the mechanical cases, tested by running it on a small fixture in `internal/repocheck/testdata/upgrade_fixture` and compiling the result on CI. The page is linked from `docs/README.md`, `README.md` and `CHANGELOG.md` (the release notes link it), and is listed in `release-please-config.json` extra-files if it carries a version marker. Nav entry is left to momo-13 story 252.

## Acceptance criteria

- [ ] `TestUpgradingCoversMoves` fails when a `moves/<package>.txt` pair is absent from `docs/upgrading.md` and when the page lists a pair with no `moves` file (red evidence from a throwaway branch)
- [ ] every move story's pairs are present at release time: the test is green on the sprint-6 head after the last move, with one `moves` file per moved package
- [ ] the fixture program that uses every moved identifier through the old names is rewritten by the documented recipe and compiles against the new packages (CI job output)
- [ ] the page states which things did not move and why: `Func`, the vector column types, the `Hook` interfaces, the `Agg` basics, `Tx.SetLocal`
- [ ] the page links to the release's CHANGELOG entry, carries the 0.4.0 version only inside a heading that the version test treats as historical, and is indexed from `docs/README.md`
- [ ] `docs/upgrading.md` has a short note on pre-v1 policy (`docs/supported-versions.md` stability section) so a reader knows more breaks can follow

## Evidence
