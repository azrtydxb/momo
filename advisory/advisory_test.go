// SPDX-License-Identifier: Apache-2.0

package advisory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/advisory"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestXactLockStatements(t *testing.T) {
	ctx := context.Background()
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{true}})
	db := momo.New(drv)
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if err := advisory.XactLock(ctx, tx, 42); err != nil {
			return err
		}
		if err := advisory.XactLockString(ctx, tx, "catalog:1"); err != nil {
			return err
		}
		if ok, err := advisory.TryXactLock(ctx, tx, 7); err != nil || !ok {
			t.Errorf("TryXactLock = %v, %v; want true, nil", ok, err)
		}
		if ok, err := advisory.TryXactLockString(ctx, tx, "doc:9"); err != nil || !ok {
			t.Errorf("TryXactLockString = %v, %v; want true, nil", ok, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"SELECT pg_advisory_xact_lock($1)",
		"SELECT pg_advisory_xact_lock(hashtext($1))",
		"SELECT pg_try_advisory_xact_lock($1)",
		"SELECT pg_try_advisory_xact_lock(hashtext($1))",
	}
	calls := drv.Calls()
	if len(calls) != len(want) {
		t.Fatalf("calls = %+v, want %d statements", calls, len(want))
	}
	for i, w := range want {
		if calls[i].SQL != w {
			t.Errorf("call %d = %q, want %q", i, calls[i].SQL, w)
		}
	}
	if calls[1].Args[0] != "catalog:1" || calls[0].Args[0] != int64(42) {
		t.Errorf("args = %v, %v", calls[0].Args, calls[1].Args)
	}
}

func TestXactLockRefusals(t *testing.T) {
	ctx := context.Background()
	db := momo.New(fakedriver.New(dialect.Postgres))
	if err := advisory.XactLock(ctx, db, 1); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("XactLock outside a tx = %v, want ErrValidation", err)
	}
	if err := advisory.XactLockString(ctx, db, "k"); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("XactLockString outside a tx = %v, want ErrValidation", err)
	}
	if _, err := advisory.TryXactLock(ctx, db, 1); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("TryXactLock outside a tx = %v, want ErrValidation", err)
	}
	if _, err := advisory.TryXactLockString(ctx, db, "k"); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("TryXactLockString outside a tx = %v, want ErrValidation", err)
	}

	for _, n := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		other := momo.New(fakedriver.New(n))
		err := other.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if err := advisory.XactLock(ctx, tx, 1); !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s XactLock = %v, want ErrUnsupported", n, err)
			}
			if _, err := advisory.TryXactLockString(ctx, tx, "k"); !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s TryXactLockString = %v, want ErrUnsupported", n, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestTryXactLockNoRow(t *testing.T) {
	ctx := context.Background()
	db := momo.New(fakedriver.New(dialect.Postgres))
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if _, err := advisory.TryXactLock(ctx, tx, 1); !errors.Is(err, momo.ErrNotFound) {
			t.Errorf("TryXactLock with no row = %v, want ErrNotFound", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
