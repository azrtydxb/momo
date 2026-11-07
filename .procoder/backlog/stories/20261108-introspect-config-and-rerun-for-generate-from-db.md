# Introspect config and re-run mode for generate --from-db

Status: done 2026-11-07
Created: 2026-11-08
Epic: momo-11-adoption
Sprint: -

## Description

GitHub issue #36. Prisma semantics (updatedAt, Json shapes, Unsupported columns) cannot be read from the database, and the schema keeps changing during a port. `momo.yaml` gets an `introspect:` section (skip globs, SQL type to Go type rules, per-column rules by `table.column` glob, per-table rules) that is applied on every `momo generate --from-db` run, and a re-run rewrites model files nobody edited while keeping hand-edited files and reporting the conflict. Files: `gen/fromdb/config.go`, `gen/fromdb/merge.go`, `gen/config/config.go`, `cmd/momo/generate_fromdb.go`.

## Acceptance criteria

- [x] `introspect:` is validated when momo.yaml loads and unknown or malformed rules are rejected naming the key (TestConfigValidate, TestIntrospectConfigFromYAML)
- [x] Column, type, table and skip rules change the generated models (TestGenerateFromDBRules, TestGenerateFromDBUnmappedColumn)
- [x] A re-run keeps a hand-edited file, writes the regenerated version as `.new`, reports `conflict:` and exits 1; `--overwrite` replaces it; `--check` writes nothing (TestPlanApplyKeepsHandEdits, TestGenerateFromDBRerun)

## Evidence

CI run 42410506839 on 7ef00ed, all jobs green including integration (postgres): the three tests above pass.
