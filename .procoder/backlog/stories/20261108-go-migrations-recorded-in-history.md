# Go migrations recorded in history (WithGoMigrations)

Status: open
Created: 2026-11-08
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

A data step that SQL cannot express runs outside Engine.Apply today, so the history does not record it and nothing stops it from running twice (issue #43, found by the Kuvryn Owl Enterprise port, whose backfillTemplates step runs on every start). Done means a GoMigration registered with WithGoMigrations is ordered with the SQL files by version, runs once under the migration lock, and is recorded in the history table with kind go and the dirty and resolve semantics of a no-transaction SQL migration.

## Acceptance criteria

- [ ] GoMigration and WithGoMigrations exist; a Go migration and a SQL file on one version, two Go migrations on one version, a bad version or name, and a migration with neither or both of Up and UpNoTx are errors (unit test and integration test)
- [ ] Go migrations are ordered with SQL files by version in Plan, Apply and Status, run once, and their history row has kind go, the name, an apply id and a checksum (integration test on CI)
- [ ] Up runs with a *momo.Tx in the migration transaction: an error or a panic rolls the work back, records rolled_back, and the next apply retries it (integration test on CI)
- [ ] UpNoTx runs outside a transaction on an OnConn handle (CREATE INDEX CONCURRENTLY works) and a failure leaves the row failed: Apply refuses with ErrDirty, Resolve applied and rolled_back both work (integration test on CI)
- [ ] The checksum is a stable identity of version, name and the optional Checksum string; changing it after apply is ErrChecksumMismatch and shows as mismatch in Status (unit and integration test)
- [ ] ValidateOffline given the Go migrations checks their snapshot like a custom SQL migration's (equal to the parent's, present, no shared version) (unit test)
- [ ] docs/migrations.md says the momo CLI cannot run Go migrations and shows how an application runs Engine.Apply itself, and that Go migrations are forward-only; GoMigration is out of the Planned list in docs/ARCHITECTURE.md

## Evidence
