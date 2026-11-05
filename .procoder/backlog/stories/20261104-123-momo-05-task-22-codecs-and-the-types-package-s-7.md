# momo-05 Task 22: Codecs and the types package (S-73)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 22: Codecs and the types package (S-73) (`.procoder/plans/momo-05-writes-queries.md:1803`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-73. Files: `types/codec.go`, `types/json.go`, `types/date.go`, `types/uuid.go`, `types/hstore.go`, `types/codec_test.go`.

## Acceptance criteria

- [x] `TestCodecsRoundTrip` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-73] `TestCodecsRoundTrip` round-trips each type on every dialect; fails if a codec's SQL type and encoding disagree.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- FAIL (before implementation): `go test ./types/` -> `undefined: Codec`, `undefined: JSON`, `undefined: Date`, `undefined: TimeOfDay` (build failed).
- PASS: `go test -race -count=1 ./types/` -> `ok`; full `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...` and `make tidy-check` green. No dependency added.
- Extra unit tests: Scan over string, []byte, nil (error) and wrong type (error) for all five codecs; UUID rejects bad length, dashes and characters; JSON null vs empty preserved; Date/TimeOfDay range and format checks; Hstore PostgreSQL text and JSON forms.
- Mutation check: making `text()` accept nil dropped the Scan(nil) error; `TestCodecsSourceTypesAndErrors` failed ("Hstore Scan(nil) must error"); restored, PASS.
- Gate: `procoder check` 0 blocking findings. Commit fe1aa8e.
- Criterion 2 (S-73 round trip on every dialect) left unchecked: only the codec-level Value/Scan round trip is proven. A live PostgreSQL/SQLite round trip needs `testopen.Each`/pgxdriver, blocked on momo-03 Task 1 and Task 4.
- Deviation: `TimeOfDay.Value` returns the text 'HH:MM:SS.nnnnnnnnn' (valid for both SQL types) rather than the int64 in the plan's unused `want` field; `Scan` also accepts int64 nanos. `Hstore.PostgresText()` gives the hstore form; `Value()` gives JSON.

