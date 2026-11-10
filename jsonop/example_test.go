// SPDX-License-Identifier: Apache-2.0

package jsonop_test

import (
	"errors"
	"fmt"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/jsonop"
	"github.com/azrtydxb/momo/types"
)

// Text extracts a member as text. The result has the comparison methods of any
// string expression. A plain key is written into the SQL as a literal, so the
// same extraction in the select list and in GROUP BY is the same expression.
func ExampleText() {
	after := momo.NewColumn[types.JSON[map[string]any]]("events", "after", 0)

	b := momo.NewBuilder(dialect.For(dialect.Postgres))
	jsonop.Text(after, "via").Eq("api").AppendSQL(b)
	fmt.Println(b.String(), b.Args())
	// Output:
	// ("events"."after" ->> 'via') = $1 [api]
}

// Path reads a nested member. On SQLite an all-digit element is an array
// index.
func ExamplePath() {
	after := momo.NewColumn[types.JSON[map[string]any]]("events", "after", 0)

	b := momo.NewBuilder(dialect.For(dialect.SQLite))
	jsonop.Path(after, "items", "0", "sku").Eq("A1").AppendSQL(b)
	fmt.Println(b.String(), b.Args())
	// Output:
	// CAST(json_extract("events"."after", '$."items"[0]."sku"') AS TEXT) = ? [A1]
}

// Contains binds the value as one JSON parameter. SQLite has no containment
// operator, so the build fails with ErrUnsupported there.
func ExampleContains() {
	after := momo.NewColumn[types.JSON[map[string]any]]("events", "after", 0)
	pred := jsonop.Contains(after, map[string]any{"via": "api"})

	pg := momo.NewBuilder(dialect.For(dialect.Postgres))
	pred.AppendSQL(pg)
	fmt.Println(pg.String(), pg.Args())

	lite := momo.NewBuilder(dialect.For(dialect.SQLite))
	pred.AppendSQL(lite)
	fmt.Println(errors.Is(lite.Err(), momo.ErrUnsupported))
	// Output:
	// ("events"."after" @> CAST($1 AS jsonb)) [{"via":"api"}]
	// true
}

// HasKey is true for a member whose value is JSON null and false for a
// missing member. The key is always bound.
func ExampleHasKey() {
	after := momo.NewColumn[types.JSON[map[string]any]]("events", "after", 0)

	b := momo.NewBuilder(dialect.For(dialect.Postgres))
	jsonop.HasKey(after, "via").AppendSQL(b)
	fmt.Println(b.String(), b.Args())
	// Output:
	// ("events"."after" ? $1) [via]
}
