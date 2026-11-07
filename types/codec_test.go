// SPDX-License-Identifier: Apache-2.0

package types

import (
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo/dialect"
)

func TestCodecsRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		codec Codec
		fresh func() Codec
		want  any
	}{
		{"JSON", &JSON[map[string]int]{V: map[string]int{"a": 1}}, func() Codec { return &JSON[map[string]int]{} }, map[string]int{"a": 1}},
		{"Date", &Date{Time: time.Date(1969, 12, 31, 0, 0, 0, 0, time.UTC)}, func() Codec { return &Date{} }, "1969-12-31"},
		{"TimeOfDay", &TimeOfDay{Nanos: 3723000000000}, func() Codec { return &TimeOfDay{} }, int64(3723000000000)},
		{"UUID", &UUID{1, 2, 3}, func() Codec { return &UUID{} }, UUID{1, 2, 3}},
		{"Hstore", &Hstore{"k": ptr("v"), "n": nil}, func() Codec { return &Hstore{} }, nil},
		{"TSVector", tsv("'cat':3 'fat':2"), func() Codec { return new(TSVector) }, "'cat':3 'fat':2"},
		{"TSQuery", tsq("'fat' & 'cat'"), func() Codec { return new(TSQuery) }, "'fat' & 'cat'"},
	}
	for _, c := range cases {
		for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
			if c.codec.SQLType(d) == "" {
				t.Fatalf("%s has no SQL type for %s", c.name, d)
			}
		}
		v, err := c.codec.Value()
		if err != nil {
			t.Fatalf("%s Value: %v", c.name, err)
		}
		back := c.fresh()
		if err := back.Scan(v); err != nil {
			t.Fatalf("%s Scan(%v): %v", c.name, v, err)
		}
		if _, ok := any(back).(driver.Valuer); !ok {
			t.Fatalf("%s does not implement driver.Valuer", c.name)
		}
		v2, err := back.Value()
		if err != nil || v2 != v {
			t.Fatalf("%s re-Value = %v, %v; want %v", c.name, v2, err, v)
		}
	}
}

func ptr(s string) *string { return &s }

func tsv(s string) *TSVector { v := TSVector(s); return &v }
func tsq(s string) *TSQuery  { v := TSQuery(s); return &v }

func TestCodecsSourceTypesAndErrors(t *testing.T) {
	fresh := map[string]func() Codec{
		"JSON":      func() Codec { return &JSON[map[string]int]{} },
		"Date":      func() Codec { return &Date{} },
		"TimeOfDay": func() Codec { return &TimeOfDay{} },
		"UUID":      func() Codec { return &UUID{} },
		"Hstore":    func() Codec { return &Hstore{} },
		"TSVector":  func() Codec { return new(TSVector) },
		"TSQuery":   func() Codec { return new(TSQuery) },
	}
	good := map[string]string{
		"JSON":      `{"a":1}`,
		"Date":      "2020-02-29",
		"TimeOfDay": "01:02:03.000000004",
		"UUID":      "01020300-0000-0000-0000-000000000000",
		"Hstore":    `"k"=>"v", "n"=>NULL`,
		"TSVector":  "'cat':3 'fat':2",
		"TSQuery":   "'fat' & 'cat'",
	}
	for name, f := range fresh {
		if err := f().Scan(nil); err == nil {
			t.Errorf("%s Scan(nil) must error", name)
		}
		if err := f().Scan(12.5); err == nil {
			t.Errorf("%s Scan(float64) must error", name)
		}
		if err := f().Scan(struct{}{}); err == nil {
			t.Errorf("%s Scan(struct) must error", name)
		}
		a, b := f(), f()
		if err := a.Scan(good[name]); err != nil {
			t.Errorf("%s Scan(string): %v", name, err)
		}
		if err := b.Scan([]byte(good[name])); err != nil {
			t.Errorf("%s Scan([]byte): %v", name, err)
		}
		va, _ := a.Value()
		vb, _ := b.Value()
		if va != vb {
			t.Errorf("%s string vs []byte differ: %v vs %v", name, va, vb)
		}
	}
}

