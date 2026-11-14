// SPDX-License-Identifier: Apache-2.0

//go:build integration

package jsonop_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/jsonop"
	"github.com/azrtydxb/momo/sqlfn"
	"github.com/azrtydxb/momo/types"
)

// eachAllDB runs fn on PostgreSQL, SQLite, MySQL 8.4, MySQL 9.7 and MariaDB
// 11.8, one subtest each. testopen.Each covers only the first two, so the MySQL
// servers are opened by name; a server whose URL is unset is skipped, and
// failed under MOMO_REQUIRE_INTEGRATION=1 for the dialects the job requires.
func eachAllDB(t *testing.T, fn func(t *testing.T, db *momo.DB, d dialect.Name)) {
	t.Helper()
	for _, s := range []struct {
		name string
		d    dialect.Name
		open func(*testing.T, ...momo.Option) *momo.DB
	}{
		{"postgres", dialect.Postgres, testopen.Postgres},
		{"sqlite", dialect.SQLite, testopen.SQLite},
		{"mysql84", dialect.MySQL, testopen.MySQL84},
		{"mysql97", dialect.MySQL, testopen.MySQL97},
		{"mariadb118", dialect.MySQL, testopen.MariaDB},
	} {
		t.Run(s.name, func(t *testing.T) {
			fn(t, s.open(t), s.d)
		})
	}
}

// seedTyped creates table with an id and a nullable JSON column v holding the
// given documents; a nil document is SQL NULL.
func seedTyped(t *testing.T, db *momo.DB, d dialect.Name, table string, docs []any) {
	t.Helper()
	ctx := context.Background()
	colType, insert := "JSON", "INSERT INTO "+table+" (id, v) VALUES (?, ?)"
	switch d {
	case dialect.Postgres:
		colType, insert = "jsonb", "INSERT INTO "+table+" (id, v) VALUES (?, CAST(? AS jsonb))"
	case dialect.SQLite:
		colType = "text" // CAST(x AS jsonb) would be a numeric cast on SQLite
	}
	for _, stmt := range []string{
		"DROP TABLE IF EXISTS " + table,
		"CREATE TABLE " + table + " (id bigint PRIMARY KEY, v " + colType + ")",
	} {
		if _, err := momo.Exec(ctx, db, stmt); err != nil {
			t.Fatal(err)
		}
	}
	for i, doc := range docs {
		if _, err := momo.Exec(ctx, db, insert, int64(i+1), doc); err != nil {
			t.Fatalf("insert %d: %v", i+1, err)
		}
	}
}

type typeRow struct {
	ID int64
	T  *string
}

