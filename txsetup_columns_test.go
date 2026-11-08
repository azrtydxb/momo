// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// colsDriver answers every transaction query with named columns.
type colsDriver struct{ *fakedriver.Driver }

func (d colsDriver) Begin(ctx context.Context, o momo.TxOptions) (momo.DriverTx, error) {
	tx, err := d.Driver.Begin(ctx, o)
	return colsTx{tx}, err
}

type colsTx struct{ momo.DriverTx }

func (colsTx) Query(context.Context, string, []any) (momo.Rows, error) {
	return &scriptedRows{cols: []string{"id", "title"}, rows: [][]any{{int64(1), "a"}}}, nil
}

// Catches: the implicit-transaction rows wrapper hiding the column names, which
// made MapRows fail with ErrUnsupported under WithImplicitTx.
func TestMapRowsUnderImplicitTx(t *testing.T) {
	drv := colsDriver{fakedriver.New(dialect.Postgres)}
	db := momo.New(drv, momo.WithImplicitTx())
	got, err := momo.MapRows(t.Context(), db, "SELECT id, title FROM documents")
	if err != nil || len(got) != 1 || got[0]["title"] != "a" || got[0]["id"] != int64(1) {
		t.Fatalf("MapRows = %v, %v", got, err)
	}
}
