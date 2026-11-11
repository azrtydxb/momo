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

// Catches: a shared helper that calls the exclusive function, a missing _shared
// suffix on a try or unlock form, and a string key that does not hash the way
// the exclusive helpers do (so Hash64(52) would not reproduce the key).
func TestSharedXactStatements(t *testing.T) {
	ctx := context.Background()
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{true}})
	db := momo.New(drv)
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if err := advisory.XactLockShared(ctx, tx, 7); err != nil {
			return err
		}
		if err := advisory.XactLockSharedString(ctx, tx, "obj"); err != nil {
			return err
		}
		if err := advisory.XactLockSharedString(ctx, tx, "obj", advisory.Hash64(52)); err != nil {
			return err
		}
		if ok, err := advisory.TryXactLockShared(ctx, tx, 7); err != nil || !ok {
			t.Errorf("TryXactLockShared = %v, %v", ok, err)
		}
		if ok, err := advisory.TryXactLockSharedString(ctx, tx, "obj", advisory.Hash64(52)); err != nil || !ok {
			t.Errorf("TryXactLockSharedString = %v, %v", ok, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"SELECT pg_advisory_xact_lock_shared($1)",
		"SELECT pg_advisory_xact_lock_shared(hashtext($1))",
		"SELECT pg_advisory_xact_lock_shared(hashtextextended($1, $2))",
		"SELECT pg_try_advisory_xact_lock_shared($1)",
		"SELECT pg_try_advisory_xact_lock_shared(hashtextextended($1, $2))",
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
	if calls[2].Args[0] != "obj" || calls[2].Args[1] != int64(52) {
		t.Errorf("Hash64(52) args = %v", calls[2].Args)
	}
}

// Catches: a session shared helper that targets the exclusive lock or its
// unlock, which would leave the wrong lock held.
func TestSharedSessionStatements(t *testing.T) {
	ctx := context.Background()
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{true}})
	db := momo.New(drv).OnConn(pinnedFake{drv})

	if err := advisory.LockShared(ctx, db, 1); err != nil {
		t.Fatal(err)
	}
	if err := advisory.LockSharedString(ctx, db, "k", advisory.Hash64(0)); err != nil {
		t.Fatal(err)
	}
	if ok, err := advisory.TryLockShared(ctx, db, 1); err != nil || !ok {
		t.Errorf("TryLockShared = %v, %v", ok, err)
	}
	if ok, err := advisory.TryLockSharedString(ctx, db, "k"); err != nil || !ok {
		t.Errorf("TryLockSharedString = %v, %v", ok, err)
	}
	if ok, err := advisory.UnlockShared(ctx, db, 1); err != nil || !ok {
		t.Errorf("UnlockShared = %v, %v", ok, err)
	}
	if ok, err := advisory.UnlockSharedString(ctx, db, "k", advisory.Hash64(0)); err != nil || !ok {
		t.Errorf("UnlockSharedString = %v, %v", ok, err)
	}
	want := []string{
		"SELECT pg_advisory_lock_shared($1)",
		"SELECT pg_advisory_lock_shared(hashtextextended($1, $2))",
		"SELECT pg_try_advisory_lock_shared($1)",
		"SELECT pg_try_advisory_lock_shared(hashtext($1))",
		"SELECT pg_advisory_unlock_shared($1)",
		"SELECT pg_advisory_unlock_shared(hashtextextended($1, $2))",
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
}

// Catches: a shared helper that skips the transaction, pinned-connection or
// dialect check the exclusive ones make.
func TestSharedRefusals(t *testing.T) {
	ctx := context.Background()

	if err := advisory.XactLockShared(ctx, nil, 1); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("XactLockShared with a nil tx = %v, want ErrValidation", err)
	}
	if _, err := advisory.TryXactLockSharedString(ctx, nil, "k"); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("TryXactLockSharedString with a nil tx = %v, want ErrValidation", err)
	}

	pooled := momo.New(fakedriver.New(dialect.Postgres))
	if err := advisory.LockShared(ctx, pooled, 1); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("LockShared on a pooled DB = %v, want ErrValidation", err)
	}
	if _, err := advisory.UnlockSharedString(ctx, pooled, "k"); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("UnlockSharedString on a pooled DB = %v, want ErrValidation", err)
	}
	if err := advisory.LockSharedString(ctx, nil, "k"); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("LockSharedString with a nil DB = %v, want ErrValidation", err)
	}

	for _, n := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		drv := fakedriver.New(n)
		err := momo.New(drv).Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if err := advisory.XactLockShared(ctx, tx, 1); !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s XactLockShared = %v, want ErrUnsupported", n, err)
			}
			if _, err := advisory.TryXactLockSharedString(ctx, tx, "k"); !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s TryXactLockSharedString = %v, want ErrUnsupported", n, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		pdrv := fakedriver.New(n)
		db := momo.New(pdrv).OnConn(pinnedFake{pdrv})
		if err := advisory.LockShared(ctx, db, 1); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s LockShared = %v, want ErrUnsupported", n, err)
		}
		if _, err := advisory.UnlockShared(ctx, db, 1); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s UnlockShared = %v, want ErrUnsupported", n, err)
		}
		if len(pdrv.Calls()) != 0 {
			t.Errorf("%s: statements ran", n)
		}
	}
}
