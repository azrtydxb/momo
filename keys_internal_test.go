// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"database/sql/driver"
	"testing"
	"time"
)

type testUUIDValuer string

func (u testUUIDValuer) Value() (driver.Value, error) { return string(u), nil }

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
	same([]any{time.Date(2020, 1, 1, 1, 0, 0, 0, time.FixedZone("x", 3600))}, []any{time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)})
}
