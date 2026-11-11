// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitStatements(t *testing.T) {
	body := "CREATE TABLE a (x int); -- trailing ; comment\nINSERT INTO a VALUES ('a;b');\n/* block ; */ SELECT 1;"
	got, err := SplitStatements([]byte(body), "postgres")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"CREATE TABLE a (x int)", "-- trailing ; comment\nINSERT INTO a VALUES ('a;b')", "/* block ; */ SELECT 1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestSplitDollarQuotes(t *testing.T) {
	body := "CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END; $$ LANGUAGE plpgsql;\nSELECT f();"
	got, err := SplitStatements([]byte(body), "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 statements, got %d: %#v", len(got), got)
	}
}

func TestSplitSQLiteTrigger(t *testing.T) {
	body := "CREATE TRIGGER tr AFTER INSERT ON t BEGIN UPDATE t SET x = 1; END;\nSELECT 1;"
	got, err := SplitStatements([]byte(body), "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("trigger body must stay one statement, got %#v", got)
	}
}

func TestDirectives(t *testing.T) {
	body := "-- momo:no-transaction\n-- momo:destructive\nDROP TABLE x;"
	d, err := ParseDirectives([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if !d.NoTransaction || !d.Destructive || d.Split {
		t.Fatalf("bad directives: %+v", d)
	}
	d2, err := ParseDirectives([]byte("DROP TABLE x;\n-- momo:destructive"))
	if err != nil {
		t.Fatal(err)
	}
	if d2.Destructive {
		t.Fatal("a directive below the first statement must not count")
	}
}

func TestDirectivesImported(t *testing.T) {
	d, err := ParseDirectives([]byte("-- momo:imported\n-- momo:no-transaction\nCREATE TABLE x (id int);"))
	if err != nil {
		t.Fatal(err)
	}
	if !d.Imported || !d.NoTransaction || d.Generated {
		t.Fatalf("bad directives: %+v", d)
	}
}

func TestDirectivesHostile(t *testing.T) {
	_, err := ParseDirectives([]byte("-- a note\n-- momo:destructiv\nDROP TABLE x;"))
	if err == nil || !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), "destructiv") {
		t.Fatalf("want positioned unknown-directive error, got %v", err)
	}
	d, err := ParseDirectives([]byte("\xef\xbb\xbf-- momo:split\r\n-- momo:generated\r\nSELECT 1;"))
	if err != nil {
		t.Fatal(err)
	}
	if !d.Split || !d.Generated {
		t.Fatalf("BOM/CRLF directives lost: %+v", d)
	}
	d, err = ParseDirectives([]byte("-- momo:sqlite-foreign-keys-off\n"))
	if err != nil || !d.SQLiteForeignKeysOff {
		t.Fatalf("sqlite-foreign-keys-off: %+v %v", d, err)
	}
	if _, err := ParseDirectives(nil); err != nil {
		t.Fatalf("empty body: %v", err)
	}
}

func TestSplitHostile(t *testing.T) {
	cases := []struct {
		name, dialect, body string
		want                []string
	}{
		{"empty", "postgres", "", nil},
		{"whitespace and comments only", "postgres", "  \n-- x;\n/* y; */\n;;", nil},
		{"no trailing semicolon", "postgres", "SELECT 1; SELECT 2", []string{"SELECT 1", "SELECT 2"}},
		{"bom", "postgres", "\xef\xbb\xbfSELECT 1;", []string{"SELECT 1"}},
		{"crlf", "postgres", "SELECT 1;\r\nSELECT 'a;\r\nb';\r\n", []string{"SELECT 1", "SELECT 'a;\r\nb'"}},
		{"tagged dollar", "postgres", "DO $body$ BEGIN PERFORM 1; END $body$; SELECT 2;", []string{"DO $body$ BEGIN PERFORM 1; END $body$", "SELECT 2"}},
		{"nested dollar tags", "postgres", "SELECT $a$ x $$ ; $$ y $a$; SELECT 2", []string{"SELECT $a$ x $$ ; $$ y $a$", "SELECT 2"}},
		{"param is not a tag", "postgres", "SELECT $1; SELECT $2", []string{"SELECT $1", "SELECT $2"}},
		{"doubled single quote", "postgres", "SELECT 'it''s;'; SELECT 2", []string{"SELECT 'it''s;'", "SELECT 2"}},
		{"E string backslash", "postgres", `SELECT E'a\';b'; SELECT 2`, []string{`SELECT E'a\';b'`, "SELECT 2"}},
		{"plain string backslash", "postgres", `SELECT 'a\'; SELECT 2`, []string{`SELECT 'a\'`, "SELECT 2"}},
		{"mysql backslash", "mysql", `SELECT 'a\';b'; SELECT 2`, []string{`SELECT 'a\';b'`, "SELECT 2"}},
		{"double quoted ident", "postgres", `CREATE TABLE "a;""b" (x int); SELECT 2`, []string{`CREATE TABLE "a;""b" (x int)`, "SELECT 2"}},
		{"backtick", "mysql", "CREATE TABLE `a;b` (x int); SELECT 2", []string{"CREATE TABLE `a;b` (x int)", "SELECT 2"}},
		{"nested block postgres", "postgres", "SELECT /* a /* b ; */ c ; */ 1; SELECT 2", []string{"SELECT /* a /* b ; */ c ; */ 1", "SELECT 2"}},
		{"block not nested sqlite", "sqlite", "SELECT /* a /* b */ 1; SELECT 2", []string{"SELECT /* a /* b */ 1", "SELECT 2"}},
		{"split directive ignores dollar", "postgres", "-- momo:split\nSELECT $$a; b$$; SELECT 2", []string{"-- momo:split\nSELECT $$a", "b$$", "SELECT 2"}},
		{"sqlite trigger case", "sqlite", "CREATE TRIGGER t AFTER INSERT ON a BEGIN SELECT CASE WHEN 1 THEN 2 END; UPDATE a SET x = 1; END; SELECT 3", []string{"CREATE TRIGGER t AFTER INSERT ON a BEGIN SELECT CASE WHEN 1 THEN 2 END; UPDATE a SET x = 1; END", "SELECT 3"}},
		{"sqlite temp trigger lowercase", "sqlite", "create temp trigger t after insert on a begin select 1; end; select 2", []string{"create temp trigger t after insert on a begin select 1; end", "select 2"}},
		{"trigger body comment end", "sqlite", "CREATE TRIGGER t AFTER INSERT ON a BEGIN SELECT 1; -- END;\n END; SELECT 2", []string{"CREATE TRIGGER t AFTER INSERT ON a BEGIN SELECT 1; -- END;\n END", "SELECT 2"}},
		{"dollar not special in sqlite", "sqlite", "SELECT '$$'; SELECT 2", []string{"SELECT '$$'", "SELECT 2"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := SplitStatements([]byte(c.body), c.dialect)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %#v\nwant %#v", got, c.want)
			}
		})
	}
}

