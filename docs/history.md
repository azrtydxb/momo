# Audit history tables

A model can keep an audit history: a `<table>_history` table that records every
version of its rows, who changed them and when. A database trigger writes it,
so a change made by another service, a script or `psql` is recorded as well as
one made through momo. The history table, the trigger function and the trigger
are ordinary schema objects that the generator declares, so `momo migrate diff`,
`baseline`, `drift` and `generate --from-db` handle them like any other table,
function and trigger. Audit history is a PostgreSQL feature; the other dialects
refuse it.

## Declare it

Call `Config.History` in the model's `MomoConfig`:

```go
package main

import "github.com/azrtydxb/momo"

// Note keeps an audit history.
type Note struct {
	momo.Model `momo:"table:notes"`
	ID         int64 `momo:"pk,identity"`
	Title      string
	Body       *string
}

// MomoConfig declares the history. The generator reads it without running it.
func (n *Note) MomoConfig(c *momo.Config) {
	c.History()
}

func main() {}
```

Run `momo generate`, then `momo migrate diff`: the migration creates the history
table, the trigger function and the trigger. The model needs a primary key, and
its table must be in the current schema (a `schema:` tag is refused). Call
`Config.History` once.

## What the generator declares

For a model whose table is `notes`:

| Object                  | What it is                                                                                 |
| ----------------------- | ------------------------------------------------------------------------------------------ |
| `notes_history`         | every column of `notes`, then the four history columns below                               |
| `notes_history_fn`      | a PL/pgSQL trigger function that inserts one history row                                   |
| `notes_history_trg`     | `AFTER INSERT OR UPDATE OR DELETE ... FOR EACH ROW` on `notes`, calling `notes_history_fn` |
| `notes_history_row_idx` | an index on the primary key columns of `notes`, `history_changed_at` and `history_id`      |

The history columns:

| Column               | Type          | Holds                                                                          |
| -------------------- | ------------- | ------------------------------------------------------------------------------ |
| `history_id`         | `bigint`      | an identity primary key; it orders the versions of a row                       |
| `history_operation`  | `text`        | `INSERT`, `UPDATE` or `DELETE`                                                 |
| `history_changed_at` | `timestamptz` | the wall-clock time of the change (`clock_timestamp()`)                        |
| `history_changed_by` | `text`        | the actor the transaction set with `momo.SetActor`, or `NULL` when it set none |

The history columns carry a `history_` prefix so that they never collide with
model columns such as `operation` or `changed_by`; a model column named like one
of the four is refused.

The trigger records the new row after an `INSERT` or an `UPDATE`, and the removed
row on a `DELETE`. The history is therefore the complete log of every state of
every row, and the state at a time is the latest version at or before it. Two
cases are handled on purpose:

- An `UPDATE` that changes nothing records nothing. The trigger compares the old
  and the new row as `jsonb`, so a column type without an equality operator
  (`json`, `xml`, `point`) does not break it.
- An `UPDATE` that changes the primary key records the old row as a `DELETE`
  before the new row, so the timeline of the old key ends and the timeline of the
  new key starts with the new row. `HistoryAsOf` of the old key then reports that
  the row is gone.

A soft delete is an ordinary `UPDATE` that sets the deleted marker (`deleted_at`),
so the history records it as an `UPDATE` and `HistoryAsOf` returns the row as
existing, with the marker set. Only a hard delete or a purge, a `DELETE`
statement, records a `DELETE`.

The history table repeats each model column by name, type, nullability, default,
enum and collation. It leaves out the identity and generated parts (the trigger
writes the value, the database does not generate it), the comment, and every
constraint and index of the model: a unique constraint copied to the history
would reject the second version of a row. Because a history table keeps the
default and NOT NULL of its model column, a NOT NULL column added with a default
works on both tables.

## Who changed it

Set the actor inside the transaction before the change:

```sql
SELECT set_config('momo.actor', 'alice', true);
```

`momo.SetActor` on the transaction does exactly that: a transaction-local setting
(`set_config` with `is_local = true`), which ends with the transaction, never
reaches a pooled connection and is safe behind PgBouncer in transaction mode.
To record an actor for every transaction, call it from a `momo.WithTxSetup`
function. A change made in a transaction that set no actor records a `NULL`
actor, including a change made outside momo.

## Read the versions

Two generic functions read the history of a model that declared `Config.History`.
Both take the primary key values in key order, on a `*momo.DB` or a `*momo.Tx`:

