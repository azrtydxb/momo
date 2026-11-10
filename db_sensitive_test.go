// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/suite"
)

// failingDriver fails every Exec with a driver message that does not carry
// the bound values, as a real driver's message does not.
type failingDriver struct{ *fakedriver.Driver }

func (failingDriver) Exec(context.Context, string, []any) (int64, error) {
	return 0, errors.New("driver: statement failed")
}

// assertNoSecret fails when s shows the sensitive value.
func assertNoSecret(t *testing.T, where, s string) {
	t.Helper()
	if strings.Contains(s, sensitiveValue) {
		t.Fatalf("%s shows the sensitive value: %s", where, s)
	}
}

// TestSensitiveColumns checks every path that can print a bound value with
// argument logging on: the hook events (before and after the statement,
// including a failed statement), the error text, and the compiled-statement
// dump. The placeholder replaces the value; the driver still gets the real one.
func TestSensitiveColumns(t *testing.T) {
	ins := func(q momo.Querier) momo.InsertQuery[suite.Document] {
		return momo.Insert[suite.Document](q).Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("s"), suite.DocSecret.Set(sensitiveValue))
	}

	t.Run("hook events", func(t *testing.T) {
		rec := &obstest.Recorder{}
		drv := fakedriver.New(dialect.Postgres)
		db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
		if _, err := ins(db).Exec(t.Context()); err != nil {
			t.Fatal(err)
		}
		assertMasked(t, rec, drv)
		for _, e := range rec.Events() {
			assertNoSecret(t, "event SQL", e.SQL)
			assertNoSecret(t, "event args", fmt.Sprint(e.Args))
			assertNoSecret(t, "event", fmt.Sprintf("%+v", e.QueryEvent))
		}
	})

	t.Run("failed statement", func(t *testing.T) {
		rec := &obstest.Recorder{}
		db := momo.New(failingDriver{fakedriver.New(dialect.Postgres)}, momo.WithHook(rec), momo.WithArgLogging())
		_, err := ins(db).Exec(t.Context())
		if err == nil {
			t.Fatal("the statement should have failed")
		}
		assertNoSecret(t, "error text", err.Error())
		var me *momo.Error
		if errors.As(err, &me) {
			assertNoSecret(t, "error detail", me.Detail())
		}
		evs := rec.Events()
		if len(evs) == 0 || evs[len(evs)-1].Err == nil {
			t.Fatalf("no failed event recorded: %+v", evs)
		}
		for _, e := range evs {
			assertNoSecret(t, "event args", fmt.Sprint(e.Args))
			assertNoSecret(t, "event error", fmt.Sprint(e.Err))
		}
		if got := evs[len(evs)-1].Args; len(got) == 0 || !containsArg(got, momo.RedactedArg) {
			t.Fatalf("failed event args = %v, want the placeholder", got)
		}
	})

	t.Run("build errors", func(t *testing.T) {
		db := momo.New(fakedriver.New(dialect.Postgres))
		// An update without a filter fails while building; its text must not
		// carry the value that was being set.
		_, err := momo.Update[suite.Document](db).Set(suite.DocSecret.Set(sensitiveValue)).Exec(t.Context())
		if !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Fatalf("err = %v", err)
		}
		assertNoSecret(t, "error text", err.Error())
	})

	t.Run("dumps", func(t *testing.T) {
		db := momo.New(fakedriver.New(dialect.Postgres), momo.WithArgLogging())
		stmts := map[string]func() (momo.Compiled, error){
			"insert": func() (momo.Compiled, error) { return ins(db).SQL() },
			"update": func() (momo.Compiled, error) {
				return momo.Update[suite.Document](db).Set(suite.DocSecret.Set(sensitiveValue)).Where(suite.DocID.Eq(1)).SQL()
			},
			"select": func() (momo.Compiled, error) {
				return momo.Select[suite.Document](db).Where(suite.DocSecret.Eq(sensitiveValue)).SQL()
			},
		}
		for name, build := range stmts {
			c, err := build()
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			assertNoSecret(t, name+" SQL text", c.SQL)
			assertNoSecret(t, name+" dump", c.String())
			assertNoSecret(t, name+" dump (%v)", fmt.Sprintf("%v", c))
			assertNoSecret(t, name+" dump (%+v)", fmt.Sprintf("%+v", c))
		}
	})
}

func containsArg(args []any, want any) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
