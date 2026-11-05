// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
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
			col := momo.NewColumn[string]("tbl", name, 0)
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
