// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// hookedDoc is the model of the typed-hook tests. Each test installs its own
// hooks on hookedDocMeta and restores the empty set on cleanup.
type hookedDoc struct {
	ID     int64
	Title  string
	Secret string
}

var (
	hookedID    = momo.NewColumn[int64]("hooked_docs", "id", momo.FlagPK|momo.FlagIdentity)
	hookedTitle = momo.NewColumn[string]("hooked_docs", "title", 0)
	hookedSec   = momo.NewColumn[string]("hooked_docs", "secret", 0)
)

var hookedDocMeta = momo.Meta[hookedDoc]{
	Table: "hooked_docs",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: hookedID.Flags()},
		{Name: "title", Flags: hookedTitle.Flags()},
		{Name: "secret", Flags: hookedSec.Flags()},
	},
	Ptr: func(m *hookedDoc, i int) any {
		switch i {
		case 0:
			return &m.ID
		case 1:
			return &m.Title
		}
		return &m.Secret
	},
	Value: func(m *hookedDoc, i int) any {
		switch i {
		case 0:
			return m.ID
		case 1:
			return m.Title
		}
		return m.Secret
	},
	Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*hookedDoc) MomoMeta() *momo.Meta[hookedDoc] { return &hookedDocMeta }

func setHooks(t *testing.T, h momo.HookSet[hookedDoc]) {
	t.Helper()
	hookedDocMeta.Hooks = h
	t.Cleanup(func() { hookedDocMeta.Hooks = momo.HookSet[hookedDoc]{} })
}

func hookedRow() [][]any { return [][]any{{int64(1), "hook", "set-by-hook"}} }

func TestModelHooks(t *testing.T) {
	var order []string
	veto := errors.New("veto")
	setHooks(t, momo.HookSet[hookedDoc]{
		BeforeInsert: func(_ context.Context, m *hookedDoc) error {
			order = append(order, "insert")
			m.Secret = "set-by-hook"
			return nil
		},
		BeforeUpdate: func(context.Context, *hookedDoc) error {
			order = append(order, "update")
			return veto
		},
		AfterLoad: func(_ context.Context, m *hookedDoc) error {
			order = append(order, "load")
			return nil
		},
	})
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	ctx := t.Context()

	d := hookedDoc{Title: "hook"}
	if _, err := momo.Insert[hookedDoc](db).Model(&d).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if d.Secret != "" {
		t.Fatalf("BeforeInsert changed the caller's model: %+v", d)
	}
	calls := drv.Calls()
	if len(calls) != 1 || !slices.Contains(calls[0].Args, any("set-by-hook")) {
		t.Fatalf("the value set by BeforeInsert was not bound: %+v", calls)
	}

	drv.SetRows(hookedRow())
	got, err := momo.Select[hookedDoc](db).Where(hookedTitle.Eq("hook")).One(ctx)
	if err != nil || got.Secret != "set-by-hook" {
		t.Fatalf("One = %+v, %v", got, err)
	}

	before := len(drv.Calls())
	_, err = momo.Update[hookedDoc](db).Model(&got).Exec(ctx)
	if !errors.Is(err, veto) || !errors.Is(err, momo.ErrValidation) || errors.Is(err, momo.ErrOptimisticLock) {
		t.Fatalf("BeforeUpdate veto = %v", err)
	}
	if len(drv.Calls()) != before {
		t.Fatalf("a statement ran after the BeforeUpdate veto: %+v", drv.Calls()[before:])
	}

	want := []string{"insert", "load", "update"}
	if !slices.Equal(order, want) {
		t.Fatalf("hook order = %v, want %v", order, want)
	}
}

