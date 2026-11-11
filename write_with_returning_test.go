// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Catches: a data-modifying CTE body without RETURNING being accepted, which
// leaves a CTE with no columns that the statement cannot read; docs say it is
// ErrValidation. A body with Returning or ReturningAll stays valid.
func TestDMLCTEBodyNeedsReturning(t *testing.T) {
	pg := withDB(dialect.Postgres)
	bodies := map[string]momo.Subquery{
		"insert": momo.Insert[delNote](pg).Values(delNoteID.Set(9), delNoteTitle.Set("n")),
		"update": momo.Update[delNote](pg).Set(delNoteTitle.Set("u")).Where(delNoteID.Eq(1)),
		"delete": momo.Delete[delNote](pg).Where(delNoteID.Eq(2)),
	}
	for name, body := range bodies {
		if _, err := momo.Select[delNote](pg).With("x", body).From(momo.CTE("x")).SQL(); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("select with %s body, no Returning: err = %v, want ErrValidation", name, err)
		}
		if _, err := momo.Delete[delNote](pg).With("x", body).Where(delNoteID.Eq(1)).SQL(); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("delete with %s body, no Returning: err = %v, want ErrValidation", name, err)
		}
	}
	ok := momo.Delete[delNote](pg).Where(delNoteID.Eq(2)).ReturningAll()
	if _, err := momo.Select[delNote](pg).With("x", ok).From(momo.CTE("x")).SQL(); err != nil {
		t.Errorf("ReturningAll body: %v", err)
	}
}
