# INSERT ... SELECT

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: 004-sprint-4-adoption-gaps-from-kuvryn-license-composite-pk

## Description

Some writes must be one atomic statement: insert a row only when none exists yet, via INSERT ... SELECT ... WHERE NOT EXISTS ... ON CONFLICT DO NOTHING (issue #16). Today this becomes an Exists check followed by an insert. Done means Insert can take a SELECT (including a FROM-less select with a WHERE) as its row source and composes with OnConflict and Returning.

## Acceptance criteria

- [x] Insert from a select with explicit target columns renders INSERT INTO t (cols) SELECT ... (golden test)
- [x] A FROM-less select with WHERE NOT EXISTS plus ON CONFLICT DO NOTHING renders as one statement and inserts only once when run twice (integration test on CI)
- [x] Column count mismatch between target columns and select list is ErrValidation (test)
- [x] Works with Returning and reports rows affected (test)
- [x] docs show the example from the issue

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

- Golden render INSERT INTO t (cols) SELECT ...: TestFromSelectGoldenSQL (PostgreSQL, SQLite, MySQL), CI run 42342932962 success.
- One statement, inserts once when run twice: TestFromSelectNotExistsOnce (integration, PostgreSQL 16 and 17), CI run 42342932962 success.
- Column count mismatch is ErrValidation: TestFromSelectValidation, CI run 42342932962 success.
- Returning and rows affected: TestFromSelectNotExistsOnce asserts RowsAffected 1 then 0 and the returned row; TestFromSelectGoldenSQL covers the RETURNING render; CI run 42342932962 success.
- Docs show the issue example: docs/ai-guide.md (Insert from a SELECT) and docs/capabilities.md.
- Red before green: CI run 42342328050 failed to compile the tests before the implementation.
- Review follow-up: soft-delete TableRef golden, SQLite WHERE true before ON CONFLICT for SelectExprs sources, valid MySQL FROM-less golden, shared writeInsertHead; CI run 42376605062 green on 44cd3a7. Known limit: a Select[T] source with FROM and no WHERE on SQLite still needs an explicit Where before ON CONFLICT.
