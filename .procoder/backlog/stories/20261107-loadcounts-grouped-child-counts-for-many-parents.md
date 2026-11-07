# LoadCounts: grouped child counts for many parents

Status: done 2026-11-08
Created: 2026-11-07
Epic: momo-06-relations
Sprint: -

## Description

Relation.Count is one statement per parent and CountOrder only orders, so a list screen that shows a child count per row issues N+1 queries (issue #39; NovaCRM has 22 count sites). Done means momo ships LoadCounts, which counts the children of many parents in one grouped statement per bind-limit chunk, for has-one, has-many and many-to-many relations with composite keys, honouring soft delete and an optional child filter.

## Acceptance criteria

- [x] LoadCounts returns one count per parent in parent order, zero for no children or a NULL key, in one statement for a small parent list (integration test on CI)
- [x] Composite parent keys, has-many, many-to-many and polymorphic relations are supported; belongs-to is ErrUnsupported (golden plus integration test)
- [x] Soft-deleted children are excluded by default and LoadWithDeleted and LoadOnlyDeleted change that; LoadWhere filters the children (integration test)
- [x] Parent keys are chunked under the bind limit (unit and integration test with a small MaxParams)
- [x] docs/relations.md documents LoadCounts

## Evidence

CI run 42409391233 green on a221630 (lint, fuzz-smoke, unit, integration postgres).

- One statement, order, zero for no children, repeated parent: TestLoadCounts (integration).
- Composite, has-many, many-to-many, belongs-to unsupported: TestCountQueriesGolden, TestLoadCounts.
- Soft delete and filter: TestLoadCounts, TestCountQueriesManyToManyFilterJoins.
- Chunking: TestCountQueriesChunkOnBindLimit, TestLoadCountsChunks.
- Docs: docs/relations.md, Counting children for many parents.
