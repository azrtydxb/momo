# Full-text search: tsvector type and FTS expressions (#30)

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: -

## Description

As an application author porting a knowledge base (NovaCRM) to Momo, I need a tsvector column type and typed full-text expressions so that search, ranking and headlines no longer need raw SQL, and so that text rank can be combined with vector distance. GitHub issue #30.

## Acceptance criteria

- [x] `types.TSVector` (and `types.TSQuery`) map to `tsvector`/`tsquery` on PostgreSQL, are listed in docs/TYPE_MAPPING.md and round-trip on a live server
- [x] A tsvector column with a GIN index and a trigger-maintained value (function and trigger) diffs, applies and inspects back with no drift
- [x] Builders for `@@`, `to_tsvector`, `websearch_to_tsquery`, `plainto_tsquery`, `to_tsquery` with a regconfig argument, `ts_rank`, `ts_rank_cd` and `ts_headline` with an options string work in Where, OrderBy and Columns
- [x] The builders return ErrUnsupported on SQLite and MySQL, and ErrValidation on an empty configuration or bad normalization
- [x] Golden unit tests pin the SQL and arguments
- [x] A documented hybrid ranking example combining CosineDistance and ts_rank_cd exists and is exercised by an integration test
- [x] CI is green on the final commit, integration jobs included

## Evidence

CI run 42409393949 (ci-agent, head aeebc3b, merged with origin/sprint-5 126646b): unit, lint, fuzz-smoke and integration (postgres) all green.

- TSVector/TSQuery mapping: TestTypeMappingDoc and TestTypeMapping rows for types.TSVector and types.TSQuery passed in unit and integration.
- Trigger-maintained column: TestTSVectorMigrationRoundTrip (migrate/postgres) passed: apply, trigger fills the column, no diff, empty second diff.
- Builders in Where, OrderBy, Columns: TestFullTextSearch passed (ranking order, negation, phrase, to_tsvector, ts_rank, ts_headline options, scanning into types.TSVector).
- ErrUnsupported and ErrValidation: TestFullTextUnsupportedAndInvalid passed.
- Goldens: TestFullTextGolden passed.
- Hybrid example: docs/fulltext.md; TestFullTextHybridRanking passed.
- CI green: run above.

Review fixes (head 367b72f, CI run 42411922211, ci-agent: unit, lint, fuzz-smoke, integration postgres all green):

- Sensitive-column masking: TestFullTextHooksSeeMaskedArgs (configuration, query and options show as RedactedArg for a sensitive operand, stay visible for a plain one) and TestDistanceArgMaskedForSensitiveColumn for DistanceExpr.
- Grouping limit documented in docs/fulltext.md and pinned by TestFullTextBindsAreNotGroupable.
- Codec edge cases: TestTSVectorAndQueryForms and the TestTypeMapping rows with quote, backslash, position and weight.