func TestModelHooksOnlyForOptedInBuilders(t *testing.T) {
	var inserts, updates, deletes, loads int
	setHooks(t, momo.HookSet[hookedDoc]{
		BeforeInsert: func(context.Context, *hookedDoc) error { inserts++; return nil },
		BeforeUpdate: func(context.Context, *hookedDoc) error { updates++; return nil },
		BeforeDelete: func(context.Context, *hookedDoc) error { deletes++; return nil },
		AfterLoad:    func(context.Context, *hookedDoc) error { loads++; return nil },
	})
	counts := func() [4]int { return [4]int{inserts, updates, deletes, loads} }
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	ctx := t.Context()
	m := hookedDoc{ID: 5, Title: "t"}

	steps := []struct {
		name string
		run  func() error
		want [4]int
	}{
		{"insert", func() error { _, err := momo.Insert[hookedDoc](db).Model(&m).Exec(ctx); return err }, [4]int{1, 0, 0, 0}},
		{"insert values", func() error {
			_, err := momo.Insert[hookedDoc](db).Values(hookedTitle.Set("x")).Exec(ctx)
			return err
		}, [4]int{1, 0, 0, 0}},
		{"update model", func() error { _, err := momo.Update[hookedDoc](db).Model(&m).Exec(ctx); return err }, [4]int{1, 1, 0, 0}},
		{"update set", func() error {
			_, err := momo.Update[hookedDoc](db).Set(hookedTitle.Set("y")).Where(hookedID.Eq(5)).Exec(ctx)
			return err
		}, [4]int{1, 1, 0, 0}},
		{"delete model", func() error { _, err := momo.Delete[hookedDoc](db).Model(&m).Exec(ctx); return err }, [4]int{1, 1, 1, 0}},
		{"delete where", func() error {
			_, err := momo.Delete[hookedDoc](db).Where(hookedID.Eq(5)).Exec(ctx)
			return err
		}, [4]int{1, 1, 1, 0}},
		{"select no rows", func() error { _, err := momo.Select[hookedDoc](db).All(ctx); return err }, [4]int{1, 1, 1, 0}},
	}
	for _, s := range steps {
		if err := s.run(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if got := counts(); got != s.want {
			t.Fatalf("after %s: hook counts (insert, update, delete, load) = %v, want %v", s.name, got, s.want)
		}
	}
}

func TestModelHooksBeforeAbortWithoutSQL(t *testing.T) {
	veto := errors.New("veto")
	fail := func(context.Context, *hookedDoc) error { return veto }
	setHooks(t, momo.HookSet[hookedDoc]{BeforeInsert: fail, BeforeUpdate: fail, BeforeDelete: fail})
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	ctx := t.Context()
	m := hookedDoc{ID: 5, Title: "t"}

	if _, err := momo.Insert[hookedDoc](db).Models([]hookedDoc{m, m}).Exec(ctx); !errors.Is(err, veto) {
		t.Fatalf("insert = %v", err)
	}
	if _, err := momo.Insert[hookedDoc](db).Model(&m).ReturningAll().Rows(ctx); !errors.Is(err, veto) {
		t.Fatalf("insert rows = %v", err)
	}
	if _, err := momo.Delete[hookedDoc](db).Model(&m).Exec(ctx); !errors.Is(err, veto) {
		t.Fatalf("delete = %v", err)
	}
	if n := len(drv.Calls()); n != 0 {
		t.Fatalf("%d statements ran after a Before hook failed", n)
	}
}

func TestModelHooksAfterLoadOnEveryRead(t *testing.T) {
	var loads int
	setHooks(t, momo.HookSet[hookedDoc]{AfterLoad: func(_ context.Context, m *hookedDoc) error {
		loads++
		m.Title += "!"
		return nil
	}})
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{int64(1), "a", "s"}, {int64(2), "b", "s"}})
	db := momo.New(drv)
	ctx := t.Context()

	all, err := momo.Select[hookedDoc](db).All(ctx)
	if err != nil || len(all) != 2 || all[0].Title != "a!" || all[1].Title != "b!" || loads != 2 {
		t.Fatalf("All = %+v, %v, loads %d", all, err, loads)
	}
	loads = 0
	var seen []string
	if err := momo.Select[hookedDoc](db).Each(ctx, func(m *hookedDoc) error { seen = append(seen, m.Title); return nil }); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(seen, []string{"a!", "b!"}) || loads != 2 {
		t.Fatalf("Each saw %v, loads %d", seen, loads)
	}
	loads, seen = 0, nil
	for m, err := range momo.Select[hookedDoc](db).Iter(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		seen = append(seen, m.Title)
	}
	if !slices.Equal(seen, []string{"a!", "b!"}) || loads != 2 {
		t.Fatalf("Iter saw %v, loads %d", seen, loads)
	}
	drv.SetRows([][]any{{int64(1), "a", "s"}})
	loads = 0
	first, err := momo.Select[hookedDoc](db).First(ctx)
	if err != nil || first.Title != "a!" || loads != 1 {
		t.Fatalf("First = %+v, %v, loads %d", first, err, loads)
	}
	loads = 0
	rows, err := momo.Insert[hookedDoc](db).Values(hookedTitle.Set("a")).ReturningAll().Rows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rows.Close() })
	var ret hookedDoc
	if !rows.Next() {
		t.Fatal("no returned row")
	}
	if err := rows.ScanModel(&ret); err != nil || ret.Title != "a!" || loads != 1 {
		t.Fatalf("ScanModel = %+v, %v, loads %d", ret, err, loads)
	}
}

func TestModelHooksAfterLoadError(t *testing.T) {
	boom := errors.New("boom")
	setHooks(t, momo.HookSet[hookedDoc]{AfterLoad: func(context.Context, *hookedDoc) error { return boom }})
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows(hookedRow())
	db := momo.New(drv)
	if _, err := momo.Select[hookedDoc](db).All(t.Context()); !errors.Is(err, boom) {
		t.Fatalf("All = %v", err)
	}
	if _, err := momo.Select[hookedDoc](db).One(t.Context()); !errors.Is(err, boom) {
		t.Fatalf("One = %v", err)
	}
}
