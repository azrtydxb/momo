# docscheck: unqualified identifiers must resolve in the root package

Status: open
Created: 2026-11-09
Epic: momo-14-structure-and-docs
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

`TestDocsIdentifiersResolve` accepts a bare backticked name such as `Case` when any checked package exports it. After sprint 6 moved `Case`, `Cast` and `Concat` to `sqlfn`, `llms.txt` still listed them unqualified next to `momo` names, and the test passed. A reader or an LLM then writes `momo.Case`, which no longer compiles. The sprint-6 to main pre-PR review caught it (fixed in 697c48c). A bare name in user-facing docs should mean the root `momo` package. A name from a feature package must be written `pkg.Name`, unless its section names the package and the allow file lists the exception with a reason.

## Acceptance criteria

- [ ] a doc with a bare `Case` (exported only by `sqlfn`) fails `TestDocsIdentifiersResolve` with file, line and the suggestion `sqlfn.Case` (red evidence on CI)
- [ ] bare names that exist in root `momo` (`Select`, `Func`) and bare method references on root types (`Relation[T].Has`) still pass
- [ ] every existing bare feature-package name in README.md, llms.txt and docs/*.md is qualified, or listed in the allow file with a reason
- [ ] the rule is described in CONTRIBUTING.md next to the docs-reference rule

## Evidence
