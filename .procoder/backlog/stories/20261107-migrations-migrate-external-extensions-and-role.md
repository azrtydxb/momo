# Migrations: migrate.external_extensions and role verification (issue 37)

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-04-migrations-core-postgresql
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

GitHub issue #37, found by the NovaCRM port. NovaCRM installs pgvector by hand as a superuser and its app role may lack CREATE on the database, while `prisma db push` keeps changing the schema until cutover. Done means the privilege behaviour is verified on CI and documented, an extension managed outside momo can be declared so no migration emits it and drift ignores it, hand-written migrations can be started with `migrate new --custom`, and a cutover playbook exists.

## Acceptance criteria

- [x] CREATE EXTENSION IF NOT EXISTS for an installed extension succeeds as a non-superuser role without CREATE on the database (integration test)
- [x] what baseline needs without CREATE on the database, and what fails, is verified by integration tests and recorded in docs/migrations.md
- [x] `migrate.external_extensions` in momo.yaml: diff and AutoMigrate never emit the extension, drift and baseline do not report it (unit and integration tests)
- [x] `momo migrate new --custom <name>` writes an empty hand-written migration wired into the checksum chain and passes validate (unit test)
- [x] docs/migrations.md has a cutover playbook from Prisma db push to momo

## Evidence

- CI run 42414194011 on f58f3752122227717996e3881c8deef2534f470d (ci-agent.yml, branch worktree-agent-ab8f63ce1b0000562): success, including the integration (postgres) job. Later commits on the branch only touch .procoder files.
- Privileges (migrate/external_extension_integration_test.go): TestAppRoleExtensionIfNotExists (the idempotent extension statement succeeds as a non-superuser without CREATE on the database); TestBaselineAsAppRole (works with CREATE on the schema for a snapshot without table text); TestBaselineAsAppRoleNeedsCreateForTableText (table-driven over default, generated column, check constraint, index predicate and index expression: ErrCanonicalizePrivilege, also with force); TestBaselineAsAppRoleNeedsCreateOnSchema (the history table CREATE TABLE IF NOT EXISTS is refused without CREATE on the schema even for the owner). Findings are in docs/migrations.md, which also corrects the old claim that only views, functions, triggers and policies need the privilege.
- Setting: TestDiffExternalExtensionsNeverEmitted and TestDiffExternalExtensionAlreadyInSnapshot (migrate, unit), TestExternalExtensionsHiddenFromBaselineAndDrift and TestAutoMigrateSkipsExternalExtension (integration), TestMigrateExternalExtensions (gen/config: load and invalid names), TestMigrateExternalExtensionsDiff and TestMigrateExternalExtensionsBadConfig (cmd/momo).
- `new --custom`: TestNewCustomWritesChainedFiles (validate clean, momo.sum covers the new files), TestNewCustomRefusals (ErrInvalidName, ErrNoSnapshot), the new cases in TestMigrateUsageErrors.
- Playbook: section 7a of docs/migrations.md.
