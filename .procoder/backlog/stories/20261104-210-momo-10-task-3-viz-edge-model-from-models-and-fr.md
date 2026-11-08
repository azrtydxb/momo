# momo-10 Task 3: Viz edge model from models and from a snapshot

Status: done 2026-11-07
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: -

## Description

Implements plan momo-10-ecosystem-generators Task 3: Viz edge model from models and from a snapshot (`.procoder/plans/momo-10-ecosystem-generators.md:156`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `gen/viz/edges.go`, `gen/viz/edges_test.go`.

## Acceptance criteria

- [x] `TestVizEdges` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Files: gen/viz/edges.go, columns.go, edges_test.go.
- TestVizEdges (and TestVizEdgesFromSchema) red: CI run 42414302688 (tests alone on sprint-5) failed with "undefined: EdgesFromGraph". Green: CI run 42412136570 (commit 015deab) succeeded.
- The example edges are comments to documents, documents to tags (many-to-many) and documents to tenants. A has_many and the matching belongs_to collapse into one edge.
- Deviation from the plan text: for many-to-many, To is the target table and Label names the join table, instead of To being the join table. The join table still appears in Tables with its two foreign keys.

