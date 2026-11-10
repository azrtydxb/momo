# momo-11 Task 14: Migration guides for GORM, Bun and Ent users

Status: done 2026-11-10
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-11-adoption Task 14: Migration guides for GORM, Bun and Ent users (`.procoder/plans/momo-11-adoption.md:1566`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-95. Files: `docs/guides/gorm.md`. Contributes to S-95; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] Step 1: Failing check
- [x] Step 2: Write the guides. GORM guide: Models (struct tags to Momo `momo:` tags and `MomoConfig`), Queries (`db.Find`/`Where` to `momo.Select[T]`), Relations (`Preload` to `momo.Load` and `.With`), Hooks (`BeforeCreate` to typed model hooks, S-85), Transactions (`db.Transaction` to `db.Tx`, no autom…
- [x] Step 3: Run the check, expect PASS for the headings check (`grep -c` returns 7 each).
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- docs/guides/gorm.md, bun.md, ent.md each have the seven required sections plus a Cheat sheet (so `grep -c '^## '` returns 8, not 7). Go blocks are complete programs compiled by TestDocsCodeBlocks (unit job of ci-agent run 42780875746, headSha 981b3c0777de48b0ac7870ec9e48304aa2dae545, all 9 jobs success); models come from examples/quickstart/models, not the plan's examples/guides/models, whose generated code cannot be produced without running the generator locally. Nav in mkdocs.yml and links in docs/README.md added.
- Step 1 (failing check before the files exist) was not run, so it stays unticked.

### Red proof (2026-11-10)

Step 1: on a throwaway branch (deleted) with the `## Hooks` heading renamed in docs/guides/gorm.md, bun.md and ent.md, the check `TestMigrationGuidesComplete` FAILS in the unit job, ci-agent run 42827469571 on 35dc039. The guides were not deleted, so this is the failing check with a section missing, not literally before the files exist.
