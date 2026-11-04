# momo-05 Task 3: Safe names and LIKE escaping (S-43)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 3: Safe names and LIKE escaping (S-43) (`.procoder/plans/momo-05-writes-queries.md:249`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-43. Files: `allowlist.go`, `allowlist_test.go`.

## Acceptance criteria

- [ ] `TestColumnByName` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestEscapeLike` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-43] `TestColumnByName` rejects unknown names and returns the typed column for known ones; `TestEscapeLike` matches literal `%`, `_` and the escape character on both dialects; fails if a name is ever quoted into SQL without the allowlist.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

