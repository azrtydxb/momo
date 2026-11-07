// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/suite"
)

const sensitiveValue = "hunter2-sensitive"

// assertMasked checks that no hook event shows the sensitive value, at least
// one shows the placeholder, and the driver received the real value.
func assertMasked(t *testing.T, rec *obstest.Recorder, drv *fakedriver.Driver) {
	t.Helper()
	masked := false
	for _, e := range rec.Events() {
		if e.NumArgs != len(e.Args) {
			t.Errorf("NumArgs %d != len(Args) %d", e.NumArgs, len(e.Args))
		}
		for _, a := range e.Args {
			if a == sensitiveValue {
				t.Fatalf("hook saw the sensitive value in %q", e.SQL)
			}
			if a == momo.RedactedArg {
				masked = true
			}
		}
	}
	if !masked {
		t.Fatal("no hook event carried the redaction placeholder")
	}
	sent := false
	for _, c := range drv.Calls() {
		if slices.Contains(c.Args, any(sensitiveValue)) {
			sent = true
		}
	}
	if !sent {
		t.Fatal("the driver did not receive the real value")
	}
}

func TestHooksSeeMaskedSensitiveInsertArgs(t *testing.T) {
	rec := &obstest.Recorder{}
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
	_, err := momo.Insert[suite.Document](db).Models([]suite.Document{{TenantID: 1, Title: "t", Secret: sensitiveValue}}).Exec(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	assertMasked(t, rec, drv)
}

func TestHooksSeeMaskedSensitiveUpdateArgs(t *testing.T) {
	rec := &obstest.Recorder{}
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
	_, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("visible"), suite.DocSecret.Set(sensitiveValue)).
		Where(suite.DocID.Eq(1)).Exec(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	assertMasked(t, rec, drv)
	for _, e := range rec.Events() {
		if !slices.Contains(e.Args, any("visible")) {
			t.Errorf("non-sensitive argument was masked: %v", e.Args)
		}
	}
}

// mutatingHook overwrites its arguments; that must not change what the driver gets.
type mutatingHook struct{}

func (mutatingHook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	for i := range e.Args {
		e.Args[i] = "tampered"
	}
	return ctx
}
func (mutatingHook) AfterQuery(context.Context, *momo.QueryEvent) {}

func TestHookCannotMutateBoundArgs(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithHook(mutatingHook{}), momo.WithArgLogging())
	_, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("keep")).Where(suite.DocID.Eq(1)).Exec(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range drv.Calls() {
		if slices.Contains(c.Args, any("tampered")) {
			t.Fatalf("a hook changed the arguments sent to the driver: %v", c.Args)
		}
	}
}

// predicateCase runs one statement and checks hook arguments position by
// position: every driver argument holding the secret, and every value in
// also (the other values bound to the same sensitive column), shows as
// RedactedArg in the last hook event; every other argument is unchanged.
func predicateCase(t *testing.T, name string, also []any, run func(ctx context.Context, db *momo.DB) error) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		rec := &obstest.Recorder{}
		drv := fakedriver.New(dialect.Postgres)
		db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
		if err := run(t.Context(), db); err != nil {
			t.Fatal(err)
		}
		calls := drv.Calls()
		evs := rec.Events()
		if len(calls) == 0 || len(evs) == 0 {
			t.Fatalf("calls %d, events %d", len(calls), len(evs))
		}
		sent, seen := calls[len(calls)-1].Args, evs[len(evs)-1].Args
		if len(sent) != len(seen) {
			t.Fatalf("driver got %d args, hook saw %d", len(sent), len(seen))
		}
		masked := 0
		for i := range sent {
			s, isStr := sent[i].(string)
			if (isStr && strings.Contains(s, sensitiveValue)) || slices.Contains(also, sent[i]) {
				if seen[i] != momo.RedactedArg {
					t.Errorf("arg %d: hook saw %v, want the placeholder", i, seen[i])
				}
				masked++
				continue
			}
			if seen[i] != sent[i] {
				t.Errorf("arg %d: hook saw %v, driver got %v", i, seen[i], sent[i])
			}
		}
		if want := 1 + len(also); masked < want && name != "non-sensitive stays visible" {
			t.Errorf("masked %d arguments, want at least %d (driver args %v)", masked, want, sent)
		}
	})
}

