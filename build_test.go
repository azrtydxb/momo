// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestPlaceholdersAndQuoting(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	c, err := momo.Select[unitDocument](pg).Where(DocTenantID.Eq(7), DocTitle.Eq("o'brien")).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.SQL, `"documents"."tenant_id" = $1`) || !strings.Contains(c.SQL, `"documents"."title" = $2`) {
		t.Fatalf("postgres numbering wrong: %s", c.SQL)
	}
	if strings.Contains(c.SQL, "o'brien") {
		t.Fatalf("value interpolated into SQL: %s", c.SQL)
	}
	if len(c.Args) != 2 || c.Args[0] != int64(7) || c.Args[1] != "o'brien" {
		t.Fatalf("args wrong: %#v", c.Args)
	}

	sq := momo.New(fakedriver.New(dialect.SQLite))
	c, err = momo.Select[unitDocument](sq).Where(DocTitle.Eq("x")).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(c.SQL, "$") || !strings.Contains(c.SQL, `"documents"."title" = ?`) {
		t.Fatalf("sqlite placeholders wrong: %s", c.SQL)
	}
}

// FuzzBuilder: identifiers are always quoted (quotes doubled) and the value
// is always bound, never present in the SQL text.
func FuzzBuilder(f *testing.F) {
	f.Add("title", "x")
	f.Add(`a"b`, "'; DROP TABLE t; --")
	f.Fuzz(func(t *testing.T, name, value string) {
		for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
			db := momo.New(fakedriver.New(d))
			col := momo.NewColumn[string]("documents", name, 0)
			c, err := momo.Select[unitDocument](db).Where(col.Eq(value)).SQL()
			if err != nil {
				t.Fatal(err)
			}
			quoted := `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
			if !strings.Contains(c.SQL, quoted) {
				t.Fatalf("%s: identifier %q not quoted in %s", d, name, c.SQL)
			}
			if len(c.Args) != 1 || c.Args[0] != value {
				t.Fatalf("%s: value not bound: %#v", d, c.Args)
			}
			// The identifier is fuzzed too: remove it before looking for the
			// value, or a name that spells "'value'" is a false positive.
			if value != "" && strings.Contains(strings.ReplaceAll(c.SQL, quoted, ""), "'"+value+"'") {
				t.Fatalf("%s: value interpolated: %s", d, c.SQL)
			}
		}
	})
}

// Catches: Builder.AppendFilter losing the FILTER text or its bound argument,
// accepting a nil predicate, or rendering FILTER where MySQL has none.
func TestBuilderAppendFilter(t *testing.T) {
	n := momo.NewColumn[int64]("docs", "n", 0)

	pg := momo.NewBuilder(dialect.For(dialect.Postgres))
	pg.WriteString("count(*)")
	pg.AppendFilter(n.Gt(1))
	if pg.Err() != nil || pg.String() != `count(*) FILTER (WHERE "docs"."n" > $1)` || len(pg.Args()) != 1 || pg.Args()[0] != int64(1) {
		t.Errorf("postgres: got %q %v, %v", pg.String(), pg.Args(), pg.Err())
	}

	nilPred := momo.NewBuilder(dialect.For(dialect.Postgres))
	nilPred.AppendFilter(nil)
	if !errors.Is(nilPred.Err(), momo.ErrValidation) {
		t.Errorf("nil predicate: err = %v; want ErrValidation", nilPred.Err())
	}

	my := momo.NewBuilder(dialect.For(dialect.MySQL))
	my.AppendFilter(n.Gt(1))
	if !errors.Is(my.Err(), momo.ErrUnsupported) {
		t.Errorf("mysql: err = %v; want ErrUnsupported", my.Err())
	}
}
