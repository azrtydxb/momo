# Getting started

This page takes you from an empty module to a program that talks to
PostgreSQL through Momo. It stays small on purpose: the first program uses no
generated code. The full loop (models, `momo generate`, migrations, typed
queries, relations, vector search) is the
[quickstart example](../examples/quickstart/), which is compiled and run
end to end in CI.

## Requirements

- Go 1.26 or newer.
- PostgreSQL with the pgvector extension installed on the server. The
  `momo migrate` command and the full vector support are PostgreSQL only.
  SQLite (cgo driver, with vectors through sqlite-vec; the pure-Go driver has no
  vectors) and MySQL 8.4, MySQL 9.7 and MariaDB 11.8 run queries and writes, and
  have library migration dialects (`migrate/sqlite`, `migrate/mysql`) but no CLI
  support (see [capabilities](capabilities.md)).
- A role that may create extensions, or a superuser who runs
  `CREATE EXTENSION vector` once in the target database. The first generated
  migration creates the extension when a model has a vector column.

## Install

Add the library to your module and install the CLI at the same version:

```sh
go get github.com/azrtydxb/momo@latest
go install github.com/azrtydxb/momo/cmd/momo@latest
```

Pin an explicit version instead of `latest` in a real project, and keep the CLI
and the library on the same one. The supported Go and PostgreSQL versions are
listed in [supported versions](supported-versions.md).

## Connect and run SQL

`pgxdriver.NewPool` opens a pgx pool, `pgxdriver.New` wraps it as a Momo driver
and `momo.New` returns the `*momo.DB`. `db.Tx` runs a function in a
transaction and commits when it returns nil. Inside it, use the `tx` handle for
every statement. `momo.Exec` and `momo.Scalars` are the raw SQL path: `?`
binds the next argument, so values never become part of the SQL text.

The program reads the connection string from `MOMO_EXAMPLE_DSN`, for example
`postgres://user:pass@localhost:5432/scratch?sslmode=disable`. It creates a
temporary table inside the transaction, so it leaves nothing behind.

<!-- docscheck:run -->

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	pool, err := pgxdriver.NewPool(ctx, os.Getenv("MOMO_EXAMPLE_DSN"))
	if err != nil {
		return err
	}
	defer pool.Close()
	db := momo.New(pgxdriver.New(pool))

	return db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if _, err := momo.Exec(ctx, tx, "CREATE TEMP TABLE notes (id bigint PRIMARY KEY, body text NOT NULL)"); err != nil {
			return err
		}
		if _, err := momo.Exec(ctx, tx, "INSERT INTO notes (id, body) VALUES (?, ?)", 1, "hello"); err != nil {
			return err
		}
		bodies, err := momo.Scalars[string](ctx, tx, "SELECT body FROM notes WHERE id = ?", 1)
		if err != nil {
			return err
		}
		fmt.Println(bodies)
		return nil
	})
}
```

## Handle errors

Momo classifies database errors into sentinels. Compare with `errors.Is`, never
with error text. This program violates a primary key on purpose and branches on
`momo.ErrUniqueViolation`; the transaction rolls back when the function returns
the error, so the table disappears with it.

<!-- docscheck:run -->

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
)

func main() {
	ctx := context.Background()
	pool, err := pgxdriver.NewPool(ctx, os.Getenv("MOMO_EXAMPLE_DSN"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	db := momo.New(pgxdriver.New(pool))

	err = db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if _, err := momo.Exec(ctx, tx, "CREATE TEMP TABLE tags (name text PRIMARY KEY)"); err != nil {
			return err
		}
		for range 2 {
			if _, err := momo.Exec(ctx, tx, "INSERT INTO tags (name) VALUES (?)", "go"); err != nil {
				return err
			}
		}
		return nil
	})
	if !errors.Is(err, momo.ErrUniqueViolation) {
		log.Fatalf("want a unique violation, got %v", err)
	}
	fmt.Println("duplicate rejected")
}
```

Every Go block in these docs is checked in CI: a block is either a complete
`package main` program that must compile, a program marked as runnable (this
one runs against PostgreSQL on every build), or an explicitly marked fragment
that must still parse as Go.

## Typed queries

Typed queries need generated code. You describe a table as a struct that embeds
`momo.Model`, run `momo generate`, and build queries from the generated
descriptors, so a misspelled column or a wrong value type fails at compile time:

```sh
momo generate
momo migrate diff init
momo migrate apply
```

The [quickstart example](../examples/quickstart/) shows the models, the
`momo.yaml` configuration, the generated packages and the first migration.
Then read:

- [Agent guide](ai-guide.md): the complete reference in one file, from tags to
  errors, with the pitfalls.
- [Type mapping](TYPE_MAPPING.md): the column type for each Go type.
- [Migrations](migrations.md): what `momo migrate` does and how to recover.
- [Relations](relations.md), [projections](projections.md) and
  [vector search](vectors.md).
