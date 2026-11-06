# Conditional upsert: WHERE on ON CONFLICT target and DO UPDATE

Status: open
Created: 2026-11-06
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

GitHub issue #8. A developer taking a lease or lock with an expiry wants one
statement that inserts the row, or updates it only when the current lease has
expired or is already theirs:

```sql
INSERT INTO jobs_lock (name, holder, until) VALUES ($1, $2, $3)
ON CONFLICT (name) DO UPDATE SET holder = EXCLUDED.holder, until = EXCLUDED.until
WHERE jobs_lock.until <= $4 OR jobs_lock.holder = $5
```

The caller holds the lock when exactly one row was affected. In v0.1.0 this
needs raw SQL, because `ConflictQuery[T]` has only `DoNothing()` and
`DoUpdate(...)` and neither takes a condition. A developer using a partial
unique index also cannot name it as the conflict target
(`ON CONFLICT (cols) WHERE predicate`).

Done means both conditions are expressible with typed predicates:

```go
momo.Insert[JobsLock](q).Values(...).
    OnConflict(jobslock.Name).Where(targetPreds...). // optional: partial index target
    DoUpdate(jobslock.Holder.SetExcluded(), jobslock.Until.SetExcluded()).
    Where(momo.Or(jobslock.Until.Lte(now), jobslock.Holder.Eq(holder))).
    Exec(ctx)
```

Decided design (`.procoder/ask/decisions.md`, "Issue #8"): released as v0.2.0.

## Acceptance criteria

- [ ] `ConflictQuery[T].Where(preds ...)` renders
      `ON CONFLICT (cols) WHERE <preds>` before the action, for both
      `DoNothing` and `DoUpdate`; it needs a conflict target (no columns gives
      `ErrValidation`).
- [ ] `InsertQuery[T].Where(preds ...)` after `DoUpdate` (and on `Upsert[T]`,
      whose default update set it keeps) renders `DO UPDATE SET ... WHERE
    <preds>`, with columns qualified by the table name so they mean the
      existing row; calling it without a `DoUpdate` action gives
      `ErrValidation`.
- [ ] `Column[T].Excluded()` (and the vector column equivalent) is an
      expression usable in predicates, rendering `EXCLUDED.<col>`.
- [ ] Integration test (PostgreSQL): the lease case. Holder A takes the lock
      (1 row), holder B is refused while it is live (`RowsAffected` 0, row
      unchanged), A renews (1 row), B takes it after expiry (1 row).
- [ ] Integration test (PostgreSQL): a partial unique index used as the
      conflict target with `OnConflict(...).Where(...)`; without the `Where`
      PostgreSQL rejects the statement.
- [ ] Integration test: `Where` combined with `Returning` returns no row when
      the condition is false.
- [ ] Rendering is covered for SQLite; MySQL returns `ErrUnsupported` for
      either `Where`; unit tests for both.
- [ ] Multiple `Where` calls AND together, matching `SelectQuery.Where`.
- [ ] A mutation check: dropping either WHERE from the rendered SQL, or the
      table qualification, makes a test fail.
- [ ] Docs: README upsert section, `docs/ai-guide.md` (writes section and
      rules), `docs/capabilities.md` row, and godoc on the new methods.

## Evidence
