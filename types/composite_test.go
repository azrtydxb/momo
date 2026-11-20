// SPDX-License-Identifier: Apache-2.0

package types

import (
	"database/sql"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo/dialect"
)

func strp(s string) *string { return &s }

// samePtrs compares two lists of nullable strings, telling NULL from "".
func samePtrs(a, b []*string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if (a[i] == nil) != (b[i] == nil) || (a[i] != nil && *a[i] != *b[i]) {
			return false
		}
	}
	return true
}

func showPtrs(in []*string) string {
	parts := make([]string, len(in))
	for i, p := range in {
		if p == nil {
			parts[i] = "NULL"
		} else {
			parts[i] = "\"" + *p + "\""
		}
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// Catches: a record written with a field quoted when it need not be, or left
// bare when PostgreSQL would misread it (a comma, a parenthesis, a quote, a
// backslash, white space), an empty string written as NULL, and a quote or a
// backslash inside quotes that is not doubled.
func TestFormatRecord(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fields []*string
		want   string
	}{
		{"plain", []*string{strp("a"), strp("b")}, "(a,b)"},
		{"null field", []*string{strp("a"), nil, strp("c")}, "(a,,c)"},
		{"only null", []*string{nil}, "()"},
		{"two nulls", []*string{nil, nil}, "(,)"},
		{"empty string is not null", []*string{strp(""), strp("x")}, `("",x)`},
		{"space", []*string{strp("a b")}, `("a b")`},
		{"tab and newline", []*string{strp("a\tb"), strp("c\nd")}, "(\"a\tb\",\"c\nd\")"},
		{"comma", []*string{strp("a,b")}, `("a,b")`},
		{"parentheses", []*string{strp("(1,2)")}, `("(1,2)")`},
		{"quote is doubled", []*string{strp(`a"b`)}, `("a""b")`},
		{"backslash is doubled", []*string{strp(`a\b`)}, `("a\\b")`},
		{"quote and backslash", []*string{strp(`"\`)}, `("""\\")`},
		{"null word is text", []*string{strp("NULL")}, "(NULL)"},
		{"non-ASCII stays bare", []*string{strp("héllo")}, "(héllo)"},
	} {
		if got := FormatRecord(tc.fields); got != tc.want {
			t.Errorf("%s: FormatRecord = %s, want %s", tc.name, got, tc.want)
		}
	}
}

// Catches: a parser that reads a quoted empty field as NULL, drops the
// escapes of a quote or a backslash, strips white space inside a field, or
// accepts text that is not a record (no parentheses, an unterminated quote, a
// dangling backslash, an unquoted parenthesis inside).
func TestParseRecord(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []*string
	}{
		{"(a,b)", []*string{strp("a"), strp("b")}},
		{"()", []*string{nil}},
		{"(,)", []*string{nil, nil}},
		{`("","")`, []*string{strp(""), strp("")}},
		{`(a,,"")`, []*string{strp("a"), nil, strp("")}},
		{`("a,b",c)`, []*string{strp("a,b"), strp("c")}},
		{`(a\,b,c)`, []*string{strp("a,b"), strp("c")}},
		{`("a""b")`, []*string{strp(`a"b`)}},
		{`("a\"b")`, []*string{strp(`a"b`)}},
		{`("a\\b")`, []*string{strp(`a\b`)}},
		{`("(1,2)",3)`, []*string{strp("(1,2)"), strp("3")}},
		{`(ab"c d"e)`, []*string{strp("abc de")}},
		{" ( a , b ) ", []*string{strp(" a "), strp(" b ")}},
		{"(NULL)", []*string{strp("NULL")}},
	} {
		got, err := ParseRecord(tc.in)
		if err != nil {
			t.Errorf("ParseRecord(%q): %v", tc.in, err)
			continue
		}
		if !samePtrs(got, tc.want) {
			t.Errorf("ParseRecord(%q) = %s, want %s", tc.in, showPtrs(got), showPtrs(tc.want))
		}
	}
	for _, bad := range []string{"", "abc", "(a", "a)", `("a)`, `(a\)`, "(a(b))", "(", ")", `(a"b)`} {
		if got, err := ParseRecord(bad); err == nil {
			t.Errorf("ParseRecord(%q) = %s, want an error", bad, showPtrs(got))
		}
	}
}

// Catches: FormatRecord output that ParseRecord reads back as other fields,
// for every shape of field the format has.
func TestRecordRoundTrip(t *testing.T) {
	for _, fields := range [][]*string{
		{strp("")},
		{nil},
		{strp(""), nil, strp("")},
		{strp(`a"b\c,d(e)f g`), strp("\t"), strp("  "), nil},
		{strp("NULL"), strp("()"), strp(`"`), strp(`\`)},
		{strp("héllo wörld"), strp("日本語")},
		{strp("line\nbreak")},
	} {
		got, err := ParseRecord(FormatRecord(fields))
		if err != nil {
			t.Errorf("%s: %v", showPtrs(fields), err)
			continue
		}
		if !samePtrs(got, fields) {
			t.Errorf("round trip of %s gave %s", showPtrs(fields), showPtrs(got))
		}
	}
}

type compAddr struct {
	Street string
	Zip    *string
	Floor  int32
	Lat    float64
	Open   bool
	Tag    []byte
	At     time.Time
	Note   sql.NullString
	ID     UUID
}

// Catches: a Composite written in another field order than the struct, a NULL
// pointer written as text, a bytea or a timestamp spelled the way PostgreSQL
// will not read, and a value that does not read back as it was written.
func TestCompositeValueAndScan(t *testing.T) {
	id, err := ParseUUID("01020304-0506-0708-090a-0b0c0d0e0f10")
	if err != nil {
		t.Fatal(err)
	}
	in := Composite[compAddr]{V: compAddr{
		Street: `Main "St"`, Zip: nil, Floor: 3, Lat: 1.5, Open: true,
		Tag: []byte{0xde, 0xad}, At: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Note: sql.NullString{String: "hi", Valid: true}, ID: id,
	}}
	got, err := in.Value()
	if err != nil {
		t.Fatal(err)
	}
	want := `("Main ""St""",,3,1.5,t,"\\xdead","2026-01-02 03:04:05Z",hi,01020304-0506-0708-090a-0b0c0d0e0f10)`
	if got != want {
		t.Fatalf("Value = %v, want %s", got, want)
	}
	if s := in.String(); s != want {
		t.Errorf("String = %s, want %s", s, want)
	}
	var out Composite[compAddr]
	if err := out.Scan(want); err != nil {
		t.Fatal(err)
	}
	// The instant is compared as an instant, the rest field by field.
	if !out.V.At.Equal(in.V.At) {
		t.Errorf("Scan read the instant %v, want %v", out.V.At, in.V.At)
	}
	out.V.At = in.V.At
	if !reflect.DeepEqual(out, in) {
		t.Errorf("Scan = %+v, want %+v", out.V, in.V)
	}
	var fromBytes Composite[compAddr]
	if err := fromBytes.Scan([]byte(want)); err != nil {
		t.Fatal(err)
	}
	fromBytes.V.At = in.V.At
	if !reflect.DeepEqual(fromBytes, in) {
		t.Errorf("Scan of []byte = %+v", fromBytes.V)
	}
}

// Catches: a zero string scanned from NULL, so a missing attribute becomes ""
// without notice, and a non-NULL pointer field that stays nil.
func TestCompositeNullFields(t *testing.T) {
	type pair struct {
		A *string
		B *int64
		C sql.NullString
		D []byte
	}
	var c Composite[pair]
	if err := c.Scan("(,,,)"); err != nil {
		t.Fatal(err)
	}
	if c.V.A != nil || c.V.B != nil || c.V.C.Valid || c.V.D != nil {
		t.Errorf("every NULL attribute must read as NULL: %+v", c.V)
	}
	if err := c.Scan(`("",7,x,"\\x00ff")`); err != nil {
		t.Fatal(err)
	}
	if c.V.A == nil || *c.V.A != "" || c.V.B == nil || *c.V.B != 7 || !c.V.C.Valid || c.V.C.String != "x" || !reflect.DeepEqual(c.V.D, []byte{0, 0xff}) {
		t.Errorf("non-NULL attributes: %+v", c.V)
	}
	v, err := Composite[pair]{}.Value()
	if err != nil || v != "(,,,)" {
		t.Errorf("zero value = %v, %v, want (,,,)", v, err)
	}
}

// Catches: a record that does not fit the struct being accepted, which would
// shift every attribute after the mismatch into the wrong field.
func TestCompositeScanErrors(t *testing.T) {
	type three struct {
		A string
		B string
		C int64
	}
	var c Composite[three]
	for name, src := range map[string]any{
		"NULL source":         nil,
		"wrong type":          42,
		"too few fields":      "(a,b)",
		"too many fields":     "(a,b,1,2)",
		"NULL into a string":  "(a,,1)",
		"text into an int":    "(a,b,x)",
		"not a record":        "a,b,1",
		"integer out of size": "(a,b,99999999999999999999)",
	} {
		if err := c.Scan(src); err == nil {
			t.Errorf("%s: Scan(%v) succeeded with %+v", name, src, c.V)
		}
	}
	var small Composite[struct{ N int8 }]
	if err := small.Scan("(300)"); err == nil {
		t.Errorf("an int8 field accepted 300: %+v", small.V)
	}
}

// Catches: a type parameter that is not a flat struct being encoded as
// something, instead of reported.
func TestCompositeRejectsBadStructs(t *testing.T) {
	type hidden struct {
		A string
		b string
	}
	type embedded struct {
		compAddr
	}
	type nested struct {
		Inner struct{ X int }
	}
	_ = hidden{b: "kept so the field is used"}
	if _, err := (Composite[int]{}).Value(); err == nil {
		t.Error("Composite[int].Value succeeded")
	}
	if _, err := (Composite[struct{}]{}).Value(); err == nil {
		t.Error("Composite[struct{}].Value succeeded")
	}
	if _, err := (Composite[hidden]{}).Value(); err == nil || !strings.Contains(err.Error(), "exported") {
		t.Errorf("an unexported field: %v", err)
	}
	if _, err := (Composite[embedded]{}).Value(); err == nil {
		t.Error("an embedded field was accepted")
	}
	if _, err := (Composite[nested]{}).Value(); err == nil {
		t.Error("a nested struct field was accepted")
	}
	var c Composite[int]
	if err := c.Scan("(1)"); err == nil {
		t.Error("Composite[int].Scan succeeded")
	}
}

// Catches: the special float values written the Go way (+Inf), which
// PostgreSQL rejects, and read back as something else.
func TestCompositeFloatSpecials(t *testing.T) {
	type f struct {
		X float64
		Y float32
	}
	got, err := Composite[f]{V: f{X: 1e100}}.Value()
	if err != nil || got != "(1e+100,0)" {
		t.Errorf("Value = %v, %v", got, err)
	}
	var c Composite[f]
	if err := c.Scan("(Infinity,-Infinity)"); err != nil {
		t.Fatal(err)
	}
	if c.V.X <= 1e308 || c.V.Y >= -3.4e38 {
		t.Errorf("Scan = %+v", c.V)
	}
	back, err := c.Value()
	if err != nil || back != "(Infinity,-Infinity)" {
		t.Errorf("Value of infinities = %v, %v", back, err)
	}
	if err := c.Scan("(NaN,0)"); err != nil || !math.IsNaN(c.V.X) {
		t.Errorf("NaN: %+v, %v", c.V, err)
	}
}

// Catches: a Composite that names a column type itself, which would override
// the type tag that says which composite type the column is.
func TestCompositeSQLType(t *testing.T) {
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		if got := (Composite[compAddr]{}).SQLType(d); got != "" {
			t.Errorf("SQLType(%s) = %q, want empty", d, got)
		}
	}
}

// FuzzCompositeRecord checks the record format both ways: text that parses
// formats back to text that parses to the same fields, and any list of fields,
// with NULLs, formats to text that parses back to it.
func FuzzCompositeRecord(f *testing.F) {
	for _, s := range []string{
		"()", "(,)", `("")`, "(a,b)", `("a""b")`, `(a\,b)`, `("a\\b",c)`, " ( x ) ",
		`("(1,2)",3)`, "(\"\x00\")", "(\xff)", `(a"b)`, `(\`, `("`, "", "NULL",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if fields, err := ParseRecord(s); err == nil {
			again, err := ParseRecord(FormatRecord(fields))
			if err != nil {
				t.Fatalf("ParseRecord(%q) = %s, whose text %q does not parse: %v", s, showPtrs(fields), FormatRecord(fields), err)
			}
			if !samePtrs(again, fields) {
				t.Fatalf("ParseRecord(%q) = %s but its text %q parses to %s", s, showPtrs(fields), FormatRecord(fields), showPtrs(again))
			}
		}
		// Fields from the input itself: \x1f separates them, and a field that
		// is just \x1e is NULL.
		parts := strings.Split(s, "\x1f")
		fields := make([]*string, len(parts))
		for i, p := range parts {
			if p != "\x1e" {
				fields[i] = strp(p)
			}
		}
		text := FormatRecord(fields)
		back, err := ParseRecord(text)
		if err != nil {
			t.Fatalf("FormatRecord(%s) = %q does not parse: %v", showPtrs(fields), text, err)
		}
		if !samePtrs(back, fields) {
			t.Fatalf("FormatRecord(%s) = %q parses to %s", showPtrs(fields), text, showPtrs(back))
		}
	})
}