- `momo.HistoryOf` returns every `momo.HistoryEntry` of a row in the order of the
  changes, which is the history `ID` (a sequence, so it is the real order even
  when two changes carry the same time). An entry holds the row (`Row`), the
  `Operation`, `ChangedAt`, `ChangedBy` (nil when no actor was set) and the
  history `ID`.
- `momo.HistoryAsOf` returns the version in force at a `time.Time`: the latest
  entry at or before it. It returns an error matching `momo.ErrNotFound` when the
  row did not exist then, because its first version is later or because its
  latest version is a `DELETE`.

The time of a version is the moment its trigger fired (`clock_timestamp()`), not
the moment its transaction committed. A transaction that commits later than a
second one that started after it can therefore be recorded with an earlier time.
`HistoryAsOf` answers by that change time.

Soft-delete filters and interceptors of the model do not apply to the versions,
because the history is an audit log. Tenant scope does: pass a predicate after
the key values, the way a `Where` takes one, and the read adds it on the history
table's column of the same name:

```text
ctx = momo.WithTenant(ctx, int64(5))
versions, err := momo.HistoryOf[Note](ctx, db, int64(1), momo.TenantFilter(ctx, NoteTenantID))
```

A context without a tenant of the column's type fails with `momo.ErrValidation`
before any statement runs, exactly as `TenantFilter` does in a `Select`. Row-level
security of the history table is yours to configure; the trigger writes the rows
with the privileges of the writing role. Both functions return
`momo.ErrUnsupported` on SQLite, MySQL and MariaDB.

## Schema changes

The history objects are rebuilt from the model's columns every time the generator
runs, so a model change reaches the history table in the same migration:

- A column added to the model is added to `notes_history`, and the trigger
  function is replaced with a version that writes it.
- A column dropped from the model stays on `notes_history`, relaxed to nullable
  (`ALTER COLUMN ... DROP NOT NULL`) when it was NOT NULL. The function is
  replaced with a version that no longer writes it, so new rows hold `NULL` there
  and the recorded values survive. The migration's snapshot records the kept
  column, so `migrate diff` finds nothing more to do and `drift` reports no
  difference. To remove it for good, write a migration by hand
  (`ALTER TABLE notes_history DROP COLUMN title`). Until then, a
  `generate --from-db` run models such a history as an ordinary table, function
  and trigger, because the table no longer equals what `Config.History` builds.
- A column whose type or nullability changes is altered on both tables.
- Turning `Config.History` off drops the trigger, the function and the history
  table. Dropping a table is destructive, so `migrate diff` refuses it without
  `--allow-destructive`.

## Renames

A rename of the model follows on its history, so no recorded version is lost:

- `--rename-column notes.title=headline` renames the column on `notes` and on
  `notes_history` in the same plan, and the trigger function is replaced with one
  that writes the new name.
- `--rename-table notes=memos` renames `notes` and `notes_history`. The trigger
  function, the trigger and the row index are rebuilt under the new names
  (`memos_history_fn`, `memos_history_trg`, `memos_history_row_idx`); the
  function body names the new history table. The rebuild of the index reads the
  history table once.

Without the flag, a renamed column or table is a drop and an add, and the
history table keeps the old column as described above.

## generate --from-db

`momo generate --from-db` writes `c.History()` for a database that has an audit
history, and gives the history table no model of its own. It recognises one by
this rule: the tables `<t>` and `<t>_history`, the function `<t>_history_fn` and
the trigger `<t>_history_trg` on `<t>` all exist, and the history table, function
and trigger equal what `c.History()` builds from `<t>` as inspected. The snapshot
keeps the three objects. Anything that differs (an extra column, another trigger
body, another timing) is not recognised: the tables, function and trigger are then
modeled as the ordinary objects they are, so a database is never declared twice.
The report notes each history it recognised.

## Limits

- PostgreSQL only. The generator refuses `Config.History` for a SQLite, MySQL or
  MariaDB dialect with a diagnostic that names the model. SQLite and MySQL have no
  transaction-local setting for the actor, so a trigger there could not record
  `changed_by`.
- `TRUNCATE` is not recorded, because a row trigger does not fire for it.
- Every write to the model's table writes a history row too. Nothing prunes the
  history; delete old rows with your own job.
- A table name over 47 bytes is refused, because the longest derived name,
  `<table>_history_row_idx`, would pass PostgreSQL's 63 byte identifier limit.
