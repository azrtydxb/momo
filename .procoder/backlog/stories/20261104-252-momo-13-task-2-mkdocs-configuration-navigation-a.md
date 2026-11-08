# momo-13 Task 2: MkDocs configuration, navigation and version selector

Status: open
Created: 2026-11-04
Epic: momo-13-docs-release
Sprint: -

## Description

Implements plan momo-13-docs-release Task 2: MkDocs configuration, navigation and version selector (`.procoder/plans/momo-13-docs-release.md:116`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-23. Files: `mkdocs.yml`, `internal/docscheck/nav.go`, `internal/docscheck/nav_test.go`. Contributes to S-23; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestMkdocsNavPagesExist` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestEveryDocPageInNav` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- red run 42454971851 (work branch suffixed -red, sha 222fa09, tests with a NavPages stub): FAIL TestMkdocsNavPagesExist and TestEveryDocPageInNav
- green: docs workflow run 42456103295 (sha 9b46480d8ec2f3944100975376d197a31720171c, success) and the unit and integration jobs of ci-agent run 42456103287 on the same sha, where TestMkdocsNavPagesExist and TestEveryDocPageInNav passes
- mkdocs.yml has a nav over the real pages (README, getting-started, ai-guide, agents, TYPE_MAPPING, projections, relations, vectors, fulltext, migrations, testing, capabilities, supported-versions, ARCHITECTURE, contributing); mkdocs build --strict is green in the docs workflow; the mike version provider is configured. The capability matrix is the existing capabilities.md.
