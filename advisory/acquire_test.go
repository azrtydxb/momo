// SPDX-License-Identifier: Apache-2.0

//lint:file-ignore SA1019 these tests compare the deprecated functions with their replacements

package advisory_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/advisory"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// acquire and release take the key as an any so one table can hold int64 and
// string keys; the generic functions need the concrete type.
func acquire(ctx context.Context, q momo.Querier, key any, opts ...advisory.Option) (bool, error) {
	if s, ok := key.(string); ok {
		return advisory.Acquire(ctx, q, s, opts...)
	}
	return advisory.Acquire(ctx, q, key.(int64), opts...)
}

func release(ctx context.Context, q momo.Querier, key any, opts ...advisory.Option) (bool, error) {
	if s, ok := key.(string); ok {
		return advisory.Release(ctx, q, s, opts...)
	}
	return advisory.Release(ctx, q, key.(int64), opts...)
}

func granted(err error) (bool, error) { return err == nil, err }

type result struct {
	ok  bool
	err error
}

// runXact runs f on a fresh PostgreSQL fake inside a transaction and returns
// the statements it sent.
func runXact(t *testing.T, f func(ctx context.Context, tx *momo.Tx) (bool, error)) ([]fakedriver.Call, result) {
	t.Helper()
	ctx := context.Background()
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{true}})
	var res result
	err := momo.New(drv).Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		res.ok, res.err = f(ctx, tx)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return drv.Calls(), res
}

// runSession runs f on a pinned fake of dialect d and returns the statements it
// sent.
func runSession(t *testing.T, d dialect.Name, rows [][]any, f func(ctx context.Context, db *momo.DB) (bool, error)) ([]fakedriver.Call, result) {
	t.Helper()
	drv := fakedriver.New(d)
	drv.SetRows(rows)
	var res result
	res.ok, res.err = f(context.Background(), momo.New(drv).OnConn(pinnedFake{drv}))
	return drv.Calls(), res
}

func sameCalls(t *testing.T, got, want []fakedriver.Call) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("new API sent %+v, deprecated function sent %+v", got, want)
	}
	for i := range want {
		if got[i].SQL != want[i].SQL || !reflect.DeepEqual(got[i].Args, want[i].Args) {
			t.Errorf("statement %d: new API sent %q %v, deprecated function sent %q %v", i, got[i].SQL, got[i].Args, want[i].SQL, want[i].Args)
		}
	}
}

// Catches: a deprecated transaction-scoped function that forwards with the wrong
// option (a missing _shared or try_, a lost Hash64 seed), which would take a
// different lock than the one its name promises, and an Acquire that builds the
// statement differently from the function it replaces.
func TestDeprecatedXactFunctionsMatchAcquire(t *testing.T) {
	cases := []struct {
		name string
		old  func(ctx context.Context, tx *momo.Tx) (bool, error)
		key  any
		opts []advisory.Option
	}{
		{"XactLock", func(ctx context.Context, tx *momo.Tx) (bool, error) { return granted(advisory.XactLock(ctx, tx, 7)) }, int64(7), []advisory.Option{advisory.Xact()}},
		{"XactLockString", func(ctx context.Context, tx *momo.Tx) (bool, error) {
			return granted(advisory.XactLockString(ctx, tx, "k"))
		}, "k", []advisory.Option{advisory.Xact()}},
		{"XactLockString Hash64", func(ctx context.Context, tx *momo.Tx) (bool, error) {
			return granted(advisory.XactLockString(ctx, tx, "k", advisory.Hash64(52)))
		}, "k", []advisory.Option{advisory.Xact(), advisory.WithHash64(52)}},
		{"TryXactLock", func(ctx context.Context, tx *momo.Tx) (bool, error) { return advisory.TryXactLock(ctx, tx, 7) }, int64(7), []advisory.Option{advisory.Xact(), advisory.Try()}},
		{"TryXactLockString", func(ctx context.Context, tx *momo.Tx) (bool, error) {
			return advisory.TryXactLockString(ctx, tx, "k", advisory.Hash64(0))
		}, "k", []advisory.Option{advisory.Xact(), advisory.Try(), advisory.WithHash64(0)}},
		{"XactLockShared", func(ctx context.Context, tx *momo.Tx) (bool, error) {
			return granted(advisory.XactLockShared(ctx, tx, 7))
		}, int64(7), []advisory.Option{advisory.Xact(), advisory.Shared()}},
		{"XactLockSharedString", func(ctx context.Context, tx *momo.Tx) (bool, error) {
			return granted(advisory.XactLockSharedString(ctx, tx, "k", advisory.Hash64(52)))
		}, "k", []advisory.Option{advisory.Xact(), advisory.Shared(), advisory.WithHash64(52)}},
		{"TryXactLockShared", func(ctx context.Context, tx *momo.Tx) (bool, error) {
			return advisory.TryXactLockShared(ctx, tx, 7)
		}, int64(7), []advisory.Option{advisory.Xact(), advisory.Shared(), advisory.Try()}},
		{"TryXactLockSharedString", func(ctx context.Context, tx *momo.Tx) (bool, error) {
			return advisory.TryXactLockSharedString(ctx, tx, "k")
		}, "k", []advisory.Option{advisory.Xact(), advisory.Shared(), advisory.Try()}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			oldCalls, oldRes := runXact(t, c.old)
			newCalls, newRes := runXact(t, func(ctx context.Context, tx *momo.Tx) (bool, error) {
				return acquire(ctx, tx, c.key, c.opts...)
			})
			if len(oldCalls) != 1 {
				t.Fatalf("deprecated function sent %+v, want one statement", oldCalls)
			}
			sameCalls(t, newCalls, oldCalls)
			if oldRes.err != nil || newRes.err != nil || oldRes.ok != newRes.ok {
				t.Errorf("results differ: deprecated %+v, new %+v", oldRes, newRes)
			}
		})
	}
}

