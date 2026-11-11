# --from-db: dbdefault for constant defaults (DEFAULT 1)

Status: done 2026-11-10
Created: 2026-11-10
Epic: -
Sprint: 008-sprint-8-fix-the-kuvryn-orbit-port-issues-73-to-79-and-the
Type: story

## Description

`--from-db` adds `dbdefault` only to NOT NULL columns with a computed default. `version bigint NOT NULL DEFAULT 1` is generated as `default:1`, and `Insert.Model` writes 0 for an unset field. Add `introspect.dbdefault: all` (integrator decision 2026-11-10), which tags every column that has a default, and document that hand-written models can use `dbdefault` on any column. GitHub issue #77.

## Acceptance criteria

- [x] the tests are shown failing on CI before the change
- [x] with `introspect.dbdefault: all`, a `DEFAULT 1` column gets `dbdefault`, and `Insert.Model` with the field unset stores 1 (integration test on PostgreSQL)
- [x] docs/upgrading.md (0.7.0) and the --from-db docs describe the option

## Evidence

Red, run 42904286499 on the throwaway branch worktree-agent-a74b43ddb346ed43c-red (commit d1d4052: config fields and a no-op migrate.applyOnly only; the branch is deleted): unit failed TestDBDefaultAllTagsConstantDefaults (version `default:1` lacked dbdefault) and TestIntrospectScopeValidation (dbdefault: "yes" accepted); integration postgres failed TestGenerateFromDBDBDefaultAll ("version must carry dbdefault").

Green, run 42913355678 (commit 141e504, ci-agent: unit, integration postgres, mysql, apidiff-docs, lint all success). gen/fromdb TestDBDefaultAllTagsConstantDefaults pins: by default DEFAULT 1 is left alone; dbdefault: all tags `default:1,dbdefault` and `default:'open'::text,dbdefault` and never a nullable or default-less column. cli TestGenerateFromDBDBDefaultAll (PostgreSQL): a real `version bigint NOT NULL DEFAULT 1` is generated with dbdefault and the generated momo_gen.go carries FlagDBDefault. The runtime half (a dbdefault column left unset is omitted and the stored default read back, constant defaults included) was already pinned by the existing TestDBDefaultLive on PostgreSQL (n integer NOT NULL DEFAULT 7); I did not add a second copy that compiles the generated model and inserts through it.

Docs: docs/upgrading.md "Upgrading to 0.7.0", docs/migrations.md "The introspect section", docs/ai-guide.md "Database defaults" (hand-written models can tag any NOT NULL column that has a default).

Follow-up review, both rounds: no change to this story's behaviour was needed. The final tree is green: ci-agent run 42938784308 on the merged commit 35ce819 (sprint-8 906fcb0 merged in; all 9 jobs success); TestDBDefaultAllTagsConstantDefaults and TestGenerateFromDBDBDefaultAll pass in it, and the 0.7.0 upgrade section keeps its text after three merges of origin/sprint-8.
