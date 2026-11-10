# momo-06 Task 11: Nested, filtered, ordered eager loading

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each
Carried: 002-generator-from-go-structs-with-its-type-prerequisites — Behaviour pre-existed in load.go, so the plan test could not fail before an implementation; mutation runs stand in for the red state (see Evidence). Decided 2026-11-06: carry rather than tick.

## Description

Implements plan momo-06-relations Task 11: Nested, filtered, ordered eager loading (`.procoder/plans/momo-06-relations.md:2614`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-81. Files: `load_test.go`.

## Acceptance criteria

- [ ] `TestNestedFilteredEagerLoad` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-81, part] `TestNestedFilteredEagerLoad` passes for the part of S-81 this task delivers; the full criterion is verified in story 152.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Criterion 1 is left open on purpose: the test exists as in the plan and passes, but it never failed before an implementation because Tasks 5-9 already deliver nesting, where and order (no production change). Its red state is mutation-based: skipping the nested load, dropping the where, dropping the order, or nesting over only the first pooled child each fail it.
- `go test -tags integration -race -run Nested .` passes on PostgreSQL: `TestNestedFilteredEagerLoad`, `TestNestedLoadDepthAndSharedChildren` (3 levels, 3 statements, shared author), `TestNestedLoadFilterAtLeaf`.
- Added tests for behaviours the review found untested (`load_nested_more_test.go`, PostgreSQL): `TestNestedLoadErrorPropagates`, `TestNestedLoadHonoursContext`, `TestNestedLoadSkipsEmptyIntermediate`, `TestNestedLoadSiblings`, `TestNestedLoadManyToMany`, `TestNestedLoadNullKeyAtIntermediate`, `TestNestedLoadSoftDeletedIntermediate`.
- Criterion 1 stays unticked (the behaviour pre-existed, no red state before the implementation). Mutation runs in `load.go` `finish()`: swallowing the nested `LoadAll` error fails `TestNestedLoadErrorPropagates` and `TestNestedLoadHonoursContext`; passing `context.Background()` instead of `ctx` to the nested `LoadAll` fails `TestNestedLoadHonoursContext`. Both were restored afterwards.
- make fmt-check vet staticcheck gosec tidy-check clean; full `go test -race ./...` run once at the end.
- Sprint 7 audit (2026-11-10): `TestNestedFilteredEagerLoad` is at rel_load_test.go:626 (root files were renamed `rel_*` in sprint 6; the nested tests are in rel_load_nested_test.go and rel_load_test.go). Nesting, Where and OrderBy are delivered by `rel_load.go`; nothing is left to build.
- Carry reason: criterion 1 ("failed before the implementation") cannot be met by any later work, because the behaviour was already delivered by Tasks 5-9 and the history cannot be rewound. The only available red state is the mutation runs recorded above. The test passes on every integration job of ci-agent (ci-agent run 42784116091, headSha 2eda85d2041b0d0f6ec5200f96facc8aeddc8f96, conclusion success; `TestNestedFilteredEagerLoad` runs in the integration jobs); the story stays open only for this one historical criterion.

