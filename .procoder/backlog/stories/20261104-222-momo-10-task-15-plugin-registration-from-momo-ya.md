# momo-10 Task 15: Plugin registration from momo.yaml

Status: open
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-10-ecosystem-generators Task 15: Plugin registration from momo.yaml (`.procoder/plans/momo-10-ecosystem-generators.md:1570`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `cmd/momo/plugins.go`, `cmd/momo/plugins_test.go`, `cmd/momo/cmd.go`.

## Acceptance criteria

- [ ] `TestPluginRegistration` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence
- Built: cmd/momo/plugins.go (buildPlugins, sorted keys, unknown plugin and option errors, relative dir resolved against momo.yaml), generate.go uses it (selectPlugins and plugins() removed), docs updated (docs/capabilities.md, docs/ARCHITECTURE.md, generate and top-level help text).
- TestPluginRegistration and TestGenerateRunsConfiguredPlugins (cmd/momo/plugins_test.go) pass in unit job of CI run 42779257792. The existing unknown-plugin tests still pass (message still names gen.Emit).
- Open: criterion 1 not ticked. Red proof: the tests were written with the code, so no run failed on `undefined` alone; per the 2026-11-08 decision a throwaway red branch (implementation reverted) is still needed to tick this criterion.
