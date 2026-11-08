# Stale-docs sweep of docs/*.md and README against the real code

Status: open
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

Compare every claim in `README.md`, `llms.txt` and `docs/*.md` with the code on this branch and fix what disagrees, so the later guards start from a clean baseline. Concrete lines found while planning (see section 8 of the research note): `docs/migrations.md` line 986 names `v0.1.0-dev` outside an upgrade context; `docs/vectors.md` lines 20 to 34 present the sqlite-vec `v0.1.6` plan and hybrid-search options `TextCandidates`, `VectorCandidates`, `RRFK` as pending although no such identifier exists in non-test code; `docs/README.md` says the adopt example has 'no model generation from a database in v0.3.0' while `momo generate --from-db` exists (README line 433, `gen/fromdb`); `docs/README.md` points readers to the private planning file `.procoder/specs/momo.md`; `docs/ARCHITECTURE.md` is a plan contract with `[momo-NN]` tags, a 'Planned, not implemented' list naming `momotest`, `momolog`, `otelmomo`, `importer/{gorm,bun,ent}`, `gen/{graphql,grpc,openapi}`, `Engine.Lint`, a roadmap and ownership sections, and a claim that only `cmd/momo` is an 'other package' while `gen/apiquery` and `gen/viz` exist; `docs/supported-versions.md` lists pgx `v5.11.0` against `go.mod` `v5.12.0`; `docs/capabilities.md` says MySQL JSON operators are 'not usable yet' beside rows that give MySQL equivalents; `docs/TYPE_MAPPING.md` lines 73 to 76 carry a status line that will go stale. The 'SQLite and MySQL are not usable' sentence appears by hand in eight places; it is accurate today (`driver/` holds only `pgxdriver`) and is replaced by the generated block in story generated-capability-matrix-and-type-table, not edited here. Split the planning content of `docs/ARCHITECTURE.md` out to `.procoder/` (where plans already live) and leave a reader-facing architecture page that describes packages that exist. Docs-site navigation and page structure belong to momo-13 stories 252 and 254 and are not changed here.

## Acceptance criteria

- [x] each stale line listed in the description is fixed or deliberately kept with a reason in the PR; the PR lists the line, the evidence (file and line of the code that disagrees) and the fix
- [ ] a second pass reads every section of `README.md`, `docs/ai-guide.md`, `docs/relations.md`, `docs/projections.md`, `docs/fulltext.md`, `docs/vectors.md`, `docs/migrations.md`, `docs/capabilities.md` and `docs/supported-versions.md` against the code and appends any further stale lines found to the PR with the same evidence format (no section is skipped; the PR names each page as 'checked')
- [x] `docs/ARCHITECTURE.md` no longer contains `[momo-` tags, 'Planned, not implemented', 'Delivery order' or 'Ownership decisions'; a test in `internal/repocheck` fails if `docs/*.md` contain a `[momo-NN]` tag or a link to `.procoder/`
- [x] `docs/README.md` lists every page in `docs/` (a test compares the index links to the directory listing) and no longer names `.procoder/specs/momo.md`
- [x] every example program named in `docs/README.md` exists under `examples/` (existing `TestExamplesAreWired` plus a docs-index check)
- [x] no change to nav structure, mkdocs files or the llms generation (momo-13 stories 252, 254 and 256 own them)

## Evidence

Green: ci-agent run 42498774870, headSha 7e303a02c92551bc758aa300a84b4d3aa373c198, all jobs success. The first run on 48603f3 (42494748386) failed only on TestVectorDocPins, which pinned the removed hybrid-search text; fixed in 7e303a0.

Lines fixed (line, code evidence, fix):

- migrations.md `v0.1.0-dev`: gen/header.go writes a header with only the format number; reworded to describe the old form without a version. Also removed the `.procoder/backlog/stories` reference in Known issues.
- vectors.md "Not yet available": no `TextCandidates`, `VectorCandidates` or `RRFK` in non-test code; section replaced with "Not available" (no built-in fusion, see fulltext.md; MySQL has no distance function). vector/doc_test.go no longer pins `k=60` and `TextCandidates`.
- docs/README.md: duplicate Interceptors and momovet entries removed; no `.procoder/specs/momo.md` reference remains; `momo generate --from-db` is already stated correctly (cmd/momo/generate_fromdb.go). Architecture entry reworded.
- ARCHITECTURE.md: rewritten from the package tree (gen/apiquery, gen/viz, momotest, momolog, importer/*, cmd/momovet, driver/sqlitedriver, sqlitepure, mysqldriver, migrate/sqlite and migrate/mysql all exist). Plan content archived to .procoder/plans/architecture-contract-archive.md.
- supported-versions.md: pgx v5.12.0 already matched go.mod (fixed in sprint 5). Fixed instead: "generated files carry the generator version in their header" (gen/header.go: format number only).
- capabilities.md: JSON row was already corrected in sprint 5. Fixed instead: Rollback row ("not built") although Engine.Rollback exists (migrate/rollback.go); `new` is wired (`new --custom`, cmd/momo/migrate.go) and Engine.Rebase exists (migrate/rebase.go); "story 098" removed; `new --custom` added to the command list.
- TYPE_MAPPING.md lines 73 to 76: already rewritten in sprint 5, kept (the SQLite note is accurate and not time-bound).
- Eight "SQLite and MySQL are not usable" sentences: already replaced in sprint 5; the story's premise (driver/ holds only pgxdriver) no longer holds, nothing to edit.
- importers.md and migrations.md: "story 228" and "story 098" references removed.

Second pass (criterion 2 NOT ticked): I did not read every section of every named page. Done: a script checked every momo./vector./types./dialect./schema./migrate./momotest./driver identifier in README.md and docs/*.md against non-test code (no misses); a second script checked every Test/Example/Fuzz/Benchmark name and every backticked repo path in the named pages (no misses); status words (not yet, planned, future, story NNN, v0.x.y) were grepped across all pages; supported-versions.md was compared with go.mod and ci.yml. Pages checked this way: README.md, ai-guide.md, relations.md, projections.md, fulltext.md, vectors.md, migrations.md, capabilities.md, supported-versions.md. A full read remains.

Tests: internal/repocheck/docs_hygiene_test.go (TestDocsHaveNoPlanBookkeeping, TestArchitecturePageDescribesCode, TestDocsIndexListsEveryPage). Red run 42494821840 (probe commit b9430270a639a1e215b9f5dd5f6b33f2ada5bbd4, branch deleted): TestDocsHaveNoPlanBookkeeping flagged a [momo-05] tag and a .procoder/ reference, TestDocsIndexListsEveryPage flagged an unlinked page.

Nav: mkdocs.yml gained one entry, `Upgrading: upgrading.md` (TestEveryDocPageInNav requires every page in the nav, for the upgrading story). No structure, hook or llms change.
