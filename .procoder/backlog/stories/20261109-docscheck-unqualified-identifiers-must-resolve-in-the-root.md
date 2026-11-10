# docscheck: unqualified identifiers must resolve in the root package

Status: done
Created: 2026-11-09
Epic: momo-14-structure-and-docs
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

`TestDocsIdentifiersResolve` accepts a bare backticked name such as `Case` when any checked package exports it. After sprint 6 moved `Case`, `Cast` and `Concat` to `sqlfn`, `llms.txt` still listed them unqualified next to `momo` names, and the test passed. A reader or an LLM then writes `momo.Case`, which no longer compiles. The sprint-6 to main pre-PR review caught it (fixed in 697c48c). A bare name in user-facing docs should mean the root `momo` package. A name from a feature package must be written `pkg.Name`, unless its section names the package and the allow file lists the exception with a reason.

## Acceptance criteria

- [x] a doc with a bare `Case` (exported only by `sqlfn`) fails `TestDocsIdentifiersResolve` with file, line and the suggestion `sqlfn.Case` (red evidence on CI)
- [x] bare names that exist in root `momo` (`Select`, `Func`) and bare method references on root types (`Relation[T].Has`) still pass
- [x] every existing bare feature-package name in README.md, llms.txt and docs/*.md is qualified, or listed in the allow file with a reason
- [x] the rule is described in CONTRIBUTING.md next to the docs-reference rule

## Evidence

- Built: internal/docscheck/reference_test.go TestBareNamesAreRootNames and TestBareOwners; allow file internal/docscheck/testdata/bare_allow.txt (3 pages: migrations, testing, observability, each documents its own package); rule in CONTRIBUTING.md.
- Red (criterion 1): ci-agent run 42768493330 on throwaway branch worktree-agent-a9cf43902d9e90e16-red (deleted), headSha fbd5d54, unit failed: 'llms.txt:39: bare Case ... write sqlfn.Case'.
- Criteria 2 and 3: TestBareOwners (Select, Func pass); all other bare feature-package names qualified. Green: ci-agent run 42780640447 on 8cf94c3ee066c5a183bd60c56ec1b3afeb505e37, all 9 jobs success.
- Criterion 4: CONTRIBUTING.md paragraph next to the docs rules.
- Extended after review: the rule also checks Type.Member (migrate.Engine.Apply, not Engine.Apply) and a leading *; the allow file now holds only docs/migrations.md; obs and momotest names in observability.md and testing.md are qualified. Not implemented: rejecting every X.Y whose X is not a root type (it would hit stdlib and third-party names such as Time.Equal); only a type that a feature package exports with that member is rejected.
