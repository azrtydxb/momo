// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

// bulkStock is a hand-written model with a composite primary key and a
// nullable column.
type bulkStock struct {
	Region string
	SKU    int64
	Qty    int64
	Note   *string
}

var (
	bulkRegion = momo.NewColumn[string]("bulk_stock", "region", momo.FlagPK)
	bulkSKU    = momo.NewColumn[int64]("bulk_stock", "sku", momo.FlagPK)
	bulkQty    = momo.NewColumn[int64]("bulk_stock", "qty", 0)
	bulkNote   = momo.NewNullColumn[string]("bulk_stock", "note", momo.FlagNullable)
)

var bulkStockMeta = momo.Meta[bulkStock]{
	Table: "bulk_stock",
	Columns: []momo.ColumnMeta{
		{Name: "region", Flags: momo.FlagPK}, {Name: "sku", Flags: momo.FlagPK},
		{Name: "qty"}, {Name: "note", Flags: momo.FlagNullable},
	},
	Ptr:        func(m *bulkStock, i int) any { return [...]any{&m.Region, &m.SKU, &m.Qty, &m.Note}[i] },
	Value:      func(m *bulkStock, i int) any { return [...]any{m.Region, m.SKU, m.Qty, m.Note}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0, 1},
}

func (*bulkStock) MomoMeta() *momo.Meta[bulkStock] { return &bulkStockMeta }

func bulkDB(d *dialect.Dialect) *momo.DB {
	return momo.New(fakedriver.NewDialect(d))
}

// Catches: a NULL value dropped or bound as a zero value instead of NULL.
func TestBulkUpdateBindsNullValues(t *testing.T) {
	c, err := momo.BulkUpdateKeys[bulkStock](bulkDB(dialect.For(dialect.Postgres)), []momo.ColumnRef{bulkRegion, bulkSKU},
		[]momo.ColumnRef{bulkNote}, [][]any{{"eu", int64(7), nil}}).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Args) != 3 || c.Args[2] != nil {
		t.Errorf("args = %v, want eu, 7, nil", c.Args)
	}
}

// Catches: a dialect rendering the wrong statement shape (PostgreSQL and
// SQLite UPDATE ... FROM, MySQL and MariaDB UPDATE ... JOIN), a value inlined
// into the SQL, a lost updated_at or version assignment, and a join that does
// not key on the listed column.
func TestBulkUpdateGoldenSQLPerDialect(t *testing.T) {
	const src = ` FROM (SELECT "id", "title" FROM "documents" WHERE false UNION ALL SELECT `
	cases := []struct {
		name string
		d    *dialect.Dialect
		want string
	}{
		{"postgres", dialect.For(dialect.Postgres),
			`UPDATE "documents" SET "title" = "momo_v"."title", "updated_at" = $1, "version" = "documents"."version" + 1` +
				src + `$2, $3) AS "momo_v" WHERE "documents"."id" = "momo_v"."id"`},
		{"sqlite", dialect.For(dialect.SQLite),
			`UPDATE "documents" SET "title" = "momo_v"."title", "updated_at" = ?, "version" = "documents"."version" + 1` +
				src + `?, ?) AS "momo_v" WHERE "documents"."id" = "momo_v"."id"`},
		{"mysql", dialect.MySQLFlavor(dialect.FlavorMySQL), mysqlGolden},
		{"mariadb", dialect.MySQLFlavor(dialect.FlavorMariaDB), mysqlGolden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := bulkQ(bulkDB(tc.d), []any{int64(2), "new"}).SQL()
			if err != nil {
				t.Fatal(err)
			}
			if c.SQL != tc.want {
				t.Errorf("SQL =\n%s\nwant\n%s", c.SQL, tc.want)
			}
			if len(c.Args) != 3 {
				t.Errorf("args = %v, want updated_at, key and title", c.Args)
			}
		})
	}
}

const mysqlGolden = "UPDATE `documents` JOIN (SELECT `id`, `title` FROM `documents` WHERE false UNION ALL SELECT ?, ?) AS `momo_v` " +
	"ON `documents`.`id` = `momo_v`.`id` SET `documents`.`title` = `momo_v`.`title`, `documents`.`updated_at` = ?, " +
	"`documents`.`version` = `documents`.`version` + 1"

