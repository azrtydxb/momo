// SPDX-License-Identifier: Apache-2.0

//lint:file-ignore SA1019 these tests exercise the deprecated functions

package advisory_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/advisory"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// Catches: a deprecated String function that accepts Try, Shared or Xact and
// silently changes what it does (a LockString that no longer blocks and drops
// the boolean), instead of refusing the option before any statement runs.
func TestDeprecatedStringFunctionsRefuseOtherOptions(t *testing.T) {
	ctx := context.Background()
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{true}})
	db := momo.New(drv).OnConn(pinnedFake{drv})

	session := map[string]func(opt advisory.Option) error{
		"LockString":          func(o advisory.Option) error { return advisory.LockString(ctx, db, "k", o) },
		"TryLockString":       func(o advisory.Option) error { _, err := advisory.TryLockString(ctx, db, "k", o); return err },
		"UnlockString":        func(o advisory.Option) error { _, err := advisory.UnlockString(ctx, db, "k", o); return err },
		"LockSharedString":    func(o advisory.Option) error { return advisory.LockSharedString(ctx, db, "k", o) },
		"TryLockSharedString": func(o advisory.Option) error { _, err := advisory.TryLockSharedString(ctx, db, "k", o); return err },
		"UnlockSharedString":  func(o advisory.Option) error { _, err := advisory.UnlockSharedString(ctx, db, "k", o); return err },
	}
	xact := map[string]func(tx *momo.Tx, opt advisory.Option) error{
		"XactLockString": func(tx *momo.Tx, o advisory.Option) error { return advisory.XactLockString(ctx, tx, "k", o) },
		"TryXactLockString": func(tx *momo.Tx, o advisory.Option) error {
			_, err := advisory.TryXactLockString(ctx, tx, "k", o)
			return err
		},
		"XactLockSharedString": func(tx *momo.Tx, o advisory.Option) error { return advisory.XactLockSharedString(ctx, tx, "k", o) },
		"TryXactLockSharedString": func(tx *momo.Tx, o advisory.Option) error {
			_, err := advisory.TryXactLockSharedString(ctx, tx, "k", o)
			return err
		},
	}
	opts := map[string]advisory.Option{
		"Try": advisory.Try(), "Shared": advisory.Shared(), "Xact": advisory.Xact(),
	}

	check := func(fn, opt string, err error) {
		t.Helper()
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s with %s = %v, want ErrValidation", fn, opt, err)
			return
		}
		if !strings.Contains(err.Error(), opt) || !strings.Contains(err.Error(), "Acquire") {
			t.Errorf("%s with %s: %q does not name the option and point to Acquire", fn, opt, err)
		}
	}
	for fn, call := range session {
		for name, o := range opts {
			check(fn, name, call(o))
		}
	}
	err := momo.New(drv).Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		for fn, call := range xact {
			for name, o := range opts {
				check(fn, name, call(tx, o))
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

	// WithHash64 and the deprecated Hash64 are still accepted.
	if err := advisory.LockString(ctx, db, "k", advisory.WithHash64(1)); err != nil {
		t.Errorf("LockString with WithHash64 = %v, want nil", err)
	}
	if ok, err := advisory.TryLockString(ctx, db, "k", advisory.Hash64(1)); err != nil || !ok {
		t.Errorf("TryLockString with Hash64 = %v, %v, want true", ok, err)
	}
}