func TestUUIDParseRejects(t *testing.T) {
	bad := []string{
		"", "0102", "01020300-0000-0000-0000-00000000000",
		"01020300-0000-0000-0000-0000000000000",
		"0102030000000000000000000000000z",
		"01020300_0000-0000-0000-000000000000",
		"0102030000-00-0000-0000-000000000000",
		"g1020300-0000-0000-0000-000000000000",
	}
	for _, s := range bad {
		var u UUID
		if err := u.Scan(s); err == nil {
			t.Errorf("Scan(%q) must error", s)
		}
	}
	var u UUID
	if err := u.Scan([]byte{1, 2}); err == nil {
		t.Error("2 raw bytes must error")
	}
	if err := u.Scan(make([]byte, 16)); err != nil {
		t.Errorf("16 raw bytes: %v", err)
	}
	if err := u.Scan("0102030000000000000000000000000A"); err != nil {
		t.Errorf("32 hex: %v", err)
	}
	v, _ := UUID{0xab, 1}.Value()
	if v != "ab010000-0000-0000-0000-000000000000" {
		t.Errorf("Value = %v", v)
	}
}

func TestJSONNullVersusEmpty(t *testing.T) {
	var nilMap JSON[map[string]int]
	empty := JSON[map[string]int]{V: map[string]int{}}
	vn, _ := nilMap.Value()
	ve, _ := empty.Value()
	if vn != "null" || ve != "{}" {
		t.Fatalf("null=%v empty=%v", vn, ve)
	}
	var a, b JSON[map[string]int]
	if err := a.Scan("null"); err != nil || a.V != nil {
		t.Fatalf("null scan: %v %#v", err, a.V)
	}
	if err := b.Scan("{}"); err != nil || b.V == nil || len(b.V) != 0 {
		t.Fatalf("empty scan: %v %#v", err, b.V)
	}
	var p JSON[*int]
	if err := p.Scan("null"); err != nil || p.V != nil {
		t.Fatalf("ptr null: %v", err)
	}
	if err := a.Scan("{bad"); err == nil {
		t.Fatal("bad JSON must error")
	}
	if _, err := (&JSON[chan int]{V: make(chan int)}).Value(); err == nil {
		t.Fatal("unencodable must error")
	}
}

func TestDateAndTimeOfDay(t *testing.T) {
	var d Date
	if err := d.Scan(time.Date(2024, 3, 5, 23, 0, 0, 0, time.FixedZone("x", 3600))); err != nil {
		t.Fatal(err)
	}
	if v, _ := d.Value(); v != "2024-03-05" {
		t.Fatalf("got %v", v)
	}
	for _, s := range []string{"2024-02-30", "2024-13-01", "20240101", "x"} {
		if err := d.Scan(s); err == nil {
			t.Errorf("Date Scan(%q) must error", s)
		}
	}
	var tod TimeOfDay
	if err := tod.Scan(int64(3723000000)); err == nil {
		t.Error("int64 source must error (drivers disagree on its unit)")
	}
	if err := tod.Scan("01:02:03"); err != nil {
		t.Fatal(err)
	}
	if v, _ := tod.Value(); v != "01:02:03.000000" {
		t.Fatalf("got %v", v)
	}
	if err := tod.Scan("01:02"); err == nil {
		t.Error("short time must error")
	}
	if err := tod.Scan("24:00:01"); err == nil {
		t.Error("24:00:01 must error")
	}
	if err := tod.Scan("24:00:00.5"); err == nil {
		t.Error("24:00:00.5 must error")
	}
	if err := tod.Scan("24:00:00"); err != nil || tod.Nanos != nanosPerDay-1 {
		t.Errorf("24:00:00 = %d, %v", tod.Nanos, err)
	}
	for digits := 0; digits <= 9; digits++ {
		in := "00:00:01"
		if digits > 0 {
			in += "." + strings.Repeat("5", digits)
		}
		if err := tod.Scan(in); err != nil {
			t.Errorf("Scan(%q): %v", in, err)
		}
	}
	if err := tod.Scan("00:00:01.1234567890"); err == nil {
		t.Error("10 fraction digits must error")
	}
	if err := tod.Scan("00:00:01."); err == nil {
		t.Error("empty fraction must error")
	}
	max := TimeOfDay{Nanos: nanosPerDay - 1}
	mv, err := max.Value()
	if err != nil || mv != "23:59:59.999999" {
		t.Errorf("max Value = %v, %v", mv, err)
	}
	if err := tod.Scan("01:02:03"); err != nil || tod.Nanos != 3723000000000 {
		t.Errorf("no fraction: %v %d", err, tod.Nanos)
	}
	if _, err := (&TimeOfDay{Nanos: 86400000000000}).Value(); err == nil {
		t.Error("out-of-range Value must error")
	}
}

