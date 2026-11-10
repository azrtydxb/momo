// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// TestRefreshMaterializedView checks the statement the helper renders: the
// name is quoted, a schema.name argument is split, CONCURRENTLY is optional.
func TestRefreshMaterializedView(t *testing.T) {
	cases := []struct {
		name         string
		concurrently bool
		want         string
	}{
		{"order_totals", false, `REFRESH MATERIALIZED VIEW "order_totals"`},
		{"order_totals", true, `REFRESH MATERIALIZED VIEW CONCURRENTLY "order_totals"`},
		{"reports.order_totals", false, `REFRESH MATERIALIZED VIEW "reports"."order_totals"`},
		{`we"ird`, false, `REFRESH MATERIALIZED VIEW "we""ird"`},
		{"what?is$1", false, `REFRESH MATERIALIZED VIEW "what?is$1"`},
	}
	for _, c := range cases {
		fd := fakedriver.New(dialect.Postgres)
		if err := momo.RefreshMaterializedView(context.Background(), momo.New(fd), c.name, c.concurrently); err != nil {
			t.Fatalf("%q: %v", c.name, err)
		}
		calls := fd.Calls()
		if len(calls) != 1 || calls[0].SQL != c.want || len(calls[0].Args) != 0 {
			t.Errorf("%q concurrently=%v: calls = %+v, want one call %q", c.name, c.concurrently, calls, c.want)
		}
	}
}

// TestRefreshMaterializedViewRefusals checks that a bad name is refused with
// ErrValidation and a dialect without materialized views with ErrUnsupported,
// in both cases before any statement is sent.
func TestRefreshMaterializedViewRefusals(t *testing.T) {
	for _, name := range []string{"", ".x", "x.", "a.b.c", "a..b"} {
		fd := fakedriver.New(dialect.Postgres)
		err := momo.RefreshMaterializedView(context.Background(), momo.New(fd), name, false)
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("name %q: err = %v, want ErrValidation", name, err)
		}
		if n := len(fd.Calls()); n != 0 {
			t.Errorf("name %q: %d statements sent, want 0", name, n)
		}
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		fd := fakedriver.New(d)
		err := momo.RefreshMaterializedView(context.Background(), momo.New(fd), "v", false)
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", d, err)
		}
		if n := len(fd.Calls()); n != 0 {
			t.Errorf("%s: %d statements sent, want 0", d, n)
		}
	}
}
