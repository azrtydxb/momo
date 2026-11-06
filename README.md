# Momo

Momo is a Go ORM that generates code from plain Go structs. You describe a
table as a struct with a few tags, `momo generate` writes typed column and
relation descriptors, and queries are built from those descriptors, so a typo
in a column name or a mismatched value type fails at compile time. SQL stays
explicit and inspectable. pgvector support is built in: vector columns,
distance expressions, and search settings are part of the core API. Schema
migrations are built in as well: `momo migrate diff` compares your structs with
the last snapshot and writes the SQL.

Status: v0.1.0, pre-v1, with no compatibility promise between releases. This
release supports PostgreSQL with pgvector. SQLite and MySQL are planned. Momo
needs Go 1.26 or newer (see `go.mod`).

## Install

Add the library to your module and install the CLI:

```sh
go get github.com/azrtydxb/momo@v0.1.0
go install github.com/azrtydxb/momo/cmd/momo@v0.1.0
```

Momo expects PostgreSQL with the pgvector extension. The first migration runs
`CREATE EXTENSION IF NOT EXISTS "vector"` for you, which needs a role that may
create extensions.

pgvector must be installed on the server itself, not only in your database.
Use an image that ships it, such as `pgvector/pgvector`, or your distribution's
pgvector package. If the migration role cannot create extensions, have a
superuser run `CREATE EXTENSION vector` once in the target database before you
apply.

## Worked example

Every Go snippet below is compiled: the full source lives in
[examples/quickstart](examples/quickstart), and a test fails when this README and
that code drift apart. Copy the example directory as a starting point, and
rewrite its import paths from `github.com/azrtydxb/momo/examples/quickstart`
to your own module path.

These are the imports of the example. The generated packages are imported as
`<your module>/models/author`, `<your module>/models/post`, and so on, one
package per model next to the models package.

```go
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
	"github.com/azrtydxb/momo/examples/quickstart/models/post"
	"github.com/azrtydxb/momo/vector"
)
```

### 1. Define the models

A model is a struct that embeds `momo.Model`. The tag on that field names the
table, and each field carries its column rules. A field of type
`vector.Vector` is a pgvector column, and `dim` sets its dimension. Use the
output size of your embedding model for `dim`, such as 1536.

Tags used here: `table:<name>` names the table, `pk` marks the primary key,
`identity` lets the database assign it, `column:<name>` overrides the column
name, `size:<n>` limits a string, `notnull` and `unique` add constraints,
`index` adds an index, `default:<sql>` sets the column default, `read_only`
means Momo never writes the column, `has_many` and `belongs_to` declare a
relation, and `fk:<Field>` names the foreign-key field. How Go types map to
column types is in [docs/TYPE_MAPPING.md](docs/TYPE_MAPPING.md).

```go
// Author writes posts.
type Author struct {
	momo.Model `momo:"table:authors"`
	ID         int64  `momo:"pk,identity"`
	Name       string `momo:"size:200,notnull,unique"`

	Posts []Post `momo:"has_many,fk:AuthorID"`
}

// Post is one article with a 3-dimensional embedding. Real embeddings are
// larger: change dim to the output size of your model, for example 1536.
type Post struct {
	momo.Model `momo:"table:posts"`
	ID         int64         `momo:"pk,identity"`
	AuthorID   int64         `momo:"column:author_id,notnull,index"`
	Title      string        `momo:"size:300,notnull"`
	Embedding  vector.Vector `momo:"dim:3"`
	CreatedAt  time.Time     `momo:"column:created_at,read_only,default:now()"`

	Author *Author `momo:"belongs_to,fk:AuthorID"`
}
```

Tags cannot express everything. pgvector's `hnsw` index method has no default
operator class, so a `MomoConfig` method names it. The generator reads this
method statically and never runs it.

```go
func (p *Post) MomoConfig(c *momo.Config) {
	c.Index("posts_embedding_hnsw", &p.Embedding).
		Method("hnsw").
		Opclass(&p.Embedding, "vector_cosine_ops")
}
```

### 2. Configure and generate

