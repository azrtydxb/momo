# momo-10 Task 6: Viz plugin and the momo viz command

Status: open
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: -

## Description

Implements plan momo-10-ecosystem-generators Task 6: Viz plugin and the momo viz command (`.procoder/plans/momo-10-ecosystem-generators.md:355`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `gen/viz/plugin.go`, `gen/viz/plugin_test.go`, `cmd/momo/viz.go`, `cmd/momo/viz_test.go`, `cmd/momo/cmd.go`, `gen/writer.go`.

## Acceptance criteria

- [ ] `TestVizCLI` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestVizPlugin` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