// Catches: the MySQL join form dropping the Where condition or placing it
// before the SET clause.
func TestBulkUpdateMySQLWhereFollowsSet(t *testing.T) {
	db := bulkDB(dialect.MySQLFlavor(dialect.FlavorMariaDB))
	c, err := bulkQ(db, []any{int64(2), "new"}).Where(suite.DocTenantID.Eq(1)).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(c.SQL, mysqlGolden+" WHERE ") || !strings.Contains(c.SQL, "`tenant_id`") {
		t.Errorf("SQL = %s", c.SQL)
	}
}

// Catches: a composite key matched on one column only (updating every row of
// a region), a lost AND between the key columns, and key and value args in the
// wrong order.
func TestBulkUpdateKeysRendersEveryKeyColumn(t *testing.T) {
	rows := [][]any{{"eu", int64(7), int64(40)}, {"us", int64(8), int64(41)}}
	for _, d := range []*dialect.Dialect{
		dialect.For(dialect.Postgres), dialect.For(dialect.SQLite),
		dialect.MySQLFlavor(dialect.FlavorMySQL), dialect.MySQLFlavor(dialect.FlavorMariaDB),
	} {
		c, err := momo.BulkUpdateKeys[bulkStock](bulkDB(d), []momo.ColumnRef{bulkRegion, bulkSKU}, []momo.ColumnRef{bulkQty}, rows).SQL()
		if err != nil {
			t.Fatalf("%s: %v", d.Name, err)
		}
		q := func(s string) string { return strings.NewReplacer("`", `"`).Replace(s) }
		hasAll(t, q(c.SQL),
			`"bulk_stock"."region" = "momo_v"."region" AND "bulk_stock"."sku" = "momo_v"."sku"`,
			`SELECT "region", "sku", "qty" FROM "bulk_stock" WHERE false UNION ALL SELECT `)
		want := []any{"eu", int64(7), int64(40), "us", int64(8), int64(41)}
		if len(c.Args) != len(want) {
			t.Fatalf("%s: args = %v, want %v", d.Name, c.Args, want)
		}
		for i := range want {
			if c.Args[i] != want[i] {
				t.Fatalf("%s: args = %v, want %v", d.Name, c.Args, want)
			}
		}
	}
}

// Catches: a malformed composite key list reaching the database.
func TestBulkUpdateKeysValidation(t *testing.T) {
	db := bulkDB(dialect.For(dialect.Postgres))
	keys := []momo.ColumnRef{bulkRegion, bulkSKU}
	cols := []momo.ColumnRef{bulkQty}
	cases := map[string]momo.UpdateQuery[bulkStock]{
		"no keys":       momo.BulkUpdateKeys[bulkStock](db, nil, cols, [][]any{{int64(1)}}),
		"nil key":       momo.BulkUpdateKeys[bulkStock](db, []momo.ColumnRef{bulkRegion, nil}, cols, [][]any{{"a", int64(1), int64(1)}}),
		"duplicate key": momo.BulkUpdateKeys[bulkStock](db, []momo.ColumnRef{bulkRegion, bulkRegion}, cols, [][]any{{"a", "a", int64(1)}}),
		"key as column": momo.BulkUpdateKeys[bulkStock](db, keys, []momo.ColumnRef{bulkSKU}, [][]any{{"a", int64(1), int64(1)}}),
		"short row":     momo.BulkUpdateKeys[bulkStock](db, keys, cols, [][]any{{"a", int64(1)}}),
		"long row":      momo.BulkUpdateKeys[bulkStock](db, keys, cols, [][]any{{"a", int64(1), int64(1), int64(1)}}),
	}
	for name, q := range cases {
		if _, err := q.SQL(); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: SQL = %v, want ErrValidation", name, err)
		}
	}
}

