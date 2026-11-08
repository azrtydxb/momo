// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"database/sql"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/jackc/pgx/v5"
)

func TestTxOptionsMapping(t *testing.T) {
	to, err := txOptions(momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if to.AccessMode != pgx.ReadWrite {
		t.Errorf("ReadOnly=false AccessMode = %q, want %q", to.AccessMode, pgx.ReadWrite)
	}
	to, err = txOptions(momo.TxOptions{ReadOnly: true, Isolation: sql.LevelSerializable})
	if err != nil {
		t.Fatal(err)
	}
	if to.AccessMode != pgx.ReadOnly || to.IsoLevel != pgx.Serializable {
		t.Errorf("got %+v", to)
	}
}
