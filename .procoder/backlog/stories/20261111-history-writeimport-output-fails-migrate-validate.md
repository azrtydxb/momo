# history WriteImport output fails migrate validate

Status: done 2026-11-11
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

Issue #88 in azrtydxb/momo.

Reproduction: `goose.Read`, `history.Convert`, `history.Baseline`,
`history.WriteImport`, then `migrate.ValidateOffline` (`momo migrate
validate`) on the output.

Observed vs expected: validate reports "custom migration ... must not change
the schema, but its snapshot differs from its parent's" for every imported
migration after the first; the output should validate.

Root cause: WriteImport wrote the up files without a directive, so check 6 of
ValidateOffline treats them as custom migrations. Integrator decision: not
`-- momo:generated`, because `momo migrate rebase` would then regenerate the
user's imported history. A new `-- momo:imported` directive is parsed in
migrate/files.go (`Directives.Imported`), skipped by validate check 6, and
refused by rebase like any hand-written file.

## Acceptance criteria

- [x] a regression test pins the fix: red before the change, green after (TestWriteImportValidates)
- [x] the written up files start with `-- momo:imported`, then `-- momo:no-transaction` for NoTx
- [x] `-- momo:imported` parses, validate accepts it, and rebase refuses an imported file as hand-written (test)
- [x] docs/importers.md, docs/migrations.md says the output passes validate; docs/upgrading.md (0.8.0) says how to fix an older import

## Evidence

- Red: ci-agent run 42978022026 on throwaway branch worktree-agent-red-87-88 (commit 1268ab8, branch deleted): TestWriteImportValidates failed with `the imported directory must validate, got: [19700101000002_b.snapshot.json: custom migration 19700101000002_b.up.sql must not change the schema ...]`;
  TestWriteImportNeverOverwrites failed on the new directive prefix; the migrate package tests did not build (`d.Imported undefined`) for TestDirectivesImported and TestMigrateRebaseRefusesImportedMigration.
- Green: ci-agent run 42978042350 on 76e9afd, all 9 jobs green.
- Docs: docs/importers.md, docs/migrations.md, docs/upgrading.md (0.8.0). No docs/reference.md change: the regenerated-docs job produced no artifact, so there was no diff.
