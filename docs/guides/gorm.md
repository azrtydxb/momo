# Coming from GORM

This page maps the GORM calls you use every day to their Momo equivalents. It
does not try to hide the differences: Momo writes exactly the SQL of the call
you made, never retries, and never saves more than the model you pass. To
convert existing GORM structs automatically, read
[importing models from other ORMs](../importers.md) (`momo import gorm`).

The programs on this page are compiled against the models of the
[quickstart example](../../examples/quickstart/): an `Author` that has many
`Post` rows. Blocks fenced as `go notest` show code that is not compiled here.

## Cheat sheet

| GORM                                     | Momo                                                                |
| ---------------------------------------- | ------------------------------------------------------------------- |
| `gorm:"primaryKey"`                      | `momo:"pk"`; add `identity` for a generated key                     |
| `gorm:"size:200;not null;uniqueIndex"`   | `momo:"size:200,notnull,unique"`                                    |
| `gorm:"column:author_id;index"`          | `momo:"column:author_id,index"`                                     |
| `db.Where("name = ?", n).Find(&rows)`    | `momo.Select[T](db).Where(col.Eq(n)).All(ctx)`                      |
| `db.First(&row, id)`                     | `momo.Select[T](db).Where(col.Eq(id)).One(ctx)`                     |
| `db.Create(&row)`                        | `momo.Insert[T](db).Model(&row).Exec(ctx)`                          |
| `db.Model(&row).Update("name", n)`       | `momo.Update[T](db).Set(col.Set(n)).Where(...).Exec(ctx)`           |
| `db.Delete(&row)`                        | `momo.Delete[T](db).Where(...).Exec(ctx)`                           |
| `db.Preload("Posts").Find(&rows)`        | `momo.Select[T](db).Preload(author.Posts).All(ctx)`, or `momo.Load` |
| `db.Transaction(func(tx *gorm.DB)...)`   | `db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {...})`       |
| `db.AutoMigrate(&T{})`                   | `momo migrate diff <name>`, then `momo migrate apply`               |
| `errors.Is(err, gorm.ErrRecordNotFound)` | `errors.Is(err, momo.ErrNotFound)`                                  |

## Models

GORM reads struct tags at run time. Momo reads the `momo:` tags and the
optional `MomoConfig` method statically, when you run `momo generate`; the
generator writes typed column handles such as `author.Name`. A Momo struct
embeds `momo.Model`, which carries the table name:

```go notest
type Author struct {
	momo.Model `momo:"table:authors"`
	ID         int64  `momo:"pk,identity"`
	Name       string `momo:"size:200,notnull,unique"`

	Posts []Post `momo:"has_many,fk:AuthorID"`
}
```

Three behaviours differ from GORM and matter when you port a model:

- GORM replaces a zero value with the column default on insert. Momo writes the
  zero value you gave it. Use a pointer field to write NULL or to leave a value
  to the database default.
- GORM treats a field named `ID` as the key and `CreatedAt` and `UpdatedAt` as
  managed timestamps by convention. Momo uses the tags you wrote.
- `gorm.DeletedAt` soft delete becomes an explicit soft-delete field; see the
  [type mapping](../TYPE_MAPPING.md).

`momo import gorm` reports each of these as a change (`zero-value-handling`,
`renamed-type`) with the position of the field.

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

// recentPosts is db.Where("author_id = ?", id).Order("title").Limit(10).Find(&posts) in GORM.
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

GORM's `Preload` runs one extra query per relation. So does Momo, and you pick
where it happens: `momo.Load` fills a relation on models you already have, and
`SelectQuery.Preload` attaches the loader to the query so `All`, `One` and
`First` run it for you. Nothing is loaded unless you ask, and a relation is
never saved with its parent; write the children with their own insert.

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

// authorsWithPosts is db.Preload("Posts").Find(&authors) in GORM.
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

GORM calls `BeforeCreate`, `BeforeUpdate`, `BeforeDelete` and `AfterFind`
methods on the model. Momo has the same four points as typed functions on the
model's metadata, `HookSet`: `BeforeInsert`, `BeforeUpdate`, `BeforeDelete` and
`AfterLoad`. They are set once at start-up on the `Meta` value, not as methods
on the struct:

```go notest
var authorMeta = momo.Meta[Author]{ /* generated metadata */ Hooks: momo.HookSet[Author]{
	BeforeInsert: func(ctx context.Context, a *Author) error {
		a.Name = strings.TrimSpace(a.Name)
		return nil
	},
}}
```

A hook runs only on the typed execution methods (`Exec`, `All`, `One` and so
on), not on raw SQL, and an error from a Before hook stops the statement before
any SQL runs. Hooks do not run for relations loaded with `Load` or `Preload`.
Per-handle rules such as a tenant filter belong in an
[interceptor](../PRIVACY_RULES.md), which runs before the hooks.

## Transactions

`db.Transaction` in GORM and `db.Tx` in Momo both commit when the function
returns nil and roll back on an error or a panic. Momo calls the function
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

`AutoMigrate` compares your structs with the live database and changes it on
the spot. Momo's `AutoMigrate` Go API also exists, but it only adds (tables,
columns, indexes), writes no history and is meant for development and tests.
For a real database use versioned files:

```sh
momo migrate diff add_authors   # writes <version>_add_authors.up.sql, a snapshot and momo.sum
momo migrate apply              # applies pending files under a lock and records them
momo migrate status
```

If the database already has a schema from GORM, put it under Momo with
`momo migrate baseline` (see [migrations](../migrations.md)); if you ran GORM
inside another tool's migrations, [import that history](../importers.md) so
nothing runs twice.

## Errors

GORM returns `gorm.ErrRecordNotFound` from `First`. Momo classifies database
errors into sentinels you compare with `errors.Is`, never with the error text:
`momo.ErrNotFound` for a single-row call that found nothing,
`momo.ErrUniqueViolation` and `momo.ErrForeignKeyViolation` for constraint
failures. `All` returns an empty slice, not an error, when nothing matches.
