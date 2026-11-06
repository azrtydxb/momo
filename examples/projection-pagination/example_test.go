// SPDX-License-Identifier: Apache-2.0

//go:build integration

package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/azrtydxb/momo/examples/internal/exampletest"
)

// TestExample runs the example end to end on a scratch database that has the
// example's migrations applied, and pins what it prints.
func TestExample(t *testing.T) {
	exampletest.CheckGenerated(t)
	db := exampletest.Open(t, "migrations")
	var out bytes.Buffer
	if err := run(context.Background(), db, &out); err != nil {
		t.Fatal(err)
	}
	const want = `sql: SELECT "articles"."id", "articles"."title", "articles"."score" FROM "articles" WHERE "articles"."score" >= $1 ORDER BY "articles"."score" DESC, "articles"."id" ASC
card a score=5
card b score=5
card c score=5
card d score=4
card e score=4
page 1: a(5) b(5) c(5)
page 2: d(4) e(4) f(3)
page 3: g(3) h(1)
count: total=8 score>=4: 5 any above 5: false
score 5: 3 articles
score 4: 2 articles
score 3: 2 articles
`
	if got := out.String(); got != want {
		t.Errorf("output differs.\ngot:\n%s\nwant:\n%s", got, want)
	}
}
