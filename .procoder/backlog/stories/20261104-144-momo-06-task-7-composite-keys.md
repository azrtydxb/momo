# momo-06 Task 7: Composite keys

Status: done 2026-11-06
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-06-relations Task 7: Composite keys (`.procoder/plans/momo-06-relations.md:2274`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-6. Files: `load_test.go`.

## Acceptance criteria

- [x] `TestEagerLoadComposite` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-6, part] this story's tests pass for the part of S-6 this task delivers; the full criterion is verified in story 146.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red-first: `TestEagerLoadComposite` is red as the plan says (`undefined: store` before the `store` and `region` imports) and then passes on the Task 4 and Task 5 code, because those paths are kind- and arity-generic. The one genuine behaviour gap found was a parent key with a NULL component reported with `ok = true`: `TestEagerLoadCompositeNullComponent` failed (1 statement issued, want 0) before the `hasNilPart` guard in `prepareWith`; plan reconciliation note added under Task 7.
- Added tests: `TestEagerLoadCompositeSemantics` (one statement per load, duplicate parent row, primary-key child order, tuple matching), `TestEagerLoadCompositeEmptyAndStale` (nil slice for a region without stores, reload with an empty scope clears both directions), `TestEagerLoadCompositeNullComponent`, and internal `TestLoadCompositeKeysDedupeNullAndChunk` (dedupe, NULL in either component dropped, chunks under the bind limit in key order).
- Mutations on `load.go`, each killed: removing the nil-component guard (fails `TestEagerLoadCompositeNullComponent`, `TestLoadCompositeKeysDedupeNullAndChunk`); keying parents by the first column only (fails `TestEagerLoadComposite`, `TestEagerLoadCompositeSemantics`, `TestEagerLoadCompositeEmptyAndStale`); keying children by the first column only (same three); dropping parent-key dedupe (fails `TestLoadCompositeKeysDedupeNullAndChunk`; invisible at the integration level because duplicate IN tuples are harmless); removing `r.clear` (fails `TestEagerLoadCompositeEmptyAndStale`).
- Close-out, `TestEagerLoadComposite` (load_test.go:216): loads Store.Region and Region.Stores over the (tenant_id, code) composite key, a NULL component gives a nil Region. Test and loader read; the integration run could not be repeated at close (local PostgreSQL on 127.0.0.1:5432 resets connections), so the run recorded here stands and CI re-runs it.
- Close-out, `TestLoadCompositeKeysDedupeNullAndChunk` (load_test.go, no database): re-run at close, PASS; proves dedupe, NULL component dropped via `hasNilPart` (load.go) and chunking.
- Close-out, [S-6, part]: composite-key arity is handled in load.go keying; the full S-6 is verified in story 146. Gate: shipped in commit c744711 (#5) and fix 36ca242; `procoder check` run at close.
- Green: `go test -tags integration -race -count=1 . ./internal/...` against Postgres (one run hit a `drop schema` timeout in `TestInsertModelOmitAndConflict` from database contention with other jobs; it passed on rerun), `go test -race -count=1 ./...`, and `make fmt-check vet staticcheck gosec tidy-check` clean.
