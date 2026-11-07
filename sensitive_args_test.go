// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"slices"
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
