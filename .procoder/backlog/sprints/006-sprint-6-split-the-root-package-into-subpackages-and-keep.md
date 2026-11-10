# Sprint 6: split the root package into subpackages and keep docs current by construction

Status: closed 2026-11-10
Created: 2026-11-08

## Goal

Deliver the 2026-11-08 decision "Root-package structure and always-current docs" (epic momo-14-structure-and-docs, plan in `.procoder/research/package-split.md`). Users import feature APIs from small subpackages (`fts`, `jsonop`, `agg`, `timeop`, `sqlfn`, `advisory`, `vecsearch`, `vecop`, `arrayop`, `obs`) instead of one root package with a hundred files. Non-API mechanics live in `internal/`. An architecture test and a file-prefix guard keep it that way. The docs can no longer go stale silently:

- version strings have one source;
- the capability matrix and type table are generated with a drift check;
- every identifier the docs name must exist;
- an apidiff gate requires a docs change for every API change;
- `docs/upgrading.md` maps every moved identifier.

This is a breaking change before v1, released later as 0.4.0 through release-please once the kuvryn-license gate allows it.

## Result

committed: 24
done: 24 (20261108-add-obs-package, 20261108-apidiff-docs-gate, 20261108-architecture-test-internal-and-subpackages, 20261108-docs-reference-exists-test, 20261108-docscheck-sees-list-indented-go-fences, 20261108-export-expression-extension-api, 20261108-extract-internal-mechanics, 20261108-generated-capability-matrix-and-type-table, 20261108-move-advisory-package, 20261108-move-agg-package, 20261108-move-arrayop-package, 20261108-move-fts-package, 20261108-move-jsonop-package, 20261108-move-sqlfn-package, 20261108-move-timeop-package, 20261108-move-vecop-package, 20261108-move-vecsearch-package, 20261108-repocheck-file-prefix-guard, 20261108-root-file-prefix-rename, 20261108-sqlite-json-and-time-operators-live, 20261108-stale-docs-sweep, 20261108-upgrade-guide-for-the-breaking-imports, 20261108-version-string-single-source-test, 20261109-guard-repo-file-names-against-windows-invalid-paths)
carried: 0

## Retro

What slowed us down:

- **The release slipped two days behind the code.** The last stories ran one at a time (file-prefix rename alone, docs stories in sequence), and each fix round waited for a fresh review and a full CI run before the next step started.
- **A GitHub Actions outage stalled the sprint-to-main PR overnight.** The retry loop treated refused re-runs as "still waiting" and reported nothing, so no one saw the stall.
- **The commit gate could not lint nested Go modules.** vecop waited on a procoder fix (3.9.5).
- **release-please dropped the sprint-5 squash commit from the changelog.** Its message listed more than 100 sub-commits; the 0.4.0 entry was written by hand.

What we change next sprint:

- **Open the sprint-to-main PR as soon as the last code story merges**, and run release work (changelog, release PR) in parallel with the last reviews instead of after them.
- **CI watchers report every state they cannot classify** (a refused re-run, a run with no jobs) instead of looping silently, and give up after a bounded number of tries.
- **Squash messages for sprint PRs are a short summary plus a `BREAKING CHANGE:` footer**, never the list of sub-commits, so release-please parses them.

One adaptation worth keeping:

- **Every guard was shown red on a throwaway branch or PR before it was trusted** (docs identifiers, undocumented exports, the apidiff PR-body gate on draft PR #51). The pre-PR review still caught one escape, bare feature-package names in `llms.txt`, now tracked as story 20261109-docscheck-unqualified-identifiers-must-resolve-in-the-root.
