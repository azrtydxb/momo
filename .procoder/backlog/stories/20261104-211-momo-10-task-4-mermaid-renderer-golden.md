# momo-10 Task 4: Mermaid renderer (golden)

Status: done 2026-11-07
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: -

## Description

Implements plan momo-10-ecosystem-generators Task 4: Mermaid renderer (golden) (`.procoder/plans/momo-10-ecosystem-generators.md:220`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `gen/viz/mermaid.go`, `gen/viz/mermaid_test.go`, `gen/viz/testdata/example.mmd`.

## Acceptance criteria

- [x] `TestVizMermaidGolden` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Files: gen/viz/mermaid.go, mermaid_test.go, testdata/example.mmd.
- TestVizMermaidGolden (and TestMermaidShape) red: CI run 42411263629 (commit 5058074) failed with "golden mismatch" printing the output, and CI run 42414302688 failed with "undefined: Mermaid". The golden was then written from that output after review: five entities with PK, FK and UK markers, sanitized types (vector_1536, varchar_300) and three edges. Green: CI run 42412136570 (commit 015deab) succeeded.
- Deviation from the plan text: an edge is drawn parent first (documents ||--o{ comments), because Mermaid reads the left entity as the one side; the plan drew From first, which reads as the wrong cardinality. The header is the current Header() text as a Mermaid comment.