func TestSplitUnterminated(t *testing.T) {
	for name, body := range map[string]string{
		"string":  "SELECT 'abc;",
		"ident":   `SELECT "abc;`,
		"block":   "SELECT 1; /* never closed ;",
		"nested":  "/* a /* b */ ;",
		"dollar":  "SELECT $x$ abc;",
		"bom-str": "\xef\xbb\xbfSELECT 'x",
	} {
		_, err := SplitStatements([]byte(body), "postgres")
		if err == nil || !strings.Contains(err.Error(), "unterminated") || !strings.Contains(err.Error(), "byte ") {
			t.Errorf("%s: want positioned unterminated error, got %v", name, err)
		}
	}
}

func TestSplitBOM(t *testing.T) {
	const bom = "\xef\xbb\xbf"
	for name, tc := range map[string]struct {
		body string
		want []string
	}{
		"bom-only":          {bom, nil},
		"bom-only-twice":    {bom + bom, nil},
		"bom-semicolon":     {bom + ";", nil},
		"bom-prefixed":      {bom + "SELECT 1; SELECT 2;", []string{"SELECT 1", "SELECT 2"}},
		"bom-between":       {"SELECT 1;" + bom + "SELECT 2;", []string{"SELECT 1", "SELECT 2"}},
		"bom-lone-chunk":    {"SELECT 1;" + bom + ";SELECT 2", []string{"SELECT 1", "SELECT 2"}},
		"bom-trailing":      {"SELECT 1;" + bom, []string{"SELECT 1"}},
		"bom-around-stmt":   {"SELECT 1;" + bom + " SELECT 2 " + bom, []string{"SELECT 1", "SELECT 2"}},
		"bom-after-comment": {"-- c\n" + bom, nil},
	} {
		for _, d := range []string{"postgres", "sqlite", "mysql"} {
			got, err := SplitStatements([]byte(tc.body), d)
			if err != nil || len(got) != len(tc.want) || len(got) > 0 && !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%s/%s: got %q (%v), want %q", name, d, got, err, tc.want)
			}
		}
	}
}

func FuzzSplitStatements(f *testing.F) {
	for _, s := range []string{
		"", ";", "SELECT 1;", "SELECT 'a;b'; -- c\nSELECT 2",
		"DO $$ BEGIN END; $$;", "/* a /* b */ c */ SELECT 1", "\xef\xbb\xbfSELECT 1\r\n;",
		"CREATE TRIGGER t AFTER INSERT ON a BEGIN SELECT 1; END;", `SELECT E'\'';`, "SELECT \"a;\"",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, d := range []string{"postgres", "sqlite", "mysql"} {
			got, err := SplitStatements([]byte(s), d)
			if err != nil {
				continue
			}
			for _, st := range got {
				if st == "" || st != strings.TrimSpace(st) {
					t.Fatalf("%s: statement not trimmed or empty: %q", d, st)
				}
			}
			if strings.Contains(s, "momo:split") || d != "postgres" {
				continue
			}
			for _, st := range got {
				again, err := SplitStatements([]byte(st), d)
				if err != nil || len(again) != 1 || again[0] != st {
					t.Fatalf("%s: statement %q re-split to %q (%v)", d, st, again, err)
				}
			}
		}
	})
}
