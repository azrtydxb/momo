// SPDX-License-Identifier: Apache-2.0

package types

import "testing"

func FuzzUUIDScan(f *testing.F) {
	f.Add("01020304-0506-0708-090a-0b0c0d0e0fff")
	f.Add("0102030405060708090a0b0c0d0e0fff")
	f.Add("")
	f.Add("zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz")
	f.Fuzz(func(t *testing.T, s string) {
		var u UUID
		if err := u.Scan(s); err != nil {
			return
		}
		var back UUID
		if err := back.Scan(u.String()); err != nil || back != u {
			t.Fatalf("round trip of %q: %v %v", s, back, err)
		}
	})
}

func FuzzTimeOfDayScan(f *testing.F) {
	f.Add("23:59:59.999999999")
	f.Add("24:00:00")
	f.Add("00:00:00.1")
	f.Add("99:99:99.")
	f.Fuzz(func(t *testing.T, s string) {
		var tod TimeOfDay
		if err := tod.Scan(s); err != nil {
			return
		}
		if tod.Nanos < 0 || tod.Nanos >= nanosPerDay {
			t.Fatalf("Scan(%q) produced out-of-range %d", s, tod.Nanos)
		}
		if _, err := tod.Value(); err != nil {
			t.Fatalf("Value after Scan(%q): %v", s, err)
		}
	})
}

func FuzzDateScan(f *testing.F) {
	f.Add("1969-12-31")
	f.Add("0000-01-01")
	f.Add("2024-02-30")
	f.Fuzz(func(t *testing.T, s string) {
		var d Date
		if err := d.Scan(s); err != nil {
			return
		}
		if _, err := d.Value(); err != nil {
			t.Fatalf("Value after Scan(%q): %v", s, err)
		}
	})
}

func FuzzDateInfinityScan(f *testing.F) {
	f.Add("infinity")
	f.Add("-infinity")
	f.Add("Infinity")
	f.Add("2024-02-29")
	f.Fuzz(func(t *testing.T, s string) {
		var d Date
		if err := d.Scan(s); err != nil {
			return
		}
		v, err := d.Value()
		if err != nil {
			t.Fatalf("Value after Scan(%q): %v", s, err)
		}
		if d.IsFinite() != (v != "infinity" && v != "-infinity") {
			t.Fatalf("Scan(%q): IsFinite %v disagrees with Value %v", s, d.IsFinite(), v)
		}
		var back Date
		if err := back.Scan(v); err != nil || back.Infinity != d.Infinity || back.NegInfinity != d.NegInfinity || !back.Time.Equal(d.Time) {
			t.Fatalf("Scan(%q) -> %v does not round trip: %+v, %v", s, v, back, err)
		}
	})
}

func FuzzNumericScan(f *testing.F) {
	for _, s := range []string{"0", "-1.50", "NaN", "Infinity", "-Infinity", "inf", ".5", "7.", "1e5", "", "--1", "\x00"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		var n Numeric
		if err := n.Scan(s); err != nil {
			return
		}
		v, err := n.Value()
		if err != nil {
			t.Fatalf("Value after Scan(%q): %v", s, err)
		}
		var back Numeric
		if err := back.Scan(v); err != nil || back != n {
			t.Fatalf("Scan(%q) -> %v does not round trip: %+v vs %+v, %v", s, v, back, n, err)
		}
		if !n.IsFinite() && n.Sign() == 0 && !n.IsNaN() {
			t.Fatalf("Scan(%q): a non-finite, non-NaN value has Sign 0", s)
		}
	})
}

func FuzzJSONScan(f *testing.F) {
	f.Add(`{"a":1}`)
	f.Add(`null`)
	f.Add(`[`)
	f.Fuzz(func(t *testing.T, s string) {
		var j JSON[map[string]any]
		_ = j.Scan(s) // must not panic
		_ = j.Scan([]byte(s))
	})
}
