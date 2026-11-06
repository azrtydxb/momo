// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"database/sql/driver"
	"testing"
	"time"
)

type testUUIDValuer string

type testUserID string

type testOrderID int64

func (u testUUIDValuer) Value() (driver.Value, error) { return string(u), nil }

// selfValuer returns itself from Value, which used to recurse without bound.
type selfValuer struct{}

func (s selfValuer) Value() (driver.Value, error) { return s, nil }

func TestKeyStringSelfReturningValuerTerminates(t *testing.T) {
	a := keyString([]any{selfValuer{}})
	if a == "" || a != keyString([]any{selfValuer{}}) {
		t.Fatalf("unstable key for a self-returning Valuer: %q", a)
	}
}

func TestKeyStringNormalization(t *testing.T) {
	same := func(a, b []any) {
		t.Helper()
		if keyString(a) != keyString(b) {
			t.Fatalf("keys differ: %q vs %q", keyString(a), keyString(b))
		}
	}
	diff := func(a, b []any) {
		t.Helper()
		if keyString(a) == keyString(b) {
			t.Fatalf("keys collide: %q", keyString(a))
		}
	}
	same([]any{int64(7)}, []any{7})                  // int vs int64 from driver
	same([]any{int32(7)}, []any{int64(7)})           // int32 scanned from a smaller column
	same([]any{[]byte("ab")}, []any{"ab"})           // bytea/text from driver
	same([]any{testUUIDValuer("u-1")}, []any{"u-1"}) // driver.Valuer is normalized through Value()
	diff([]any{"a", "b"}, []any{"ab", ""})           // composite components are delimited
	diff([]any{int64(1)}, []any{"1"})                // type tag keeps 1 and "1" apart
	diff([]any{"a", "b"}, []any{"as:b"})             // delimiter, not the type tag, is what separates components
	diff([]any{nil}, []any{""})                      // NULL is distinct from empty string
	same([]any{testUserID("u-9")}, []any{"u-9"})     // named string field vs driver string
	same([]any{testOrderID(42)}, []any{int64(42)})   // named int field vs driver int64
	same([]any{uint64(42)}, []any{int64(42)})        // unsigned field vs signed driver value
	same([]any{uint32(7)}, []any{7})                 // small unsigned vs int
	diff([]any{uint64(1 << 63)}, []any{int64(-1 << 63)})
	diff([]any{"a\x1fs:b"}, []any{"a", "b"}) // a key containing the delimiter byte does not collide
	same([]any{time.Date(2020, 1, 1, 1, 0, 0, 0, time.FixedZone("x", 3600))}, []any{time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)})
}
