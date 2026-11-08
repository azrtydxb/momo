// SPDX-License-Identifier: Apache-2.0

//go:build cgo && !windows

package sqlitedriver

import (
	"context"
	"database/sql"
	"errors"
	"strings"
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

func TestNullTimeScanners(t *testing.T) {
	want := time.Date(2026, 11, 7, 20, 10, 12, 123000000, time.UTC)
	var nt sql.NullTime
	var ng sql.Null[time.Time]
	for _, d := range wrapDests([]any{&nt, &ng}) {
		sc, ok := d.(sql.Scanner)
		if !ok {
			t.Fatalf("%T was not replaced by a scanner", d)
		}
		if err := sc.Scan("2026-11-07 20:10:12.123"); err != nil {
			t.Fatal(err)
		}
	}
	if !nt.Valid || !nt.Time.Equal(want) || !ng.Valid || !ng.V.Equal(want) {
		t.Fatalf("NullTime = %+v, Null[time.Time] = %+v", nt, ng)
	}
	for _, d := range wrapDests([]any{&nt, &ng}) {
		if err := d.(sql.Scanner).Scan(nil); err != nil {
			t.Fatal(err)
		}
	}
	if nt.Valid || ng.Valid {
		t.Fatalf("NULL left Valid set: %+v %+v", nt, ng)
	}
}

func TestScanErrPassesThroughNonConversionErrors(t *testing.T) {
	for _, in := range []error{context.Canceled, context.DeadlineExceeded, sql.ErrNoRows, errors.New("sql: Rows are closed")} {
		if got := scanErr(in); got != in {
			t.Errorf("scanErr(%v) = %v, want it unchanged", in, got)
		}
	}
	conv := errors.New(`sql: Scan error on column index 0, name "x": converting driver.Value type string ("secret") to a int64`)
	got := scanErr(conv)
	if strings.Contains(got.Error(), "secret") || !errors.Is(got, conv) {
		t.Errorf("conversion failure = %v; want it masked with the cause kept", got)
	}
}

func TestParseTimeErrorHidesInput(t *testing.T) {
	_, err := parseTime("secret-user-value")
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("err = %v; it must fail without echoing the text", err)
	}
}

func TestWrapDestsLeavesOthers(t *testing.T) {
	var s string
	dest := []any{&s}
	if got := wrapDests(dest); &got[0] != &dest[0] {
		t.Error("wrapDests copied a slice with nothing to replace")
	}
}
