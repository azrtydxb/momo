# RawRows exposes the hooked cursor of a raw statement

Status: done 2026-11-11
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #104. Reproduction: a query whose column count depends on its
input (the Kuvryn Orbit OQL page query scans 1+2n columns) cannot use `Query`
or `QueryRows`, which need a known row type, and `MapRows` buffers every row
into maps. The only `Scan(dest...)` cursor was `db.Driver().Query`.

Observed: that cursor bypasses momo's hooks, `WithTxSetup` tenant setup, error
classification and argument redaction. Expected: a cursor that goes through the
same path as `Query`.

Root cause: the hooked, tx-set-up, classified path (`execer.query`) was reachable
only through the typed readers; no exported function returned its `Rows`. Fix:
`momo.RawRows` compiles the statement like `Query`, runs it through
`execer.query` with op `raw`, and returns it as `ColumnRows` (a `Rows` with
`Columns`). A canceled context ends the iteration and closes the cursor.

## Acceptance criteria

- [x] a regression test pins the fix: red before the change, green after
- [x] the hook event's op is `raw`
- [x] the rows close when the context is canceled, and Err is ErrCanceled
- [x] it works on `*DB`, on `*Tx` and on an adopted transaction
- [x] `WithTxSetup` runs for a `*DB` with `WithImplicitTx`
- [x] docs/upgrading.md, docs/ai-guide.md, docs/capabilities.md and docs/reference.md describe it

## Evidence

- Red: ci-agent run 42977719417 (test-only commit 2fe4963) failed to compile: `raw_rows_integration_test.go:68:37: undefined: momo.ColumnRows`, `:102:22` and `:158:21: undefined: momo.RawRows`.
- Green: ci-agent run 42985136779 on SHA 99810f4d2494b2b4876098c8d6b91cae48ff8b55 concluded success (all jobs). It runs TestRawRowsPaths (op raw, `*DB`, `*Tx`, adopted tx, WithTxSetup via WithImplicitTx), TestRawRowsClosesOnCancel and TestRawRowsErrors on PostgreSQL and SQLite.
- Docs changed: docs/upgrading.md (0.8.0 section), docs/ai-guide.md, docs/capabilities.md, docs/momovet.md, docs/reference.md (from the regenerated-docs artifact of run 42978245443).
