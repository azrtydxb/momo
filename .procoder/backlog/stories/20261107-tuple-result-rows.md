# Tuple result rows without a projection struct per shape

Status: open
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: 004-sprint-4-adoption-gaps-from-kuvryn-license-composite-pk

## Description

Store code that reads aggregates, joins or a few columns needs a projection struct with generated Meta for every result shape (issue #19). The kuvryn-license port hand-wrote generic R1..R10 row types with a hand-built Meta, used by 67 queries. Done means momo ships generic tuple rows that scan positionally from a Columns list, so applications declare no per-shape struct and no Meta.

## Acceptance criteria

- [ ] Generic tuple row types for 1 to 10 columns exist in momo and work as Select type parameters with Columns of matching count (golden or unit test)
- [ ] A column count mismatch between the tuple and Columns is ErrValidation, not a scan panic (test)
- [ ] Tuple rows scan aggregates, nullable left-join columns into pointer types, and subquery columns (integration test on CI)
- [ ] Tuple rows are read-only projections: writes are refused (test)
- [ ] docs show the R2-style examples from the port

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