// Catches: a deprecated session function that forwards with the wrong option, a
// Release that targets the exclusive unlock for a shared lock, and an Acquire
// that builds the statement differently from the function it replaces.
func TestDeprecatedSessionFunctionsMatchAcquireAndRelease(t *testing.T) {
	type call = func(ctx context.Context, db *momo.DB) (bool, error)
	cases := []struct {
		name string
		old  call
		now  func(ctx context.Context, db *momo.DB) (bool, error)
	}{
		{"Lock", func(ctx context.Context, db *momo.DB) (bool, error) { return granted(advisory.Lock(ctx, db, 42)) },
			func(ctx context.Context, db *momo.DB) (bool, error) { return acquire(ctx, db, int64(42)) }},
		{"LockString", func(ctx context.Context, db *momo.DB) (bool, error) {
			return granted(advisory.LockString(ctx, db, "k", advisory.Hash64(3)))
		}, func(ctx context.Context, db *momo.DB) (bool, error) {
			return acquire(ctx, db, "k", advisory.WithHash64(3))
		}},
		{"TryLock", func(ctx context.Context, db *momo.DB) (bool, error) { return advisory.TryLock(ctx, db, 42) },
			func(ctx context.Context, db *momo.DB) (bool, error) {
				return acquire(ctx, db, int64(42), advisory.Try())
			}},
		{"TryLockString", func(ctx context.Context, db *momo.DB) (bool, error) { return advisory.TryLockString(ctx, db, "k") },
			func(ctx context.Context, db *momo.DB) (bool, error) { return acquire(ctx, db, "k", advisory.Try()) }},
		{"Unlock", func(ctx context.Context, db *momo.DB) (bool, error) { return advisory.Unlock(ctx, db, 42) },
			func(ctx context.Context, db *momo.DB) (bool, error) { return release(ctx, db, int64(42)) }},
		{"UnlockString", func(ctx context.Context, db *momo.DB) (bool, error) {
			return advisory.UnlockString(ctx, db, "k", advisory.Hash64(3))
		}, func(ctx context.Context, db *momo.DB) (bool, error) {
			return release(ctx, db, "k", advisory.WithHash64(3))
		}},
		{"LockShared", func(ctx context.Context, db *momo.DB) (bool, error) { return granted(advisory.LockShared(ctx, db, 42)) },
			func(ctx context.Context, db *momo.DB) (bool, error) {
				return acquire(ctx, db, int64(42), advisory.Shared())
			}},
		{"LockSharedString", func(ctx context.Context, db *momo.DB) (bool, error) {
			return granted(advisory.LockSharedString(ctx, db, "k", advisory.Hash64(0)))
		}, func(ctx context.Context, db *momo.DB) (bool, error) {
			return acquire(ctx, db, "k", advisory.Shared(), advisory.WithHash64(0))
		}},
		{"TryLockShared", func(ctx context.Context, db *momo.DB) (bool, error) { return advisory.TryLockShared(ctx, db, 42) },
			func(ctx context.Context, db *momo.DB) (bool, error) {
				return acquire(ctx, db, int64(42), advisory.Shared(), advisory.Try())
			}},
		{"TryLockSharedString", func(ctx context.Context, db *momo.DB) (bool, error) {
			return advisory.TryLockSharedString(ctx, db, "k")
		}, func(ctx context.Context, db *momo.DB) (bool, error) {
			return acquire(ctx, db, "k", advisory.Shared(), advisory.Try())
		}},
		{"UnlockShared", func(ctx context.Context, db *momo.DB) (bool, error) { return advisory.UnlockShared(ctx, db, 42) },
			func(ctx context.Context, db *momo.DB) (bool, error) {
				return release(ctx, db, int64(42), advisory.Shared())
			}},
		{"UnlockSharedString", func(ctx context.Context, db *momo.DB) (bool, error) {
			return advisory.UnlockSharedString(ctx, db, "k", advisory.Hash64(52))
		}, func(ctx context.Context, db *momo.DB) (bool, error) {
			return release(ctx, db, "k", advisory.Shared(), advisory.WithHash64(52))
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			oldCalls, oldRes := runSession(t, dialect.Postgres, [][]any{{true}}, c.old)
			newCalls, newRes := runSession(t, dialect.Postgres, [][]any{{true}}, c.now)
			if len(oldCalls) != 1 {
				t.Fatalf("deprecated function sent %+v, want one statement", oldCalls)
			}
			sameCalls(t, newCalls, oldCalls)
			if oldRes.err != nil || newRes.err != nil || oldRes.ok != newRes.ok {
				t.Errorf("results differ: deprecated %+v, new %+v", oldRes, newRes)
			}
		})
	}
}

