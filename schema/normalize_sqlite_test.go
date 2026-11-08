// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestNormalizeSQLiteIdentity(t *testing.T) {
	cols := func(id Column, more ...Column) []Column { return append([]Column{id}, more...) }
	cases := []struct {
		name string
		d    dialect.Name
		tbl  Table
		want string // identity of column "id" after Normalized
	}{
		{"integer key is the rowid", dialect.SQLite, Table{Name: "t", Columns: cols(Column{Name: "id", Type: "INTEGER"}), PrimaryKey: []string{"id"}}, "by_default"},
		{"lower-case type", dialect.SQLite, Table{Name: "t", Columns: cols(Column{Name: "id", Type: "integer"}), PrimaryKey: []string{"id"}}, "by_default"},
		{"always becomes by_default", dialect.SQLite, Table{Name: "t", Columns: cols(Column{Name: "id", Type: "INTEGER", Identity: "always"}), PrimaryKey: []string{"id"}}, "by_default"},
		{"by_default stays", dialect.SQLite, Table{Name: "t", Columns: cols(Column{Name: "id", Type: "INTEGER", Identity: "by_default"}), PrimaryKey: []string{"id"}}, "by_default"},
		{"always on a composite key still becomes by_default", dialect.SQLite, Table{Name: "t", Columns: cols(Column{Name: "id", Type: "INTEGER", Identity: "always"}, Column{Name: "b", Type: "INTEGER"}), PrimaryKey: []string{"id", "b"}}, "by_default"},
		{"composite key is not a rowid", dialect.SQLite, Table{Name: "t", Columns: cols(Column{Name: "id", Type: "INTEGER"}, Column{Name: "b", Type: "INTEGER"}), PrimaryKey: []string{"id", "b"}}, ""},
		{"WITHOUT ROWID has no rowid alias", dialect.SQLite, Table{Name: "t", WithoutRowID: true, Columns: cols(Column{Name: "id", Type: "INTEGER"}), PrimaryKey: []string{"id"}}, ""},
		{"bigint key is not the rowid", dialect.SQLite, Table{Name: "t", Columns: cols(Column{Name: "id", Type: "BIGINT"}), PrimaryKey: []string{"id"}}, ""},
		{"text key", dialect.SQLite, Table{Name: "t", Columns: cols(Column{Name: "id", Type: "TEXT"}), PrimaryKey: []string{"id"}}, ""},
		{"no key", dialect.SQLite, Table{Name: "t", Columns: cols(Column{Name: "id", Type: "INTEGER"})}, ""},
		{"key is another column", dialect.SQLite, Table{Name: "t", Columns: cols(Column{Name: "id", Type: "INTEGER"}, Column{Name: "k", Type: "INTEGER"}), PrimaryKey: []string{"k"}}, ""},
		{"serial is left alone", dialect.SQLite, Table{Name: "t", Columns: cols(Column{Name: "id", Type: "INTEGER", Identity: "serial"}), PrimaryKey: []string{"id"}}, "serial"},
		{"PostgreSQL integer key is untouched", dialect.Postgres, Table{Name: "t", Columns: cols(Column{Name: "id", Type: "integer"}), PrimaryKey: []string{"id"}}, ""},
		{"PostgreSQL always is untouched", dialect.Postgres, Table{Name: "t", Columns: cols(Column{Name: "id", Type: "integer", Identity: "always"}), PrimaryKey: []string{"id"}}, "always"},
	}
	for _, c := range cases {
		s := &Schema{Dialect: c.d, Tables: []Table{c.tbl}}
		n := Normalized(s)
		col, _ := n.Tables[0].Column("id")
		if col.Identity != c.want {
			t.Errorf("%s: identity %q, want %q", c.name, col.Identity, c.want)
		}
		if !Equal(n, Normalized(n)) {
			t.Errorf("%s: not idempotent", c.name)
		}
		if orig, _ := s.Tables[0].Column("id"); orig.Identity != c.tbl.Columns[0].Identity {
			t.Errorf("%s: Normalized modified its argument", c.name)
		}
	}
}

func TestNormalizeDefinition(t *testing.T) {
	cases := []struct {
		name string
		d    dialect.Name
		in   string
		want string
	}{
		{"empty", dialect.SQLite, "", ""},
		{"trailing semicolon and case", dialect.SQLite, "CREATE TRIGGER t AFTER INSERT ON x BEGIN SELECT 1; END;", "create trigger t after insert on x begin select 1; end"},
		{"if not exists is dropped", dialect.SQLite, "CREATE TRIGGER IF NOT EXISTS t AFTER INSERT ON x BEGIN SELECT 1; END", "create trigger t after insert on x begin select 1; end"},
		{"whitespace and newlines", dialect.SQLite, "CREATE  TRIGGER t\n  AFTER INSERT ON x\nBEGIN\n  SELECT 1;\nEND ;\n", "create trigger t after insert on x begin select 1; end"},
		{"quoted text is kept", dialect.SQLite, "CREATE TRIGGER t AFTER INSERT ON x WHEN NEW.a = 'Ab  C;' BEGIN SELECT 1; END", "create trigger t after insert on x when new.a = 'Ab  C;' begin select 1; end"},
		{"several semicolons", dialect.SQLite, "CREATE TRIGGER t AFTER INSERT ON x BEGIN SELECT 1; END;;; ", "create trigger t after insert on x begin select 1; end"},
		{"PostgreSQL is untouched", dialect.Postgres, "CREATE  TRIGGER X;", "CREATE  TRIGGER X;"},
	}
	for _, c := range cases {
		got := normalizeDefinition(c.d, c.in)
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
		if again := normalizeDefinition(c.d, got); again != got {
			t.Errorf("%s: not idempotent: %q then %q", c.name, got, again)
		}
	}
}
