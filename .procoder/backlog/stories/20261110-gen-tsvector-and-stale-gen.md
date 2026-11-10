# gen: tsvector maps by default, and generate ignores its own stale output (issues #61, #62)

Status: open
Created: 2026-11-10
Epic: momo-02-generator
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

The NovaCRM port (Prisma to momo v0.4.0) hit two generator problems:
- #61: `--from-db` refuses `tsvector` unless `momo.yaml` maps it, although `types.TSVector` exists.
- #62: `momo generate` type-checks its own previous `momo_gen.go`. After a field rename or removal, that stale file stops compiling, so generate fails on the very file it is about to rewrite, and the user must delete it by hand.

## Acceptance criteria

- [ ] `--from-db` maps `tsvector` to `types.TSVector` and a nullable one to `*types.TSVector` by default; an `introspect.types` rule still overrides it (test)
- [ ] `momo generate` leaves out files carrying momo's generated header when loading the package, so a renamed or removed field regenerates in one run (test reproducing the issue's steps: generate, rename the field, generate again succeeds)
- [ ] a hand-written file with a type error still fails generate with its error (test)
- [ ] docs (CHANGELOG via the commit, generator page) mention both
