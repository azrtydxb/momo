# Types: map google/uuid.UUID and []uuid.UUID without a type tag

Status: done 2026-11-10
Created: 2026-11-10
Epic: -
Sprint: 008-sprint-8-fix-the-kuvryn-orbit-port-issues-73-to-79-and-the
Type: story

## Description

A field of `github.com/google/uuid.UUID` needs `momo:"type:uuid"` on every field, and `--from-db` needs an `introspect.types` rule to write it. Map `uuid.UUID`, `*uuid.UUID` and `[]uuid.UUID` to `uuid` and `uuid[]` in the type map, and let `--from-db` choose google/uuid with one config switch. GitHub issue #76.

## Acceptance criteria

- [x] the tests are shown failing on CI before the change
- [x] a model field of `uuid.UUID`, `*uuid.UUID` and `[]uuid.UUID` maps to uuid and uuid[] with no type tag, and round-trips on PostgreSQL
- [x] one `--from-db` config switch generates google/uuid fields with no type tags
- [x] the momo root module gains no new required dependency for users who do not use google/uuid, or the docs state the dependency

## Evidence

- Red first: throwaway branch worktree-agent-a418febe208302c67-red76 (commit fd4b346, tests only), ci-agent run 42904209450 failed: TestMapGoogleUUID (gen/typemap), TestGoogleUUIDFieldsMapWithoutTypeTag (gen), TestArrayOperatorsGoogleUUIDGolden (arrayop), TestGenerateFromDBGoogleUUID, TestGenerateFromDBDefaultUUIDIsMomo and TestConfigValidateUUID (gen/fromdb). The branch is deleted after this story.
- Green: branch worktree-agent-a418febe208302c67 at ba9e070, ci-agent run 42921314183, all 9 jobs success. Integration jobs cover TestGoogleUUIDLive (PostgreSQL: uuid, *uuid.UUID NULL, uuid[], arrayop Contains/ContainedBy/Overlaps/AnyOf on a []uuid.UUID column), TestGoogleUUIDLiveSQLite (TEXT round trip) and the TestTypeMapping rows for uuid.UUID and []uuid.UUID.
- Dependency: the generator and arrayop match the type by import path and import nothing from google/uuid. The root go.mod already required it indirectly; tests import it, so tidy made it a direct requirement (tests only), listed in docs/supported-versions.md. Docs: docs/upgrading.md "Upgrading to 0.7.0", docs/TYPE_MAPPING.md, docs/migrations.md; generated blocks taken from CI regenerated-docs artifacts (runs 42912798690, 42918912824).
- MySQL: no uuid mapping in typemap, same as types.UUID (documented). SQLite: uuid.UUID is TEXT; []uuid.UUID has no SQLite column.

### Review fix round
- Negative and alias tests added: TestForeignUUIDPackageIsNotMapped (fixture package named uuid, V and Vs unmapped), TestGoogleUUIDThroughAliases (import alias gu, type ID = gu.UUID, []ID), TestArrayOperatorsForeignUUIDGetsNoCast.
- Red by mutation: throwaway branch worktree-agent-a418febe208302c67-red76b (commit 1c4155c disables the baseType guard and the arrayop import-path check), ci-agent run 42924344056 failed TestForeignUUIDPackageIsNotMapped and TestArrayOperatorsForeignUUIDGetsNoCast. The branch is deleted.
- TestGoogleUUIDThroughAliases found a real gap: run 42924654806 (487bb20) failed it because a slice of an alias ([]ID) was spelled []pkg.ID and stayed unmapped. Fixed in baseType (alias elements of a slice are unaliased).
- Minor items: isGoogleUUID removed in favor of knownNamed, UUIDMomo constant, single "Slices as arrays" bullet plus the named-type note, MySQL notes in upgrading.md and migrations.md, capabilities.md array row, deps note "tests only (root, arrayop)". Generated dependency row from the regenerated-docs artifact of run 42924654806.
- Merged origin/sprint-8 (9ba0eee); one "## Upgrading to 0.7.0" heading holds all entries. Green: ci-agent run 42928327067 on 999c4f2.
