# history WriteImport output fails migrate validate

Status: open
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

- [ ] a regression test pins the fix: red before the change, green after (TestWriteImportValidates)
- [ ] the written up files start with `-- momo:imported`, then `-- momo:no-transaction` for NoTx
- [ ] `-- momo:imported` parses, validate accepts it, and rebase refuses an imported file as hand-written (test)
- [ ] docs/importers.md, docs/migrations.md says the output passes validate; docs/upgrading.md (0.8.0) says how to fix an older import

## Evidence

(pending CI)
