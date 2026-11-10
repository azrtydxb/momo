# momo-10 Task 15: Plugin registration from momo.yaml

Status: done
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-10-ecosystem-generators Task 15: Plugin registration from momo.yaml (`.procoder/plans/momo-10-ecosystem-generators.md:1570`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `cmd/momo/plugins.go`, `cmd/momo/plugins_test.go`, `cmd/momo/cmd.go`.

## Acceptance criteria

- [x] `TestPluginRegistration` exists`TestPluginRegistration` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean`procoder check` gate is clean and the task's commit is made

## Evidence

- Built: cmd/momo/plugins.go (buildPlugins, sorted keys, unknown plugin and option errors, relative dir resolved against momo.yaml), generate.go uses it (selectPlugins and plugins() removed), docs updated (docs/capabilities.md, docs/ARCHITECTURE.md, generate and top-level help text).
- TestPluginRegistration and TestGenerateRunsConfiguredPlugins (cmd/momo/plugins_test.go) pass in unit job of CI run 42779257792. The existing unknown-plugin tests still pass (message still names gen.Emit).
- Open: criterion 1 not ticked. Red proof: the tests were written with the code, so no run failed on `undefined` alone; per the 2026-11-08 decision a throwaway red branch (implementation reverted) is still needed to tick this criterion.
- Review round: TestPluginRegistration asserts the value each option maps to, TestPluginOps covers ops and read_only, option maps use slices.Sorted(maps.Keys). Rebase note: the generator branch moves cmd/momo into package cli; plugins.go and its tests move with it when that lands.
- Red: ci-agent run 42796316952 (headSha 7d9a9185, branch worktree-agent-a358c0410dfa8de9b-red, deleted) with the implementation broken: TestGraphQLScaffold and TestPluginRegistration failed in the unit and integration (postgres) jobs.
- Green: ci-agent run 42796596700 (headSha 40b42404, the branch merged with origin/sprint-7): unit, integration (postgres), integration-mysql (mysql84, mysql97, mariadb), fuzz-smoke, bench-module, lint (including gen-integration is current) and apidiff-docs.
- Gate: every commit passed the procoder commit hook; `procoder check --paths-from <73 files changed against origin/sprint-7>`: 58 clean, 0 unchecked, 0 blocking (the six story files were the only unformatted ones and were formatted).
