// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momotest_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/momotest"
)

// printTB stands in for *testing.T in an example: it prints the first line of
// each failure and ignores the rest.
type printTB struct{ testing.TB }

func (printTB) Helper() {}

func (printTB) Errorf(format string, args ...any) {
	first, _, _ := strings.Cut(fmt.Sprintf(format, args...), "\n")
	fmt.Println("failed:", first)
}

func (printTB) Fatalf(format string, args ...any) {
	first, _, _ := strings.Cut(fmt.Sprintf(format, args...), "\n")
	fmt.Println("fatal:", first)
}

// LoadFixtures inserts the rows of a YAML file in file order, one INSERT per
// row. The example runs against a recording driver; in a test q is the
// *momo.DB or *momo.Tx of the test.
func ExampleLoadFixtures() {
	drv := fakedriver.New(dialect.Postgres)
	momotest.LoadFixtures(printTB{}, momo.New(drv), "testdata/fixtures.yaml")
	for _, c := range drv.Calls() {
		table, _, _ := strings.Cut(strings.TrimPrefix(c.SQL, `INSERT INTO "`), `"`)
		fmt.Println("insert into", table)
	}
	// Output:
	// insert into tags
	// insert into authors
	// insert into documents
	// insert into documents
}

// A test compiles a query with its SQL method and compares the text. Nothing
// is executed and no database is needed; the message of a failure never shows
// an argument value.
func ExampleAssertSQL() {
	db := momo.New(fakedriver.New(dialect.Postgres))
	c, err := momo.Select[models.Author](db).Where(author.ID.Eq(1)).SQL()
	if err != nil {
		fmt.Println(err)
		return
	}
	// A differently wrapped copy of the same text matches.
	momotest.AssertSQL(printTB{}, c, strings.ReplaceAll(c.SQL, " FROM ", "\nFROM "))
	// A different statement does not.
	momotest.AssertSQL(printTB{}, c, "SELECT 1")
	// Output:
	// failed: momotest.AssertSQL: SQL mismatch
}

// AssertUsesIndex runs a plain EXPLAIN of the statement and fails unless the
// plan reads the named index. The example scripts the plan rows; in a test the
// querier is a *momo.DB on PostgreSQL or SQLite.
func ExampleAssertUsesIndex() {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{"Index Scan using items_sku_idx on items"}})
	db := momo.New(drv)
	c := momo.Compiled{SQL: "SELECT id FROM items WHERE sku = $1", Args: []any{"a"}}

	momotest.AssertUsesIndex(printTB{}, db, c, "items_sku_idx")
	momotest.AssertUsesIndex(printTB{}, db, c, "items_qty_idx")
	// Output:
	// failed: momotest.AssertUsesIndex: the plan does not use index "items_qty_idx"
}