func TestHooksSeeMaskedSensitivePredicateArgs(t *testing.T) {
	sel := func(p momo.Predicate) func(context.Context, *momo.DB) error {
		return func(ctx context.Context, db *momo.DB) error {
			_, err := momo.Select[suite.Document](db).Where(p).All(ctx)
			return err
		}
	}
	predicateCase(t, "eq", nil, sel(momo.And(suite.DocTitle.Eq("visible"), suite.DocSecret.Eq(sensitiveValue))))
	predicateCase(t, "ne", nil, sel(suite.DocSecret.Ne(sensitiveValue)))
	predicateCase(t, "distinct", nil, sel(momo.Or(suite.DocSecret.IsDistinctFrom(sensitiveValue), suite.DocSecret.IsNotDistinctFrom(sensitiveValue))))
	predicateCase(t, "in", []any{"other"}, sel(momo.And(suite.DocSecret.In(sensitiveValue, "other"), suite.DocTitle.In("a", "b"))))
	predicateCase(t, "not in", nil, sel(suite.DocSecret.NotIn(sensitiveValue)))
	predicateCase(t, "between", []any{"a"}, sel(suite.DocSecret.Between("a", sensitiveValue)))
	predicateCase(t, "like", nil, sel(momo.Like(suite.DocSecret, "%"+sensitiveValue+"%")))
	predicateCase(t, "ilike", nil, sel(momo.ILike(suite.DocSecret, "%"+sensitiveValue+"%")))
	predicateCase(t, "contains literal", nil, sel(momo.ContainsLiteral(suite.DocSecret, sensitiveValue)))
	predicateCase(t, "non-sensitive stays visible", nil, sel(suite.DocTitle.Eq("visible")))
	predicateCase(t, "update where", nil, func(ctx context.Context, db *momo.DB) error {
		_, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("visible")).Where(suite.DocSecret.Eq(sensitiveValue)).Exec(ctx)
		return err
	})
	predicateCase(t, "delete where", nil, func(ctx context.Context, db *momo.DB) error {
		_, err := momo.ForceDelete[suite.Document](db).Where(suite.DocSecret.Eq(sensitiveValue)).Exec(ctx)
		return err
	})
	predicateCase(t, "set expr", nil, func(ctx context.Context, db *momo.DB) error {
		_, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("visible"), suite.DocSecret.SetExpr(momo.Value(sensitiveValue))).
			Where(suite.DocID.Eq(1)).Exec(ctx)
		return err
	})
	predicateCase(t, "insert set expr", nil, func(ctx context.Context, db *momo.DB) error {
		_, err := momo.Insert[suite.Document](db).Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("visible"), suite.DocSecret.SetExpr(momo.Value(sensitiveValue))).Exec(ctx)
		return err
	})
}

// byteHook overwrites the bytes of any []byte argument in place.
type byteHook struct{}

func (byteHook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	for _, a := range e.Args {
		if b, ok := a.([]byte); ok {
			for i := range b {
				b[i] = 'X'
			}
		}
	}
	return ctx
}
func (byteHook) AfterQuery(context.Context, *momo.QueryEvent) {}

func TestHookCannotMutateByteArgsInPlace(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithHook(byteHook{}), momo.WithArgLogging())
	_, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("t")).Where(momo.SQL("title = ?", []byte("keep"))).Exec(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range drv.Calls() {
		for _, a := range c.Args {
			if b, ok := a.([]byte); ok && string(b) != "keep" {
				t.Fatalf("a hook changed a byte argument in place: %q", b)
			}
		}
	}
}
