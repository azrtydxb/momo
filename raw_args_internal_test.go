// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func rawCompile(d dialect.Name, sql string, args ...any) (Compiled, error) {
	return compile(dialect.For(d), func(b *Builder) { b.appendRaw(sql, args) })
}

// Table-driven: placeholders inside literals, quoted identifiers, comments,
// dollar quotes and PostgreSQL operators are never rewritten or counted.
// Would fail if the scanner treated every '?' or '@' as a placeholder.
func TestRawTokenizerLeavesLiteralsAlone(t *testing.T) {
	type tc struct {
		name string
		d    dialect.Name
		sql  string
		args []any
		want string
		n    int // expected bound args
	}
	pg, lite, my := dialect.Postgres, dialect.SQLite, dialect.MySQL
	cases := []tc{
		{"plain", pg, "a = ? AND b = ?", []any{1, 2}, "a = $1 AND b = $2", 2},
		{"plain sqlite", lite, "a = ? AND b = ?", []any{1, 2}, "a = ? AND b = ?", 2},
		{"string literal", pg, "SELECT '?' , ?", []any{1}, "SELECT '?' , $1", 1},
		{"string doubled quote", pg, "SELECT 'it''s ?', ?", []any{1}, "SELECT 'it''s ?', $1", 1},
		{"string at-name", pg, "SELECT '@x', ?", []any{1}, "SELECT '@x', $1", 1},
		{"quoted ident", pg, `SELECT "wh?y", ?`, []any{1}, `SELECT "wh?y", $1`, 1},
		{"quoted ident doubled", pg, `SELECT "a""?", ?`, []any{1}, `SELECT "a""?", $1`, 1},
		{"backtick ident", my, "SELECT `a?`, ?", []any{1}, "SELECT `a?`, ?", 1},
		{"bracket ident sqlite", lite, "SELECT [a?], ?", []any{1}, "SELECT [a?], ?", 1},
		{"line comment", pg, "SELECT 1 -- why ?\n, ?", []any{1}, "SELECT 1 -- why ?\n, $1", 1},
		{"line comment eof", pg, "SELECT ? -- ?", []any{1}, "SELECT $1 -- ?", 1},
		{"block comment", pg, "SELECT /* ? @x */ ?", []any{1}, "SELECT /* ? @x */ $1", 1},
		{"nested block comment pg", pg, "SELECT /* a /* ? */ ? */ ?", []any{1}, "SELECT /* a /* ? */ ? */ $1", 1},
		{"block comment not nested sqlite", lite, "SELECT /* a /* ? */ ?", []any{1}, "SELECT /* a /* ? */ ?", 1},
		{"mysql hash comment", my, "SELECT 1 # ?\n, ?", []any{1}, "SELECT 1 # ?\n, ?", 1},
		{"hash is not a comment on pg", pg, "SELECT '{}'::jsonb #> '{a}', ?", []any{1}, "SELECT '{}'::jsonb #> '{a}', $1", 1},
		{"dollar quote", pg, "SELECT $$ ? $$, ?", []any{1}, "SELECT $$ ? $$, $1", 1},
		{"tagged dollar quote", pg, "SELECT $f$ ? $$ ' $f$, ?", []any{1}, "SELECT $f$ ? $$ ' $f$, $1", 1},
		{"escape string", pg, `SELECT E'it\'s ?', ?`, []any{1}, `SELECT E'it\'s ?', $1`, 1},
		{"mysql backslash escape", my, `SELECT 'it\'s ?', ?`, []any{1}, `SELECT 'it\'s ?', ?`, 1},
		{"jsonb has-key", pg, "SELECT data ?? 'k' FROM t WHERE x = ?", []any{1}, "SELECT data ? 'k' FROM t WHERE x = $1", 1},
		{"jsonb any-key", pg, "SELECT data ?| array['a'] FROM t WHERE x = ?", []any{1}, "SELECT data ?| array['a'] FROM t WHERE x = $1", 1},
		{"jsonb all-keys", pg, "SELECT data ?& array['a'] FROM t WHERE x = ?", []any{1}, "SELECT data ?& array['a'] FROM t WHERE x = $1", 1},
		{"bind before concat", pg, "SELECT ? || 'x'", []any{"a"}, "SELECT $1 || 'x'", 1},
		{"bind concat nospace", pg, "SELECT ?||'x'", []any{"a"}, "SELECT $1||'x'", 1},
		{"jsonpath exists", pg, "SELECT data @? '$.a' FROM t WHERE x = ?", []any{1}, "SELECT data @? '$.a' FROM t WHERE x = $1", 1},
		{"jsonpath match and contains", pg, "SELECT data @@ '$.a', data @> '{}', ?", []any{1}, "SELECT data @@ '$.a', data @> '{}', $1", 1},
		{"cast", pg, "SELECT ?::int, a::text", []any{1}, "SELECT $1::int, a::text", 1},
		{"json arrows", pg, "SELECT data->>'a', data->'b'->>?", []any{"c"}, "SELECT data->>'a', data->'b'->>$1", 1},
		{"native dollar left alone", pg, "SELECT $1, ?", []any{1}, "SELECT $1, $1", 1},
		{"mysql user variable", my, "SELECT @v, @@sql_mode, ?", []any{1}, "SELECT @v, @@sql_mode, ?", 1},
		{"named", pg, "a = @s AND b = @s", []any{NamedArg{"s", 1}}, "a = $1 AND b = $2", 2},
		{"named and positional", lite, "a = @s AND b = ?", []any{NamedArg{"s", 1}, 2}, "a = ? AND b = ?", 2},
		{"at inside word is not named", pg, "SELECT 'a@b', x@y", nil, "SELECT 'a@b', x@y", 0},
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

func TestRawNamedMissingAndMismatch(t *testing.T) {
	if _, err := rawCompile(dialect.Postgres, "a = @nope", NamedArg{"s", 1}); err == nil || !strings.Contains(err.Error(), "@nope") {
		t.Fatalf("missing named: err = %v", err)
	}
	if _, err := rawCompile(dialect.Postgres, "a = @s", NamedArg{"s", 1}, NamedArg{"s", 2}); err == nil {
		t.Fatal("duplicate named argument must fail")
	}
	if _, err := rawCompile(dialect.Postgres, "a = ?", NamedArg{"s", 1}); err == nil {
		t.Fatal("named arguments do not satisfy positional placeholders")
	}
	if _, err := rawCompile(dialect.Postgres, "a = ?", 1, 2); err == nil {
		t.Fatal("extra positional argument must fail")
	}
	// MySQL user variable that is not a supplied named argument stays verbatim.
	c, err := rawCompile(dialect.MySQL, "SET @v = @s", NamedArg{"s", 1})
	if err != nil || c.SQL != "SET @v = ?" {
		t.Fatalf("mysql: %q %v", c.SQL, err)
	}
}

func TestRawInExpansionAndIdent(t *testing.T) {
	c, err := rawCompile(dialect.Postgres, "a IN (?) AND b = ?", In([]int{1, 2, 3}), "z")
	if err != nil || c.SQL != "a IN ($1, $2, $3) AND b = $4" || len(c.Args) != 4 {
		t.Fatalf("%q %v %v", c.SQL, c.Args, err)
	}
	c, err = rawCompile(dialect.MySQL, "SELECT ? FROM t WHERE ? = 1", Ident("a`b"), Ident("c"))
	if err != nil || c.SQL != "SELECT `a``b` FROM t WHERE `c` = 1" || len(c.Args) != 0 {
		t.Fatalf("%q %v %v", c.SQL, c.Args, err)
	}
	if _, err := rawCompile(dialect.Postgres, "a IN (?)", In("notaslice")); err == nil {
		t.Fatal("In of a non-slice must be a build error")
	}
	if _, err := rawCompile(dialect.Postgres, "?", Ident("")); err == nil {
		t.Fatal("empty identifier must be a build error")
	}
	var me *Error
	_, err = rawCompile(dialect.Postgres, "a IN (?)", In([]int{}))
	if !errors.As(err, &me) || me.Kind != ErrValidation {
		t.Fatalf("err = %v", err)
	}
}

func TestRawInHonoursMaxParams(t *testing.T) {
	d := dialect.For(dialect.SQLite)
	big := make([]int, d.MaxParams+1)
	_, err := rawCompile(dialect.SQLite, "a IN (?)", In(big))
	var me *Error
	if !errors.As(err, &me) || me.Kind != ErrValidation || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("err = %v, want a build error naming the limit", err)
	}
	if _, err := rawCompile(dialect.SQLite, "a IN (?)", In(big[:d.MaxParams])); err != nil {
		t.Fatalf("exactly MaxParams must pass: %v", err)
	}
}

func TestRawTypedSQLIdentIsNeverBound(t *testing.T) {
	c, err := compile(dialect.For(dialect.Postgres), func(b *Builder) {
		TypedSQL[bool]("? = ?", Ident("title"), "a").AppendSQL(b)
	})
	if err != nil || !strings.Contains(c.SQL, `"title"`) || len(c.Args) != 1 || c.Args[0] != "a" {
		t.Fatalf("%q %v %v", c.SQL, c.Args, err)
	}
}