// Catches: a list over the bind-parameter limit sent as one statement, rows
// lost or repeated at a chunk boundary, the fixed parameters (updated_at)
// missed when sizing a chunk, a chunked run outside one transaction, and the
// single-statement SQL silently chunking.
func TestBulkUpdateChunksUnderTheParameterLimit(t *testing.T) {
	// Each statement binds updated_at plus 2 values per row: a limit of 9
	// leaves room for 4 rows.
	for _, d := range []*dialect.Dialect{dialect.For(dialect.Postgres), dialect.For(dialect.SQLite), dialect.MySQLFlavor(dialect.FlavorMariaDB)} {
		drv := fakedriver.NewDialect(d)
		db := momo.New(drv, momo.WithMaxParams(9))
		rows := make([][]any, 10)
		for i := range rows {
			rows[i] = []any{int64(i + 1), "t"}
		}
		if _, err := momo.BulkUpdate[suite.Document](db, suite.DocID, []momo.ColumnRef{suite.DocTitle}, rows).Exec(t.Context()); err != nil {
			t.Fatalf("%s: %v", d.Name, err)
		}
		calls := drv.Calls()
		var keys []int64
		for _, c := range calls {
			if len(c.Args) > 9 {
				t.Errorf("%s: a chunk binds %d parameters, limit 9", d.Name, len(c.Args))
			}
			for _, a := range c.Args {
				if k, ok := a.(int64); ok { // only the keys are int64 here
					keys = append(keys, k)
				}
			}
		}
		if len(calls) != 3 || len(keys) != 10 {
			t.Errorf("%s: %d statements with keys %v, want 3 statements and 10 keys", d.Name, len(calls), keys)
		}
		if drv.Commits() != 1 || drv.Rollbacks() != 0 {
			t.Errorf("%s: commits %d rollbacks %d, want one transaction", d.Name, drv.Commits(), drv.Rollbacks())
		}
		if _, err := momo.BulkUpdate[suite.Document](db, suite.DocID, []momo.ColumnRef{suite.DocTitle}, rows).SQL(); !errors.Is(err, momo.ErrLimitExceeded) {
			t.Errorf("%s: SQL over the limit = %v, want ErrLimitExceeded", d.Name, err)
		}
	}
}

// Catches: a row whose own parameters exceed the limit being sent anyway.
func TestBulkUpdateSingleRowOverTheLimit(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithMaxParams(3)) // updated_at, key, title is the minimum of 3, so a 2-column row cannot fit
	_, err := momo.BulkUpdate[suite.Document](db, suite.DocID, []momo.ColumnRef{suite.DocTitle, suite.DocBody},
		[][]any{{int64(1), "t", "b"}}).Exec(t.Context())
	if !errors.Is(err, momo.ErrLimitExceeded) {
		t.Fatalf("err = %v, want ErrLimitExceeded", err)
	}
	if len(drv.Calls()) != 0 {
		t.Fatalf("calls = %v, want none", drv.Calls())
	}
}

// Catches: RETURNING on MySQL and MariaDB reaching the server, which has no
// UPDATE ... RETURNING.
func TestBulkUpdateReturningUnsupportedOnMySQLFamily(t *testing.T) {
	for _, f := range []dialect.Flavor{dialect.FlavorMySQL, dialect.FlavorMariaDB} {
		drv := fakedriver.NewDialect(dialect.MySQLFlavor(f))
		_, err := bulkQ(momo.New(drv), []any{int64(2), "new"}).Returning(suite.DocID).Rows(t.Context())
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: Rows = %v, want ErrUnsupported", f, err)
		}
		if len(drv.Calls()) != 0 {
			t.Errorf("%s: calls = %v, want none", f, drv.Calls())
		}
	}
}

// Catches: RETURNING dropped from the PostgreSQL and SQLite statements.
func TestBulkUpdateReturningRendersOnPostgresAndSQLite(t *testing.T) {
	for _, n := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
		c, err := bulkQ(momo.New(fakedriver.New(n)), []any{int64(2), "new"}).Returning(suite.DocID).SQL()
		if err != nil || !strings.HasSuffix(c.SQL, ` RETURNING "id"`) {
			t.Errorf("%s: %q, %v", n, c.SQL, err)
		}
	}
}
