# jsonop: root scalar, key delete, concat, set with computed path, array constructors

Status: done 2026-11-11
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #96. Owl's audit snapshots, settings reads and retention queries
need jsonb operations momo cannot express, so they fall back to raw SQL.

Reproduce: try to build `value #>> '{}'`, `to_jsonb(x) - 'password_hash'`,
`release || jsonb_build_object(...)`, `jsonb_set(skipped, ARRAY['at_' || $1::text], ...)`
or `ARRAY(SELECT ... UNION SELECT ...)` with momo. `jsonop.Path` with no keys
fails with "JSON path has no elements" (ErrValidation), there is no `-`, `||`
or computed-path `jsonb_set` in jsonop, and no `ARRAY[...]` or `ARRAY(subquery)`
expression in momo.

Observed: none of these can be written as a typed expression; the caller drops
to raw SQL, which loses the sensitive-flag masking and, for a key delete,
forces a column list that silently picks up a future secret column.

Expected: `jsonop.Root`, `jsonop.DeleteKey`, `jsonop.Concat`, `jsonop.Set`,
`momo.ArrayOf` and `momo.ArraySubquery` render the PostgreSQL operators, with
MySQL and SQLite renderings where an equivalent exists and ErrUnsupported
where none does.

Root cause: the jsonop package only had extraction, containment and key tests,
and the root package had no array-constructor expression. Root extraction was
blocked by the non-empty-path check, which is correct for Path.

## Acceptance criteria

- [x] `jsonop.Root(col)` renders `#>> '{}'` (PostgreSQL), `json_extract(col, '$')` (SQLite) and `JSON_UNQUOTE(JSON_EXTRACT(col, '$'))` (MySQL); `Path` with no keys stays ErrValidation
- [x] `jsonop.DeleteKey` renders `-` / `json_remove` / `JSON_REMOVE` with the key bound, and keeps every other member when run against PostgreSQL and SQLite
- [x] `jsonop.Concat` and `jsonop.Set` run on PostgreSQL and return ErrUnsupported on SQLite and MySQL
- [x] `momo.ArrayOf` and `momo.ArraySubquery` run on PostgreSQL (including a UNION subquery), refuse an empty `ArrayOf`, and return ErrUnsupported elsewhere
- [x] values bound through the new operators from a sensitive JSON column are masked in hook events
- [x] reader docs (ai-guide, capabilities, TYPE_MAPPING, upgrading 0.8.0) and docs/reference.md describe the new API
- [x] a regression test pins the fix: red before the change, green after

## Evidence

- Red: ci-agent run 42978470024 on worktree-agent-red-96 (af3e1ab, tests only) failed to compile: `expr_array_test.go:38:55: undefined: momo.ArrayOf`, `expr_array_test.go:42:44: undefined: momo.ArraySubquery`, `jsonop/jsonop_edit_test.go:24:56: undefined: momo.ArrayOf` (lint vet and fuzz-smoke jobs). The red branch is deleted.
- Green: ci-agent run 42994909120, conclusion success, headSha c633ced0532c3b29cb8e343e5b2dba083abcc264 on worktree-agent-a02be359058185aae. All criteria are covered by TestJSONEditOperatorsGolden, TestJSONEditOperatorErrors, TestJSONEditOperatorsMaskSensitiveColumn, TestArrayConstructorsSQL, TestArrayConstructorErrors and the integration test TestJSONEditOperatorsAgainstDatabase.
- Docs changed: docs/ai-guide.md, docs/capabilities.md, docs/TYPE_MAPPING.md, docs/upgrading.md (Upgrading to 0.8.0), docs/reference.md (from the regenerated-docs artifact of run 42991319570).
- Review round, red: ci-agent run 43003343119 on worktree-agent-red-96b (8ec7656a, tests only) failed with `expr_array_test.go:83: subquery of two columns: err = <nil>, want momo: validation failed`, `expr_array_test.go:95: flags = 0, want sensitive`, `expr_array_test.go:98: flags = 0, want nullable`, `jsonop_edit_test.go:111: set path: lost the sensitive flag` and `jsonop_edit_test.go:115: Set lost the nullable flag of its path`. The red branch is deleted.
- Review round, tests: TestArrayConstructorErrors (two-column subquery is ErrValidation), TestArrayOfFlags, TestJSONEditColumnsAndFlags (Set path flags), and the SQLite case of TestJSONEditOperatorsAgainstDatabase that selects ArrayOf directly and expects ErrUnsupported.
- Review round, green: ci-agent run 43011942969, all jobs green, headSha ec23396b on worktree-agent-a02be359058185aae. The whole-package jsonop entries in internal/docscheck/testdata/bare_allow.txt were removed; the bare Set mentions are qualified instead.