Put a `momo.yaml` at the module root. `version` is the config format version
(1). `packages` lists the model packages. `naming` picks the convention for
table and column names you do not set with a tag. `dialects` lists the
databases to generate for. `migrate.dir` is where migrations are written, and
`migrate.url_env` names the environment variable that holds the database URL.

```yaml
version: 1
packages:
  - ./models
naming: gorm
dialects: [postgres]
migrate:
  dir:
    postgres: migrations
  url_env:
    postgres: DATABASE_URL
```

Generate the typed descriptors:

```sh
momo generate
```

This writes a `momo_gen.go` next to the models and one package per model
(`models/author`, `models/post`) holding its columns, such as `post.Title`
and `post.AuthorID`, and its relations, such as `author.Posts` and
`post.Author`. Commit the generated files. `momo generate --check` exits 1
when they are stale, which makes it a good CI step. You can also add
`//go:generate momo generate` to a source file.

On Windows, git may convert generated files to CRLF line endings, which makes
`--check` report them as stale. Add this line to `.gitattributes`:

```text
*_gen.go text eol=lf
```

### 3. Create and apply the migration

`momo migrate diff` compares the models with the latest snapshot and writes the
migration offline, without a database. The first run against an empty
directory writes the initial schema. Review the generated SQL before you apply
it.

```sh
momo migrate diff init
```

Apply it with the URL in the environment variable that `momo.yaml` names:

```sh
export DATABASE_URL='postgres://user:password@localhost:5432/app?sslmode=disable'
momo migrate apply
```

`sslmode=disable` is for local development only. Use `sslmode=verify-full` or
another TLS mode against any database you do not reach over a trusted local
connection.

Momo v0.1.0 writes forward migrations only. There is no rollback command, so
undo a change with a new migration.

Run `momo migrate help` for the other commands: `plan`, `status`, `validate`,
`hash`, `drift`, `baseline`, and `resolve`. Run migrations over a direct connection to
PostgreSQL, not through a transaction-mode pooler: the migration lock is a
session advisory lock.

### Changing a model

Edit the struct, then run these in order:

```sh
momo generate
momo migrate diff add_subtitle
momo migrate apply
```

`diff` compares the structs with the latest snapshot. A renamed field or
struct looks like a drop plus an add, and `diff` refuses that without
`--allow-destructive` because it would lose data. Say it is a rename instead
with `--rename-column table.old=new` or `--rename-table old=new`. Review the
SQL before you apply it.

### 4. Open a database

`pgxdriver.NewPool` opens a pgx pool and `pgxdriver.New` wraps it as a Momo
driver. The caller owns the pool and closes it.

```go
func openDB(ctx context.Context, url string) (*momo.DB, func(), error) {
	pool, err := pgxdriver.NewPool(ctx, url)
	if err != nil {
		return nil, nil, err
	}
	return momo.New(pgxdriver.New(pool)), pool.Close, nil
}
```

PgBouncer: the driver's default exec mode sends unnamed statements and binds
values on the server, so it is safe behind PgBouncer in transaction pooling
mode without further settings. `pgxdriver.WithExecMode` selects a different
pgx mode, but not the simple protocol, which interpolates arguments on the
client and is rejected.

### 5. Insert, select, update, delete

Queries take a `momo.Querier`, which both `*momo.DB` and `*momo.Tx` satisfy.
Columns come from the generated packages. `Returning` reads database-assigned
values, such as the identity column, back into the struct.

```go
func createAuthor(ctx context.Context, q momo.Querier, name string) (models.Author, error) {
	a := models.Author{Name: name}
	rows, err := momo.Insert[models.Author](q).Model(&a).Returning(author.ID).Rows(ctx)
	if err != nil {
		return a, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := rows.ScanModel(&a); err != nil {
			return a, err
		}
	}
	return a, rows.Err()
}
```

`Models` inserts a slice as one multi-row statement:

```go
func createPosts(ctx context.Context, q momo.Querier, authorID int64) error {
	_, err := momo.Insert[models.Post](q).Models([]models.Post{
		{AuthorID: authorID, Title: "Notes on the engine", Embedding: vector.Vector{1, 0, 0}},
		{AuthorID: authorID, Title: "Poetical science", Embedding: vector.Vector{0, 1, 0}},
		{AuthorID: authorID, Title: "Bernoulli numbers", Embedding: vector.Vector{0.9, 0.1, 0}},
	}).Exec(ctx)
	return err
}
```

