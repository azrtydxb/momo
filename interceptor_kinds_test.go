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

// kindRecorder records the kind of every operation and denies those in deny.
func kindRecorder(kinds *[]string, deny ...string) momo.Interceptor {
	return func(_ context.Context, op *momo.Operation) error {
		*kinds = append(*kinds, op.Kind)
		if slices.Contains(deny, op.Kind) {
			return errors.New(op.Kind + " denied")
		}
		return nil
	}
}

func TestInterceptorsSeeAnUpsertAsItsOwnKind(t *testing.T) {
	m := hookedDoc{ID: 3, Title: "t"}
	ctx := t.Context()
	run := map[string]func(db *momo.DB) error{
		"upsert": func(db *momo.DB) error { _, err := momo.Upsert[hookedDoc](db).Model(&m).Exec(ctx); return err },
		"insert with DoUpdate": func(db *momo.DB) error {
			_, err := momo.Insert[hookedDoc](db).Model(&m).OnConflict(hookedID).DoUpdate(hookedTitle.SetExcluded()).Exec(ctx)
			return err
		},
		"insert with DoNothing": func(db *momo.DB) error {
			_, err := momo.Insert[hookedDoc](db).Model(&m).OnConflict(hookedID).DoNothing().Exec(ctx)
			return err
		},
		"plain insert": func(db *momo.DB) error { _, err := momo.Insert[hookedDoc](db).Model(&m).Exec(ctx); return err },
	}
	want := map[string]string{
		"upsert": "upsert", "insert with DoUpdate": "upsert", "insert with DoNothing": "insert", "plain insert": "insert",
	}
	for name, fn := range run {
		var kinds []string
		if err := fn(momo.New(fakedriver.New(dialect.Postgres), momo.WithInterceptor(kindRecorder(&kinds)))); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(kinds) != 1 || kinds[0] != want[name] {
			t.Errorf("%s: interceptor saw %v, want [%s]", name, kinds, want[name])
		}
		// A rule that denies upserts leaves plain inserts alone and stops the
		// upserts before any SQL.
		var denied []string
		drv := fakedriver.New(dialect.Postgres)
		err := fn(momo.New(drv, momo.WithInterceptor(kindRecorder(&denied, "upsert"))))
		if want[name] == "upsert" {
			if err == nil || !strings.Contains(err.Error(), "upsert denied") || len(drv.Calls()) != 0 {
				t.Errorf("%s: err = %v, calls = %d, want a denial before SQL", name, err, len(drv.Calls()))
			}
		} else if err != nil {
			t.Errorf("%s: denying upserts blocked it: %v", name, err)
		}
	}
}

func TestInterceptorPredicatesDoNotSatisfyTheGuardForDeletes(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithInterceptor(func(_ context.Context, op *momo.Operation) error {
		if op.Kind == "delete" {
			op.Where = append(op.Where, DocTitle.Eq("mine"))
		}
		return nil
	}))
	ctx := t.Context()

	if _, err := momo.ForceDelete[unitDocument](db).Exec(ctx); !errors.Is(err, momo.ErrUnfilteredMutation) {
		t.Fatalf("delete without Where = %v", err)
	}
	if _, err := momo.SoftDelete[unitDocument](db).Exec(ctx); !errors.Is(err, momo.ErrUnfilteredMutation) {
		t.Fatalf("soft delete without Where = %v", err)
	}
	if n := len(drv.Calls()); n != 0 {
		t.Fatalf("%d statements ran past the guard", n)
	}
	for name, run := range map[string]func() error{
		"delete":      func() error { _, err := momo.ForceDelete[unitDocument](db).AllRows().Exec(ctx); return err },
		"soft delete": func() error { _, err := momo.SoftDelete[unitDocument](db).AllRows().Exec(ctx); return err },
	} {
		before := len(drv.Calls())
		if err := run(); err != nil {
			t.Fatalf("%s with AllRows: %v", name, err)
		}
		last := drv.Calls()[before]
		if !slices.Contains(last.Args, any("mine")) {
			t.Errorf("%s with AllRows lost the interceptor predicate: %q %v", name, last.SQL, last.Args)
		}
	}
}

// versionedDoc has an optimistic-lock version column.
type versionedDoc struct {
	ID      int64
	Title   string
	Version int64
}

var (
	versionedID    = momo.NewColumn[int64]("versioned_docs", "id", momo.FlagPK|momo.FlagIdentity)
	versionedTitle = momo.NewColumn[string]("versioned_docs", "title", 0)
	versionedVer   = momo.NewColumn[int64]("versioned_docs", "version", momo.FlagVersion)
)

var versionedDocMeta = momo.Meta[versionedDoc]{
	Table: "versioned_docs",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: versionedID.Flags()},
		{Name: "title", Flags: versionedTitle.Flags()},
		{Name: "version", Flags: versionedVer.Flags()},
	},
	Ptr: func(m *versionedDoc, i int) any {
		switch i {
		case 0:
			return &m.ID
		case 1:
			return &m.Title
		}
		return &m.Version
	},
	Value: func(m *versionedDoc, i int) any {
		switch i {
		case 0:
			return m.ID
		case 1:
			return m.Title
		}
		return m.Version
	},
	Identity: 0, Version: 2, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*versionedDoc) MomoMeta() *momo.Meta[versionedDoc] { return &versionedDocMeta }

// TestStaleUpdateLookupHonoursInterceptorPredicates: a version-checked update
// that matches no row looks the row up to tell a stale version from a missing
// row. The lookup must carry the update interceptor's predicates, so a row the
// handle may not see is reported as not found.
func TestStaleUpdateLookupHonoursInterceptorPredicates(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	// Only update statements are scoped, so the predicate can reach the lookup
	// only through the update's own interceptor predicates.
	db := momo.New(drv, momo.WithInterceptor(func(_ context.Context, op *momo.Operation) error {
		if op.Kind == "update" {
			op.Where = append(op.Where, versionedTitle.Eq("mine"))
		}
		return nil
	}))
	drv.SetRows([][]any{{int64(1)}}) // without the scope the lookup would find the row
	m := versionedDoc{ID: 7, Title: "other tenant", Version: 1}
	_, err := momo.Update[versionedDoc](db).Model(&m).Exec(t.Context())
	if err == nil {
		t.Fatal("an update matching no row should fail")
	}
	calls := drv.Calls()
	lookup := calls[len(calls)-1]
	if !strings.Contains(lookup.SQL, "COUNT") || !slices.Contains(lookup.Args, any("mine")) {
		t.Fatalf("the lookup did not carry the interceptor predicate: %q %v", lookup.SQL, lookup.Args)
	}
}