func TestHstoreForms(t *testing.T) {
	h := Hstore{"a\"b": ptr(`c\d`), "n": nil, "e": ptr("")}
	v, _ := h.Value()
	txt, _ := v.(string)
	var back Hstore
	if err := back.Scan(txt); err != nil {
		t.Fatalf("scan %q: %v", txt, err)
	}
	if len(back) != 3 || back["n"] != nil || *back["a\"b"] != `c\d` || *back["e"] != "" {
		t.Fatalf("round trip lost data: %q -> %#v", txt, back)
	}
	if _, ok := back["n"]; !ok {
		t.Fatal("NULL key lost")
	}
	var fromJSON Hstore
	if err := fromJSON.Scan(`{"a\"b":"c\\d","n":null,"e":""}`); err != nil || len(fromJSON) != 3 || fromJSON["n"] != nil {
		t.Fatalf("json form: %v %#v", err, fromJSON)
	}
	var empty Hstore
	if err := empty.Scan(""); err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty: %v", err)
	}
	for _, s := range []string{`"a"=>`, `"a"`, `"a"=>"b" "c"=>"d"`, `"a"=>"b`, `a=>"b"`} {
		if err := empty.Scan(s); err == nil {
			t.Errorf("Scan(%q) must error", s)
		}
	}
	if !strings.Contains(txt, "=>NULL") {
		t.Errorf("NULL form missing in %q", txt)
	}
}

func FuzzHstore(f *testing.F) {
	f.Add("k", "v", false, "n", `"a"=>"b", "a"=>"c"`)
	f.Add(`q"\`, "", true, "", `"a"=>NULL`)
	f.Fuzz(func(t *testing.T, k, v string, null bool, k2, raw string) {
		h := Hstore{k: &v, k2 + "x": nil}
		if null {
			h[k] = nil
		}
		txt := h.format()
		var back Hstore
		if err := back.Scan(txt); err != nil {
			t.Fatalf("scan(format) %q: %v", txt, err)
		}
		if len(back) != len(h) {
			t.Fatalf("size %d != %d for %q", len(back), len(h), txt)
		}
		for key, want := range h {
			got, ok := back[key]
			if !ok || (want == nil) != (got == nil) || (want != nil && *want != *got) {
				t.Fatalf("key %q lost in %q", key, txt)
			}
		}
		var any Hstore
		_ = any.Scan(raw) // must not panic
	})
}

func TestHstoreDuplicateKeysFirstWins(t *testing.T) {
	var h Hstore
	if err := h.Scan(`"a"=>"1", "a"=>"2", "b"=>NULL, "b"=>"x"`); err != nil {
		t.Fatal(err)
	}
	if *h["a"] != "1" || h["b"] != nil {
		t.Fatalf("got %v %v", *h["a"], h["b"])
	}
}
