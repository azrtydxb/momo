// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

type ownerKey struct{}

// ownerInterceptor scopes selects, updates and deletes to the owner in ctx,
// and denies deletes outright. It records the operations it saw.
func ownerInterceptor(seen *[]string) momo.Interceptor {
	return func(ctx context.Context, op *momo.Operation) error {
		*seen = append(*seen, op.Kind+":"+op.Table)
		if op.Kind == "delete" {
			return errors.New("deletes are denied")
		}
		if owner, ok := ctx.Value(ownerKey{}).(string); ok {
			op.Where = append(op.Where, hookedTitle.Eq(owner))
		}
		return nil
	}
}

func TestInterceptors(t *testing.T) {
	var seen []string
	drv := fakedriver.New(dialect.Postgres)
	scoped := momo.New(drv, momo.WithInterceptor(ownerInterceptor(&seen)))
	plain := momo.New(drv)
	ctx := context.WithValue(t.Context(), ownerKey{}, "mine")

	// The predicate is visible in compiled SQL (SQL has no ctx of its own, so
	// it sees none of the owner and the interceptor adds nothing there).
	scopedCtxless, err := momo.Select[hookedDoc](scoped).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(scopedCtxless.SQL, "WHERE") {
		t.Fatalf("SQL without an owner in ctx should add no predicate: %s", scopedCtxless.SQL)
	}

	// An interceptor adds a predicate to the statement a read runs.
	if _, err := momo.Select[hookedDoc](scoped).Where(hookedID.Gt(0)).All(ctx); err != nil {
		t.Fatal(err)
	}
	last := drv.Calls()[len(drv.Calls())-1]
	if !strings.Contains(last.SQL, `"title"`) || !strings.Contains(last.SQL, `"id"`) || !slices.Contains(last.Args, any("mine")) {
		t.Fatalf("interceptor predicate missing from the executed SQL: %q %v", last.SQL, last.Args)
	}
	if n, err := momo.Select[hookedDoc](scoped).Count(ctx); err != nil || n != 0 {
		t.Fatalf("Count = %d, %v", n, err)
	}
	last = drv.Calls()[len(drv.Calls())-1]
	if !slices.Contains(last.Args, any("mine")) {
		t.Fatalf("Count skipped the interceptor: %q %v", last.SQL, last.Args)
	}

	// Another handle on the same driver is not affected.
	before := len(drv.Calls())
	if _, err := momo.Select[hookedDoc](plain).All(ctx); err != nil {
		t.Fatal(err)
	}
	other := drv.Calls()[before]
	if strings.Contains(other.SQL, "WHERE") || slices.Contains(other.Args, any("mine")) {
		t.Fatalf("an interceptor leaked to another handle: %q %v", other.SQL, other.Args)
	}

	// An update gets the predicate too.
	if _, err := momo.Update[hookedDoc](scoped).Set(hookedSec.Set("s")).Where(hookedID.Eq(1)).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	last = drv.Calls()[len(drv.Calls())-1]
	if !strings.HasPrefix(last.SQL, "UPDATE") || !slices.Contains(last.Args, any("mine")) {
		t.Fatalf("update lacks the interceptor predicate: %q %v", last.SQL, last.Args)
	}

	// An interceptor can deny a mutation: nothing runs.
	before = len(drv.Calls())
	_, err = momo.Delete[hookedDoc](scoped).Where(hookedID.Eq(1)).Exec(ctx)
	if err == nil || !strings.Contains(err.Error(), "denied") || !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("delete not denied: %v", err)
	}
	if len(drv.Calls()) != before {
		t.Fatalf("a denied delete reached the driver: %+v", drv.Calls()[before:])
	}
	if _, err := momo.Delete[hookedDoc](plain).Where(hookedID.Eq(1)).Exec(ctx); err != nil {
		t.Fatalf("delete on the plain handle: %v", err)
	}

	for _, want := range []string{"select:hooked_docs", "update:hooked_docs", "delete:hooked_docs"} {
		if !slices.Contains(seen, want) {
			t.Errorf("interceptor never saw %q (saw %v)", want, seen)
		}
	}
}

func TestInterceptorsAreNotAFilterForTheMutationGuard(t *testing.T) {
	var seen []string
	drv := fakedriver.New(dialect.Postgres)
	scoped := momo.New(drv, momo.WithInterceptor(ownerInterceptor(&seen)))
	ctx := context.WithValue(t.Context(), ownerKey{}, "mine")

	_, err := momo.Update[hookedDoc](scoped).Set(hookedSec.Set("s")).Exec(ctx)
	if !errors.Is(err, momo.ErrUnfilteredMutation) {
		t.Fatalf("an interceptor predicate satisfied the guard: %v", err)
	}
	if len(drv.Calls()) != 0 {
		t.Fatalf("statement ran: %+v", drv.Calls())
	}
	if _, err := momo.Update[hookedDoc](scoped).Set(hookedSec.Set("s")).AllRows().Exec(ctx); err != nil {
		t.Fatal(err)
	}
	last := drv.Calls()[0]
	if !slices.Contains(last.Args, any("mine")) {
		t.Fatalf("AllRows update lost the interceptor predicate: %q %v", last.SQL, last.Args)
	}
}

func TestInterceptorsFollowTheHandleIntoTransactions(t *testing.T) {
	var seen []string
	drv := fakedriver.New(dialect.Postgres)
	scoped := momo.New(drv, momo.WithInterceptor(ownerInterceptor(&seen)))
	plain := momo.New(drv)
	ctx := context.WithValue(t.Context(), ownerKey{}, "mine")

	run := func(db *momo.DB) fakedriver.Call {
		t.Helper()
		var call fakedriver.Call
		err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if _, err := momo.Select[hookedDoc](tx).All(ctx); err != nil {
				return err
			}
			calls := drv.Calls()
			call = calls[len(calls)-1]
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return call
	}
	if c := run(scoped); !slices.Contains(c.Args, any("mine")) {
		t.Fatalf("a transaction lost its handle's interceptor: %q %v", c.SQL, c.Args)
	}
	if c := run(plain); slices.Contains(c.Args, any("mine")) {
		t.Fatalf("a transaction picked up another handle's interceptor: %q %v", c.SQL, c.Args)
	}
}

func TestInterceptorDeniesInsert(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithInterceptor(func(_ context.Context, op *momo.Operation) error {
		if op.Kind == "insert" {
			return errors.New("read-only handle")
		}
		return nil
	}))
	m := hookedDoc{Title: "t"}
	if _, err := momo.Insert[hookedDoc](db).Model(&m).Exec(t.Context()); err == nil || !strings.Contains(err.Error(), "read-only handle") {
		t.Fatalf("insert = %v", err)
	}
	if len(drv.Calls()) != 0 {
		t.Fatalf("a denied insert reached the driver: %+v", drv.Calls())
	}
}
