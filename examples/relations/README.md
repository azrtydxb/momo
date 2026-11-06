# relations

## What this shows

Authors write documents; documents have comments and many tags.

- Declaring relations as tagged fields in `models/models.go`: `has_many`,
  `belongs_to`, `many2many` with a shared `join:` table. The generator makes a
  descriptor per relation (`document.Comments`), and `momo migrate diff` adds the
  foreign keys and the join table.
- `momo.Load` and `momo.LoadAll` load relations for a slice of parents in one
  statement per relation, not one per parent. `LoadWhere`, `LoadOrderBy` and
  `LoadPerParentLimit` shape the child statement (for example the two newest
  approved comments of each document); `Relation.Scope` carries them into
  `LoadAll`.
- `Select(...).Preload(...)` loads as part of the query; `Relation.Joined()` folds
  a belongs-to into the parent statement as a LEFT JOIN.
- `Has` and `HasNo` filter parents by their children with `EXISTS`, and
  `CountOrder` orders by the number of children. Predicates inside `Has` name the
  child as `momo_r`.
- `Link`, `Unlink`, `Replace` and `Count` edit a many-to-many link directly.
- `CreateGraph` inserts a parent and the children you list with `Include()` in one
  transaction and copies the generated keys. `FindOrCreate` is a get-or-insert on
  a unique key.

## Run

```sh
cd examples/relations
export DATABASE_URL=postgres://user:pass@localhost:5432/scratch
momo migrate apply
go run .
```

The test (`example_test.go`) runs the same steps in a scratch database and
checks the output. The full reference is [docs/relations.md](../../docs/relations.md).
