// SPDX-License-Identifier: Apache-2.0

package pgxdriver_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// TestOnConnDriverQueryAndExecUseThePinnedSession: Driver() of an OnConn
// handle must not fall back to the pool, so a TEMP table is visible to it.
func TestOnConnDriverQueryAndExecUseThePinnedSession(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	pc, err := db.Driver().(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pc.Release() }()
	drv := db.OnConn(pc).Driver()
	if _, err := drv.Exec(ctx, "CREATE TEMP TABLE onconn_qe (id int)", nil); err != nil {
		t.Fatal(err)
	}
	if n, err := drv.Exec(ctx, "INSERT INTO onconn_qe VALUES (1), (2)", nil); err != nil || n != 2 {
		t.Fatalf("insert: n=%d err=%v", n, err)
	}
	rows, err := drv.Query(ctx, "SELECT count(*) FROM onconn_qe", nil)
	if err != nil {
		t.Fatal(err)
	}
	var got int
	if !rows.Next() {
		t.Fatalf("no row: %v", rows.Err())
	}
	if err := rows.Scan(&got); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	if got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
	if _, err := drv.Begin(ctx, momo.TxOptions{}); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Begin on a pinned driver: %v, want ErrUnsupported", err)
	}
}
