# crud-zero-null

## What this shows

Insert, select, update and delete on one table, with the question every ORM has
to answer: is this field a zero value, NULL, or "not mentioned"?

- `Insert.Model` writes every column of the struct, so `Priority: 0` is stored as 0. `Insert.Values` writes only the columns you name; the rest take the database
  default (`Priority` becomes 3).
- `Update.Set(col.Set(v))` writes exactly `v`, including `0`, `false` and `""`.
  The columns you do not name are left alone.
- NULL needs two things: a pointer field (`Note *string`), which makes the
  generated column a `NullColumn`, and `SetNull()`, which only a `NullColumn` has.
  `task.Priority.SetNull()` does not compile. Find NULLs with `IsNull()`.
- `Update.Model(&row)` replaces the whole row from a struct. With a `version`
  column a stale copy fails with `ErrOptimisticLock` instead of overwriting.
- A `soft_delete` column makes `SoftDelete` stamp the row, hides it from every
  `Select`, and lets `WithDeleted` and `OnlyDeleted` find it. A plain `Delete`
  on such a model is refused; `ForceDelete` removes the row for good.

## Run

```sh
cd examples/crud-zero-null
export DATABASE_URL=postgres://user:pass@localhost:5432/scratch
momo migrate apply
go run .
```

The test (`example_test.go`) runs the same steps in a scratch database and
checks the output.
