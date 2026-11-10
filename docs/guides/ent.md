# Coming from Ent

This page maps the Ent calls you use every day to their Momo equivalents. Ent
generates a client from schema code; Momo generates typed column handles from
plain structs. The differences are in what Momo does not do for you: it writes
exactly the SQL of the call you made, never retries, and never saves an edge you
did not write. To convert an existing Ent schema automatically, run
`momo import ent` (see [importing models from other ORMs](../importers.md)): it
reads the schema package with `go/parser` and never compiles or runs it.

The programs on this page are compiled against the models of the
[quickstart example](../../examples/quickstart/): an `Author` that has many
`Post` rows. Blocks fenced as `go notest` show code that is not compiled here.

## Cheat sheet

| Ent                                              | Momo                                                          |
| ------------------------------------------------ | ------------------------------------------------------------- |
| `field.String("name").NotEmpty().Unique()`       | `Name string` with `momo:"notnull,unique"`                    |
| `field.String("bio").Optional().Nillable()`      | `Bio *string`                                                 |
| `field.Int64("id")` (the implicit key)           | `ID int64` with `momo:"pk,identity"`                          |
| `edge.To("posts", Post.Type)`                    | `Posts []Post` with `momo:"has_many,fk:AuthorID"`             |
| `edge.From("author", Author.Type).Ref("posts")`  | `Author *Author` with `momo:"belongs_to,fk:AuthorID"`         |
| `client.Author.Query().Where(author.NameEQ(n))`  | `momo.Select[T](db).Where(col.Eq(n)).All(ctx)`                |
| `client.Author.Create().SetName(n).Save(ctx)`    | `momo.Insert[T](db).Model(&row).Exec(ctx)`                    |
| `client.Author.UpdateOneID(id).SetName(n)`       | `momo.Update[T](db).Set(col.Set(n)).Where(...).Exec(ctx)`     |
| `client.Author.DeleteOneID(id).Exec(ctx)`        | `momo.Delete[T](db).Where(...).Exec(ctx)`                     |
| `.WithPosts()` (eager loading)                   | `.Preload(author.Posts)` on the select, or `momo.Load`        |
| `client.Tx(ctx)`, `tx.Commit()`, `tx.Rollback()` | `db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {...})` |
| `client.Schema.Create(ctx)`                      | `momo migrate diff <name>`, then `momo migrate apply`         |
| `ent.IsNotFound(err)`                            | `errors.Is(err, momo.ErrNotFound)`                            |

## Models

Ent describes a table with builder chains in a `Fields` method, and generates a
client and a package of predicates per entity. Momo describes a table as a
struct and generates typed column handles such as `author.Name`, which are the
predicates and setters you use in queries.

```go notest
type Author struct {
	momo.Model `momo:"table:authors"`
	ID         int64  `momo:"pk,identity"`
	Name       string `momo:"size:200,notnull,unique"`

	Posts []Post `momo:"has_many,fk:AuthorID"`
}
```

`momo import ent` converts the fields, edges, indexes, mixins and the
`entsql.Annotation` table name it can read as literals. Anything it cannot read
without running your code (a many-to-many edge, JSON and UUID fields, a custom
key, an unknown builder method) is an `unsupported` change at its `file:line:col`:
nothing is written until you remove or convert it. Two behaviours differ from Ent
and are reported as changes:

- Ent applies a field's `Default` in Go when the setter was not called. Momo
  writes the zero value you gave it (`zero-value-handling`); keep the database
  default with a `default:` tag or a pointer field.
- Ent saves an edge only when its setter was called, and Momo saves a relation
  only when you write it, so `edge.From` with `.Ref` is reported as
  `implicit-save` to make you check the write paths.

## Queries

A query is built from typed columns, so a misspelled column or a value of the
wrong type fails to compile. `All` returns every row, `One` returns exactly one
row and fails with `momo.ErrNotFound` when there is none.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/examples/quickstart/models"
	"github.com/azrtydxb/momo/examples/quickstart/models/post"
)

// recentPosts is client.Post.Query().Where(post.AuthorID(id)).Order(ent.Asc("title")).Limit(10).All(ctx) in Ent.
func recentPosts(ctx context.Context, q momo.Querier, authorID int64) ([]models.Post, error) {
	return momo.Select[models.Post](q).
		Where(post.AuthorID.Eq(authorID)).
		OrderBy(post.Title.Asc()).
		Limit(10).
		All(ctx)
}