Select, update, and delete take typed predicates. An update or delete without
a `Where` is refused unless you call `AllRows()`, so a forgotten filter cannot
touch every row.

```go
func readAndChange(ctx context.Context, q momo.Querier, authorID int64) error {
	posts, err := momo.Select[models.Post](q).
		Where(post.AuthorID.Eq(authorID)).
		OrderBy(post.Title.Asc()).
		All(ctx)
	if err != nil {
		return err
	}
	fmt.Println("posts:", len(posts))

	first, err := momo.Select[models.Post](q).Where(post.Title.Eq("Poetical science")).One(ctx)
	if err != nil {
		return err
	}

	if _, err := momo.Update[models.Post](q).
		Set(post.Title.Set("Poetical science, revised")).
		Where(post.ID.Eq(first.ID)).
		Exec(ctx); err != nil {
		return err
	}
	_, err = momo.Delete[models.Post](q).Where(post.ID.Eq(first.ID)).Exec(ctx)
	return err
}
```

`One` returns an error that matches `momo.ErrNotFound` when no row matches.

#### Conditional upsert

`OnConflict(...).DoUpdate(...)` takes a condition with `Where`, so one statement can take a lease: insert the row, or update it only when the current lease expired or is already yours. `RowsAffected` is 1 when you hold the lease and 0 when someone else does. In the condition, columns mean the existing row and `Excluded()` means the proposed one.

The full example, with the model and the partial-index form, is in
[docs/ai-guide.md](docs/ai-guide.md) under upserts.

To conflict on a partial unique index, give the index predicate with `OnConflict(cols...).Where(...)` before `DoNothing` or `DoUpdate`. Both forms work on PostgreSQL; MySQL returns `ErrUnsupported`. On SQLite the target predicate must match the index text, so write it as raw SQL, for example `momo.SQL("live = 1")`.

### 6. Load relations

Relations are loaded eagerly after the parents are fetched, with one query per
relation instead of one per parent. `momo.Load` loads one relation, and
`momo.LoadAll` loads several for the same parents. Has-many (`author.Posts`)
and belongs-to (`post.Author`) work the same way.

To load relations as part of a query, add them with `Preload`, for example
`Preload(author.Posts)` on an author query. `All`, `One` and `First` then load
the relations after the rows are read, with the same one query per relation,
and a result with no rows runs no extra query. `Preload` returns a copy, so a base query can be reused. `Each` and `Iter` stream rows
and return an error matching `momo.ErrUnsupported` when the query has preloads.

Scoped and nested loads, top-N per parent, single-query `Joined()` loading,
relation predicates such as `Has`, the `Link` and `Replace` helpers, graph
writes and the error behavior of each are covered in
[docs/relations.md](docs/relations.md).

```go
func loadRelations(ctx context.Context, q momo.Querier, authorID int64) error {
	authors, err := momo.Select[models.Author](q).Where(author.ID.Eq(authorID)).All(ctx)
	if err != nil {
		return err
	}
	if err := momo.Load(ctx, q, authors, author.Posts); err != nil {
		return err
	}
	posts := authors[0].Posts
	if err := momo.Load(ctx, q, posts, post.Author); err != nil {
		return err
	}
	for _, p := range posts {
		fmt.Println(p.Title, "by", p.Author.Name)
	}
	return nil
}
```

### 7. Similarity search

Distance expressions take a vector column and a query vector, and the result
orders a select. The generator emits a `momo.VectorColumn` for every field of
a `vector` type, such as `post.Embedding`. It records the dimension from the
`dim` tag, so a wrong-sized query vector is rejected when the query is built.

`WithSearch` runs a function in a transaction with pgvector settings applied,
here `hnsw.ef_search`. The settings end with the transaction. `SearchSettings`
also covers iterative scans for filtered searches, IVFFlat probes, and
`ExactSearch`.

