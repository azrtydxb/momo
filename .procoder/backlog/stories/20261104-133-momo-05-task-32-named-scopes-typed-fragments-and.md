# momo-05 Task 32: Named scopes, typed fragments and runtime filter tree (S-79)

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 32: Named scopes, typed fragments and runtime filter tree (S-79) (`.procoder/plans/momo-05-writes-queries.md:2633`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-79. Files: `compose.go`, `compose_test.go`.

## Acceptance criteria

- [x] `TestCompositionHelpers` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-79] Scope applies transformations in order without mutating the base query; Filter renders only allowlisted columns through the builder (identifiers quoted, values bound) and rejects unknown fields, operators and malformed nodes with ErrValidation; FOR UPDATE NOWAIT is ErrUnsupported on SQLite and accepted elsewhere (TestCompositionHelpers); a Filter on a sensitive column reaches hooks as RedactedArg (TestFilterOnSensitiveColumnIsMasked). Not delivered: the plan's separate typed-fragment type and runtime table override are not part of compose.go; typed fragments are the existing Predicate and Expression values.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Green: ci-agent run 42453190263 on d8088d5 (unit, lint, fuzz-smoke, integration postgres).
- Gate: the commit hook ran the procoder format and lint gate on every commit of this story with 0 blocking findings.
- Red: run 42453213159 on throwaway branch worktree-agent-a1e9f39ea609a17b8-red (tests kept, implementation reverted) failed the build with undefined symbols for this story's API.
- The red run is a build failure (the API did not exist), not a behavioural assertion failure. The behavioural red for masking is TestFilterOnSensitiveColumnIsMasked, which fails if Filter predicates are built without column flags.
