// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestScanValueNullHandling(t *testing.T) {
	var s *string
	if err := ScanValue(&s, nil); err != nil || s != nil {
		t.Fatalf("NULL into **string must set nil: err=%v s=%v", err, s)
	}
	var n int64
	if err := ScanValue(&n, nil); !errors.Is(err, ErrValidation) {
		t.Fatalf("NULL into non-nullable int64 must fail with ErrValidation, got %v", err)
	}
	var v int64
	if err := ScanValue(&v, int64(9)); err != nil || v != 9 {
		t.Fatalf("int64 round trip: %v %d", err, v)
	}
}

func TestScanValueTypes(t *testing.T) {
	now := time.Now()
	var (
		str  string
		pstr *string
		i32  int32
		pint *int
		b    bool
		f    float64
		raw  []byte
		tm   time.Time
		ptm  *time.Time
		ns   sql.NullString
	)
	steps := []struct {
		name string
		dst  any
		src  any
	}{
		{"string from bytes", &str, []byte("x")},
		{"**string", &pstr, "y"},
		{"int32 from int32", &i32, int32(4)},
		{"**int", &pint, int64(5)},
		{"bool", &b, true},
		{"float64", &f, 1.5},
		{"bytes", &raw, "z"},
		{"time", &tm, now},
		{"**time", &ptm, now},
		{"scanner", &ns, "q"},
	}
	for _, s := range steps {
		if err := ScanValue(s.dst, s.src); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
	}
	if str != "x" || *pstr != "y" || i32 != 4 || *pint != 5 || !b || f != 1.5 || string(raw) != "z" || !tm.Equal(now) || !ptm.Equal(now) || ns.String != "q" {
		t.Fatalf("unexpected values: %v %v %v %v %v %v %q %v %v %v", str, *pstr, i32, *pint, b, f, raw, tm, ptm, ns)
	}
	if err := ScanValue(&ns, nil); err != nil || ns.Valid {
		t.Fatalf("NULL into sql.Scanner: %v %+v", err, ns)
	}
	// An unconvertible source and an unsupported destination are validation
	// errors, and a failed conversion leaves the destination untouched.
	str = "keep"
	if err := ScanValue(&str, int64(1)); !errors.Is(err, ErrValidation) || str != "keep" {
		t.Fatalf("wrong source type: err=%v str=%q", err, str)
	}
	if err := ScanValue(&struct{}{}, "x"); !errors.Is(err, ErrValidation) {
		t.Fatalf("unsupported destination: %v", err)
	}
	var small int16
	if err := ScanValue(&small, int64(70000)); !errors.Is(err, ErrValidation) {
		t.Fatalf("out-of-range int16 must be a validation error, got %v", err)
	}
	raw2 := []byte("old")
	if err := ScanValue(&raw2, nil); err != nil || raw2 != nil {
		t.Fatalf("NULL into *[]byte: %v %v", err, raw2)
	}
}
