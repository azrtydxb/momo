# Relations

This guide covers how Momo models relations between tables: declaring them,
loading them, filtering by them, writing them, and observing them. Every
snippet uses the real API. The models are a small document store: `Document`
belongs to an `Author`, has many `Comment`s, and is tagged with many `Tag`s.

Relation operations are methods and functions on the generated `Relation[P, C]`
descriptor, where `P` is the parent model and `C` the child model. A descriptor
is immutable and safe for concurrent use.

- [Declaring relations](#1-declaring-relations)
- [Eager loading](#2-eager-loading)
- [Preload and single-query loading](#3-preload-and-single-query-loading)
- [Pipelined loading](#4-pipelined-loading)
- [Relation predicates and traversal](#5-relation-predicates-and-traversal)
- [Association helpers](#6-association-helpers)
- [Graph writes](#7-graph-writes)
- [Observability](#8-observability)
- [Errors and limitations](#9-errors-and-limitations)

## 1. Declaring relations

A relation is a field on the parent model. Its tag names the kind, and the Go
type of the field decides the shape:

| Tag          | Field type | Foreign key lives on |
| ------------ | ---------- | -------------------- |
| `belongs_to` | `*Target`  | the parent           |
| `has_one`    | `*Target`  | the child            |
| `has_many`   | `[]Target` | the child            |
| `many2many`  | `[]Target` | a join table         |

<!-- docscheck:fragment -->

```go
type Author struct {
	momo.Model `momo:"table:authors"`
	ID         int64 `momo:"pk,identity"`
	Name       string

	Documents []Document `momo:"has_many,fk:AuthorID"`
}

type Document struct {
	momo.Model `momo:"table:documents"`
	ID         int64 `momo:"pk,identity"`
	AuthorID   *int64
	Title      string

	Author   *Author   `momo:"belongs_to,fk:AuthorID"`
	Comments []Comment `momo:"has_many,fk:DocumentID"`
	Tags     []Tag     `momo:"many2many,join:taggings"`
}

type Comment struct {
	momo.Model `momo:"table:comments"`
	ID         int64 `momo:"pk,identity"`
	DocumentID int64
	Body       string
	Approved   bool
}

type Tag struct {
	momo.Model `momo:"table:tags"`
	ID         int64 `momo:"pk,identity"`
	Name       string
}
```

The generated code lives in one package per model (`author`, `document`,
`comment`, `tag`), imported next to the `models` package.

### Generating relation code

Run `momo generate` in the module that holds your models (or add
`//go:generate momo generate` to a source file and run `go generate ./...`).
It reads `momo.yaml` and writes the descriptors into the model packages. Commit
the generated files, and use `momo generate --check` in CI to catch stale ones.
The README walks through the setup in
[Configure and generate](../README.md#2-configure-and-generate). The join
tables and foreign keys come from `momo migrate diff` and `momo migrate apply`,
see [Create and apply the migration](../README.md#3-create-and-apply-the-migration).

Tag options for relations:

- `fk:<Field>` is required for `belongs_to`, `has_one` and `has_many`. For
  `belongs_to` it names the foreign-key field on the parent (the model that
  declares the relation). For `has_one` and `has_many` it names the field on
  the child.
- `join:<table>` is required for `many2many`. The join columns are named after
  each model and its primary key column, for example `document_id` and
  `tag_id`. Rules:
  - Set `join:` (same table) on every many2many side you declare. The
    reverse side is optional, but a reverse side declared without `join:`
    makes the generator report a diagnostic.
  - A many-to-many to the model's own type is not supported.
  - Both models need a primary key.
  - The join table has exactly two key columns, named `<model>_<pkcol>`. Extra
    join-table columns are not modelled by this guide; use an explicit join
    model such as a `Tagging` struct for those.
- `polytype:<column>` together with `polyvalue:<value>` makes a `has_one` or
  `has_many` polymorphic: the child row belongs to the parent only when its
  type column equals the value.
- `fk_name:<name>` names the foreign key constraint of a `belongs_to` instead of
  the derived `<table>_<column>_fkey` (for example `_fkey1` on a partitioned
  table). `MomoConfig` can do the same with `c.FKName(&m.AuthorID, "name")`; the
  field is the key field. Declaring it both ways is an error. A migration diff
  and `baseline` compare the constraint by this name.
- `on_delete`, `on_update` and `no_constraint` affect only the foreign key
  emitted for `belongs_to`. `has_one` and `has_many` create no constraint, and
  many-to-many join tables always get two foreign keys with
  `ON DELETE CASCADE`.

`momo generate` reads these tags and writes one descriptor per relation field
into the model's package. The descriptor holds the relation name and kind, the
parent and child tables, the key columns, `Through` for the join table of a
many-to-many relation, `Discriminator` for a polymorphic one, and the typed
functions that read and write keys and the field itself. You refer to a
relation as `<model package>.<Field>`:

<!-- docscheck:fragment -->

```go
document.Author   // belongs-to: momo.Relation[models.Document, models.Author]
document.Comments // has-many
document.Tags     // many-to-many
```

The generator reports a diagnostic, and writes nothing for that package, when:

- a relation has more than one kind, or the field type does not match the kind;
- the target is not a Momo model;
- `fk` is missing, or names a field that does not exist;
- `many2many` has no `join:`, or a declared reverse side has no `join:`;
- `polytype` is given without `polyvalue` or the other way round, or
  `polytype` is used on a relation that is not `has_one` or `has_many`;
- the `polytype` column is not a field of the target.

A `belongs_to` is keyed by its own `fk` field against the target's primary
key, so the referencing model may have any primary key, including a composite
one that contains the foreign key column. For example `exams` with primary key
`(license_id, exam_id)` can declare `License *License` with
`momo:"belongs_to,fk:LicenseID"`, which emits a foreign key
from `license_id` to the `licenses` table's `license_id` column and matches a live
database that already has that constraint. The target of a `belongs_to` must
have a single-column primary key: a foreign key to a composite target key is
refused with a diagnostic that names the target's key columns. A `has_one` or
`has_many` needs a single-column primary key on the parent, and the child may
have a composite key. `many2many` needs a single-column primary key on both
models. Other composite-key relations work with hand-written descriptors but
cannot be expressed with tags.

Generated relation field values (`Document.Comments`) are only ever written by
Momo when you load them or list them for `CreateGraph`. A plain `Insert` or
`Update` never touches a relation field.

## 2. Eager loading

`momo.Load` loads one relation for a slice of parents and assigns the result
onto each element. `momo.LoadAll` loads several.

<!-- docscheck:fragment -->

```go
docs, err := momo.Select[models.Document](db).Limit(20).All(ctx)
if err != nil {
	return err
}
if err := momo.Load(ctx, db, docs, document.Comments); err != nil {
	return err
}
if err := momo.LoadAll(ctx, db, docs, document.Author, document.Tags); err != nil {
	return err
}
```

The cost is one statement per relation (per bind-limit chunk, see
[Chunking](#chunking)), not one per parent. Parent order and length never
change. Parents with a NULL key get an empty association: a nil pointer or a
nil slice.

Behavior to rely on:

- Associations are cleared before anything is assigned. A parent that matches
  no children ends up empty rather than stale, and a load that fails part-way
  leaves empty associations, never partial ones.
- Soft-deleted children are excluded by default when the child model has a
  soft-delete column, as in `Select`.
- A to-one child shared by several parents is copied shallowly for each
  parent. Pointers inside the child, including relations loaded with `Then`,
  are shared among those copies.
- A has-one relation that matches more than one child for a parent fails with
  `ErrTooManyRows`.
- With no parents, `LoadAll` returns nil and runs nothing.

### Scoping a load

`Relation.Scope` takes load options for the child statement:

| Option                  | Effect                                                                                                      |
| ----------------------- | ----------------------------------------------------------------------------------------------------------- |
| `LoadWhere[C]`          | extra predicates on the child table                                                                         |
| `LoadOrderBy[C]`        | child order; the relation key and child primary key are always appended as tie-breakers, so order is stable |
| `LoadPerParentLimit[C]` | keep at most `n` children per parent (top-N)                                                                |
| `LoadWithDeleted[C]`    | include soft-deleted children                                                                               |
| `LoadOnlyDeleted[C]`    | only soft-deleted children                                                                                  |

`Load` accepts the same options directly after the relation. Use `Scope` when
you pass the relation to `LoadAll` or `Preload`:

<!-- docscheck:fragment -->

```go
err := momo.Load(ctx, db, docs, document.Comments,
	momo.LoadWhere[models.Comment](comment.Approved.Eq(true)),
	momo.LoadOrderBy[models.Comment](comment.ID.Desc()),
	momo.LoadPerParentLimit[models.Comment](3),
)

err = momo.LoadAll(ctx, db, docs,
	document.Author,
	document.Comments.Scope(momo.LoadPerParentLimit[models.Comment](3)),
)
```

`LoadWhere` filters before the per-parent limit is applied, so "the three
newest approved comments" is what the example above loads.

`LoadPerParentLimit` uses a window function, so the load stays one statement
per chunk. It needs PostgreSQL, MySQL 8, MariaDB 10.2 or SQLite 3.25 or later.
It applies to has-many and many-to-many relations only. On belongs-to and
has-one it fails with `ErrUnsupported`, and a negative `n` fails with
`ErrValidation`, both before any statement runs. Zero means unlimited. For a
many-to-many relation whose child has a primary key, duplicated join-table rows
share a rank and cannot use up the limit. Without a child primary key, each
duplicated row takes a rank and can count against the limit.

`LoadWithDeleted` and `LoadOnlyDeleted` replace each other: the last one given
wins. They have no effect on a child model without a soft-delete column.

### Nested loads

`Relation.Then` loads relations on the children, in one pass over all children
of the level above:

<!-- docscheck:fragment -->

```go
err := momo.LoadAll(ctx, db, authors,
	author.Documents.Then(document.Comments),
)
```

Nested loads run over the children that were kept, so combined with a
per-parent limit they only load for the survivors.

`Scope` and `Then` combine on one relation. Both return a `RelationLoader`,
which has `Scope` and `Then` methods itself, so a filtered, ordered relation
with nested loads is one loader ("approved comments, newest first, each with
its author"):

<!-- docscheck:fragment -->

```go
err := momo.LoadAll(ctx, db, documents,
	document.Comments.Scope(
		momo.LoadWhere[models.Comment](comment.Approved.Eq(true)),
		momo.LoadOrderBy[models.Comment](comment.ID.Desc()),
	).Then(comment.Author),
)
```

- API note: `Relation.Scope` and `Relation.Then` used to return `Loader[P]`
  and now return `RelationLoader[P, C]`, which implements `Loader[P]`. Passing
  them to `Load`, `LoadAll`, `Preload` or `Then`, or assigning them to a
  `Loader[P]`, compiles unchanged. Only code that declares a variable with the
  inferred type and then assigns a different loader to it needs a
  `var l momo.Loader[P]` declaration.
- `Scope(...).Then(...)` and `Then(...).Scope(...)` are the same loader. Calling
  either more than once accumulates: predicates AND together, order items
  append, a later `LoadPerParentLimit` replaces an earlier one, nested loaders
  append. A `RelationLoader` is a value, so a base loader can be shared.
- The options apply to this relation's children only. Nested loaders carry
  their own options, so `author.Documents.Then(document.Comments.Scope(...))`
  scopes the comments, not the documents.
- The nested level runs once over exactly the children that survived the
  options, so the statement count stays one per bind-limit chunk at the top
  and one per level below it. A child excluded by `LoadWhere` is never a
  nested parent.
- With no options and no nested loaders a `RelationLoader` behaves like the
  plain relation. An error or cancelled context at the nested level is
  returned from `LoadAll`.

### Chunking

Parent keys are sent as bind parameters, and every dialect caps how many a
statement may carry. Momo splits the parent keys into chunks that fit under the
dialect's limit and runs one statement per chunk, so a load of any size works.
`WithMaxParams(n)` lowers the limit for a `DB`. Use it for tests that want to
exercise chunking with small data, or for databases configured below the
dialect's default:

<!-- docscheck:fragment -->

```go
db := momo.New(drv, momo.WithMaxParams(100))
```

If a scope alone binds so many arguments that no key fits under the limit, the
load fails with `ErrValidation` before any statement runs.

### Counting children for many parents

`Relation.Count` counts one parent per statement. To show a count next to each
row of a list, `LoadCounts` counts all parents at once: one grouped
`SELECT key, COUNT(*) ... GROUP BY key` per bind-limit chunk.

<!-- docscheck:fragment -->

```go
counts, err := momo.LoadCounts(ctx, db, docs, document.Comments,
	momo.LoadWhere[models.Comment](comment.Approved.Eq(true)))
// counts[i] is the number of approved comments of docs[i]
```

- The result has one entry per parent, in parent order. A parent with no
  children or a NULL key counts zero; parents sharing a key share the count.
- Has-one, has-many (including polymorphic) and many-to-many relations are
  supported, with composite keys. Belongs-to fails with `ErrUnsupported`, as
  `Count` does.
- The load options that narrow the children apply: `LoadWhere`, and the
  soft-delete default with `LoadWithDeleted` and `LoadOnlyDeleted`.
  `LoadOrderBy` is ignored; `LoadPerParentLimit` fails with `ErrValidation`.
- Many-to-many counts join-table rows, like `Count`. The far table is joined
  when it has a soft-delete column and `LoadWithDeleted` is not set, or when a
  `LoadWhere` filter applies. Otherwise nothing is joined, so join rows whose
  far row no longer exists (dangling rows) are counted too.
- Parent keys are chunked under the dialect bind limit (see Chunking), and a
  relation hook sees one event per call.

To order or filter parents by their child count inside a query, use
`CountOrder` (section 5).

## 3. Preload and single-query loading

`SelectQuery.Preload` attaches loaders to a query. `All`, `One` and `First` run
them after the rows are read, with the same statements `LoadAll` would use.
Preload returns a copy, so a base query can be reused, and a query with no
result rows runs no relation statement. Inside a transaction the loads run in
that transaction.

<!-- docscheck:fragment -->

```go
docs, err := momo.Select[models.Document](db).
	Preload(document.Author, document.Comments.Scope(
		momo.LoadPerParentLimit[models.Comment](3))).
	Limit(20).
	All(ctx)
```

`Each` and `Iter` stream rows without collecting them, so they cannot batch
relation loads. With preloads they return `ErrUnsupported`.

### Joined

`Relation.Joined()` folds a belongs-to relation into the parent statement as a
LEFT JOIN, so it costs no extra round trip. Pass it to `Preload`. It does not
work with `Load`, `LoadAll`, `Scope` or `Then`. A matched joined child with
NULL in a column its Go field cannot hold fails with `ErrValidation` and an
error naming the child column (`column=...`).

<!-- docscheck:fragment -->

```go
docs, err := momo.Select[models.Document](db).
	Preload(document.Author.Joined(), document.Comments).
	Limit(20).
	All(ctx)
```

Here `Author` arrives with the document rows, and `Comments` is still loaded
with a second statement. Restrictions:

- To-one belongs-to only, and only when the relation's child key is the child's
  primary key or a single unique column. That is what guarantees the join
  cannot multiply parent rows, so `Limit`, `Offset`, `OrderBy` and `Where` of
  the parent query keep their meaning. Has-one, has-many and many-to-many fail
  with `ErrUnsupported` when the query runs.
- Not with `GroupBy`, `Having` or row locking on the same query (`FOR UPDATE`
  cannot lock the nullable side of an outer join), and not with `Each` or
  `Iter`. All of these return `ErrUnsupported`.
- If the query sets its own select list with `Columns`, it must have one
  expression per mapped field, or the query fails with `ErrValidation`.
- The parent's columns are qualified by the parent table in the select list, so
  a raw, unqualified column name in the parent's `Where` can become ambiguous
  with the joined table's columns. Qualify raw column names.
- Semantics match `Load` for belongs-to: a NULL foreign key, or a key that
  matches no child, leaves the field nil (never a zero struct), soft-deleted
  children are excluded by a condition in the `ON` clause so the parent row
  survives, and each parent gets its own copy of a shared child.
- A joined load runs no statement of its own, so it emits no `RelationEvent`.
  The parent statement is reported to the query hook as usual.

## 4. Pipelined loading

`LoadAll` (and so `Load` and `Preload`) works in levels: the top level holds
the relations you listed, and each `Then` adds a level. On a connection whose
driver implements `Batcher`, which the pgx driver does on the pool, on a
transaction and on a pinned connection, all statements of a level, including
every bind-limit chunk, are sent in one round trip. Other drivers (the
`database/sql` adapters, SQLite, MySQL) load sequentially. The result is the
same either way. There is nothing to enable and no per-call opt-out.

Two differences to know about:

- Memory. A pipelined level buffers all of its child rows before assigning any,
  so peak memory is roughly twice that of sequential loading, which holds one
  relation's rows at a time. Large child sets with a per-parent limit or a
  `LoadWhere` stay cheap; an unscoped load of a huge has-many does not.
- Hook event order. Sequential loading interleaves each relation's events with
  its statements. A pipelined level reports `BeforeRelation` for every
  relation, then `BeforeQuery` for every statement and one round trip, then
  `AfterQuery` per statement as result sets are read, then nested loads,
  assignment and `AfterRelation` per relation. The batch runs under the context
  of the first statement's hooks, so cancelling that context stops the whole
  batch, and a statement's `AfterQuery` duration includes its wait for the
  batch.

A batch is all or nothing: if one statement fails, every relation's
`AfterRelation` reports the error and the parents keep the empty associations.
If the driver rejects the batch before sending anything (`ErrUnsupported` with
`Op` `"batch"`), Momo falls back to sequential loading for that level.

## 5. Relation predicates and traversal

Relations also filter and order queries without loading anything.

<!-- docscheck:fragment -->

```go
// Documents with at least one approved comment.
withApproved := momo.Select[models.Document](db).
	Where(document.Comments.Has(comment.Approved.As("momo_r").Eq(true)))

// Documents without any comments.
quiet := momo.Select[models.Document](db).Where(document.Comments.HasNo())

// Documents ordered by number of comments, most-commented first.
busy := momo.Select[models.Document](db).
	OrderBy(document.Comments.CountOrder(momo.DirDesc), document.ID.Asc())
```

- `Has(scope...)` renders `EXISTS (...)` over the child table, correlated with
  the parent through the relation's key columns. It is always a subquery, never
  a join, so it cannot duplicate parent rows.
- `HasNo(scope...)` renders `NOT EXISTS`.
- `HasFrom(parentAlias, scope...)` is `Has` with an explicit parent alias.
  Self-referencing relations need it: when the outer query aliases the parent
  table, pass that alias so the subquery correlates with the right instance.
- `CountOrder(dir)` orders parents by their number of children, using a
  correlated `COUNT(*)`. `dir` is `momo.DirAsc` or `momo.DirDesc`.
- Soft-deleted children do not count. For many-to-many relations the join table
  gets no soft-delete filter: only the child table's marker applies.

Scope predicates must name the child through the alias `momo_r`, because the
subquery owns that alias. Build them with `As` on the generated column:
`comment.Approved.As("momo_r").Eq(true)`. A predicate that uses the column
without the alias refers to the outer query's table.

`Relation.From(parents)` goes the other way: it returns a `SelectQuery[C]` of
the children whose key is in the key set of the given parent query. The parent
query is embedded as a subquery, so no parent row is fetched:

<!-- docscheck:fragment -->

```go
comments := document.Comments.From(
	momo.Select[models.Document](db).Where(document.ID.Eq(int64(1))))
rows, err := comments.All(ctx)
```

`From` does not support many-to-many relations. The returned query fails with
`ErrValidation` when it runs.

## 6. Association helpers

`Link`, `Unlink`, `Replace` and `Count` write and count the link itself, with
no model in between:

<!-- docscheck:fragment -->

```go
doc := models.Document{ID: 1}
tags := []*models.Tag{{ID: 1}, {ID: 2}}

err := document.Tags.Link(ctx, db, &doc, tags...)    // add pairs
err = document.Tags.Unlink(ctx, db, &doc, tags[0])   // remove one pair
err = document.Tags.Replace(ctx, db, &doc, tags[1])  // make this the whole set
n, err := document.Tags.Count(ctx, db, &doc)         // how many are linked
```

They only need the keys of the parent and children. They never modify the
values you pass in.

**They write SQL directly.** Model hooks, field hooks and an `UpdatedAt`
column are not touched. Use the model's own update path when those must run.

Statement contract, per relation kind (one statement per chunk; a small call is
one statement):

| Call      | Has-one, has-many                                                                                                       | Many-to-many                                                                                                                                                                             |
| --------- | ----------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Link`    | `UPDATE child SET fk = ? WHERE pk IN (...)`; a polymorphic relation also sets the discriminator                         | `INSERT` into the join table that skips existing pairs (`ON CONFLICT DO NOTHING`; MySQL `ON DUPLICATE KEY UPDATE` with a no-op); relies on a primary or unique key over the join columns |
| `Unlink`  | `UPDATE child SET fk = NULL WHERE fk = ? AND pk IN (...)`; a polymorphic relation also adds the discriminator predicate | `DELETE FROM join WHERE parent cols = ? AND (child cols) IN (...)`                                                                                                                       |
| `Replace` | diff, then unlink the removed and link the added                                                                        | same                                                                                                                                                                                     |
| `Count`   | `SELECT COUNT(*)` over the children                                                                                     | counts join rows, joined to the far table when it has a soft-delete column                                                                                                               |

What this means in practice:

- **Join tables need a unique key.** Many-to-many `Link` relies on a primary or
  unique key over the join columns to detect an existing pair. Join tables
  generated from `many2many` tags already have a composite primary key over the
  two columns, plus two foreign keys with `ON DELETE CASCADE`. A hand-written
  join table needs a primary or unique key too, or `Link` inserts duplicate
  rows.
- **Has-one and has-many `Unlink` and `Replace` need a nullable foreign key**,
  because they clear it. On a NOT NULL column they fail with
  `ErrNotNullViolation`, with a message saying so. Delete the children instead
  if they cannot exist without a parent. For the same reason they cannot be
  used when the child's foreign key column is part of its primary key (for
  example `exams` keyed by `license_id, exam_id`): the database rejects the
  NULL. Delete those children instead.
- **`Replace` is a transaction.** On a `*DB` it opens one, on a `*Tx` it runs in
  a savepoint. With no children it clears the set in one statement. Otherwise
  it reads the current child keys, diffs them in Go against the requested set,
  unlinks only the removed keys and links only the added ones, in chunks, so
  kept children are not rewritten and a set of any size works.
- **`Replace` locks the parent row** (`SELECT 1 ... FOR UPDATE`) on PostgreSQL
  and MySQL, so concurrent `Replace` calls on the same parent run one after the
  other. SQLite serialises writers and skips the lock. `Replace` needs a
  transaction, so on a `*DB` pinned with `OnConn` it fails with
  `ErrUnsupported` (`Op` `"begin"`); run it on the pool or inside a `*Tx`.
  `Link`, `Unlink` and `Count` accept a pinned handle. The lock is a row lock:
  a parent that does not exist locks nothing. On a `*Tx` the lock is held until
  the caller's transaction ends. `Link` and `Unlink` take no parent lock and
  can interleave with a concurrent `Replace`.
- **Belongs-to is not linkable.** Every helper returns `ErrUnsupported`. Set the
  foreign key on the parent and update it.
- **`Link`, `Unlink` and `Replace` return `ErrValidation`** when the parent key
  is NULL, or when a child key (the child primary key, or the join key for
  many-to-many) is NULL. `Link` and `Unlink`
  with no children are no-ops that issue no statement. `Count` of a parent with
  a NULL key is zero.
- **Chunks and transactions.** When `Link` or `Unlink` needs more than one chunk
  and the Querier is a `*DB`, the chunks run in one transaction, so the call is
  atomic. On a `*Tx` they run in your transaction without a savepoint. If a
  later chunk fails, the earlier chunks stay applied until you roll back.
  Chunk size follows the dialect bind limit, see `WithMaxParams`.

## 7. Graph writes

Graph writes are explicit. Nothing saves a relation you did not name.

### CreateGraph

`CreateGraph` inserts a parent and the children of the relations you list, in
one transaction. List relations with `Relation.Include()`:

<!-- docscheck:fragment -->

```go
doc := models.Document{
	Title:    "Hello",
	Author:   &models.Author{Name: "Ada"},
	Comments: []models.Comment{{Body: "first"}, {Body: "second"}},
}
err := momo.CreateGraph(ctx, db, &doc,
	document.Author.Include(),
	document.Comments.Include(),
)
```

- Listed belongs-to children are inserted first and their key is copied into
  the parent. Listed has-one and has-many children are inserted after the
  parent, with the parent's key (read back with `RETURNING` when it is
  generated) copied into each child, and the discriminator set for a
  polymorphic relation. Many-to-many children are inserted and then linked
  through the join table. Relations are processed in the order listed.
- The graph is one level deep. Children's own relations are not followed, so a
  self-referencing relation or an in-memory cycle cannot recurse. A model that
  appears as its own child fails with `ErrValidation`.
- Every row goes through the ordinary insert path, so field hooks, automatic
  timestamps, Go-side defaults and validation run as for a plain `Insert`.
- Many-to-many children are always inserted, not looked up. A child that is
  already stored is inserted again and fails with `ErrUniqueViolation`, which
  undoes the whole graph. Link existing rows with `Link` instead.
- Has-many and many-to-many children are inserted with the chunked bulk insert:
  one multi-row statement per bind-limit chunk, so 1000 children cost one
  statement, not 1000. A has-one or belongs-to child is a single insert.
- The bulk insert's `RETURNING` rows are matched to the children in insertion
  order. If the database returns fewer or more rows than were inserted (a
  `BEFORE INSERT` trigger that skips a row, say), the graph fails with
  `ErrValidation` and is undone instead of leaving children without keys.
- A dialect without `RETURNING` (MySQL) returns `ErrUnsupported` before a
  transaction is opened.
- On a `*DB` the graph runs in a new transaction. On a `*Tx` it runs in a
  savepoint, so a failure undoes the whole graph and leaves your transaction
  usable. A dialect without savepoints returns `ErrUnsupported` there. A `*DB`
  pinned with `OnConn` also returns `ErrUnsupported` (`Op` `"begin"`), because
  the graph needs a transaction; run it on the pool or inside a `*Tx`.

### FindOrCreate

<!-- docscheck:fragment -->

```go
a := models.Author{Name: "Ada"}
created, err := momo.FindOrCreate(ctx, db, []momo.ColumnRef{author.Name}, &a)
```

`FindOrCreate` inserts the model unless a row with the same key columns exists.
Afterwards the model holds the stored row, and `created` says whether this call
inserted it.

- **The key columns need a unique constraint.** The statement is
  `INSERT ... ON CONFLICT (key) DO NOTHING`, so the constraint decides the
  winner and concurrent callers never create duplicates; the losers re-select
  the winner's row. Without a constraint on exactly those columns the database
  rejects the statement. Dialects without `ON CONFLICT` or `RETURNING` return
  `ErrUnsupported` before any statement runs.
- When the row exists, all mapped columns of the model are overwritten by the
  stored values, and its relation fields are reset.
- A soft-deleted row is not "found". `FindOrCreate` never returns or restores a
  row you deleted, and it does not create a second row with the same key: the
  unique constraint still holds the soft-deleted row's key, so the insert
  conflicts and the call fails with `ErrUniqueViolation` (the deleted row is
  left untouched, and your model is not overwritten). This holds when the
  unique constraint covers soft-deleted rows, the case `FindOrCreate`
  supports. A partial unique index (`WHERE deleted_at IS NULL`) would let a new
  row in, but `FindOrCreate` cannot target it: its `ON CONFLICT (key)` has no
  index predicate, so the database rejects the statement. For keys that
  soft-deleted rows release, use `Insert` with `OnConflict(key).Where(...)`
  and `DoNothing`, then re-select. To reuse the old row, restore it first; to
  create a new one, use a different key. A model without a
  soft-delete column is unaffected. This is the least surprising choice:
  returning the deleted row would hand back data the application removed, and
  restoring it would undo a delete silently.
- Isolation. Under READ COMMITTED the conflicting insert waits for a concurrent
  transaction and the re-select then sees its row. Under REPEATABLE READ or
  SERIALIZABLE, the re-select can miss a row committed after the snapshot and
  return `ErrNotFound`. Retry the transaction in that case.
- An empty key, a key column the model does not map, or a nil model returns
  `ErrValidation`.

### UpsertByPK

<!-- docscheck:fragment -->

```go
err := momo.UpsertByPK(ctx, db, &a)
```

`UpsertByPK` inserts the model, or updates the row with the same primary key:
`INSERT ... ON CONFLICT (pk) DO UPDATE SET col = EXCLUDED.col`. It is the
one-call form of `Upsert(q).Model(m)`. Generated columns are read back into the
model. The primary key, version, soft-delete and created-at columns are never
overwritten. Use the `Upsert` builder directly when the conflict target is a
different unique key, or when you want to choose which columns update.

## 8. Observability

A hook registered with `WithHook` that also implements `RelationHook` receives
one event per relation load:

<!-- docscheck:fragment -->

```go
type relLogger struct{ momo.Hook } // embed a Hook for the query events

func (relLogger) BeforeRelation(ctx context.Context, e *momo.RelationEvent) context.Context {
	return ctx
}

func (relLogger) AfterRelation(ctx context.Context, e *momo.RelationEvent) {
	log.Printf("%s.%s %s: %d parents, %d children, %d queries, %s, err=%v",
		e.Parent, e.Name, e.Kind, e.Parents, e.Children, e.Queries, e.Duration, e.Err)
}

db := momo.New(drv, momo.WithHook(relLogger{queryHook}))
```

`RelationEvent` fields:

| Field            | Meaning                                                                  |
| ---------------- | ------------------------------------------------------------------------ |
| `Parent`, `Name` | parent table and relation name                                           |
| `Kind`           | belongs-to, has-one, has-many or many-to-many                            |
| `Parents`        | number of parents the load ran for                                       |
| `Queries`        | child statements compiled for the load; a failed load may have run fewer |
| `Children`       | children loaded; set only when the statements succeeded, zero on error   |
| `Duration`       | filled before `AfterRelation`                                            |
| `Err`            | the load's error                                                         |

Events fire only for loads that execute statements. They do not fire when there
are no parents, when the load fails while being prepared (validation or
unsupported options), or for joined loads. `BeforeRelation` may return a derived
context, which later hooks and the load's statements see. For event order under
pipelining, see [Pipelined loading](#4-pipelined-loading).

## 9. Errors and limitations

Errors are `*momo.Error` values. Test them with `errors.Is(err, momo.ErrX)`.
The `Op` field names the operation.

| Operation                            | Error                 | When                                                                                                                             |
| ------------------------------------ | --------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `Load`, `LoadAll`, `Preload`         | `ErrValidation`       | negative `LoadPerParentLimit`; invalid descriptor; scope binds too many arguments for the bind limit                             |
|                                      | `ErrUnsupported`      | `LoadPerParentLimit` on belongs-to or has-one; `Each` or `Iter` on a query with preloads                                         |
|                                      | `ErrTooManyRows`      | has-one matched more than one child for a parent                                                                                 |
| `Joined`                             | `ErrUnsupported`      | not belongs-to, child key not unique, `GroupBy`, `Having`, locks, `Each`, `Iter`; used with `Load`, `LoadAll`, `Scope` or `Then` |
|                                      | `ErrValidation`       | select list does not have one expression per mapped field                                                                        |
| `Has`, `HasNo`, `CountOrder`         | none of their own     | an error in the scope predicate surfaces when the query runs                                                                     |
| `From`                               | `ErrValidation`       | many-to-many relation                                                                                                            |
| `Link`, `Unlink`, `Replace`, `Count` | `ErrUnsupported`      | belongs-to                                                                                                                       |
|                                      | `ErrValidation`       | `Link`, `Unlink`, `Replace`: NULL parent key, or NULL child key. `Count` with a NULL parent key returns 0, no error              |
|                                      | `ErrNotNullViolation` | has-one or has-many `Unlink` or `Replace` on a NOT NULL foreign key                                                              |
|                                      | `ErrUniqueViolation`  | has-one `Link` that would break a unique foreign key on the child                                                                |
|                                      | `ErrTxDone`           | the `*Tx` passed in is already committed or rolled back                                                                          |
| `CreateGraph`                        | `ErrValidation`       | nil parent, a model listed as its own child, a NULL key that must be copied                                                      |
|                                      | `ErrUnsupported`      | `*Tx` on a dialect without savepoints; identity read-back on a dialect without `RETURNING`                                       |
| `FindOrCreate`                       | `ErrValidation`       | nil model, empty key, unmapped key column                                                                                        |
|                                      | `ErrNotFound`         | re-select misses the winner's row under REPEATABLE READ or SERIALIZABLE                                                          |
|                                      | `ErrUniqueViolation`  | the key is held by a soft-deleted row                                                                                            |
|                                      | `ErrUnsupported`      | dialect without `ON CONFLICT` or `RETURNING`                                                                                     |
| `UpsertByPK`                         | `ErrValidation`       | nil model                                                                                                                        |

Database errors (unique, foreign key, check, not-null violations, serialization
failures) are classified the same way as for any other statement.

Known limitations and follow-ups:

- A tag-declared `belongs_to` needs a single-column primary key on its target
  (the referencing model's key may be composite); `has_one` and `has_many`
  need one on the parent; `many2many` needs one on both models. Other
  composite keys work with hand-written descriptors but cannot be expressed
  with tags.
- `Joined` covers belongs-to only. Has-one is not joinable.
- Pipelining has no public opt-out and doubles peak memory for a level.
- `Link`, `Unlink` and `Replace` bypass model and field hooks and `UpdatedAt`.
- `Replace` serialises concurrent callers only when the parent row exists, and
  `Link` and `Unlink` are never serialised against it.
- `CreateGraph` is one level deep and always inserts; it does not match
  existing rows.
