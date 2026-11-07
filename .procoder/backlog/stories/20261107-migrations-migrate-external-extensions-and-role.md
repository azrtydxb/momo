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

- CI run 42410529827 on c2107a55807362186f9eaba839d070654722dfb7 (workflow ci-agent.yml, branch worktree-agent-ab8f63ce1b0000562): conclusion success, including the integration (postgres) job.
- Privileges: TestAppRoleExtensionIfNotExists (idempotent extension statement succeeds as a non-superuser without CREATE on the database); TestBaselineAsAppRole (works with CREATE on the schema, snapshot without table text); TestBaselineAsAppRoleNeedsCreateForTableText (ErrCanonicalizePrivilege for any table with default, check or index text, also with force); TestBaselineAsAppRoleNeedsCreateOnSchema (the history table CREATE TABLE IF NOT EXISTS is refused without CREATE on the schema even for the owner). Findings recorded in docs/migrations.md; this also corrected the old claim that only views, functions, triggers and policies need the privilege.
- Setting: TestDiffExternalExtensionsNeverEmitted (unit), TestExternalExtensionsHiddenFromBaselineAndDrift (integration), TestMigrateExternalExtensions (config).
- `new --custom`: TestNewCustomWritesChainedFiles (validate clean, momo.sum covers the new files), TestNewCustomRefusals, and the usage cases in TestMigrateUsageErrors.
- Playbook: section 7a of docs/migrations.md.
