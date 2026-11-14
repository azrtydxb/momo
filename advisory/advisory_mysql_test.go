// SPDX-License-Identifier: Apache-2.0

package advisory_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/advisory"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func mysqlPinned(rows [][]any) (*momo.DB, *fakedriver.Driver) {
	drv := fakedriver.New(dialect.MySQL)
	drv.SetRows(rows)
	return momo.New(drv).OnConn(pinnedFake{drv}), drv
}

// Catches: the lock name mapping drifting, which would make two releases of an
// application take different locks for the same key.
func TestMySQLSessionLockStatements(t *testing.T) {
	ctx := context.Background()
	db, drv := mysqlPinned([][]any{{int64(1)}})

	sum := sha256.Sum256([]byte("migrate"))
	strName := "momo:advisory:s:" + hex.EncodeToString(sum[:])[:40]

	if err := advisory.Lock(ctx, db, 42); err != nil {
		t.Fatal(err)
	}
	if err := advisory.LockString(ctx, db, "migrate", advisory.Hash64(9)); err != nil {
		t.Fatal(err)
	}
	if ok, err := advisory.TryLock(ctx, db, -7); err != nil || !ok {
		t.Errorf("TryLock = %v, %v", ok, err)
	}
	if ok, err := advisory.TryLockString(ctx, db, "migrate"); err != nil || !ok {
		t.Errorf("TryLockString = %v, %v", ok, err)
	}
	if ok, err := advisory.Unlock(ctx, db, 42); err != nil || !ok {
		t.Errorf("Unlock = %v, %v", ok, err)
	}
	if ok, err := advisory.UnlockString(ctx, db, "migrate"); err != nil || !ok {
		t.Errorf("UnlockString = %v, %v", ok, err)
	}

	type call struct {
		sql  string
		args []any
	}
	want := []call{
		{"SELECT GET_LOCK(?, ?)", []any{"momo:advisory:42", int64(1)}},
		{"SELECT GET_LOCK(?, ?)", []any{strName, int64(1)}},
		{"SELECT GET_LOCK(?, 0)", []any{"momo:advisory:-7"}},
		{"SELECT GET_LOCK(?, 0)", []any{strName}},
		{"SELECT RELEASE_LOCK(?)", []any{"momo:advisory:42"}},
		{"SELECT RELEASE_LOCK(?)", []any{strName}},
	}
	calls := drv.Calls()
	if len(calls) != len(want) {
		t.Fatalf("calls = %+v, want %d statements", calls, len(want))
	}
	for i, w := range want {
		if calls[i].SQL != w.sql {
			t.Errorf("call %d = %q, want %q", i, calls[i].SQL, w.sql)
		}
		if len(calls[i].Args) != len(w.args) {
			t.Errorf("call %d args = %v, want %v", i, calls[i].Args, w.args)
			continue
		}
		for j := range w.args {
			if fmt.Sprint(calls[i].Args[j]) != fmt.Sprint(w.args[j]) {
				t.Errorf("call %d arg %d = %v, want %v", i, j, calls[i].Args[j], w.args[j])
			}
		}
	}
	if len(strName) > 64 {
		t.Errorf("lock name %q is %d characters, over the 64 GET_LOCK allows", strName, len(strName))
	}
	// A text key that spells an int64 key must not meet it.
	if strings.HasPrefix(strName, "momo:advisory:42") {
		t.Error("a text key shares the int64 name space")
	}
}

// Catches: GET_LOCK's 0 (timeout) and NULL (error) both reading as "granted",
// or an unlock of a lock this session does not hold reading as success.
func TestMySQLResultMapping(t *testing.T) {
	ctx := context.Background()

	db, _ := mysqlPinned([][]any{{int64(0)}})
	if ok, err := advisory.TryLock(ctx, db, 1); err != nil || ok {
		t.Errorf("TryLock on result 0 = %v, %v; want false, nil", ok, err)
	}
	if ok, err := advisory.Unlock(ctx, db, 1); err != nil || ok {
		t.Errorf("Unlock on result 0 = %v, %v; want false, nil", ok, err)
	}

	db, _ = mysqlPinned([][]any{{nil}})
	if ok, err := advisory.TryLock(ctx, db, 1); err == nil || ok {
		t.Errorf("TryLock on NULL = %v, %v; want an error", ok, err)
	}
	if err := advisory.Lock(ctx, db, 1); err == nil {
		t.Error("Lock on NULL returned nil, want an error")
	}
	if ok, err := advisory.Unlock(ctx, db, 1); err != nil || ok {
		t.Errorf("Unlock on NULL = %v, %v; want false, nil (nobody holds it)", ok, err)
	}
}

// Catches: the blocking Lock giving up on the first timed-out slice, or
// spinning after the context ended.
func TestMySQLLockRetriesUntilGrantedOrCanceled(t *testing.T) {
	db, _ := mysqlPinned([][]any{{int64(0)}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := advisory.Lock(ctx, db, 5)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Lock with a canceled context = %v, want context.Canceled", err)
	}
}

// Catches: a transaction-scoped or shared lock silently becoming a session
// lock on MySQL and MariaDB.
func TestMySQLRefusesTransactionAndSharedLocks(t *testing.T) {
	ctx := context.Background()
	drv := fakedriver.New(dialect.MySQL)
	db := momo.New(drv)
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if err := advisory.XactLock(ctx, tx, 1); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("XactLock = %v, want ErrUnsupported", err)
		}
		if _, err := advisory.TryXactLockString(ctx, tx, "k"); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("TryXactLockString = %v, want ErrUnsupported", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	pinned := db.OnConn(pinnedFake{drv})
	if err := advisory.LockShared(ctx, pinned, 1); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("LockShared = %v, want ErrUnsupported", err)
	}
	if _, err := advisory.UnlockSharedString(ctx, pinned, "k"); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("UnlockSharedString = %v, want ErrUnsupported", err)
	}
	for _, c := range drv.Calls() {
		if strings.Contains(c.SQL, "GET_LOCK") {
			t.Errorf("a refused lock ran %q", c.SQL)
		}
	}
}

func TestMySQLSessionLockNeedsPinnedConnection(t *testing.T) {
	ctx := context.Background()
	drv := fakedriver.New(dialect.MySQL)
	pooled := momo.New(drv)
	if err := advisory.Lock(ctx, pooled, 1); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("Lock on a pooled MySQL DB = %v, want ErrValidation", err)
	}
	if n := len(drv.Calls()); n != 0 {
		t.Errorf("%d statements ran on the pooled DB, want none", n)
	}
}
