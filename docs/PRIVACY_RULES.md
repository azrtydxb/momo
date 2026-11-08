# Privacy rules with interceptors

An interceptor is a function attached to one `*momo.DB` handle with `momo.WithInterceptor`. It runs before every typed statement of that handle, may append predicates to it, and may deny it by returning an error. This page shows the tenant-scoping recipe and states what an interceptor does not do.

## What an interceptor sees

```go
type Operation struct {
    Kind  string      // "select", "insert", "update", "delete"
    Table string
    Where []Predicate // starts empty; append to it
}

type Interceptor func(ctx context.Context, op *Operation) error
```

- Select, update and delete: the predicates in `Where` are ANDed to the statement and appear in the SQL that `SQL` returns and that the hooks see.
- Insert: the interceptor runs so it can deny the statement; predicates appended to an insert are ignored.
- A soft delete reports the kind `delete`.
- Interceptors run in registration order. The first error stops the chain and the statement; the error is returned as a `momo.Error` of kind `ErrValidation` that still reaches the interceptor's own error with `errors.Is`. No SQL runs.
- The predicates an interceptor adds are not a caller filter: an update or delete with no `Where` of its own is still refused with `ErrUnfilteredMutation` unless `AllRows` confirms it.

## Recipe: a tenant handle

Build one handle per tenant (or per request) and give it the tenant predicate. The tenant comes from the handle's own configuration, so a statement cannot choose another one.

```go
func tenantDB(drv momo.Driver, tenantID int64) *momo.DB {
    return momo.New(drv, momo.WithInterceptor(func(ctx context.Context, op *momo.Operation) error {
        if op.Table != "documents" {
            return nil
        }
        switch op.Kind {
        case "select", "update", "delete":
            op.Where = append(op.Where, DocTenantID.Eq(tenantID))
        }
        return nil
    }))
}
```

A read-only handle denies every write:

```go
readOnly := momo.New(drv, momo.WithInterceptor(func(ctx context.Context, op *momo.Operation) error {
    if op.Kind != "select" {
        return fmt.Errorf("handle is read-only: %s on %s", op.Kind, op.Table)
    }
    return nil
}))
```

## Scope

An interceptor belongs to the handle it was registered on:

- A transaction begun with `Begin` or `Tx` from that handle runs the same interceptors.
- A handle created by another call to `momo.New`, even over the same driver, has none of them.
- Registering an interceptor on one handle never changes another.

## What an interceptor does not cover

An interceptor is an application-level guard. It is not a database security boundary.

- It does not see raw SQL (`momo.Exec`, `momo.Scalars`, `momo.SQL` fragments), so a caller can bypass it with raw SQL.
- It does not run for relation loading (`Load`, `Preload`, `Joined`), the association helpers, or a subquery rendered inside another statement. Scope those queries explicitly.
- Inserts are checked, but a tenant column on an insert is the model's own value; set it in a model hook.
- A caller that holds the driver, or another `*momo.DB` over it, is not restricted.

For isolation that holds even when application code is wrong, use database row-level security with the transaction setup hook (`WithTxSetup`) and a policy on the table. Treat interceptors as the first layer and row-level security as the enforcing one.

## Related

- Sensitive columns are masked in hook events: bound values of columns tagged sensitive reach hooks as `momo.RedactedArg`.
- Typed model hooks (`Meta.Hooks`) change values on a model before a write and after a read.
