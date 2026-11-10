// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// Catches: a JoinSQL with no parts, which renders 1 = 1 or 1 = 0, counting as a
// filter and letting an unfiltered Delete or Update through.
func TestEmptyJoinSQLIsNotAFilter(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	var none []momo.Expr
	for name, p := range map[string]momo.Predicate{
		"AND":        momo.JoinSQL(" AND ", none...),
		"or lower":   momo.JoinSQL("or", none...),
		"inside And": momo.And(momo.JoinSQL(" AND ", none...)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := momo.Delete[delNote](pg).Where(p).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
				t.Errorf("Delete err = %v, want ErrUnfilteredMutation", err)
			}
			if _, err := momo.Update[unitDocument](pg).Set(DocTitle.Set("x")).Where(p).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
				t.Errorf("Update err = %v, want ErrUnfilteredMutation", err)
			}
			if _, err := momo.Delete[delNote](pg).Where(p).AllRows().SQL(); err != nil {
				t.Errorf("Delete AllRows: %v", err)
			}
		})
	}
	// A join with a real part is a filter.
	real := momo.JoinSQL(" AND ", momo.SQL("id = ?", 1))
	if _, err := momo.Delete[delNote](pg).Where(real).SQL(); err != nil {
		t.Errorf("Delete with a real JoinSQL part: %v", err)
	}
	if _, err := momo.Update[unitDocument](pg).Set(DocTitle.Set("x")).Where(real).SQL(); err != nil {
		t.Errorf("Update with a real JoinSQL part: %v", err)
	}
}
