# SQLite: run and support the JSON operators and time expressions

Status: in-progress (criterion 5 waits on the generated capability matrix)
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

The JSON operators (`JSONText`, `JSONPath`, `JSONHasKey`, `JSONContains`) and the time expressions (`Extract`, `DateTrunc`, interval arithmetic, `Now`) already render SQL for SQLite (golden-tested in `expr_json_test.go` and `expr_time_test.go`), but the integration tests run through `testopen.Each`, which only runs dialects with a driver, and no SQLite driver ships, so nothing has ever executed on SQLite. `JSONContains` returns `ErrUnsupported` on SQLite, and `docs/capabilities.md` says so. Once the SQLite driver of the epics `momo-03-drivers-sqlite` and `momo-04-migrations-sqlite` is available, make these expressions work and be tested on a live SQLite: run `expr_json_integration_test.go` and `expr_time_integration_test.go` on SQLite with no skip; fix any difference between golden SQL and real behaviour (json_extract returns booleans as 1 or 0, `strftime` fractions and zone handling, interval month overflow normalisation); implement `JSONContains` on SQLite through `json_each` or keep it `ErrUnsupported` with a documented reason; and remove the 'SQLite: no containment' caveat or state it as a measured limit. These identifiers live in `momo/jsonop` and `momo/timeop` after the moves, so this story is ordered after those two stories. It does not add the driver: it depends on the SQLite driver stories and is blocked until the first of them (`momo-03-drivers-sqlite` story 182) is done; until then it stays open and untouched.

## Acceptance criteria

- [x] the JSON integration suite passes on SQLite for text extraction, path extraction and has-key, with the SQL NULL versus JSON null cases from the package doc asserted (integration job for SQLite, run id in evidence)
- [x] the time integration suite passes on SQLite for `Extract` on every `ExtractField`, `DateTrunc` on every `TruncUnit`, `AddInterval` and `SubInterval` including month overflow (Feb 28 plus one month), and `Now` in the stored timestamp text format
- [x] `JSONContains` on SQLite is either implemented and covered by an integration test, or returns `ErrUnsupported` with a test and the reason in the package doc
- [x] no `t.Skip` for SQLite remains in `expr_json_integration_test.go` or `expr_time_integration_test.go` (search output), apart from `AtTimeZone`, which is PostgreSQL only and documented as such
- [ ] `docs/capabilities.md` rows for JSON operators and date and time expressions are regenerated from the capability data (story generated-capability-matrix-and-type-table) and show SQLite as supported with the measured limits
- [ ] the story stays blocked, not closed, if no SQLite driver exists at sprint close; the close note says so

## Evidence

- Run 42497123151 (ci-agent, headSha 0586dbf, all jobs green) covers criteria 1 to 4. SQLite runs inside the integration (postgres) cell through `testopen.Each`; the pure-Go driver runs in `unit` (`make test-sqlitepure`) through `internal/sqlshared/expr_integration_test.go`.
- Criteria 1, 2: `expr_json_integration_test.go` (booleans read as `1`/`0`, SQL NULL versus JSON null, has-key) and `expr_time_integration_test.go` (all ExtractField and TruncUnit values, Feb 28 + 1 month, Jan 31 + 1 month = Mar 3 on SQLite, `Now` text format).
- Criterion 3: `JSONContains` stays `ErrUnsupported` on SQLite, asserted in both suites; the reason is in the package doc of `expr_json.go`.
- Criterion 4: the only remaining `t.Skip` is `AtTimeZone is PostgreSQL only` (`grep -n Skip expr_json_integration_test.go expr_time_integration_test.go`).
- Red: the new tests passed against the unchanged code (run 42494472904, integration green), because the behaviour was already correct. Mutation check instead: inverting SQLite `JSONHasKey` (`IS NOT NULL` to `IS NULL`) failed `unit` and `integration (postgres)` in run 42497130770.
- Criterion 5 is open: `docs/capabilities.md` rows were edited by hand, because the generated capability matrix story is not done.
- Criterion 6: not applicable, the SQLite drivers exist.
