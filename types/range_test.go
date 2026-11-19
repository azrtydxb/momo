// SPDX-License-Identifier: Apache-2.0

package types

import (
	"database/sql"
	"database/sql/driver"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo/dialect"
)

func utc(y int, m time.Month, d, h, mi, s, ns int) time.Time {
	return time.Date(y, m, d, h, mi, s, ns, time.UTC)
}

// Catches: a range written with the wrong brackets, a comma or quote left
// unescaped, an infinite side written with a value, the empty range spelled
// differently from what PostgreSQL reads, and a time zone that changes the
// instant.
func TestRangeValue(t *testing.T) {
	zone := time.FixedZone("UTC+4", 4*3600)
	check := func(name string, got driver.Value, err error, want string) {
		t.Helper()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			return
		}
		if got != want {
			t.Errorf("%s: Value = %q, want %q", name, got, want)
		}
	}
	v, err := ClosedOpen[int32](1, 10).Value()
	check("int4 closed open", v, err, "[1,10)")
	v, err = NewRange(Excl[int64](-5), Incl[int64](5)).Value()
	check("int8 open closed", v, err, "(-5,5]")
	v, err = NewRange(Incl[int32](5), NoBound[int32]()).Value()
	check("unbounded above", v, err, "[5,)")
	v, err = NewRange(NoBound[int32](), Excl[int32](5)).Value()
	check("unbounded below", v, err, "(,5)")
	v, err = NewRange(NoBound[int32](), NoBound[int32]()).Value()
	check("unbounded", v, err, "(,)")
	v, err = EmptyRange[int32]().Value()
	check("empty", v, err, "empty")
	// An infinite bound ignores its value and inclusiveness.
	v, err = NewRange(Bound[int32]{Value: 9, Inclusive: true, Infinite: true}, Incl[int32](3)).Value()
	check("infinite bound with a value", v, err, "(,3]")
	// Empty wins over the bounds.
	v, err = Range[int32]{Lower: Incl[int32](1), Upper: Incl[int32](2), Empty: true}.Value()
	check("empty with bounds", v, err, "empty")

	v, err = ClosedOpen(Numeric{Decimal: "1.5"}, Numeric{Decimal: "2.5"}).Value()
	check("numrange", v, err, "[1.5,2.5)")
	v, err = ClosedOpen(Date{Time: utc(2026, 1, 1, 0, 0, 0, 0)}, Date{Time: utc(2026, 2, 1, 0, 0, 0, 0)}).Value()
	check("daterange", v, err, "[2026-01-01,2026-02-01)")
	v, err = ClosedOpen(Date{Time: utc(1969, 12, 31, 0, 0, 0, 0)}, Date{Time: utc(1970, 1, 1, 0, 0, 0, 0)}).Value()
	check("daterange before the epoch", v, err, "[1969-12-31,1970-01-01)")
	v, err = NewRange(Incl(Date{Time: utc(2026, 1, 1, 0, 0, 0, 0)}), Incl(Date{Infinity: true})).Value()
	check("daterange with infinity", v, err, "[2026-01-01,infinity]")
	v, err = ClosedOpen(Timestamp{Time: utc(2026, 1, 1, 10, 0, 0, 0)}, Timestamp{Time: utc(2026, 1, 1, 12, 30, 0, 500000000)}).Value()
	check("tsrange", v, err, `["2026-01-01 10:00:00","2026-01-01 12:30:00.5")`)
	v, err = ClosedOpen(time.Date(2026, 1, 1, 14, 0, 0, 0, zone), time.Date(2026, 1, 1, 16, 0, 0, 0, zone)).Value()
	check("tstzrange in another zone", v, err, `["2026-01-01 10:00:00Z","2026-01-01 12:00:00Z")`)

	if _, err := ClosedOpen(Numeric{Decimal: "1x"}, Numeric{Decimal: "2"}).Value(); err == nil {
		t.Error("an invalid numeric bound was accepted")
	}
	if _, err := ClosedOpen(Date{Infinity: true, NegInfinity: true}, Date{}).Value(); err == nil {
		t.Error("a date with both infinities was accepted")
	}
}

