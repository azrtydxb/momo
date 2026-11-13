// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
)

type fakeRows struct {
	vals []string // one row of a single text column; empty means no row
	done bool
}

func (r *fakeRows) Next() bool {
	if r.done || len(r.vals) == 0 {
		return false
	}
	r.done = true
	return true
}

func (r *fakeRows) Scan(dest ...any) error { *(dest[0].(*string)) = r.vals[0]; return nil }
func (*fakeRows) Err() error               { return nil }
func (*fakeRows) Close() error             { return nil }

// fakeConn answers the lookup of the history table with no row and
// current_schema() with "cur", and records every statement.
type fakeConn struct {
	momo.PinnedConn
	sqls []string
	args [][]any
}

func (f *fakeConn) Query(_ context.Context, sql string, args []any) (momo.Rows, error) {
	f.sqls = append(f.sqls, sql)
	f.args = append(f.args, args)
	if strings.Contains(sql, "current_schema()") {
		return &fakeRows{vals: []string{"cur"}}, nil
	}
	return &fakeRows{}, nil
}

// TestHistoryTableWithHistorySchemaIsQualified catches a history lookup that
// goes through the search_path although a schema was pinned: it would find
// public.momo_migrations and skip every migration. With the option the one
// lookup names the schema, nothing reads current_schema(), and a new table is
// created in the pinned schema.
func TestHistoryTableWithHistorySchemaIsQualified(t *testing.T) {
	c := &fakeConn{}
	ref, err := mpg.New(mpg.WithHistorySchema(`te"st`)).HistoryTable(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"te""st"."momo_migrations"`; ref != want {
		t.Fatalf("HistoryTable = %s, want %s", ref, want)
	}
	if len(c.sqls) != 1 || !strings.Contains(c.sqls[0], "to_regclass($1)") {
		t.Fatalf("statements = %q, want one to_regclass($1) lookup", c.sqls)
	}
	if got, want := c.args[0][0], `"te""st".momo_migrations`; got != want {
		t.Fatalf("lookup argument = %v, want %s", got, want)
	}
	ddl := mpg.New(mpg.WithHistorySchema(`te"st`)).HistoryDDLFor(ref)
	if len(ddl) != 1 || !strings.HasPrefix(ddl[0], `CREATE TABLE IF NOT EXISTS "te""st"."momo_migrations" (`) {
		t.Fatalf("DDL = %q", ddl)
	}
}

// TestHistoryTableDefaultUnchanged catches an option that changes the default:
// without WithHistorySchema the lookup goes through the search_path with the
// bare name and a new table goes to current_schema().
func TestHistoryTableDefaultUnchanged(t *testing.T) {
	for name, d := range map[string]mpg.Dialect{"New()": mpg.New(), "empty schema": mpg.New(mpg.WithHistorySchema(""))} {
		c := &fakeConn{}
		ref, err := d.HistoryTable(t.Context(), c)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if ref != `"cur"."momo_migrations"` {
			t.Fatalf("%s: HistoryTable = %s", name, ref)
		}
		if len(c.sqls) != 2 || c.args[0][0] != "momo_migrations" {
			t.Fatalf("%s: statements %q args %v, want the bare-name lookup then current_schema()", name, c.sqls, c.args)
		}
	}
}
