// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
)

// Catches: WindowOf dropping the call or the OVER clause, or typing the
// window expression wrongly for a call defined outside the package.
func TestWindowOfRendersCallOverWindow(t *testing.T) {
	tenant := momo.NewColumn[int64]("docs", "tenant", 0)
	w := momo.WindowOf[int64](momo.CountAll(), momo.Over().PartitionBy(tenant))
	got, err := momo.Select[exprtest.Tally](exprtest.DB(dialect.Postgres)).
		From(momo.TableRef{Name: "docs"}).Columns(w).SQL()
	want := `SELECT COUNT(*) OVER (PARTITION BY "docs"."tenant") FROM "docs"`
	if err != nil || got.SQL != want {
		t.Errorf("got %q, %v; want %q", got.SQL, err, want)
	}
}
