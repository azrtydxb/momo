# momo-02 Task 21: momo.yaml configuration and naming configuration

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 21: momo.yaml configuration and naming configuration (`.procoder/plans/momo-02-generator.md:2276`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-69. Files: `gen/config/config.go`, `gen/config/config_test.go`, `gen/config/testdata/momo.yaml`, `gen/config/testdata/bad.yaml`, `gen/config/testdata/overrides/o.go`. Contributes to S-69; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestConfigLoad` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestConfigMigrateValidation` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestNamingOverridesFromConfig` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

