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
		{"dollar after digit is not a quote", pg, "SELECT 1$a$, ?, 2$a$", []any{1}, "SELECT 1$a$, $1, 2$a$", 1},
		{"pg contains-operator at-less", pg, "a <@b, ?", []any{1}, "a <@b, $1", 1},
		{"lt at is a named bind off pg", lite, "x<@b", []any{NamedArg{"b", 1}}, "x<?", 1},
		{"pg named after space", pg, "x < @b", []any{NamedArg{"b", 1}}, "x < $1", 1},
		{"dollar binds its argument", pg, "a = $1 AND b = $2", []any{1, 2}, "a = $1 AND b = $2", 2},
		{"dollar reordered", pg, "a = $2 AND b = $1", []any{1, 2}, "a = $1 AND b = $2", 2},
		{"dollar reused shares one bind", pg, "a = $1 OR b = $1", []any{7}, "a = $1 OR b = $1", 1},
		{"dollar reused after another", pg, "a = $2 AND b = $1 AND c = $2", []any{1, 2}, "a = $1 AND b = $2 AND c = $1", 2},
		{"dollar with In", pg, "a IN ($1) AND b = $2", []any{In([]int{1, 2}), 3}, "a IN ($1, $2) AND b = $3", 3},
		{"dollar with cast", pg, "SELECT $1::int, $1::text", []any{1}, "SELECT $1::int, $1::text", 1},
		{"dollar in string is text", pg, "SELECT '$1', $1", []any{1}, "SELECT '$1', $1", 1},
		{"dollar in quoted ident is text", pg, `SELECT "a$1", $1`, []any{1}, `SELECT "a$1", $1`, 1},
		{"dollar in dollar quote is text", pg, "SELECT $q$ $1 $q$, $1", []any{1}, "SELECT $q$ $1 $q$, $1", 1},
		{"dollar in comments is text", pg, "SELECT /* $2 */ $1 -- $3\n", []any{1}, "SELECT /* $2 */ $1 -- $3\n", 1},
		{"dollar after identifier byte is text", pg, "SELECT a$1, $1", []any{1}, "SELECT a$1, $1", 1},
		{"dollar in string off pg is text", my, "SELECT '$1', ?", []any{1}, "SELECT '$1', ?", 1},
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

// $n on a dialect other than PostgreSQL is ErrUnsupported, not a silent
// passthrough. Would fail if the scanner ignored $n off PostgreSQL.
func TestRawDollarUnsupportedOffPostgres(t *testing.T) {
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		_, err := rawCompile(d, "SELECT $1", 1)
		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%s: err = %v, want ErrUnsupported", d, err)
		}
	}
}

// A $n fragment embedded after prior binds keeps its own argument numbering
// and lands on the right statement placeholders.
func TestRawDollarRenumbersInsideLargerStatement(t *testing.T) {
	c, err := compile(dialect.For(dialect.Postgres), func(b *Builder) {
		b.Arg("first")
		b.WriteString(" AND ")
		b.appendRaw("a = $2 OR b = $1 OR c = $2", []any{"x", "y"})
		b.WriteString(" AND ")
		b.appendRaw("d = $1", []any{"z"})
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "$1 AND a = $2 OR b = $3 OR c = $2 AND d = $4"
	if c.SQL != want || len(c.Args) != 4 || c.Args[1] != "y" || c.Args[2] != "x" || c.Args[3] != "z" {
		t.Fatalf("SQL = %q args %v, want %q", c.SQL, c.Args, want)
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
		{"native with unrelated Named arg", pg, "SELECT $1", []any{1, NamedArg{"a", 1}}, "native $n"},
		{"dollar out of range", pg, "SELECT $1, $3", []any{1, 2}, "uses $3"},
		{"dollar zero", pg, "SELECT $0", []any{1}, "uses $0"},
		{"dollar without arguments", pg, "SELECT $1", nil, "uses $1"},
		{"dollar gap", pg, "SELECT $1, $3", []any{1, 2, 3}, "never uses argument $2"},
		{"dollar unused trailing argument", pg, "SELECT $1", []any{1, 2}, "never uses argument $2"},
		{"dollar huge number", pg, "SELECT $99999999999999999999", []any{1}, "uses $99999999999999999999"},
		{"dollar empty In", pg, "a IN ($1)", []any{In([]int{})}, "empty IN"},
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
