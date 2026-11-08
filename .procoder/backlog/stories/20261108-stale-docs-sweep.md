# Stale-docs sweep of docs/*.md and README against the real code

Status: open
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: -

## Description

Compare every claim in `README.md`, `llms.txt` and `docs/*.md` with the code on this branch and fix what disagrees, so the later guards start from a clean baseline. Concrete lines found while planning (see section 8 of the research note): `docs/migrations.md` line 986 names `v0.1.0-dev` outside an upgrade context; `docs/vectors.md` lines 20 to 34 present the sqlite-vec `v0.1.6` plan and hybrid-search options `TextCandidates`, `VectorCandidates`, `RRFK` as pending although no such identifier exists in non-test code; `docs/README.md` says the adopt example has 'no model generation from a database in v0.3.0' while `momo generate --from-db` exists (README line 433, `gen/fromdb`); `docs/README.md` points readers to the private planning file `.procoder/specs/momo.md`; `docs/ARCHITECTURE.md` is a plan contract with `[momo-NN]` tags, a 'Planned, not implemented' list naming `momotest`, `momolog`, `otelmomo`, `importer/{gorm,bun,ent}`, `gen/{graphql,grpc,openapi}`, `Engine.Lint`, a roadmap and ownership sections, and a claim that only `cmd/momo` is an 'other package' while `gen/apiquery` and `gen/viz` exist; `docs/supported-versions.md` lists pgx `v5.11.0` against `go.mod` `v5.12.0`; `docs/capabilities.md` says MySQL JSON operators are 'not usable yet' beside rows that give MySQL equivalents; `docs/TYPE_MAPPING.md` lines 73 to 76 carry a status line that will go stale. The 'SQLite and MySQL are not usable' sentence appears by hand in eight places; it is accurate today (`driver/` holds only `pgxdriver`) and is replaced by the generated block in story generated-capability-matrix-and-type-table, not edited here. Split the planning content of `docs/ARCHITECTURE.md` out to `.procoder/` (where plans already live) and leave a reader-facing architecture page that describes packages that exist. Docs-site navigation and page structure belong to momo-13 stories 252 and 254 and are not changed here.

## Acceptance criteria

- [ ] each stale line listed in the description is fixed or deliberately kept with a reason in the PR; the PR lists the line, the evidence (file and line of the code that disagrees) and the fix
- [ ] a second pass reads every section of `README.md`, `docs/ai-guide.md`, `docs/relations.md`, `docs/projections.md`, `docs/fulltext.md`, `docs/vectors.md`, `docs/migrations.md`, `docs/capabilities.md` and `docs/supported-versions.md` against the code and appends any further stale lines found to the PR with the same evidence format (no section is skipped; the PR names each page as 'checked')
- [ ] `docs/ARCHITECTURE.md` no longer contains `[momo-` tags, 'Planned, not implemented', 'Delivery order' or 'Ownership decisions'; a test in `internal/repocheck` fails if `docs/*.md` contain a `[momo-NN]` tag or a link to `.procoder/`
- [ ] `docs/README.md` lists every page in `docs/` (a test compares the index links to the directory listing) and no longer names `.procoder/specs/momo.md`
- [ ] every example program named in `docs/README.md` exists under `examples/` (existing `TestExamplesAreWired` plus a docs-index check)
- [ ] no change to nav structure, mkdocs files or the llms generation (momo-13 stories 252, 254 and 256 own them)

## Evidence