func main() {
	ctx := context.Background()
	pool, err := pgxdriver.NewPool(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	db := momo.New(pgxdriver.New(pool))

	posts, err := recentPosts(ctx, db, 1)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(len(posts), "posts")
}
```

Raw SQL stays available through `momo.Exec` and `momo.Scalars`; `?` binds the
next argument, so a value never becomes part of the SQL text.

## Relations

Ent loads an edge when you ask with `WithPosts()`, one extra query per edge.
Momo does the same and you pick where it happens: `momo.Load` fills a relation
on models you already have, and `SelectQuery.Preload` attaches the loader to the
query so `All`, `One` and `First` run it for you. A relation is never saved
with its parent; write the children with their own insert.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/examples/quickstart/models"
	"github.com/azrtydxb/momo/examples/quickstart/models/author"
)

// authorsWithPosts is client.Author.Query().WithPosts().All(ctx) in Ent.
func authorsWithPosts(ctx context.Context, q momo.Querier) ([]models.Author, error) {
	return momo.Select[models.Author](q).Preload(author.Posts).All(ctx)
}

// loadPosts fills Posts on authors you already have.
func loadPosts(ctx context.Context, q momo.Querier, authors []models.Author) error {
	return momo.Load(ctx, q, authors, author.Posts)
}

func main() {
	ctx := context.Background()
	pool, err := pgxdriver.NewPool(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	db := momo.New(pgxdriver.New(pool))

	authors, err := authorsWithPosts(ctx, db)
	if err != nil {
		log.Fatal(err)
	}
	if err := loadPosts(ctx, db, authors); err != nil {
		log.Fatal(err)
	}
	fmt.Println(len(authors), "authors")
}
```

See [relations](../relations.md) for filtering a loaded relation, joins and
writing related rows.

## Hooks

Ent hooks wrap mutations (`ent.Hook`, added with `client.Use` or in the schema's
`Hooks` method) and see the mutation, not the model. Momo's model hooks are typed
functions on the model's metadata, `HookSet`: `BeforeInsert`, `BeforeUpdate`,
`BeforeDelete` and `AfterLoad`, set once at start-up on the `Meta` value. They
receive the model pointer and may change its values. Observing every statement
is `momo.WithHook`, and per-handle rules such as a tenant filter are
[interceptors](../PRIVACY_RULES.md).

```go notest
var authorMeta = momo.Meta[Author]{ /* generated metadata */ Hooks: momo.HookSet[Author]{
	BeforeInsert: func(ctx context.Context, a *Author) error {
		a.Name = strings.TrimSpace(a.Name)
		return nil
	},
}}
```

A model hook runs only on the typed execution methods (`Exec`, `All`, `One` and
so on), not on raw SQL, and an error from a Before hook stops the statement
before any SQL runs. Ent validators, immutable fields and privacy policies are
reported by the importer as `go-side-rule` changes: Momo does not run them.

## Transactions

`client.Tx` in Ent hands you a transactional client that you commit or roll back
yourself. `db.Tx` in Momo takes a function instead: it commits when the function
returns nil and rolls back on an error or a panic. Momo calls the function
exactly once: there is no automatic retry, so retry a serialization failure in
your own code. Use the `tx` handle for every statement inside the function.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/examples/quickstart/models"
	"github.com/azrtydxb/momo/examples/quickstart/models/author"
)

func rename(ctx context.Context, db *momo.DB, id int64, name string) error {
	return db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		a, err := momo.Select[models.Author](tx).Where(author.ID.Eq(id)).One(ctx)
		if errors.Is(err, momo.ErrNotFound) {
			return fmt.Errorf("author %d does not exist", id)
		}
		if err != nil {
			return err
		}
		_, err = momo.Update[models.Author](tx).
			Set(author.Name.Set(name)).
			Where(author.ID.Eq(a.ID)).
			Exec(ctx)
		return err
	})
}

func main() {
	ctx := context.Background()
	pool, err := pgxdriver.NewPool(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	db := momo.New(pgxdriver.New(pool))

	if err := rename(ctx, db, 1, "Ada Lovelace"); err != nil {
		log.Fatal(err)
	}
}
```

## Migrations

`client.Schema.Create` applies the schema to the database on the spot, and Ent
can also write versioned migration files through Atlas. Momo's migrations are SQL
files that `momo migrate diff` writes from your models, each with a schema
snapshot and a `momo.sum` checksum file:

```sh
momo migrate diff add_authors   # writes <version>_add_authors.up.sql, a snapshot and momo.sum
momo migrate apply              # applies pending files under a lock and records them
momo migrate status
```

If you used Ent's Atlas versioned migrations, the Atlas directory reader
(`importer/history/atlas`, see [importing](../importers.md)) verifies `atlas.sum`
and converts the history so nothing runs twice. A database created with
`client.Schema.Create` has no history file; run `momo migrate baseline` on it
(see [migrations](../migrations.md)).

## Errors

Ent returns errors you test with `ent.IsNotFound`, `ent.IsConstraintError` and
related helpers. Momo classifies database errors into sentinels you compare with
`errors.Is`, never with the error text: `momo.ErrNotFound` for a single-row call
that found nothing, `momo.ErrUniqueViolation` and `momo.ErrForeignKeyViolation`
for constraint failures. `All` returns an empty slice, not an error, when
nothing matches.
