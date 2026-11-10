# momo-11 Task 12: Bun migration history importer

Status: done 2026-11-10
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-11-adoption Task 12: Bun migration history importer (`.procoder/plans/momo-11-adoption.md:1349`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-93. Files: `importer/history/bun/bun.go`, `internal/compat/bunmigfixture/migrations/20260101000000_init.up.sql`.

## Acceptance criteria

- [x] `TestHistoryImporters` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-93] `TestHistoryImporters` imports each fixture history, asserts status shows all versions applied and drift is zero; fails if an applied migration would re-run.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- importer/history/bun/bun.go, fixture internal/compat/bunmigfixture/migrations. `TestHistoryImporters/bun` (import, status applied for all versions, drift clean via internal/importtest CheckImport), `bun/go-function-refused` and `bun/pending-refused` pass in the integration (postgres) job of ci-agent run 42780875746, headSha 981b3c0777de48b0ac7870ec9e48304aa2dae545, all 9 jobs success.
- Table layout and file naming read from github.com/uptrace/bun@v1.3.0 migrate/migration.go, migrations.go (fnameRE), migrator.go.
- Red-first not evidenced: the implementation was written before its test ran and no failing CI run was recorded, so the 'failed before the implementation' criterion stays unticked (the tests pass).
- Review fix: Read takes a table name (default bun_migrations, quoted); .up.sql with .tx.up.sql for one version is refused (bun/conflicting-up-files-refused, bun/custom-table).

### Red proof (2026-11-10)

Throwaway branch (deleted), bun.Read returning an empty source: `TestHistoryImporters/bun` and its subtests (go-function-refused, pending-refused, conflicting-up-files-refused, custom-table) FAIL in the integration (postgres) job, ci-agent run 42827469571 on 35dc039. Passes on sprint-7 (run 42780875746).
