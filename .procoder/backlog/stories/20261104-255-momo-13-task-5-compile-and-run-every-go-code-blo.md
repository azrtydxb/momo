# momo-13 Task 5: Compile and run every Go code block in the docs

Status: open
Created: 2026-11-04
Epic: momo-13-docs-release
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-13-docs-release Task 5: Compile and run every Go code block in the docs (`.procoder/plans/momo-13-docs-release.md:558`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-23, S-95. Files: `internal/docscheck/codeblocks_test.go`.

## Acceptance criteria

- [x] `TestDocsCodeBlocks` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestDocsRunnableBlocks` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-23, part] `TestDocsCodeBlocks` passes for the part of S-23 this task delivers; the full criterion is verified in story 256.
- [ ] [S-95] `TestDocsCodeBlocks` covers the guide pages and `TestMigrationGuidesComplete` asserts each guide has sections for models, queries, relations, hooks, transactions, migrations and errors; fails if a guide section is missing.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- red run 42454971851 (work branch suffixed -red, sha 222fa09): FAIL TestDocsCodeBlocks and TestDocsRunnableBlocks (integration job of ci-agent run 42454971851)
- green: docs workflow run 42456103295 (sha 9b46480d8ec2f3944100975376d197a31720171c, success), where TestDocsCodeBlocks and TestDocsRunnableBlocks passes
- Every docs go block is a compiled complete program, a program marked docscheck:run (executed against PostgreSQL), or a block marked docscheck:fragment that must still parse as Go; the fragment count is logged and capped by maxFragmentBlocks.
- Left open: the S-95 criterion needs docs/guides/gorm.md, bun.md and ent.md (migration guides, plan momo-11) and a TestMigrationGuidesComplete; those pages do not exist in this repository, so it cannot be met here.