// Catches: the MySQL branch of Acquire and Release diverging from the session
// functions they replace (the GET_LOCK slice, the zero-wait try, RELEASE_LOCK).
func TestMySQLAcquireMatchesDeprecatedFunctions(t *testing.T) {
	rows := [][]any{{int64(1)}}
	type call = func(ctx context.Context, db *momo.DB) (bool, error)
	cases := []struct {
		name     string
		old, now call
	}{
		{"Lock", func(ctx context.Context, db *momo.DB) (bool, error) { return granted(advisory.Lock(ctx, db, 42)) },
			func(ctx context.Context, db *momo.DB) (bool, error) { return acquire(ctx, db, int64(42)) }},
		{"TryLockString", func(ctx context.Context, db *momo.DB) (bool, error) { return advisory.TryLockString(ctx, db, "k") },
			func(ctx context.Context, db *momo.DB) (bool, error) { return acquire(ctx, db, "k", advisory.Try()) }},
		{"Unlock", func(ctx context.Context, db *momo.DB) (bool, error) { return advisory.Unlock(ctx, db, 42) },
			func(ctx context.Context, db *momo.DB) (bool, error) { return release(ctx, db, int64(42)) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			oldCalls, oldRes := runSession(t, dialect.MySQL, rows, c.old)
			newCalls, newRes := runSession(t, dialect.MySQL, rows, c.now)
			if len(oldCalls) != 1 {
				t.Fatalf("deprecated function sent %+v, want one statement", oldCalls)
			}
			sameCalls(t, newCalls, oldCalls)
			if oldRes.err != nil || newRes.err != nil || oldRes.ok != newRes.ok {
				t.Errorf("results differ: deprecated %+v, new %+v", oldRes, newRes)
			}
		})
	}
	// Shared has no MySQL counterpart, however it is asked for.
	_, res := runSession(t, dialect.MySQL, rows, func(ctx context.Context, db *momo.DB) (bool, error) {
		return acquire(ctx, db, int64(1), advisory.Shared())
	})
	if !errors.Is(res.err, momo.ErrUnsupported) {
		t.Errorf("Acquire with Shared on MySQL = %v, want ErrUnsupported", res.err)
	}
}

