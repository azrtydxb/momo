// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
)

// obsEach runs fn once per dialect against a database-free fake driver. The
// statements are recorded, never executed, which is all these tests need:
// they check event fields, not results.
func obsEach(t *testing.T, fn func(t *testing.T, drv momo.Driver, d dialect.Name)) {
	t.Helper()
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
		t.Run(string(d), func(t *testing.T) { fn(t, fakedriver.New(d), d) })
	}
}

// waitDriver reports a fixed pool wait twice per statement, the way a pool
// tracer that observes two acquisitions would.
type waitDriver struct {
	momo.Driver
	wait time.Duration
}

func (d waitDriver) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	momo.RecordPoolWait(ctx, d.wait)
	momo.RecordPoolWait(ctx, d.wait)
	return d.Driver.Query(ctx, sql, args)
}

func (d waitDriver) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	momo.RecordPoolWait(ctx, d.wait)
	momo.RecordPoolWait(ctx, d.wait)
	return d.Driver.Exec(ctx, sql, args)
}

func TestQueryEventPoolWait(t *testing.T) {
	ctx := context.Background()
	rec := &obstest.Recorder{}
	obsEach(t, func(t *testing.T, drv momo.Driver, d dialect.Name) {
		rec.Reset()
		wdb := momo.New(waitDriver{Driver: drv, wait: 5 * time.Millisecond}, momo.WithHook(rec))
		if _, err := momo.Exec(ctx, wdb, `CREATE TABLE pw (x INTEGER)`); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, wdb, `INSERT INTO pw (x) VALUES (?)`, 7); err != nil {
			t.Fatal(err)
		}
		ev := rec.Events()
		if len(ev) != 2 {
			t.Fatalf("want 2 events, got %d", len(ev))
		}
		for i, e := range ev {
			if e.PoolWait != 10*time.Millisecond {
				t.Fatalf("event %d PoolWait = %v, want 10ms (two recorded waits are summed)", i, e.PoolWait)
			}
			if e.InTx {
				t.Fatalf("event %d InTx = true outside a transaction", i)
			}
		}
		if ev[1].NumArgs != 1 || ev[1].Args != nil {
			t.Fatalf("NumArgs = %d Args = %v, want 1 and nil without WithArgLogging", ev[1].NumArgs, ev[1].Args)
		}
		if ev[0].Dialect == nil || ev[0].Dialect.Name != d {
			t.Fatalf("event Dialect = %+v, want %s", ev[0].Dialect, d)
		}
	})
}

func TestRecordPoolWaitWithoutHooks(t *testing.T) {
	ctx := context.Background()
	momo.RecordPoolWait(ctx, time.Hour) // no slot in the context: must be a no-op
	obsEach(t, func(t *testing.T, drv momo.Driver, d dialect.Name) {
		wdb := momo.New(waitDriver{Driver: drv, wait: time.Hour})
		if _, err := momo.Exec(ctx, wdb, `CREATE TABLE pw (x INTEGER)`); err != nil {
			t.Fatal(err)
		}
	})
}

func TestQueryEventInTx(t *testing.T) {
	ctx := context.Background()
	rec := &obstest.Recorder{}
	obsEach(t, func(t *testing.T, drv momo.Driver, d dialect.Name) {
		db := momo.New(drv, momo.WithHook(rec))
		if _, err := momo.Exec(ctx, db, `CREATE TABLE pw (x INTEGER)`); err != nil {
			t.Fatal(err)
		}
		rec.Reset()
		err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			_, err := momo.Exec(ctx, tx, `INSERT INTO pw (x) VALUES (?)`, 1)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		var ops []string
		var inTx []bool
		var owners []string
		for _, e := range rec.Events() {
			ops = append(ops, e.Op)
			inTx = append(inTx, e.InTx)
			owners = append(owners, fmt.Sprintf("%T", e.Querier))
		}
		wantOps := []string{"begin", "exec", "commit"}
		wantTx := []bool{false, true, true}
		wantOwners := []string{"*momo.DB", "*momo.Tx", "*momo.Tx"}
		for i := range wantOps {
			if i >= len(ops) || ops[i] != wantOps[i] || inTx[i] != wantTx[i] || owners[i] != wantOwners[i] {
				t.Fatalf("ops %v inTx %v owners %v, want %v %v %v", ops, inTx, owners, wantOps, wantTx, wantOwners)
			}
		}
	})
}
