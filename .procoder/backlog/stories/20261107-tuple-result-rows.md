# Tuple result rows without a projection struct per shape

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: 004-sprint-4-adoption-gaps-from-kuvryn-license-composite-pk

## Description

Store code that reads aggregates, joins or a few columns needs a projection struct with generated Meta for every result shape (issue #19). The kuvryn-license port hand-wrote generic R1..R10 row types with a hand-built Meta, used by 67 queries. Done means momo ships generic tuple rows that scan positionally from a Columns list, so applications declare no per-shape struct and no Meta.

## Acceptance criteria

- [x] Generic tuple row types for 1 to 10 columns exist in momo and work as Select type parameters with Columns of matching count (golden or unit test)
- [x] A column count mismatch between the tuple and Columns is ErrValidation, not a scan panic (test)
- [x] Tuple rows scan aggregates, nullable left-join columns into pointer types, and subquery columns (integration test on CI)
- [x] Tuple rows are read-only projections: writes are refused (test)
- [x] docs show the R2-style examples from the port

## Evidence

CI run 42375911161 green on 1649658 (unit, race, lint, PostgreSQL 16 and 17 integration with MOMO_REQUIRE_INTEGRATION=1 per Makefile line 48, so integration tests cannot skip).

- Row1 to Row10 as Select type parameters: TestTupleRowGolden (one case per width, Row1 to Row10), TestTupleRowScansPositionally, TestTupleRowMeta.
- Count mismatch is ErrValidation: TestTupleRowColumnCountMismatch covers SQL, All, One, Each, Iter, Count and a mismatched inner subquery.
- Aggregates, nullable left-join pointers, subquery columns: TestTupleRowAggregates, TestTupleRowLeftJoinNullablePointers, TestTupleRowSubqueryColumnAndCTE, TestTupleRowNullableAggregate, TestTupleRowWithRecursive (integration).
- Writes refused: TestTupleRowWritesRefused covers Insert, Update, Delete, Upsert, Insert with Models, SoftDelete and ForceDelete.
- Docs: docs/projections.md and docs/ai-guide.md show Row2 examples matching the port's R2 usage; llms.txt links projections.
