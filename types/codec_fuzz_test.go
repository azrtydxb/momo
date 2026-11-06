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
