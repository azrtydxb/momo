// SPDX-License-Identifier: Apache-2.0

package types

import (
	"testing"
	"time"

	"github.com/azrtydxb/momo/dialect"
)

func TestDateInfinity(t *testing.T) {
	for _, c := range []struct {
		in     Date
		wire   string
		finite bool
	}{
		{Date{Infinity: true}, "infinity", false},
		{Date{NegInfinity: true}, "-infinity", false},
		{Date{Time: time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)}, "2024-02-29", true},
	} {
		if c.in.IsFinite() != c.finite {
			t.Errorf("%+v IsFinite = %v", c.in, !c.finite)
		}
		v, err := c.in.Value()
		if err != nil || v != c.wire {
			t.Fatalf("%+v Value = %v, %v; want %q", c.in, v, err, c.wire)
		}
		for _, src := range []any{c.wire, []byte(c.wire)} {
			var back Date
			back.Infinity = !c.in.Infinity // Scan must reset every field
			if err := back.Scan(src); err != nil {
				t.Fatalf("Scan(%v): %v", src, err)
			}
			if back.Infinity != c.in.Infinity || back.NegInfinity != c.in.NegInfinity || !back.Time.Equal(c.in.Time) {
				t.Errorf("Scan(%v) = %+v, want %+v", src, back, c.in)
			}
		}
	}
	if _, err := (Date{Infinity: true, NegInfinity: true}).Value(); err == nil {
		t.Error("both flags set must error")
	}
	var d Date
	for _, s := range []string{"Infinity", "+infinity", "infinity ", "inf"} {
		if err := d.Scan(s); err == nil {
			t.Errorf("Date Scan(%q) must error", s)
		}
	}
}

func TestParseNumeric(t *testing.T) {
	good := map[string]Numeric{
		"0":                      {Decimal: "0"},
		"-0.000000000000000001":  {Decimal: "-0.000000000000000001"},
		"12345678901234567890.5": {Decimal: "12345678901234567890.5"},
		"+7":                     {Decimal: "+7"},
		".5":                     {Decimal: ".5"},
		"7.":                     {Decimal: "7."},
		" 1.50 ":                 {Decimal: "1.50"},
		"NaN":                    {NaN: true},
		"nan":                    {NaN: true},
		"Infinity":               {Infinity: true},
		"+Infinity":              {Infinity: true},
		"inf":                    {Infinity: true},
		"-Infinity":              {NegInfinity: true},
		"-INF":                   {NegInfinity: true},
	}
	for in, want := range good {
		got, err := ParseNumeric(in)
		if err != nil || got != want {
			t.Errorf("ParseNumeric(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"", " ", "-", "+", ".", "1.2.3", "1e5", "0x10", "--1", "1-", "1 2", "NaN1", "infinit", "--inf", "1,5", "١٢"} {
		if got, err := ParseNumeric(in); err == nil {
			t.Errorf("ParseNumeric(%q) = %+v, want an error", in, got)
		}
	}
}

func TestNumericSignAndPredicates(t *testing.T) {
	for _, c := range []struct {
		n      Numeric
		sign   int
		finite bool
		nan    bool
		inf    bool
		text   string
	}{
		{Numeric{}, 0, true, false, false, "0"},
		{Numeric{Decimal: "0.00"}, 0, true, false, false, "0.00"},
		{Numeric{Decimal: "-0.00"}, 0, true, false, false, "-0.00"},
		{Numeric{Decimal: "0.001"}, 1, true, false, false, "0.001"},
		{Numeric{Decimal: "-12.5"}, -1, true, false, false, "-12.5"},
		{Numeric{Decimal: "+12.5"}, 1, true, false, false, "+12.5"},
		{Numeric{NaN: true}, 0, false, true, false, "NaN"},
		{Numeric{Infinity: true}, 1, false, false, true, "Infinity"},
		{Numeric{NegInfinity: true}, -1, false, false, true, "-Infinity"},
	} {
		if c.n.Sign() != c.sign || c.n.IsFinite() != c.finite || c.n.IsNaN() != c.nan || c.n.IsInfinite() != c.inf || c.n.String() != c.text {
			t.Errorf("%+v: Sign %d IsFinite %v IsNaN %v IsInfinite %v String %q", c.n, c.n.Sign(), c.n.IsFinite(), c.n.IsNaN(), c.n.IsInfinite(), c.n.String())
		}
	}
}

func TestNumericValueAndScan(t *testing.T) {
	for _, bad := range []Numeric{
		{NaN: true, Infinity: true},
		{Infinity: true, NegInfinity: true},
		{NaN: true, Decimal: "1"},
		{Decimal: "1e5"},
		{Decimal: "'; DROP TABLE t; --"},
	} {
		if v, err := bad.Value(); err == nil {
			t.Errorf("%+v Value = %v, want an error", bad, v)
		}
	}
	for _, n := range []Numeric{{NaN: true}, {Infinity: true}, {NegInfinity: true}, {Decimal: "-1.50"}, {}} {
		v, err := n.Value()
		if err != nil {
			t.Fatalf("%+v Value: %v", n, err)
		}
		var back Numeric
		if err := back.Scan(v); err != nil {
			t.Fatalf("Scan(%v): %v", v, err)
		}
		if again, _ := back.Value(); again != v {
			t.Errorf("%+v round trip %v -> %v", n, v, again)
		}
	}
	var n Numeric
	if err := n.Scan(int64(-42)); err != nil || n.Decimal != "-42" {
		t.Errorf("Scan(int64) = %+v, %v", n, err)
	}
	if err := n.Scan("NaN"); err != nil || !n.NaN || n.Decimal != "" {
		t.Errorf("Scan(NaN) = %+v, %v", n, err)
	}
	if err := n.Scan("1.5"); err != nil || n.NaN || n.Decimal != "1.5" {
		t.Errorf("Scan must reset the flags: %+v, %v", n, err)
	}
	if err := n.Scan("abc"); err == nil {
		t.Error("Scan(abc) must error")
	}
}

func TestNumericSQLType(t *testing.T) {
	if got := (Numeric{}).SQLType(dialect.Postgres); got != "numeric" {
		t.Errorf("postgres = %q", got)
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		if got := (Numeric{}).SQLType(d); got != "TEXT" {
			t.Errorf("%s = %q", d, got)
		}
	}
}
