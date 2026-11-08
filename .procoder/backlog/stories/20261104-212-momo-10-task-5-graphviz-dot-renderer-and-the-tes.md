# momo-10 Task 5: Graphviz DOT renderer and the TestViz acceptance test

Status: done 2026-11-07
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: -

## Description

Implements plan momo-10-ecosystem-generators Task 5: Graphviz DOT renderer and the TestViz acceptance test (`.procoder/plans/momo-10-ecosystem-generators.md:274`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-66. Files: `gen/viz/dot.go`, `gen/viz/viz_test.go`, `gen/viz/testdata/example.dot`.

## Acceptance criteria

- [x] `TestViz` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-66] `TestViz` asserts golden Mermaid and DOT output for the example schema; fails if a relation or table is omitted.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Files: gen/viz/dot.go, viz_test.go, testdata/example.dot.
- TestViz (mermaid and dot subtests, and TestDOTShape) red: CI run 42411263629 (commit 5058074) failed with "differs from testdata/example.dot" printing the output, and CI run 42414302688 failed with "undefined: DOT". The golden was then written from that output after review: five tables and three edges. Green: CI run 42412136570 (commit 015deab) succeeded.
- Deviation from the plan text: the arrowhead sits at the parent end and the arrowtail at the child end, with dir=both, so the crow foot is on the many side.

