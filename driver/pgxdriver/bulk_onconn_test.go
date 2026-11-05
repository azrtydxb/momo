// SPDX-License-Identifier: Apache-2.0

package pgxdriver_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// TestCopyAndBatchOnConnUseThePinnedSession proves that a DB made with OnConn
// reaches COPY and batch through the pinned session, both through CopierOf and
// through Driver(): a TEMP table exists on that session only.
func TestCopyAndBatchOnConnUseThePinnedSession(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	pn, ok := db.Driver().(momo.Pinner)
	if !ok {
		t.Fatal("driver is not a Pinner")
	}
	pc, err := pn.Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pc.Release() }()
	pdb := db.OnConn(pc)
	if _, err := momo.Exec(ctx, pdb, "CREATE TEMP TABLE onconn_copy (id bigint)"); err != nil {
		t.Fatal(err)
	}
	c, ok := momo.CopierOf(pdb)
	if !ok {
		t.Fatal("OnConn DB is not a Copier")
	}
	if n, err := c.CopyFrom(ctx, "onconn_copy", []string{"id"}, [][]any{{int64(1)}, {int64(2)}}); err != nil || n != 2 {
		t.Fatalf("CopierOf copy: n=%d err=%v", n, err)
	}
	dc, ok := pdb.Driver().(momo.Copier)
	if !ok {
		t.Fatal("OnConn Driver() is not a Copier")
	}
	if n, err := dc.CopyFrom(ctx, "onconn_copy", []string{"id"}, [][]any{{int64(3)}}); err != nil || n != 1 {
		t.Fatalf("Driver() copy must hit the pinned session: n=%d err=%v", n, err)
	}
	bt, ok := pdb.Driver().(momo.Batcher)
	if !ok {
		t.Fatal("OnConn Driver() is not a Batcher")
	}
	rs, err := bt.SendBatch(ctx, []momo.Compiled{{SQL: "SELECT count(*) FROM onconn_copy"}})
	if err != nil {
		t.Fatalf("Driver() batch must hit the pinned session: %v", err)
	}
	if !rs[0].Next() {
		t.Fatal("no row")
	}
	var n int
	if err := rs[0].Scan(&n); err != nil {
		t.Fatal(err)
	}
	_ = rs[0].Close()
	if n != 3 {
		t.Fatalf("batch saw %d rows, want 3", n)
	}
}
