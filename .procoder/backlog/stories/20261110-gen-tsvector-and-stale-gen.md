# gen: tsvector maps by default, and generate ignores its own stale output (issues #61, #62)

Status: done
Created: 2026-11-10
Epic: momo-02-generator
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

The NovaCRM port (Prisma to momo v0.4.0) hit two generator problems:
- #61: `--from-db` refuses `tsvector` unless `momo.yaml` maps it, although `types.TSVector` exists.
- #62: `momo generate` type-checks its own previous `momo_gen.go`. After a field rename or removal, that stale file stops compiling, so generate fails on the very file it is about to rewrite, and the user must delete it by hand.

## Acceptance criteria

- [x] `--from-db` maps `tsvector` to `types.TSVector` and a nullable one to `*types.TSVector` by default; an `introspect.types` rule still overrides it (test)
- [x] `momo generate` leaves out files carrying momo's generated header when loading the package, so a renamed or removed field regenerates in one run (test reproducing the issue's steps: generate, rename the field, generate again succeeds)
- [x] a hand-written file with a type error still fails generate with its error (test)
- [x] docs (CHANGELOG via the commit, generator page) mention both

## Evidence

- Red: run 42770229186 on 499bae2 failed TestLoadIgnoresErrorsInGeneratedFile, TestLoadKeepsErrorsInHandwrittenFile and TestGenerateFromDBMapping.
- Green: run 42773100267, headSha fd63685c804ccc3c2483e647bdcda74e0a56ca40, conclusion success.
- Criterion 3: gen/load_test.go TestLoadKeepsErrorsInHandwrittenFile.
- Criterion 4: docs/migrations.md section 7.
- Criterion 1: gen/fromdb/mapping_test.go TestGenerateFromDBMapping (tsvector default), TestGenerateFromDBTSVector (nullable gives *types.TSVector), TestGenerateFromDBTSVectorRuleOverrides (introspect.types rule wins). Green: run 42775652019, headSha ce17605cb93f7d2ef9a5d9364754993349ae06e2.
- Criterion 2: gen/stale_generate_test.go TestGenerateAfterFieldRename (generate, rename Model2 to EmbeddingModel, generate again; the new momo_gen.go has no Model2) and gen/load_test.go TestLoadIgnoresErrorsInGeneratedFile. Same run 42775652019.
