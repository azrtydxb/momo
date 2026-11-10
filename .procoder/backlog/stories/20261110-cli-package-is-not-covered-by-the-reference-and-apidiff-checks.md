# The cli package is not covered by the reference and apidiff checks

Status: done 2026-11-10
Created: 2026-11-10
Epic: -
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each
Type: story

## Description

Sprint 7 moved the command line into the public package `cli` (`cli.Main`,
`cli.Run`), so a project can build its own `momo` binary with custom generator
plugins. README, `docs/ai-guide.md` and `docs/capabilities.md` document
`cli.Main`. But `cli` is not in `APIPackages` (internal/docscheck/apiscan.go),
so `docs/reference.md` has no section for it, the every-export-documented check
skips it, and the apidiff-docs gate does not see changes to it. Found by the
sprint-7 pre-PR integration review.

## Acceptance criteria

- [x] `cli` is in `APIPackages`, and `docs/reference.md` lists `cli.Main` and `cli.Run`, taken from the CI regenerated-docs artifact (the reference check passes on CI)
- [x] both exported names carry doc comments that say what a custom binary does with them

## Evidence

- `cli` is in `APIPackages` (internal/docscheck/apiscan.go); docs/reference.md has the `cli` section with `cli.Main` and `cli.Run`, taken from the regenerated-docs artifact of ci-agent run 42835763522 (sha c1e1da6). The reference check (`TestReferenceListsEveryIdentifier`) failed on that run before the file was updated and passes in ci-agent run 42839158680 (sha 5e86d37, all 9 jobs success). Both names carry doc comments that say what a custom binary does with them (cli/cmd.go). The commits were made through the procoder commit gate.
- Review round 2026-11-10: the `cli.Run` doc lists exit code 3 (database, lock or execution error), and docs/reference.md was regenerated from the regenerated-docs artifact of ci-agent run 42843740559.
