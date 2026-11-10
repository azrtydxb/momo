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

// pinnedFake pins statements to a fake driver.
type pinnedFake struct{ *fakedriver.Driver }

func (pinnedFake) Release() error { return nil }

func TestHash64Statements(t *testing.T) {
	ctx := context.Background()
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{true}})
	db := momo.New(drv)
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if err := advisory.XactLockString(ctx, tx, "k1", advisory.Hash64(0)); err != nil {
			return err
		}
		if ok, err := advisory.TryXactLockString(ctx, tx, "k2", advisory.Hash64(7)); err != nil || !ok {
			t.Errorf("TryXactLockString = %v, %v", ok, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := drv.Calls()
	if len(calls) != 2 {
		t.Fatalf("calls = %+v", calls)
	}
	if want := "SELECT pg_advisory_xact_lock(hashtextextended($1, $2))"; calls[0].SQL != want {
		t.Errorf("call 0 = %q, want %q", calls[0].SQL, want)
	}
	if calls[0].Args[0] != "k1" || calls[0].Args[1] != int64(0) {
		t.Errorf("call 0 args = %v", calls[0].Args)
	}
	if want := "SELECT pg_try_advisory_xact_lock(hashtextextended($1, $2))"; calls[1].SQL != want {
		t.Errorf("call 1 = %q, want %q", calls[1].SQL, want)
	}
	if calls[1].Args[0] != "k2" || calls[1].Args[1] != int64(7) {
		t.Errorf("call 1 args = %v", calls[1].Args)
	}
}

func TestSessionLockStatements(t *testing.T) {
	ctx := context.Background()
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{true}})
	db := momo.New(drv).OnConn(pinnedFake{drv})

	if err := advisory.Lock(ctx, db, 42); err != nil {
		t.Fatal(err)
	}
	if err := advisory.LockString(ctx, db, "migrate"); err != nil {
		t.Fatal(err)
	}
	if err := advisory.LockString(ctx, db, "migrate", advisory.Hash64(0)); err != nil {
		t.Fatal(err)
	}
	if ok, err := advisory.TryLock(ctx, db, 1); err != nil || !ok {
		t.Errorf("TryLock = %v, %v", ok, err)
	}
	if ok, err := advisory.TryLockString(ctx, db, "a", advisory.Hash64(3)); err != nil || !ok {
		t.Errorf("TryLockString = %v, %v", ok, err)
	}
	if ok, err := advisory.Unlock(ctx, db, 42); err != nil || !ok {
		t.Errorf("Unlock = %v, %v", ok, err)
	}
	if ok, err := advisory.UnlockString(ctx, db, "migrate"); err != nil || !ok {
		t.Errorf("UnlockString = %v, %v", ok, err)
	}
	want := []string{
		"SELECT pg_advisory_lock($1)",
		"SELECT pg_advisory_lock(hashtext($1))",
		"SELECT pg_advisory_lock(hashtextextended($1, $2))",
		"SELECT pg_try_advisory_lock($1)",
		"SELECT pg_try_advisory_lock(hashtextextended($1, $2))",
		"SELECT pg_advisory_unlock($1)",
		"SELECT pg_advisory_unlock(hashtext($1))",
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

func TestSessionLockRefusals(t *testing.T) {
	ctx := context.Background()

	pooled := momo.New(fakedriver.New(dialect.Postgres))
	for name, f := range map[string]func() error{
		"Lock":            func() error { return advisory.Lock(ctx, pooled, 1) },
		"LockString":      func() error { return advisory.LockString(ctx, pooled, "k") },
		"TryLock":         func() error { _, err := advisory.TryLock(ctx, pooled, 1); return err },
		"TryLockString":   func() error { _, err := advisory.TryLockString(ctx, pooled, "k"); return err },
		"Unlock":          func() error { _, err := advisory.Unlock(ctx, pooled, 1); return err },
		"UnlockString":    func() error { _, err := advisory.UnlockString(ctx, pooled, "k"); return err },
		"Lock with nil":   func() error { return advisory.Lock(ctx, nil, 1) },
		"Unlock with nil": func() error { _, err := advisory.Unlock(ctx, nil, 1); return err },
	} {
		if err := f(); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s on a pooled or nil DB = %v, want ErrValidation", name, err)
		}
	}
	if n := len(pooled.Driver().(*fakedriver.Driver).Calls()); n != 0 {
		t.Errorf("%d statements ran on the pooled DB, want none", n)
	}

	for _, n := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		drv := fakedriver.New(n)
		db := momo.New(drv).OnConn(pinnedFake{drv})
		if err := advisory.Lock(ctx, db, 1); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s Lock = %v, want ErrUnsupported", n, err)
		}
		if _, err := advisory.TryLockString(ctx, db, "k"); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s TryLockString = %v, want ErrUnsupported", n, err)
		}
		if len(drv.Calls()) != 0 {
			t.Errorf("%s: statements ran", n)
		}
	}
}

func TestIsPinned(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	if db.IsPinned() {
		t.Error("a pooled DB reports IsPinned")
	}
	if !db.OnConn(pinnedFake{drv}).IsPinned() {
		t.Error("an OnConn handle does not report IsPinned")
	}
}
