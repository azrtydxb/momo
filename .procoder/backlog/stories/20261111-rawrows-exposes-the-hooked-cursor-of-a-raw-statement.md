# RawRows exposes the hooked cursor of a raw statement

Status: open
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

- [ ] a regression test pins the fix: red before the change, green after
- [ ] the hook event's op is `raw`
- [ ] the rows close when the context is canceled, and Err is ErrCanceled
- [ ] it works on `*DB`, on `*Tx` and on an adopted transaction
- [ ] `WithTxSetup` runs for a `*DB` with `WithImplicitTx`
- [ ] docs/upgrading.md, docs/ai-guide.md, docs/capabilities.md and docs/reference.md describe it

## Evidence

<!-- Filled at close time. -->
