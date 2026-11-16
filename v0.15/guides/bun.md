# Coming from Bun

This page maps the Bun calls you use every day to their Momo equivalents. Bun
and Momo are both close to SQL, so most calls translate one to one. The
differences are in what Momo does not do for you: it writes exactly the SQL of
the call you made, never retries, and never saves more than the model you pass.
To convert existing Bun structs automatically, read
[importing models from other ORMs](../importers.md) (`momo import bun`).

The programs on this page are compiled against the models of the
[quickstart example](../../examples/quickstart/): an `Author` that has many
`Post` rows. Blocks fenced as `go notest` show code that is not compiled here.

## Cheat sheet

| Bun                                          | Momo                                                          |
| -------------------------------------------- | ------------------------------------------------------------- |
| `bun:"table:users,alias:u"` on `BaseModel`   | `momo:"table:users"` on the embedded `momo.Model`             |
| `bun:",pk,autoincrement"`                    | `momo:"pk,identity"`                                          |
| `bun:",notnull,unique"`                      | `momo:"notnull,unique"`                                       |
| `bun:",nullzero"`                            | a pointer field (Momo writes the zero value otherwise)        |
| `bun:"rel:has-many,join:id=author_id"`       | `momo:"has_many,fk:AuthorID"`                                 |
| `bun:"rel:belongs-to,join:author_id=id"`     | `momo:"belongs_to,fk:AuthorID"`                               |
| `NewSelect().Model(&rows).Where(...)`        | `momo.Select[T]` with `Where` and `All`                       |
| `NewInsert().Model(&row).Exec(ctx)`          | `momo.Insert[T]` with `Model` and `Exec`                      |
| `NewUpdate().Model(&row).WherePK()`          | `momo.Update[T]` with `UpdateQuery.Set`, `Where` and `Exec`   |
| `NewDelete().Model(&row).WherePK()`          | `momo.Delete[T]` with `Where` and `Exec`                      |
| `.Relation("Posts")`                         | `.Preload(author.Posts)` on the select, or `momo.Load`        |
| `db.RunInTx(ctx, nil, func(ctx, tx bun.Tx))` | `db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {...})` |
| `bun migrate.NewMigrator(db, migrations)`    | `momo migrate diff <name>`, then `momo migrate apply`         |
| `errors.Is(err, sql.ErrNoRows)`              | `errors.Is(err, momo.ErrNotFound)`                            |

## Models

Bun reads its struct tags at run time through reflection. Momo reads the
`momo:` tags and the optional `MomoConfig` method statically, when you run
`momo generate`; the generator writes typed column handles such as `author.Name`.

```go notest
type Author struct {
	momo.Model `momo:"table:authors"`
	ID         int64  `momo:"pk,identity"`
	Name       string `momo:"size:200,notnull,unique"`

	Posts []Post `momo:"has_many,fk:AuthorID"`
}
```

One behaviour differs from Bun and matters when you port a model: `nullzero`
makes Bun write NULL for a zero value. Momo writes the zero value you gave it;
use a pointer field to write NULL. `momo import bun` reports each `nullzero`
field as `zero-value-handling` and each `default:` as a database default.

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

// recentPosts is the Momo form of NewSelect().Model(&posts).Where("author_id = ?", id).Order("title").Limit(10).
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
next argument, as in Bun, so a value never becomes part of the SQL text.

## Relations

Bun's `.Relation("Posts")` joins or runs a second query depending on the
relation. Momo always says which: `momo.Load` fills a relation on models you
already have with one extra query, and `SelectQuery.Preload` attaches the loader
to the query so `All`, `One` and `First` run it for you. A relation is never
saved with its parent; write the children with their own insert.

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

// authorsWithPosts is NewSelect().Model(&authors).Relation("Posts") in Bun.
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

Bun has model hooks such as `BeforeAppendModel` and query hooks added with
`AddQueryHook`. Momo separates the two. Model hooks are typed functions on the
model's metadata, `HookSet`: `BeforeInsert`, `BeforeUpdate`, `BeforeDelete` and
`AfterLoad`, set once at start-up on the `Meta` value. Observing every
statement is `momo.WithHook`, and per-handle rules such as a tenant filter are
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
before any SQL runs.

## Transactions

`RunInTx` in Bun and `db.Tx` in Momo both commit when the function returns nil
and roll back on an error or a panic. Momo calls the function exactly once:
there is no automatic retry, so retry a serialization failure in your own code.
Use the `tx` handle for every statement inside the function.

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

Bun's migrator runs SQL files or Go functions registered in your program and
records them in `bun_migrations`. Momo's migrations are SQL files that
`momo migrate diff` writes from your models, each with a schema snapshot and a
`momo.sum` checksum file:

```sh
momo migrate diff add_authors   # writes <version>_add_authors.up.sql, a snapshot and momo.sum
momo migrate apply              # applies pending files under a lock and records them
momo migrate status
```

A database that Bun already migrated keeps its schema. Import the SQL
migrations and the `bun_migrations` rows with the Bun history reader
(`importer/history/bun`, see [importing](../importers.md)) so none of them runs
again. Bun migrations written as Go functions cannot be imported; the reader
lists them and refuses the import until you convert them to SQL.

## Errors

Bun returns the driver's error, and `sql.ErrNoRows` from `Scan` on an empty
result. Momo classifies database errors into sentinels you compare with
`errors.Is`, never with the error text: `momo.ErrNotFound` for a single-row call
that found nothing, `momo.ErrUniqueViolation` and `momo.ErrForeignKeyViolation`
for constraint failures. `All` returns an empty slice, not an error, when
nothing matches.