var typeRowMeta = momo.Meta[typeRow]{
	Columns:  []momo.ColumnMeta{{Name: "id"}, {Name: "t"}},
	Ptr:      func(m *typeRow, i int) any { return [...]any{&m.ID, &m.T}[i] },
	Value:    func(m *typeRow, i int) any { return [...]any{m.ID, m.T}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*typeRow) MomoMeta() *momo.Meta[typeRow] { return &typeRowMeta }

type floatRow struct {
	ID int64
	F  float64
}

var floatRowMeta = momo.Meta[floatRow]{
	Columns:  []momo.ColumnMeta{{Name: "id"}, {Name: "f"}},
	Ptr:      func(m *floatRow, i int) any { return [...]any{&m.ID, &m.F}[i] },
	Value:    func(m *floatRow, i int) any { return [...]any{m.ID, m.F}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*floatRow) MomoMeta() *momo.Meta[floatRow] { return &floatRowMeta }

// Catches: TypeOf returning a dialect's own type names (integer, text, DOUBLE,
// true) instead of PostgreSQL's six, the JSON null read as SQL NULL or the SQL
// NULL read as the text "null", and a JSON type missing from the MySQL or
// SQLite mapping (which would read back as NULL). Runs on PostgreSQL, SQLite,
// MySQL 8.4, MySQL 9.7 and MariaDB 11.8.
func TestTypeOfAgainstDatabase(t *testing.T) {
	eachAllDB(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		docs := []any{
			`{"a":1}`,              // 1 object
			`[1,2]`,                // 2 array
			`"str"`,                // 3 string
			`1`,                    // 4 whole number
			`1.5`,                  // 5 fractional number
			`true`,                 // 6 boolean
			`false`,                // 7 boolean
			`null`,                 // 8 JSON null
			nil,                    // 9 SQL NULL
			`-7`,                   // 10 negative number
			`12345678901234567890`, // 11 number beyond int64
			`{}`,                   // 12 empty object
			`[]`,                   // 13 empty array
			`""`,                   // 14 empty string
		}
		seedTyped(t, db, d, "typeof_docs", docs)
		want := []string{"object", "array", "string", "number", "number", "boolean", "boolean", "null", "",
			"number", "number", "object", "array", "string"}
		v := momo.NewNullColumn[types.JSON[any]]("typeof_docs", "v", 0)
		id := momo.ColumnOf[int64]("typeof_docs", "id")
		rows, err := momo.Select[typeRow](db).From(momo.TableRef{Name: "typeof_docs"}).
			Columns(id, jsonop.TypeOf(v)).OrderBy(momo.Asc(id)).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != len(want) {
			t.Fatalf("%d rows, want %d", len(rows), len(want))
		}
		for i, r := range rows {
			switch {
			case want[i] == "" && r.T != nil:
				t.Errorf("row %d (SQL NULL) = %q, want NULL", r.ID, *r.T)
			case want[i] != "" && r.T == nil:
				t.Errorf("row %d (%v) = NULL, want %q", r.ID, docs[i], want[i])
			case want[i] != "" && *r.T != want[i]:
				t.Errorf("row %d (%v) = %q, want %q", r.ID, docs[i], *r.T, want[i])
			}
		}
		// The result compares like a string: 'number' selects the four numbers.
		numbers, err := momo.Select[typeRow](db).From(momo.TableRef{Name: "typeof_docs"}).
			Columns(id, jsonop.TypeOf(v)).Where(jsonop.TypeOf(v).Eq("number")).All(context.Background())
		if err != nil || len(numbers) != 4 {
			t.Errorf("TypeOf = 'number' gave %d rows (%v), want 4", len(numbers), err)
		}
	})
}

// Catches: the retention shape of issue 131, GREATEST(1, CASE WHEN
// jsonb_typeof(v) = 'number' THEN floor(v::numeric) ELSE default END), failing
// on a dialect: a TypeOf that does not compare equal to 'number', a Floor over a
// cast JSON root, a Greatest that returns NULL for the NULL setting, or a CASE
// that evaluates the cast of a non-number. Runs on PostgreSQL, SQLite, MySQL
// 8.4, MySQL 9.7 and MariaDB 11.8.
func TestRetentionShapeAgainstDatabase(t *testing.T) {
	eachAllDB(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedTyped(t, db, d, "tunables", []any{
			`0.5`,     // 1 number below the floor of one
			`2.7`,     // 2
			`7.9`,     // 3
			`"x"`,     // 4 string: default
			`true`,    // 5 boolean: default
			`null`,    // 6 JSON null: default
			nil,       // 7 SQL NULL (no setting): default
			`{"a":1}`, // 8 object: default
		})
		castType := map[dialect.Name]string{dialect.Postgres: "double precision", dialect.SQLite: "real", dialect.MySQL: "double"}[d]
		v := momo.NewNullColumn[types.JSON[any]]("tunables", "v", 0)
		id := momo.ColumnOf[int64]("tunables", "id")
		years := sqlfn.Greatest[float64](
			momo.Lit(1.0),
			sqlfn.Case[float64]().
				WhenExpr(jsonop.TypeOf(v).Eq("number"), sqlfn.Floor[float64](sqlfn.Cast[float64](jsonop.Root(v), castType))).
				Else(5))
		rows, err := momo.Select[floatRow](db).From(momo.TableRef{Name: "tunables"}).
			Columns(id, years).OrderBy(momo.Asc(id)).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		want := []float64{1, 2, 7, 5, 5, 5, 5, 5}
		if len(rows) != len(want) {
			t.Fatalf("%d rows, want %d", len(rows), len(want))
		}
		for i, r := range rows {
			if r.F != want[i] {
				t.Errorf("row %d = %v, want %v", r.ID, r.F, want[i])
			}
		}
	})
}
