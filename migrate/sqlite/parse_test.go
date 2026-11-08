// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"slices"
	"testing"
)

func TestTokenizeKinds(t *testing.T) {
	src := "CREATE TABLE \"a b\" (x INTEGER, y TEXT DEFAULT 'it''s') -- c\n/* d */ WITHOUT ROWID"
	toks := tokenize(src)
	var got []string
	for _, tk := range toks {
		got = append(got, tk.text)
	}
	want := []string{"CREATE", "TABLE", `"a b"`, "(x INTEGER, y TEXT DEFAULT 'it''s')", "WITHOUT", "ROWID"}
	if !slices.Equal(got, want) {
		t.Fatalf("tokens:\n got %q\nwant %q", got, want)
	}
	if toks[2].kind != tokIdent || toks[2].name() != "a b" || toks[3].kind != tokGroup {
		t.Fatalf("kinds: %+v", toks)
	}
	if unquoteString("'it''s'") != "it's" || unquoteIdent(`"a""b"`) != `a"b` || unquoteIdent("[x y]") != "x y" {
		t.Fatal("unquote")
	}
}

func TestParseTableSQL(t *testing.T) {
	sql := `CREATE TABLE kids (a INTEGER NOT NULL, b INTEGER NOT NULL, label TEXT NOT NULL COLLATE NOCASE DEFAULT 'x, (y)',
  mood TEXT CONSTRAINT kids_mood CHECK (mood IN ('happy', 'sad')),
  total INTEGER GENERATED ALWAYS AS (a + b) STORED,
  parent_id INTEGER CONSTRAINT kids_parent_col REFERENCES parents (id),
  code TEXT CONSTRAINT kids_code_key UNIQUE,
  CONSTRAINT kids_pk PRIMARY KEY (a, b),
  CONSTRAINT kids_label_len CHECK (length(label) > 0),
  CONSTRAINT kids_parent FOREIGN KEY (parent_id) REFERENCES "Parents" (id),
  CONSTRAINT kids_label_uq UNIQUE (label, parent_id)
) WITHOUT ROWID`
	d := parseTableSQL(sql)
	if !d.withoutRowID {
		t.Error("WITHOUT ROWID not seen")
	}
	if d.columns["label"].collate != "NOCASE" || d.columns["total"].generated != "a + b" {
		t.Errorf("column text: %+v", d.columns)
	}
	if len(d.checks) != 2 || d.checks[0].name != "kids_mood" || d.checks[0].expr != "mood IN ('happy', 'sad')" || d.checks[1].name != "kids_label_len" {
		t.Errorf("checks: %+v", d.checks)
	}
	if d.uniqueNames["code"] != "kids_code_key" || d.uniqueNames["label\x00parent_id"] != "kids_label_uq" {
		t.Errorf("unique names: %+v", d.uniqueNames)
	}
	// The table-level constraint and the column-level one name the same key; the later wins.
	if d.fkNames["parent_id\x01parents"] != "kids_parent" {
		t.Errorf("fk names: %+v", d.fkNames)
	}
}

func TestParseTableSQLGarbage(t *testing.T) {
	for _, s := range []string{"", "CREATE TABLE x", "CREATE TABLE x (", "((((", "CREATE TABLE x (a, , b) )", "'unterminated"} {
		_ = parseTableSQL(s) // must not panic
	}
}

func TestEnumCheck(t *testing.T) {
	cases := []struct {
		in     string
		col    string
		values []string
		ok     bool
	}{
		{`"status" IN ('a', 'b')`, "status", []string{"a", "b"}, true},
		{`(status IN ('it''s'))`, "status", []string{"it's"}, true},
		{`status IN (1, 2)`, "", nil, false},
		{`status IN ()`, "", nil, false},
		{`status NOT IN ('a')`, "", nil, false},
		{`length(status) > 0`, "", nil, false},
		{``, "", nil, false},
	}
	for _, c := range cases {
		col, values, ok := enumCheck(c.in)
		if ok != c.ok || col != c.col || !slices.Equal(values, c.values) {
			t.Errorf("enumCheck(%q) = %q, %q, %v; want %q, %q, %v", c.in, col, values, ok, c.col, c.values, c.ok)
		}
	}
}

func TestParseIndexSQL(t *testing.T) {
	parts, where := parseIndexSQL(`CREATE UNIQUE INDEX docs_title ON docs (lower(title) COLLATE NOCASE DESC, "id", (a + b)) WHERE status = 'a';`)
	if !slices.Equal(parts, []string{"lower(title)", `"id"`, "(a + b)"}) || where != "status = 'a'" {
		t.Fatalf("parts %q where %q", parts, where)
	}
	parts, where = parseIndexSQL(`CREATE INDEX i ON t (a)`)
	if !slices.Equal(parts, []string{"a"}) || where != "" {
		t.Fatalf("parts %q where %q", parts, where)
	}
}

func TestViewQuery(t *testing.T) {
	q, err := viewQuery("CREATE VIEW v(a, b) AS\nSELECT x, y FROM t WHERE z = 'as';")
	if err != nil || q != "SELECT x, y FROM t WHERE z = 'as'" {
		t.Fatalf("%q %v", q, err)
	}
	if _, err := viewQuery("CREATE VIEW v"); err == nil {
		t.Fatal("a view without AS must be an error")
	}
}

func TestDependsOf(t *testing.T) {
	known := map[string]string{"orders": "orders", "users": "users", "v": "v"}
	got := dependsOf(`SELECT o.id FROM main.orders o JOIN (SELECT * FROM "Users") u ON 1 WHERE note = 'orders'`, "v", known)
	if !slices.Equal(got, []string{"orders", "users"}) {
		t.Fatalf("got %v", got)
	}
}

// FuzzParseDDL feeds arbitrary text to every parser of stored SQL. None may
// panic, and the tokens must tile the text they came from in order, so that
// slicing the original by token offsets is always safe.
func FuzzParseDDL(f *testing.F) {
	for _, s := range []string{
		"CREATE TABLE t (a INTEGER PRIMARY KEY, b TEXT CHECK (b IN ('x','y')))",
		"CREATE INDEX i ON t (lower(a) COLLATE NOCASE DESC) WHERE a > 1",
		"CREATE VIEW v AS SELECT 1",
		`"unterminated`, "((((", "'", "/*", "--", "[", "x'AB'", "",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		prev := 0
		for _, tk := range tokenize(s) {
			if tk.start < prev || tk.end < tk.start || tk.end > len(s) || s[tk.start:tk.end] != tk.text {
				t.Fatalf("token %+v does not match %q", tk, s)
			}
			prev = tk.end
		}
		_ = parseTableSQL(s)
		_, _ = parseIndexSQL(s)
		_, _ = viewQuery(s)
		_, _, _ = enumCheck(s)
		_ = dependsOf(s, "x", map[string]string{"t": "t", "a": "a"})
	})
}
