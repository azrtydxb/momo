# The cli package is not covered by the reference and apidiff checks

Status: open
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

- [ ] `cli` is in `APIPackages`, and `docs/reference.md` lists `cli.Main` and `cli.Run`, taken from the CI regenerated-docs artifact (the reference check passes on CI)
- [ ] both exported names carry doc comments that say what a custom binary does with them

## Evidence