// Catches: PostgreSQL's range text read wrongly: the bounds' inclusiveness,
// the empty range, a blank side as an infinite bound, quoted timestamps with
// their zone, and text with escapes or spaces.
func TestRangeScan(t *testing.T) {
	var i4 Range[int32]
	if err := i4.Scan("[1,10)"); err != nil || !reflect.DeepEqual(i4, ClosedOpen[int32](1, 10)) {
		t.Errorf("int4: %+v, %v", i4, err)
	}
	if err := i4.Scan([]byte("(,5]")); err != nil || !reflect.DeepEqual(i4, NewRange(NoBound[int32](), Incl[int32](5))) {
		t.Errorf("int4 from bytes: %+v, %v", i4, err)
	}
	if err := i4.Scan("(,)"); err != nil || !reflect.DeepEqual(i4, NewRange(NoBound[int32](), NoBound[int32]())) {
		t.Errorf("int4 unbounded: %+v, %v", i4, err)
	}
	if err := i4.Scan("EMPTY"); err != nil || !i4.IsEmpty() {
		t.Errorf("int4 empty: %+v, %v", i4, err)
	}
	// A value scanned over a non-empty one replaces it entirely.
	i4 = ClosedOpen[int32](1, 2)
	if err := i4.Scan("empty"); err != nil || !reflect.DeepEqual(i4, EmptyRange[int32]()) {
		t.Errorf("rescan into a used range: %+v, %v", i4, err)
	}

	var i8 Range[int64]
	if err := i8.Scan("[-9000000000,9000000000)"); err != nil || !reflect.DeepEqual(i8, ClosedOpen[int64](-9000000000, 9000000000)) {
		t.Errorf("int8: %+v, %v", i8, err)
	}

	var n Range[Numeric]
	if err := n.Scan("[1.5,2.5]"); err != nil || !reflect.DeepEqual(n, NewRange(Incl(Numeric{Decimal: "1.5"}), Incl(Numeric{Decimal: "2.5"}))) {
		t.Errorf("numeric: %+v, %v", n, err)
	}

	var d Range[Date]
	if err := d.Scan("[2026-01-01,infinity)"); err != nil || !reflect.DeepEqual(d, NewRange(Incl(Date{Time: utc(2026, 1, 1, 0, 0, 0, 0)}), Excl(Date{Infinity: true}))) {
		t.Errorf("date: %+v, %v", d, err)
	}

	var ts Range[Timestamp]
	if err := ts.Scan(`["2026-01-01 10:00:00","2026-01-01 12:30:00.5")`); err != nil ||
		!reflect.DeepEqual(ts, ClosedOpen(Timestamp{Time: utc(2026, 1, 1, 10, 0, 0, 0)}, Timestamp{Time: utc(2026, 1, 1, 12, 30, 0, 500000000)})) {
		t.Errorf("timestamp: %+v, %v", ts, err)
	}

	var tz Range[time.Time]
	for _, in := range []string{
		`["2026-01-01 10:00:00+00","2026-01-01 12:00:00+00")`,
		`["2026-01-01 14:00:00+04","2026-01-01 16:00:00+04")`,
		`["2026-01-01 15:30:00+05:30","2026-01-01 17:30:00+05:30")`,
		`["2026-01-01 10:00:00Z","2026-01-01 12:00:00Z")`,
	} {
		want := ClosedOpen(utc(2026, 1, 1, 10, 0, 0, 0), utc(2026, 1, 1, 12, 0, 0, 0))
		if err := tz.Scan(in); err != nil || !tz.Lower.Value.Equal(want.Lower.Value) || !tz.Upper.Value.Equal(want.Upper.Value) ||
			!tz.Lower.Inclusive || tz.Upper.Inclusive {
			t.Errorf("timestamptz %s: %+v, %v", in, tz, err)
		}
	}

	var s Range[int32]
	for _, bad := range []string{"", "1,2", "[1,2", "[1;2)", "[1,2,3)", "[a,2)", "[1,99999999999)", "[1,2)x", `["1,2)`, `[\`} {
		if err := s.Scan(bad); err == nil {
			t.Errorf("Scan(%q) = nil, want an error", bad)
		}
	}
	if err := s.Scan(nil); err == nil || !strings.Contains(err.Error(), "NULL") {
		t.Errorf("Scan(nil) = %v, want an error that names NULL", err)
	}
	if err := s.Scan(42); err == nil {
		t.Error("Scan(42) = nil, want an error")
	}
}

// Catches: a range that does not read back as written for each element type, in
// every bound shape, and a quoted element (one with a space or a comma) that
// splits at the wrong place.
func TestRangeTextRoundTrip(t *testing.T) {
	roundTrip := func(name string, r driver.Valuer, fresh sql.Scanner, same func() bool) {
		t.Helper()
		v, err := r.Value()
		if err != nil {
			t.Fatalf("%s: Value: %v", name, err)
		}
		if err := fresh.Scan(v); err != nil {
			t.Fatalf("%s: Scan(%v): %v", name, v, err)
		}
		if !same() {
			t.Errorf("%s: %v did not read back as written", name, v)
		}
	}
	for _, in := range []Range[int32]{
		ClosedOpen[int32](-1, 1), EmptyRange[int32](), NewRange(NoBound[int32](), Incl[int32](0)),
		NewRange(Excl[int32](7), NoBound[int32]()), NewRange(NoBound[int32](), NoBound[int32]()),
	} {
		var out Range[int32]
		roundTrip("int4 "+in.String(), in, &out, func() bool { return reflect.DeepEqual(in, out) })
	}
	for _, in := range []Range[Timestamp]{
		ClosedOpen(Timestamp{Time: utc(2026, 1, 1, 10, 0, 0, 123456000)}, Timestamp{Time: utc(2026, 1, 2, 10, 0, 0, 0)}),
		NewRange(NoBound[Timestamp](), Excl(Timestamp{Time: utc(2026, 1, 2, 10, 0, 0, 0)})),
		ClosedOpen(Timestamp{Time: utc(2026, 1, 1, 23, 59, 45, 0)}, Timestamp{Time: utc(2026, 1, 2, 0, 0, 1, 0)}),
	} {
		var out Range[Timestamp]
		roundTrip("ts "+in.String(), in, &out, func() bool { return reflect.DeepEqual(in, out) })
	}
	for _, in := range []Range[time.Time]{
		ClosedOpen(utc(2026, 1, 1, 10, 0, 0, 123456000), utc(2026, 1, 2, 10, 0, 0, 0)),
		NewRange(Incl(utc(2026, 1, 1, 10, 0, 0, 0)), NoBound[time.Time]()),
	} {
		var out Range[time.Time]
		roundTrip("tstz "+in.String(), in, &out, func() bool { return reflect.DeepEqual(in, out) })
	}
}

// Catches: a range element quoted wrongly, which would change what the server
// reads: a plain number quoted, a timestamp with a space left bare, a quote or
// a backslash not escaped.
func TestQuoteRangeElem(t *testing.T) {
	for in, want := range map[string]string{
		"12":         "12",
		"-1.5":       "-1.5",
		"2026-01-01": "2026-01-01",
		"a b":        `"a b"`,
		"a,b":        `"a,b"`,
		`a"b`:        `"a\"b"`,
		`a\b`:        `"a\\b"`,
		"":           `""`,
		"(x)":        `"(x)"`,
	} {
		if got := quoteRangeElem(in); got != want {
			t.Errorf("quoteRangeElem(%q) = %s, want %s", in, got, want)
		}
	}
}

// Catches: a range type name that does not match its element, which would make
// a migration create a column the codec cannot write, and a dialect other than
// PostgreSQL claiming a column type.
func TestRangeSQLType(t *testing.T) {
	for got, want := range map[string]string{
		Range[int32]{}.SQLType(dialect.Postgres):     "int4range",
		Range[int64]{}.SQLType(dialect.Postgres):     "int8range",
		Range[Numeric]{}.SQLType(dialect.Postgres):   "numrange",
		Range[Date]{}.SQLType(dialect.Postgres):      "daterange",
		Range[Timestamp]{}.SQLType(dialect.Postgres): "tsrange",
		Range[time.Time]{}.SQLType(dialect.Postgres): "tstzrange",
	} {
		if got != want {
			t.Errorf("SQLType = %q, want %q", got, want)
		}
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		if got := (Range[int32]{}).SQLType(d); got != "" {
			t.Errorf("SQLType(%s) = %q, want none: only PostgreSQL has range types", d, got)
		}
	}
	if got := (Timestamp{}).SQLType(dialect.Postgres); got != "timestamp" {
		t.Errorf("Timestamp SQLType = %q", got)
	}
}

// Catches: a Timestamp that shifts by its zone: the column has no zone, so the
// wall clock is what is stored, and what is read back is that wall clock.
func TestTimestampCodec(t *testing.T) {
	zone := time.FixedZone("UTC+4", 4*3600)
	v, err := Timestamp{Time: time.Date(2026, 1, 1, 10, 0, 0, 0, zone)}.Value()
	if err != nil || v != "2026-01-01 10:00:00" {
		t.Fatalf("Value = %v, %v; want the wall clock without a zone", v, err)
	}
	var back Timestamp
	if err := back.Scan(time.Date(2026, 1, 1, 10, 0, 0, 0, zone)); err != nil || !back.Time.Equal(utc(2026, 1, 1, 10, 0, 0, 0)) {
		t.Fatalf("Scan(time.Time) = %+v, %v; want the wall clock as UTC", back, err)
	}
	if err := back.Scan("2026-01-01 10:00:00.25"); err != nil || back.Time.Nanosecond() != 250000000 {
		t.Fatalf("Scan(string) = %+v, %v", back, err)
	}
	if err := back.Scan("not a time"); err == nil {
		t.Fatal("Scan(garbage) = nil, want an error")
	}
	if err := back.Scan(nil); err == nil {
		t.Fatal("Scan(nil) = nil, want an error")
	}
}

func FuzzRangeScan(f *testing.F) {
	for _, s := range []string{"[1,10)", "(,)", "empty", `["a b",c)`, "[1,2", `[\`, "(,5]"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		var r Range[int32]
		if err := r.Scan(s); err != nil {
			return
		}
		v, err := r.Value()
		if err != nil {
			t.Fatalf("Value after Scan(%q): %v", s, err)
		}
		var back Range[int32]
		// A range with no bound set reads back as the empty range, which is
		// the same range under another Go value: compare the text.
		if err := back.Scan(v); err != nil || back.String() != r.String() {
			t.Fatalf("round trip of %q through %v: %+v, %v", s, v, back, err)
		}
	})
}

// Catches: an unset Range field stored as the range (0,0), which then reads
// back as an empty range, so a field the caller never set would change on a
// round trip. The zero value is the empty range: Value writes "empty", and a
// range read back from it is empty, equal by IsEmpty and by its text, though
// not by reflect.DeepEqual.
func TestZeroRangeIsEmpty(t *testing.T) {
	check := func(name string, v driver.Value, err error, empty bool) {
		t.Helper()
		if err != nil || v != "empty" || !empty {
			t.Errorf("%s: Value = %v, %v, IsEmpty = %v; want \"empty\" and true", name, v, err, empty)
		}
	}
	var i4 Range[int32]
	v, err := i4.Value()
	check("int4", v, err, i4.IsEmpty())
	var i8 Range[int64]
	v, err = i8.Value()
	check("int8", v, err, i8.IsEmpty())
	var n Range[Numeric]
	v, err = n.Value()
	check("numeric", v, err, n.IsEmpty())
	var d Range[Date]
	v, err = d.Value()
	check("date", v, err, d.IsEmpty())
	var ts Range[Timestamp]
	v, err = ts.Value()
	check("timestamp", v, err, ts.IsEmpty())
	var tz Range[time.Time]
	v, err = tz.Value()
	check("timestamptz", v, err, tz.IsEmpty())

	var back Range[int32]
	if err := back.Scan("empty"); err != nil || !back.IsEmpty() || back.String() != i4.String() {
		t.Fatalf("the zero value scanned back as %+v, %v", back, err)
	}
	if !back.Empty {
		t.Errorf("a range read as empty does not have Empty set: %+v", back)
	}
	// A range with a bound set is not the zero value, even when it holds no
	// point: ClosedOpen(0, 0) keeps its bounds in Go.
	if ClosedOpen[int32](0, 0).IsEmpty() {
		t.Error("ClosedOpen(0, 0) was taken for the zero value")
	}
	if v, err := ClosedOpen[int32](0, 0).Value(); err != nil || v != "[0,0)" {
		t.Errorf("ClosedOpen(0, 0).Value = %v, %v", v, err)
	}
	// PostgreSQL reads (0,0) as empty too, and so does the text form.
	if err := back.Scan("(0,0)"); err != nil || back.String() != "empty" {
		t.Errorf("Scan((0,0)) = %+v, %v", back, err)
	}
}

// Catches: a tsrange with an infinity bound failing to scan, or a Timestamp
// infinity written as a wall clock.
func TestTimestampInfinity(t *testing.T) {
	var r Range[Timestamp]
	if err := r.Scan(`["2026-01-01 10:00:00",infinity)`); err != nil {
		t.Fatal(err)
	}
	if !r.Upper.Value.Infinity || r.Upper.Infinite || r.Upper.Inclusive || r.Lower.Value.Time.IsZero() {
		t.Errorf("tsrange with an infinity bound = %+v", r)
	}
	if err := r.Scan(`(-infinity,"2026-01-01 10:00:00"]`); err != nil || !r.Lower.Value.NegInfinity {
		t.Errorf("tsrange from -infinity = %+v, %v", r, err)
	}
	v, err := NewRange(Incl(Timestamp{Time: utc(2026, 1, 1, 10, 0, 0, 0)}), Excl(Timestamp{Infinity: true})).Value()
	if err != nil || v != `["2026-01-01 10:00:00",infinity)` {
		t.Errorf("Value = %v, %v", v, err)
	}
	if v, err := (Timestamp{NegInfinity: true}).Value(); err != nil || v != "-infinity" {
		t.Errorf("-infinity Value = %v, %v", v, err)
	}
	if _, err := (Timestamp{Infinity: true, NegInfinity: true}).Value(); err == nil {
		t.Error("a timestamp with both infinities was accepted")
	}
	var tstz Range[time.Time]
	if err := tstz.Scan(`["2026-01-01 10:00:00+00",infinity)`); err == nil {
		t.Error("a tstzrange infinity bound scanned into a time.Time")
	}
}
