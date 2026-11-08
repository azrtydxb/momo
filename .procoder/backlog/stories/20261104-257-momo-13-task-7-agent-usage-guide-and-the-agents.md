# momo-13 Task 7: Agent usage guide and the AGENTS.md snippet

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-13-docs-release
Sprint: -

## Description

Implements plan momo-13-docs-release Task 7: Agent usage guide and the AGENTS.md snippet (`.procoder/plans/momo-13-docs-release.md:818`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-23. Files: `docs/agents.md`, `internal/docscheck/agents_test.go`. Contributes to S-23; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestAgentGuideSnippet` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- red run 42454971851 (work branch suffixed -red, sha 222fa09, tests with a NavPages stub): FAIL TestAgentGuideSnippet
- green: docs workflow run 42456103295 (sha 9b46480d8ec2f3944100975376d197a31720171c, success) and the unit and integration jobs of ci-agent run 42456103287 on the same sha, where TestAgentGuideSnippet passes
- docs/agents.md carries the AGENTS.md snippet between markers; each rule is taken from section 12 of ai-guide.md.
