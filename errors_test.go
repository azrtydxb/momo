// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/vector"
)

var allSentinels = []error{
	momo.ErrNotFound, momo.ErrTooManyRows, momo.ErrUniqueViolation,
	momo.ErrForeignKeyViolation, momo.ErrCheckViolation, momo.ErrNotNullViolation,
	momo.ErrSerialization, momo.ErrOptimisticLock, momo.ErrUnsupported,
	momo.ErrUnfilteredMutation, momo.ErrTxDone, momo.ErrValidation, momo.ErrBusy,
	momo.ErrLimitExceeded, momo.ErrInvalidVector, momo.ErrCancelled,
}

func TestErrorSentinels(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range allSentinels {
		if s == nil {
			t.Fatal("nil sentinel")
		}
		if seen[s.Error()] {
			t.Errorf("duplicate sentinel message %q", s.Error())
		}
		seen[s.Error()] = true
	}
	if momo.ErrInvalidVector != vector.ErrInvalidVector {
		t.Fatal("momo.ErrInvalidVector must be vector.ErrInvalidVector")
	}
}

func TestErrorIsAs(t *testing.T) {
	driverErr := errors.New("driver says no")
	for _, kind := range allSentinels {
		err := fmt.Errorf("query failed: %w", &momo.Error{Kind: kind, Op: "select", Table: "documents", Err: driverErr})
		if !errors.Is(err, kind) {
			t.Errorf("errors.Is(%v) = false", kind)
		}
		var me *momo.Error
		if !errors.As(err, &me) || me.Table != "documents" {
			t.Errorf("errors.As failed for %v", kind)
		}
		if !errors.Is(err, driverErr) {
			t.Errorf("driver error not reachable for %v", kind)
		}
		if me.Detail() != driverErr.Error() {
			t.Errorf("Detail() = %q", me.Detail())
		}
	}
	if (&momo.Error{Kind: momo.ErrNotFound}).Detail() != "" {
		t.Error("Detail() must be empty when Err is nil")
	}
}

// A streaming read stopped by its context carries a sentinel kind and still
// unwraps to the context error.
func TestStreamCancelledHasKind(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{int64(1)}})
	db := momo.New(drv)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := momo.Select[suite.Document](db).Each(ctx, func(*suite.Document) error { return nil })
	if !errors.Is(err, momo.ErrCancelled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Each = %v, want ErrCancelled and context.Canceled", err)
	}
	for _, e := range func() []error {
		var out []error
		for _, err := range momo.Select[suite.Document](db).Iter(ctx) {
			out = append(out, err)
		}
		return out
	}() {
		if !errors.Is(e, momo.ErrCancelled) || !errors.Is(e, context.Canceled) {
			t.Fatalf("Iter yielded %v, want ErrCancelled and context.Canceled", e)
		}
	}
}
