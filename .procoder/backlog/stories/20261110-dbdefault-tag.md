# Insert.Model keeps database defaults: the dbdefault tag (issue #69)

Status: done
Created: 2026-11-10
Epic: momo-05-writes-queries
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

`Insert.Model(&m)` writes every writable column, so `DEFAULT gen_random_uuid()` or `DEFAULT now()` is overwritten by the Go zero value unless every call site uses `Omit`. Most of Kuvryn Orbit's 253 tables default ids and timestamps in the database. The ask is in issue #69.

## Acceptance criteria

- [x] a `dbdefault` tag: when the field holds its zero value the column is left out of the INSERT, and its value is read back through RETURNING into the model (unit test pins SQL; PostgreSQL integration test with `gen_random_uuid()` and `now()`)
- [x] a non-zero value is written as given; bulk `Insert.Models` with mixed zero/non-zero rows is handled correctly (split or refused with a clear error) (test)
- [x] MySQL and SQLite behave correctly (RETURNING where supported, otherwise documented re-read or refusal) (tests); the generator and `--from-db` emit the tag for columns with a default where appropriate
- [x] docs describe it

## Evidence

CI: ci-agent run 42789114971, headSha 78817fec5b1b9a8b07953a6df2a897df6661c7b3, all 9 jobs green (includes the review fixes below).

- Criterion 1 (zero field left out, read back via RETURNING): `insert_dbdefault_test.go` TestDBDefaultZeroFieldsAreLeftOutAndReadBack pins the SQL; `insert_dbdefault_integration_test.go` TestDBDefaultGenRandomUUIDAndNow (PostgreSQL gen_random_uuid() and now()).
- Criterion 2 (non-zero written as given; mixed Models): TestDBDefaultSetValueIsWrittenAsGiven, TestDBDefaultModelsMixedRowsWriteDEFAULT (per-row DEFAULT), TestDBDefaultChunksShareOneColumnList; SQLite, Unnest and Copy refuse a mix with ErrUnsupported (TestDBDefaultMixedRowsRefusedWhereDEFAULTIsNotAvailable).
- Criterion 3 (MySQL/SQLite): TestDBDefaultLive runs on PostgreSQL, SQLite, MySQL 8.4/9.7 and MariaDB. MariaDB and SQLite read back with RETURNING; plain MySQL refuses a zero dbdefault column with ErrUnsupported (TestDBDefaultMySQLRefusesWhatItCannotReadBack). Generator: gen/dbdefault_test.go, gen/tags/tags_test.go; --from-db: gen/fromdb TestDBDefaultFits.
- Criterion 4 (docs): docs/ai-guide.md "Database defaults", llms.txt, docs/reference.md.

Design choice to confirm: plain MySQL refuses instead of re-reading, since it has no RETURNING.

Review fixes (same run):

- `--from-db` emits dbdefault only for PostgreSQL: gen/fromdb TestDBDefaultFitsOnlyOnPostgres; a nextval default on a non-identity column gets the tag (TestDBDefaultFits). docs/upgrading.md has the regeneration note; the commit carries a BREAKING CHANGE footer.
- The generator refuses dbdefault on a nullable field: gen/dbdefault_test.go TestDBDefaultNullableDiagnostic (an error-level diagnostic, the generator has no warning level).
- docs/ai-guide.md states that false, 0, empty string and a zero time cannot be written through Model.
- ON CONFLICT DO NOTHING that skips rows fails with ErrUnsupported and rolls back: TestDBDefaultDoNothingSkippedRowsFail.
- Copy refuses a zero dbdefault column: TestDBDefaultCopyRefusesZeroColumn.
- The dbdefault plan is computed once per statement (insert_dbdefault.go) and the write-back helper is shared with write_idgen.go; the bench gate shows no extra allocations.