// Catches: an option that is accepted and then ignored (a Try that blocks, a
// Shared that takes the exclusive lock) and a key type that binds as something
// other than int64 or string.
func TestAcquireOptionsChooseTheLockFunction(t *testing.T) {
	type orderID int64
	type name string
	ctx := context.Background()
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{true}})
	db := momo.New(drv).OnConn(pinnedFake{drv})

	steps := []struct {
		do   func() (bool, error)
		want string
	}{
		{func() (bool, error) { return advisory.Acquire(ctx, db, 1) }, "SELECT pg_advisory_lock($1)"},
		{func() (bool, error) { return advisory.Acquire(ctx, db, 1, advisory.Try()) }, "SELECT pg_try_advisory_lock($1)"},
		{func() (bool, error) { return advisory.Acquire(ctx, db, 1, advisory.Shared()) }, "SELECT pg_advisory_lock_shared($1)"},
		{func() (bool, error) { return advisory.Acquire(ctx, db, orderID(1), advisory.Shared(), advisory.Try()) }, "SELECT pg_try_advisory_lock_shared($1)"},
		{func() (bool, error) { return advisory.Acquire(ctx, db, name("k")) }, "SELECT pg_advisory_lock(hashtext($1))"},
		{func() (bool, error) { return advisory.Acquire(ctx, db, "k", advisory.WithHash64(9)) }, "SELECT pg_advisory_lock(hashtextextended($1, $2))"},
		{func() (bool, error) { return advisory.Release(ctx, db, 1) }, "SELECT pg_advisory_unlock($1)"},
		{func() (bool, error) { return advisory.Release(ctx, db, 1, advisory.Shared()) }, "SELECT pg_advisory_unlock_shared($1)"},
	}
	for _, s := range steps {
		if ok, err := s.do(); err != nil || !ok {
			t.Fatalf("%s = %v, %v; want true", s.want, ok, err)
		}
	}
	calls := drv.Calls()
	if len(calls) != len(steps) {
		t.Fatalf("calls = %+v, want %d", calls, len(steps))
	}
	for i, s := range steps {
		if calls[i].SQL != s.want {
			t.Errorf("call %d = %q, want %q", i, calls[i].SQL, s.want)
		}
	}
	if calls[3].Args[0] != int64(1) {
		t.Errorf("a named integer key bound as %T, want int64", calls[3].Args[0])
	}
	if calls[4].Args[0] != "k" {
		t.Errorf("a named string key bound as %T, want string", calls[4].Args[0])
	}
}

// Catches: a handle that does not fit the requested lock being used anyway (a
// *momo.DB for a transaction lock, a *momo.Tx for a session lock), and a
// Release that accepts an option with no meaning for it.
func TestAcquireAndReleaseRefusals(t *testing.T) {
	ctx := context.Background()
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{true}})
	pinned := momo.New(drv).OnConn(pinnedFake{drv})

	checks := []struct {
		name string
		do   func(tx *momo.Tx) error
	}{
		{"Xact on a *DB", func(*momo.Tx) error { _, err := advisory.Acquire(ctx, pinned, 1, advisory.Xact()); return err }},
		{"Xact on a nil Querier", func(*momo.Tx) error { _, err := advisory.Acquire(ctx, nil, 1, advisory.Xact()); return err }},
		{"Xact on a nil *Tx", func(*momo.Tx) error {
			var tx *momo.Tx
			_, err := advisory.Acquire(ctx, tx, 1, advisory.Xact())
			return err
		}},
		{"session lock on a *Tx", func(tx *momo.Tx) error { _, err := advisory.Acquire(ctx, tx, 1); return err }},
		{"session lock on a nil Querier", func(*momo.Tx) error { _, err := advisory.Acquire(ctx, nil, 1); return err }},
		{"session lock on a nil *DB", func(*momo.Tx) error {
			var db *momo.DB
			_, err := advisory.Acquire(ctx, db, 1)
			return err
		}},
		{"session lock on a pooled DB", func(*momo.Tx) error {
			_, err := advisory.Acquire(ctx, momo.New(fakedriver.New(dialect.Postgres)), 1)
			return err
		}},
		{"Release with Xact", func(*momo.Tx) error { _, err := advisory.Release(ctx, pinned, 1, advisory.Xact()); return err }},
		{"Release with Try", func(*momo.Tx) error { _, err := advisory.Release(ctx, pinned, 1, advisory.Try()); return err }},
		{"Release on a *Tx", func(tx *momo.Tx) error { _, err := advisory.Release(ctx, tx, 1); return err }},
	}
	err := momo.New(drv).Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		for _, c := range checks {
			if err := c.do(tx); !errors.Is(err, momo.ErrValidation) {
				t.Errorf("%s = %v, want ErrValidation", c.name, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range drv.Calls() {
		t.Errorf("a refused call sent %q", c.SQL)
	}

	// A transaction lock off PostgreSQL is unsupported, not invalid.
	mdrv := fakedriver.New(dialect.MySQL)
	err = momo.New(mdrv).Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if _, err := advisory.Acquire(ctx, tx, "k", advisory.Xact()); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("Xact on MySQL = %v, want ErrUnsupported", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