```go
func similarPosts(ctx context.Context, db *momo.DB, query vector.Vector) error {
	settings := momo.SearchSettings{HNSWEfSearch: 100}
	return db.WithSearch(ctx, settings, func(tx *momo.Tx) error {
		nearest, err := momo.Select[models.Post](tx).
			OrderBy(momo.CosineDistance(post.Embedding, query).Asc()).
			Limit(5).
			All(ctx)
		if err != nil {
			return err
		}
		for _, p := range nearest {
			fmt.Println("near:", p.Title)
		}
		return nil
	})
}
```

For dense vectors the functions are `L2Distance`, `InnerProductDistance`,
`CosineDistance`, and `L1Distance`. For bit vectors they are `HammingDistance`
and `JaccardDistance`. Match the function to the operator
class of your index: this example indexes with `vector_cosine_ops`, so it
searches with `CosineDistance`.

### 8. Transactions

`db.Tx` begins a transaction, commits when the function returns nil, and rolls
back when it returns an error or panics. Pass `momo.TxOptions` to set the
isolation level or read-only mode.

```go
func renameInTx(ctx context.Context, db *momo.DB, id int64, name string) error {
	return db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if _, err := momo.Update[models.Author](tx).
			Set(author.Name.Set(name)).
			Where(author.ID.Eq(id)).
			Exec(ctx); err != nil {
			return err
		}
		n, err := momo.Select[models.Author](tx).Where(author.Name.Eq(name)).Count(ctx)
		if err != nil {
			return err
		}
		if n != 1 {
			return errors.New("rename did not take effect") // rolls the transaction back
		}
		return nil
	})
}
```

To run the whole example against a database with the migrations applied:

```sh
DATABASE_URL='postgres://user:password@localhost:5432/app?sslmode=disable' go run ./examples/quickstart
```

### Projections

A projection reads a subset of columns or an aggregate into a struct that is
not a table. Embed `momo.Projection` and select it with `From` and `Columns`;
see [examples/projection-pagination](examples/projection-pagination/) and the
`Hit` type in [examples/vector-search](examples/vector-search/). In v0.1.0 the
generator does not emit projections, so you write the `MomoMeta` by hand. A projection has no identity, version, soft-delete or timestamp column, so set
`Identity`, `Version`, `SoftDelete`, `CreatedAt` and `UpdatedAt` to -1 in its
`Meta`. The `Card` type in the projection example shows the whole thing.

A projection does not get the automatic soft-delete filter (its `SoftDelete` is
`-1`). When the table has a soft-delete column, add the filter yourself, for
example `.Where(article.DeletedAt.IsNull())`.

Generating this is tracked in the backlog story
[Generate Meta for projection structs](.procoder/backlog/stories/20261106-generate-meta-for-projection-structs.md).

## Command line

`momo help` lists the commands. The ones used above:

- `momo generate [-c momo.yaml] [--check] [patterns...]` writes the generated
  code, or with `--check` lists stale files and exits 1.
- `momo migrate diff <name>` writes a migration for the difference between the
  models and the latest snapshot. It refuses destructive changes unless you
  pass `--allow-destructive`.
- `momo migrate apply [--to N] [--lock-wait 30s]` applies pending migrations.
- `momo migrate validate` checks the migration directory offline.

Exit codes are 0 for success, 1 for findings such as stale generated files, 2
for usage or configuration errors, and 3 for database errors.

## More

- [docs/README.md](docs/README.md) is the documentation index.
- [docs/ai-guide.md](docs/ai-guide.md) is a single-file guide for AI coding
  agents that use Momo in an application (also indexed by [llms.txt](llms.txt)).
- [docs/vectors.md](docs/vectors.md) covers pgvector types, distance
  metrics, index limits and search settings.
- [docs/TYPE_MAPPING.md](docs/TYPE_MAPPING.md) maps every Go type to its
  column type.
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) describes the layers and
  contracts.
- [CONTRIBUTING.md](CONTRIBUTING.md) explains how to build and test Momo.
- [SECURITY.md](SECURITY.md) explains how to report a vulnerability.
- [LICENSE](LICENSE) is Apache-2.0.
