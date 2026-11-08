# momo-10 Task 6: Viz plugin and the momo viz command

Status: done 2026-11-07
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: -

## Description

Implements plan momo-10-ecosystem-generators Task 6: Viz plugin and the momo viz command (`.procoder/plans/momo-10-ecosystem-generators.md:355`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `gen/viz/plugin.go`, `gen/viz/plugin_test.go`, `cmd/momo/viz.go`, `cmd/momo/viz_test.go`, `cmd/momo/cmd.go`, `gen/writer.go`.

## Acceptance criteria

- [x] `TestVizCLI` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestVizPlugin` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Files: gen/viz/plugin.go, plugin_test.go; cmd/momo/viz.go, viz_test.go, cmd.go; gen/writer.go (Text and Scaffold) with gen/writer_text_test.go.
- TestVizCLI (and TestVizCLISnapshot) and TestVizPlugin red: CI run 42414302688 (tests alone on sprint-5) failed with "undefined: Plugin" and a TestVizCLI failure for the unknown command. Green: CI run 42412136570 (commit 015deab) succeeded. TestCLIExitCodes now covers a real missing-snapshot exit 2.
- Writer.Text refuses data without the generated header (comment styles //, %% and #) and follows the File refusal rules; TestWriterTextRefusals and TestWriterScaffoldNeverOverwrites cover it.
- Selecting viz from momo.yaml is plan Task 15 (story 222), not part of this story.

