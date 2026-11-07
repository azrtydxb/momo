// SPDX-License-Identifier: Apache-2.0

//go:build cgo && !windows

package sqlitedriver

import (
	"testing"
	"time"
)

func TestTimeScanners(t *testing.T) {
	want := time.Date(2026, 11, 7, 20, 10, 12, 123000000, time.UTC)
	for _, in := range []any{
		"2026-11-07 20:10:12.123",
		"2026-11-07T20:10:12.123Z",
		"2026-11-07 21:10:12.123+01:00",
		[]byte("2026-11-07 20:10:12.123"),
		want,
	} {
		var got time.Time
		if err := (timeScanner{&got}).Scan(in); err != nil {
			t.Fatalf("Scan(%v): %v", in, err)
		}
		if !got.Equal(want) {
			t.Errorf("Scan(%v) = %v, want %v", in, got, want)
		}
	}
	var d time.Time
	if err := (timeScanner{&d}).Scan("2026-11-07"); err != nil || d.Day() != 7 {
		t.Errorf("date-only scan = %v, %v", d, err)
	}
	var bad time.Time
	if err := (timeScanner{&bad}).Scan("yesterday"); err == nil {
		t.Error("garbage parsed as a time")
	}
	if err := (timeScanner{&bad}).Scan(nil); err == nil {
		t.Error("NULL scanned into time.Time")
	}
	var p *time.Time
	if err := (timePtrScanner{&p}).Scan(nil); err != nil || p != nil {
		t.Errorf("NULL into **time.Time = %v, %v", p, err)
	}
	if err := (timePtrScanner{&p}).Scan("2026-11-07 20:10:12.123"); err != nil || p == nil || !p.Equal(want) {
		t.Errorf("value into **time.Time = %v, %v", p, err)
	}
}

func TestWrapDestsLeavesOthers(t *testing.T) {
	var s string
	dest := []any{&s}
	if got := wrapDests(dest); &got[0] != &dest[0] {
		t.Error("wrapDests copied a slice with nothing to replace")
	}
}
