// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Catches: a RANGE frame with an offset over one order item that carries an
// explicit NULLS term passing validation on MySQL and MariaDB, where the
// emulation turns it into two sort keys and the server rejects the statement
// (error 3587) instead of momo refusing it with ErrUnsupported.
func TestRangeOffsetFrameRefusesEmulatedNulls(t *testing.T) {
	frame := func(o momo.Order) momo.WindowSpec {
		return momo.Over().OrderBy(o).Range(momo.Preceding(1), momo.CurrentRow())
	}
	build := func(d dialect.Name, w momo.WindowSpec) error {
		_, err := selectOne(d, momo.Sum(DocTenantID).Over(w))
		return err
	}
	for _, o := range []momo.Order{DocTenantID.Asc().NullsLast(), DocTenantID.Desc().NullsFirst()} {
		if err := build(dialect.MySQL, frame(o)); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("mysql RANGE offset with NULLS term = %v, want ErrUnsupported", err)
		}
	}
	// Where the emulation is not in play, or no NULLS term is asked for, it builds.
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
		if err := build(d, frame(DocTenantID.Asc().NullsLast())); err != nil {
			t.Errorf("%s RANGE offset with NULLS term: %v", d, err)
		}
	}
	if err := build(dialect.MySQL, frame(DocTenantID.Asc())); err != nil {
		t.Errorf("mysql RANGE offset without NULLS term: %v", err)
	}
	rows := momo.Over().OrderBy(DocTenantID.Asc().NullsLast()).Rows(momo.Preceding(1), momo.CurrentRow())
	if err := build(dialect.MySQL, rows); err != nil {
		t.Errorf("mysql ROWS frame with NULLS term: %v", err)
	}
}
