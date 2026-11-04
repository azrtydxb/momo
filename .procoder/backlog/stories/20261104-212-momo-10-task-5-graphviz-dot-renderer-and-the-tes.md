# momo-10 Task 5: Graphviz DOT renderer and the TestViz acceptance test

Status: open
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: -

## Description

Implements plan momo-10-ecosystem-generators Task 5: Graphviz DOT renderer and the TestViz acceptance test (`.procoder/plans/momo-10-ecosystem-generators.md:274`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-66. Files: `gen/viz/dot.go`, `gen/viz/viz_test.go`, `gen/viz/testdata/example.dot`.

## Acceptance criteria

- [ ] `TestViz` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-66] `TestViz` asserts golden Mermaid and DOT output for the example schema; fails if a relation or table is omitted.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

