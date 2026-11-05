// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// Each case names the scanner rule it pins; a mutant that drops the rule
// changes the rendered SQL, the bound count or the error.
func TestRawScannerRules(t *testing.T) {
	pg, lite, my := dialect.Postgres, dialect.SQLite, dialect.MySQL
	cases := []struct {
		name string
		d    dialect.Name
		sql  string
		args []any
		want string
		n    int
	}{
		{"mysql backslash in single quotes", my, `SELECT 'a\'', ?`, []any{1}, `SELECT 'a\'', ?`, 1},
		{"mysql backslash in double quotes", my, `SELECT "a\"", ?`, []any{1}, `SELECT "a\"", ?`, 1},
		{"pg backslash is plain in single quotes", pg, `SELECT 'a\', ?`, []any{1}, `SELECT 'a\', $1`, 1},
		{"pg E-string escapes", pg, `SELECT E'a\'', ?`, []any{1}, `SELECT E'a\'', $1`, 1},
		{"pg E-string at start of sql", pg, `E'a\'', ?`, []any{1}, `E'a\'', $1`, 1},
		{"pg E after identifier byte is not an E-string", pg, `SELECT CASE'a\' END, ?`, []any{1}, `SELECT CASE'a\' END, $1`, 1},
		{"mysql double dash needs a space", my, `SELECT 5--?`, []any{1}, `SELECT 5--?`, 1},
		{"mysql double dash with space is a comment", my, "SELECT ? -- ?", []any{1}, "SELECT ? -- ?", 1},
		{"pg double dash is a comment without space", pg, "SELECT ? --?", []any{1}, "SELECT $1 --?", 1},
		{"at after identifier byte is text", pg, "SELECT x@y, ?", []any{1}, "SELECT x@y, $1", 1},
		{"at after identifier byte is text mysql", my, "SELECT x@y, ?", []any{1}, "SELECT x@y, ?", 1},
		{"dollar inside identifier is not a quote", pg, "SELECT x$a$, ?, y$a$", []any{1}, "SELECT x$a$, $1, y$a$", 1},
		{"pg contains-operator at-less", pg, "a <@b, ?", []any{1}, "a <@b, $1", 1},
		{"lt at is a named bind off pg", lite, "x<@b", []any{NamedArg{"b", 1}}, "x<?", 1},
		{"pg named after space", pg, "x < @b", []any{NamedArg{"b", 1}}, "x < $1", 1},
		{"native alone is left alone", pg, "SELECT $1", nil, "SELECT $1", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := rawCompile(c.d, c.sql, c.args...)
			if err != nil {
				t.Fatal(err)
			}
			if got.SQL != c.want || len(got.Args) != c.n {
				t.Fatalf("SQL = %q (%d args)\nwant  %q (%d args)", got.SQL, len(got.Args), c.want, c.n)
			}
		})
	}
}

func TestRawBuildErrors(t *testing.T) {
	pg, lite, my := dialect.Postgres, dialect.SQLite, dialect.MySQL
	cases := []struct {
		name string
		d    dialect.Name
		sql  string
		args []any
		frag string
	}{
		{"unused named pg", pg, "a = ?", []any{1, NamedArg{"x", 1}}, "uses it"},
		{"unused named sqlite", lite, "a = @a", []any{NamedArg{"a", 1}, NamedArg{"b", 2}}, "Named(\"b\""},
		{"unused named mysql typo", my, "a IN (@ids)", []any{NamedArg{"id", 1}}, "Named(\"id\""},
		{"named never matches mysql system variable", my, "SELECT @@sql_mode", []any{NamedArg{"sql_mode", 1}}, "uses it"},
		{"named never matches after identifier byte", pg, "SELECT x@y", []any{NamedArg{"y", 1}}, "uses it"},
		{"native mixed with positional", pg, "SELECT $1, ?", []any{1}, "native $n"},
		{"native mixed with named", pg, "SELECT $1, @a", []any{NamedArg{"a", 1}}, "native $n"},
		{"unterminated string", pg, "SELECT 'abc ?", []any{1}, "unterminated string literal"},
		{"unterminated string mysql escape", my, `SELECT 'a\'`, nil, "unterminated string literal"},
		{"unterminated quoted identifier", pg, `SELECT "abc ?`, []any{1}, "unterminated quoted identifier"},
		{"unterminated backtick", my, "SELECT `abc ?", []any{1}, "unterminated quoted identifier"},
		{"unterminated bracket", lite, "SELECT [abc ?", []any{1}, "unterminated quoted identifier"},
		{"unterminated block comment", pg, "SELECT /* ? ", []any{1}, "unterminated block comment"},
		{"unterminated nested block comment", pg, "SELECT /* a /* b */ ?", []any{1}, "unterminated block comment"},
		{"unterminated dollar quote", pg, "SELECT $a$ ? ", []any{1}, "unterminated dollar quote"},
		{"identifier then dollar opener", pg, "x$a$, ? $a$", []any{1}, "unterminated dollar quote"},
		{"In of Ident", pg, "a IN (?)", []any{In([]any{Ident("a")})}, "In element"},
		{"In of In", pg, "a IN (?)", []any{In([]any{In([]int{1})})}, "In element"},
		{"In of Named", pg, "a IN (?)", []any{In([]any{NamedArg{"a", 1}})}, "In element"},
		{"In of Expr", pg, "a IN (?)", []any{In([]any{TypedSQL[int]("1")})}, "In element"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := rawCompile(c.d, c.sql, c.args...)
			if err == nil || !strings.Contains(err.Error(), c.frag) {
				t.Fatalf("err = %v, want one containing %q", err, c.frag)
			}
		})
	}
}

func TestRawInBytesExpandsElements(t *testing.T) {
	c, err := rawCompile(dialect.Postgres, "a IN (?)", In([]byte{1, 2}))
	if err != nil || c.SQL != "a IN ($1, $2)" || len(c.Args) != 2 {
		t.Fatalf("%q %v %v", c.SQL, c.Args, err)
	}
}

func TestRawPlainBindsHonourMaxParams(t *testing.T) {
	d := dialect.For(dialect.SQLite)
	sql := strings.TrimSuffix(strings.Repeat("?,", d.MaxParams+1), ",")
	args := make([]any, d.MaxParams+1)
	if _, err := rawCompile(dialect.SQLite, sql, args...); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("err = %v, want the limit error", err)
	}
	sql = strings.TrimSuffix(strings.Repeat("?,", d.MaxParams), ",")
	if _, err := rawCompile(dialect.SQLite, sql, args[:d.MaxParams]...); err != nil {
		t.Fatalf("exactly MaxParams must pass: %v", err)
	}
}
