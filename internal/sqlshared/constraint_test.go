// SPDX-License-Identifier: Apache-2.0

package sqlshared

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
)

func TestConstraintTarget(t *testing.T) {
	for msg, want := range map[string][2]string{
		"UNIQUE constraint failed: users.email":     {"users", "email"},
		"NOT NULL constraint failed: u.name":        {"u", "name"},
		"UNIQUE constraint failed: a.x, a.y":        {"a", "x"},
		"UNIQUE constraint failed: t.rowid":         {"t", "rowid"},
		"FOREIGN KEY constraint failed":             {"", ""},
		"CHECK constraint failed: positive_balance": {"", ""},
		"database is locked":                        {"", ""},
		"something: users.email":                    {"", ""},
	} {
		table, column := ConstraintTarget(msg)
		if table != want[0] || column != want[1] {
			t.Errorf("ConstraintTarget(%q) = %q, %q; want %q, %q", msg, table, column, want[0], want[1])
		}
	}
}

func TestConstraintKind(t *testing.T) {
	for msg, want := range map[string]error{
		"UNIQUE constraint failed: t.rowid":   momo.ErrUniqueViolation,
		"PRIMARY KEY constraint failed: t.id": momo.ErrUniqueViolation,
		"FOREIGN KEY constraint failed":       momo.ErrForeignKeyViolation,
		"CHECK constraint failed: positive":   momo.ErrCheckViolation,
		"NOT NULL constraint failed: t.name":  momo.ErrNotNullViolation,
		"constraint failed":                   nil,
		"database is locked":                  nil,
		"":                                    nil,
	} {
		if got := ConstraintKind(msg); got != want {
			t.Errorf("ConstraintKind(%q) = %v, want %v", msg, got, want)
		}
	}
}

func TestMaskConversionErrHidesQuotedValue(t *testing.T) {
	never := func(error) bool { return false }
	in := errors.New(`sql: Scan error on column index 0, name "x": converting driver.Value type string ("secret-value") to a int: invalid syntax`)
	got := MaskConversionErr(in, "fixed", never)
	if got.Error() != "fixed" || !errors.Is(got, in) {
		t.Fatalf("got %q; want the fixed text with the cause reachable", got)
	}
	other := errors.New("boom")
	if MaskConversionErr(other, "fixed", never) != other {
		t.Error("an unrelated error was masked")
	}
	if MaskConversionErr(in, "fixed", func(error) bool { return true }) != in {
		t.Error("a driver error was masked")
	}
	if MaskConversionErr(nil, "fixed", never) != nil {
		t.Error("nil became an error")
	}
}
