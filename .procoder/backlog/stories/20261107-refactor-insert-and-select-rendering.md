# Refactor: split InsertQuery modes and unify select rendering

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Maintainability items 6 and 8: InsertQuery is a five-mode type in a 1072-line insert.go that duplicates transaction scope from db.go; renderBody is 160 lines and ExprSelect is a second partial renderer. Audit report: .procoder/research/security-audit-2026-11-07.md and .procoder/research/maintainability-audit-2026-11-07.md.

## Acceptance criteria

- [x] insert.go is split into insert.go (builder), insert_render.go (one resolve step and insertMode), insert_chunk.go; the chunk transaction scope is now the shared beginAtomic/runAtomic in db_atomic.go (DB.Tx and Tx.Tx keep their own, differently-behaving, scope)
- [x] Select and ExprSelect share the selectCore clause writers (FROM, soft-delete marker, WHERE); golden tests unchanged
- [x] No behaviour change (full CI)

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

CI run 42409368006 on 76cc882 (ci-agent.yml): success; no golden or test file changed by this refactor.
