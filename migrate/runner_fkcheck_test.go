// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo"
)

func TestForeignKeyCheckStatement(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		hint string
		want bool
		stmt string // the text that is run
	}{
		{"plain", "PRAGMA foreign_key_check", "", true, "PRAGMA foreign_key_check"},
		{"semicolon and case", "pragma Foreign_Key_Check ;\n", "", true, "pragma Foreign_Key_Check"},
		{"leading line comment", "-- rebuild step\nPRAGMA foreign_key_check;", "", true, "PRAGMA foreign_key_check"},
		{"leading block comment", "/* check */  \n PRAGMA foreign_key_check", "", true, "PRAGMA foreign_key_check"},
		{"several comments", "-- a\n-- b\n/* c */\nPRAGMA foreign_key_check", "", true, "PRAGMA foreign_key_check"},
		{"schema", "PRAGMA main.foreign_key_check", "", true, "PRAGMA main.foreign_key_check"},
		{"quoted schema", `PRAGMA "main".foreign_key_check`, "", true, `PRAGMA "main".foreign_key_check`},
		{"table argument", "PRAGMA foreign_key_check(items)", "", true, "PRAGMA foreign_key_check(items)"},
		{"quoted table argument", `PRAGMA foreign_key_check( "items" );`, "", true, `PRAGMA foreign_key_check( "items" )`},
		{"schema and table", "PRAGMA main.foreign_key_check('items')", "", true, "PRAGMA main.foreign_key_check('items')"},
		{"hint without text match", "SELECT 1", "fk-check", true, ""},
		{"trailing line comment", "PRAGMA foreign_key_check; -- done", "", true, "PRAGMA foreign_key_check"},
		{"trailing block comment", "PRAGMA foreign_key_check /* done */;", "", true, "PRAGMA foreign_key_check"},
		{"comment between words", "PRAGMA /*x*/ foreign_key_check", "", true, "PRAGMA   foreign_key_check"},
		{"dashes in an argument", "PRAGMA foreign_key_check('a--b')", "", true, "PRAGMA foreign_key_check('a--b')"},
		{"another pragma", "PRAGMA foreign_keys = OFF", "", false, ""},
		{"integrity check", "PRAGMA integrity_check", "", false, ""},
		{"trailing statement", "PRAGMA foreign_key_check; DROP TABLE t", "", false, ""},
		{"inside a comment only", "-- PRAGMA foreign_key_check", "", false, ""},
		{"unterminated comment", "/* PRAGMA foreign_key_check", "", false, ""},
		{"set form", "PRAGMA foreign_key_check = 1", "", false, ""},
		{"plain statement", "CREATE TABLE foreign_key_check (a int)", "", false, ""},
	}
	for _, c := range cases {
		if got := isForeignKeyCheck(Statement{SQL: c.sql, Hint: c.hint}); got != c.want {
			t.Errorf("%s: isForeignKeyCheck = %v, want %v", c.name, got, c.want)
		}
		if c.want && c.hint == "" {
			if stmt, ok := foreignKeyCheckSQL(c.sql); !ok || stmt != c.stmt {
				t.Errorf("%s: foreignKeyCheckSQL = %q, %v; want %q", c.name, stmt, ok, c.stmt)
			}
		}
	}
}

// recordingExecer records the statements it is asked to run.
type recordingExecer struct{ ran []string }

func (r *recordingExecer) Exec(_ context.Context, sql string, _ []any) (int64, error) {
	r.ran = append(r.ran, sql)
	return 0, nil
}

// TestForeignKeyCheckFailsClosed checks that a statement naming foreign_key_check
// in a form the matcher does not know is refused instead of run without its
// check, and that unrelated statements run.
func TestForeignKeyCheckFailsClosed(t *testing.T) {
	for _, sql := range []string{
		"PRAGMA foreign_key_check; DROP TABLE t",
		"PRAGMA foreign_key_check = 1",
		"-- c\nPRAGMA foreign_key_check(a, b)",
	} {
		x := &recordingExecer{}
		err := execStatement(context.Background(), x, Statement{SQL: sql})
		if !errors.Is(err, momo.ErrValidation) || len(x.ran) != 0 {
			t.Errorf("%q: err = %v, ran = %v; want ErrValidation and nothing run", sql, err, x.ran)
		}
	}
	x := &recordingExecer{}
	if err := execStatement(context.Background(), x, Statement{SQL: "CREATE TABLE foreign_key_check (a int)"}); err != nil || len(x.ran) != 1 {
		t.Errorf("an unrelated statement must run: %v %v", err, x.ran)
	}
}

type stateError struct{ state string }

func (e stateError) Error() string    { return "boom " + e.state }
func (e stateError) SQLState() string { return e.state }

func TestIsUndefinedTable(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{stateError{"42P01"}, true},
		{stateError{"42S02"}, true},
		{stateError{"23505"}, false},
		{errors.New("no such table: momo_migrations"), true},
		{fmt.Errorf("migrate: read history: %w", errors.New("no such table: main.momo_migrations")), true},
		{errors.New("no such table: users"), false},
		{errors.New("no such column: momo_migrations"), false},
	}
	for _, c := range cases {
		if got := isUndefinedTable(c.err); got != c.want {
			t.Errorf("isUndefinedTable(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}
