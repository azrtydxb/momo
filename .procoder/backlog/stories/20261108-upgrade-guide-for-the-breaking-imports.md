# Migration guide docs/upgrading.md for the breaking imports

Status: open
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

The split breaks every user who calls a moved identifier (section 3 of `.procoder/research/package-split.md`). Create `docs/upgrading.md` with a section 'Upgrading to 0.4.0' that has one table of old identifier, new import path and new identifier for every move, a short rewrite recipe, and the reason (methods live with their type; subpackages import `momo`, so no alias is possible). The table is checked, not trusted: each move story appends its pairs to `internal/repocheck/testdata/moves/<package>.txt`, and a repocheck test reads every file in that directory and fails if a pair is missing from `docs/upgrading.md`, or if `docs/upgrading.md` lists a pair no `moves` file has. The skeleton and the test land first, so each move story fails its own PR until it adds its rows. The rewrite recipe gives a `gofmt -r` rule or `sed` line per package that handles the mechanical cases, tested by running it on a small fixture in `internal/repocheck/testdata/upgrade_fixture` and compiling the result on CI. The page is linked from `docs/README.md`, `README.md` and `CHANGELOG.md` (the release notes link it), and is listed in `release-please-config.json` extra-files if it carries a version marker. Nav entry is left to momo-13 story 252.

## Acceptance criteria

- [x] `TestUpgradingCoversMoves` fails when a `moves/<package>.txt` pair is absent from `docs/upgrading.md` and when the page lists a pair with no `moves` file (red evidence from a throwaway branch)
- [ ] every move story's pairs are present at release time: the test is green on the sprint-6 head after the last move, with one `moves` file per moved package
- [ ] the fixture program that uses every moved identifier through the old names is rewritten by the documented recipe and compiles against the new packages (CI job output)
- [x] the page states which things did not move and why: `Func`, the vector column types, the `Hook` interfaces, the `Agg` basics, `Tx.SetLocal`
- [ ] the page links to the release's CHANGELOG entry, carries the 0.4.0 version only inside a heading that the version test treats as historical, and is indexed from `docs/README.md`
- [x] `docs/upgrading.md` has a short note on pre-v1 policy (`docs/supported-versions.md` stability section) so a reader knows more breaks can follow

## Evidence

Step 1 only (skeleton and moves check); the story stays open until step 6.

- Criterion 1: red run 42494821840 on probe commit b9430270a639a1e215b9f5dd5f6b33f2ada5bbd4 (throwaway branch, deleted): TestUpgradingCoversMoves failed with `docs/upgrading.md lacks the row | momo.ArrayContains | .../arrayop | arrayop.Contains |` (moves file with no row) and `lists a move that no testdata/moves file records: momo.Stale` (row with no moves file). TestUpgradingDiffDetectsDrift pins the comparison in memory. Green: run 42498774870 on 7e303a02c92551bc758aa300a84b4d3aa373c198.
- Criterion 4: docs/upgrading.md "What did not move" lists Func/FuncExpr, the vector column types, the hook interfaces, the basic aggregates and Tx.SetLocal with reasons; each identifier exists in the root package (expr_funcs.go, vector_column.go, db.go, relhook.go, expr.go, search.go).
- Criterion 6: docs/upgrading.md opens with the pre-v1 note linking supported-versions.md#stability-policy.
- Not ticked: 2 (needs the move stories), 3 (fixture and recipe come later), 5 (CHANGELOG link at release). The page is already indexed from docs/README.md and README.md, and is in the mkdocs nav so TestEveryDocPageInNav passes. Format of testdata/moves/<package>.txt: one `<old> <new>` pair per line.

