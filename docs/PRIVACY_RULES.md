# Interceptors: application-level statement rules

An interceptor is a function attached to one `*momo.DB` handle with `momo.WithInterceptor`. It runs before the typed statements of that handle, may append predicates to them, and may deny them by returning an error. **It is an application-level guard, not a privacy or security boundary: it filters only the primary table of the typed Select, Insert, Upsert, Update, SoftDelete and Delete builders, and every path listed under "What an interceptor does not cover" bypasses it.** Use it to remove accidents, and use database row-level security where isolation must hold when application code is wrong.

## What an interceptor sees

<!-- docscheck:fragment -->

```go
type Operation struct {
    Kind  string      // "select", "insert", "upsert", "update", "delete", "truncate"
    Table string
    Where []Predicate // starts empty; append to it
}

type Interceptor func(ctx context.Context, op *Operation) error
```

- Select, update and delete: the predicates in `Where` are ANDed to the statement and appear in the SQL that `SQL` returns and that the hooks see.
- Insert and upsert: the interceptor runs so it can deny the statement; predicates appended to them are ignored. It runs when the statement executes (`Exec`, `Rows`), never for `SQL`, which for an insert runs neither interceptors nor hooks.
- An `Upsert`, or an `Insert` with `OnConflict` and `DoUpdate`, has the kind `upsert`, not `insert`. ON CONFLICT DO UPDATE can overwrite an existing row, including a row of another tenant that shares the conflict key, and an appended predicate cannot stop it. A tenant rule must therefore deny the kind `upsert` or constrain it by other means (see the recipe). An `Insert` with `DoNothing` is an `insert`.
- A soft delete has the kind `delete`.
- `Truncate` has the kind `truncate`. It removes every row of the table, so a tenant-scoped handle should deny it; a predicate appended to it is ignored.
- `BulkUpdate` is an `update` and keeps the predicates. `Merge` is an `upsert` when it has a matched update and an `insert` otherwise; a predicate an interceptor appends makes it fail with `ErrValidation`, because it cannot be applied without changing which rows match. `Insert.Copy` is an `insert` (deny only).
- Interceptors run in registration order, before the typed model hooks (`Meta.Hooks`), so a denied statement never reaches a Before hook. The first error stops the chain and the statement; it is returned as a `momo.Error` of kind `ErrValidation` that still reaches the interceptor's own error with `errors.Is`. No SQL runs.
- The predicates an interceptor adds are not a caller filter: an update or delete with no `Where` of its own is still refused with `ErrUnfilteredMutation` unless `AllRows` confirms it. A version-checked model update whose row is hidden by an interceptor predicate returns `ErrNotFound`, not `ErrOptimisticLock`. The count that decides between the two is a select, so your select interceptors run for it too.

## The context

Interceptors receive the statement's context, so a tenant usually comes from `ctx`. `SQL`, which has no context argument, runs the interceptors with `context.Background`. An interceptor that reads the tenant from `ctx` sees none there: the SQL it returns shows no tenant predicate, and an interceptor that denies when the tenant is missing denies every `SQL` call. Inspect the statement a handle really runs through the hook events, or give the handle its tenant at construction (below) so the predicate does not depend on `ctx`.

## Recipe: a tenant handle

Build one handle per tenant (or per request) and give it the tenant predicate. The tenant comes from the handle's own configuration, so a statement cannot choose another one.

<!-- docscheck:fragment -->

```go
func tenantDB(drv momo.Driver, tenantID int64) *momo.DB {
    return momo.New(drv, momo.WithInterceptor(func(ctx context.Context, op *momo.Operation) error {
        if op.Table != "documents" {
            return nil
        }
        switch op.Kind {
        case "select", "update", "delete":
            op.Where = append(op.Where, DocTenantID.Eq(tenantID))
        case "upsert":
            return errors.New("upsert is not allowed on a tenant handle")
        }
        return nil
    }))
}
```

Inserts are not filtered by a predicate: the tenant column of a new row is the model's own value, so set it in a `BeforeInsert` model hook and do not trust the caller to set it. `BeforeInsert` runs only for `Model` and `Models` inserts: `Values`, `ValuesRows` and `FromSelect` inserts have no model and skip it, so a tenant handle that relies on the hook must also deny those forms, or deny inserts altogether and write through a trusted handle.

A read-only handle denies every write:

```go
package main

import (
	"context"
	"fmt"

	"github.com/azrtydxb/momo"
)

func readOnly(drv momo.Driver) *momo.DB {
	return momo.New(drv, momo.WithInterceptor(func(ctx context.Context, op *momo.Operation) error {
		if op.Kind != "select" {
			return fmt.Errorf("handle is read-only: %s on %s", op.Kind, op.Table)
		}
		return nil
	}))
}

func main() { _ = readOnly }
```

## Scope

An interceptor belongs to the handle it was registered on:

- A transaction begun with `Begin` or `Tx` from that handle runs the same interceptors.
- A handle created by another call to `momo.New`, even over the same driver, has none of them.
- Registering an interceptor on one handle never changes another.

## What an interceptor does not cover

Each of these bypasses every interceptor. Scope them explicitly or use row-level security.

- Relation loading: `Load`, `Preload` and `LoadCounts` query the related tables without interceptors.
- Joined relation tables: for a `Joined` relation only the primary table is filtered; the joined table is not.
- The association helpers (`Link`, `Unlink`, `Replace` and the like) and graph creation.
- Subqueries and other sources rendered inside a statement: subquery operands, `UPDATE ... FROM` sources, and the `FromSelect` source of an insert.
- Raw SQL of every kind: `momo.Exec`, `momo.MapRows`, `momo.Scalars`, `momo.SQL` fragments and `momo.RawSubquery`.
- Bulk loading with COPY: `Insert[T].Copy` runs the insert interceptors (deny only) and `BeforeInsert`, but a predicate an interceptor adds is ignored, and `momo.CopierOf(...).CopyFrom` skips them entirely.
- Anything run on the driver directly, and any other `*momo.DB` over the same driver.

For isolation that holds even when application code is wrong, use database row-level security with the transaction setup hook (`WithTxSetup`) and a policy on the table. Treat interceptors as the first layer and row-level security as the enforcing one.

## Related

- Sensitive columns are masked in hook events: bound values of columns tagged sensitive reach hooks as `momo.RedactedArg`.
- Typed model hooks (`Meta.Hooks`) change values on a model before a write and after a read; see the `HookSet` documentation for how to set them and for what a Before hook may leave behind when a statement then fails.
