# Conditional upsert: WHERE on ON CONFLICT target and DO UPDATE

Status: done 2026-11-06
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

- [x] `ConflictQuery[T].Where(preds ...)` renders
      `ON CONFLICT (cols) WHERE <preds>` before the action, for both
      `DoNothing` and `DoUpdate`; it needs a conflict target (no columns gives
      `ErrValidation`).
- [x] `InsertQuery[T].Where(preds ...)` after `DoUpdate` (and on `Upsert[T]`,
      whose default update set it keeps) renders `DO UPDATE SET ... WHERE
<preds>`, with columns qualified by the table name so they mean the
      existing row; calling it without a `DoUpdate` action gives
      `ErrValidation`.
- [x] `Column[T].Excluded()` (and the vector column equivalent) is an
      expression usable in predicates, rendering `EXCLUDED.<col>`.
- [x] Integration test (PostgreSQL): the lease case. Holder A takes the lock
      (1 row), holder B is refused while it is live (`RowsAffected` 0, row
      unchanged), A renews (1 row), B takes it after expiry (1 row).
- [x] Integration test (PostgreSQL): a partial unique index used as the
      conflict target with `OnConflict(...).Where(...)`; without the `Where`
      PostgreSQL rejects the statement.
- [x] Integration test: `Where` combined with `Returning` returns no row when
      the condition is false.
- [x] Rendering is covered for SQLite; MySQL returns `ErrUnsupported` for
      either `Where`; unit tests for both.
- [x] Multiple `Where` calls AND together, matching `SelectQuery.Where`.
- [x] A mutation check: dropping either WHERE from the rendered SQL, or the
      table qualification, makes a test fail.
- [x] Docs: README upsert section, `docs/ai-guide.md` (writes section and
      rules), `docs/capabilities.md` row, and godoc on the new methods.

## Evidence

- Target WHERE: TestConflictWhereGoldenSQL (target cases) renders `ON CONFLICT ("name") WHERE "live" = $n` for DoNothing and DoUpdate; TestConflictWhereValidation covers no target columns -> ErrValidation.
- Action WHERE: TestConflictWhereGoldenSQL and TestConflictWhereQualification show `DO UPDATE SET ... WHERE ("jobs_lock"."until" <= $n OR ...)` table-qualified; TestUpsertWhere covers Upsert[T]; Where without DO UPDATE -> ErrValidation (TestConflictWhereValidation).
- Excluded(): TestExcludedRendersEXCLUDEDColumn (Column, VectorColumn, NullVectorColumn); permission columns via TestPermissionTypesMirrorColumn; TestConflictWhereExcludedComparison runs LtExpr(Excluded()) on PostgreSQL.
- Lease: TestConflictWhereLease, RowsAffected 1/0/1/1 with the row unchanged after the refusal (PostgreSQL 16 and 17, CI run 42283129446).
- Partial index: TestConflictWherePartialIndexTarget updates the existing row in place and asserts SQLSTATE 42P10 without the target Where.
- Returning: TestConflictWhereReturning returns no row when the condition is false.
- SQLite rendering and MySQL ErrUnsupported: TestConflictWhereGoldenSQL (sqlite cases), TestConflictWhereMySQLUnsupported. SQLite partial-index literal matching is a follow-up story (20261106-conditional-upsert-follow-ups-sqlite-target-literals-excluded-in-target).
- AND of several Where calls: golden case "and of two wheres".
- Mutation check: dropping the action WHERE, the target WHERE, or the table qualification each failed Lease/Returning/UpsertWhere/golden/Qualification tests (implementer's manual run).
- Docs: README "Conditional upsert", docs/ai-guide.md upserts section and rules, docs/capabilities.md upsert row; godoc on every new method. Shipped in v0.2.0 (PR #9, ffae95a).
