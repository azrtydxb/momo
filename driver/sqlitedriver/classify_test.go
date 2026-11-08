// SPDX-License-Identifier: Apache-2.0

//go:build cgo && !windows

package sqlitedriver

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/mattn/go-sqlite3"
)

func TestClassifyUniqueKeepsCause(t *testing.T) {
	in := sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintUnique}
	got := ClassifyError(in)
	if !errors.Is(got, momo.ErrUniqueViolation) {
		t.Fatalf("errors.Is(ErrUniqueViolation) = false for %v", got)
	}
	var se sqlite3.Error
	if !errors.As(got, &se) || se.ExtendedCode != sqlite3.ErrConstraintUnique {
		t.Fatalf("driver error not reachable via errors.As: %v", got)
	}
}

func TestClassifyBusy(t *testing.T) {
	got := ClassifyError(sqlite3.Error{Code: sqlite3.ErrBusy, ExtendedCode: sqlite3.ErrBusy.Extend(0)})
	if !errors.Is(got, momo.ErrBusy) {
		t.Fatalf("SQLITE_BUSY not classified as momo.ErrBusy: %v", got)
	}
}

func TestClassifyPassesThroughUnknown(t *testing.T) {
	plain := errors.New("boom")
	if got := ClassifyError(plain); got != plain {
		t.Fatalf("unknown error changed: %v", got)
	}
}

func TestClassifyKinds(t *testing.T) {
	for _, c := range []struct {
		in   sqlite3.Error
		want error
	}{
		{sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintPrimaryKey}, momo.ErrUniqueViolation},
		{sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintRowID}, momo.ErrUniqueViolation},
		{sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintForeignKey}, momo.ErrForeignKeyViolation},
		{sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintCheck}, momo.ErrCheckViolation},
		{sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintNotNull}, momo.ErrNotNullViolation},
		{sqlite3.Error{Code: sqlite3.ErrLocked, ExtendedCode: sqlite3.ErrLocked.Extend(0)}, momo.ErrBusy},
		{sqlite3.Error{Code: sqlite3.ErrBusy, ExtendedCode: sqlite3.ErrBusySnapshot}, momo.ErrSerialization},
	} {
		if got := ClassifyError(c.in); !errors.Is(got, c.want) {
			t.Errorf("ClassifyError(%d) = %v, want kind %v", c.in.ExtendedCode, got, c.want)
		}
	}
}
