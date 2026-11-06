# momo-02 Task 21: momo.yaml configuration and naming configuration

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 21: momo.yaml configuration and naming configuration (`.procoder/plans/momo-02-generator.md:2276`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-69. Files: `gen/config/config.go`, `gen/config/config_test.go`, `gen/config/testdata/momo.yaml`, `gen/config/testdata/bad.yaml`, `gen/config/testdata/overrides/o.go`. Contributes to S-69; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestConfigLoad` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestConfigMigrateValidation` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestNamingOverridesFromConfig` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Commit e4a3ac4 `feat(gen): momo.yaml configuration with naming presets`: gen/config/{config.go,config_test.go,testdata/}.
- RED: `go test ./gen/config -count=1` failed to build with `undefined: Load` / `undefined: ToConfig`. GREEN: `ok github.com/azrtydxb/momo/gen/config`.
- Hostile cases (TestLoadHostile): empty file, unknown top-level key, version 2, unknown preset, unknown dialect, bad durations naming the key, unknown dialect under migrate.dir and migrate.url_env, nonexistent path, ToConfig defaults.
- Mutation check: removing the statement_timeout validation made TestConfigMigrateValidation and TestLoadHostile fail; restored, they pass.
- Gate: go build, go vet, go test -race -count=1 ./..., make tidy-check pass; procoder check reports 0 blocking findings.
- No change was needed outside gen/config: gen.Load already honours cfg.Naming.

