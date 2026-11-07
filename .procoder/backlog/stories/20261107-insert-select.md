# INSERT ... SELECT

Status: open
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: 004-sprint-4-adoption-gaps-from-kuvryn-license-composite-pk

## Description

Some writes must be one atomic statement: insert a row only when none exists yet, via INSERT ... SELECT ... WHERE NOT EXISTS ... ON CONFLICT DO NOTHING (issue #16). Today this becomes an Exists check followed by an insert. Done means Insert can take a SELECT (including a FROM-less select with a WHERE) as its row source and composes with OnConflict and Returning.

## Acceptance criteria

- [ ] Insert from a select with explicit target columns renders INSERT INTO t (cols) SELECT ... (golden test)
- [ ] A FROM-less select with WHERE NOT EXISTS plus ON CONFLICT DO NOTHING renders as one statement and inserts only once when run twice (integration test on CI)
- [ ] Column count mismatch between target columns and select list is ErrValidation (test)
- [ ] Works with Returning and reports rows affected (test)
- [ ] docs show the example from the issue

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
