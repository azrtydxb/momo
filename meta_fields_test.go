// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

type countHook struct{ n int }

func (c *countHook) BeforeQuery(ctx context.Context, _ *momo.QueryEvent) context.Context {
	c.n++
	return ctx
}
func (c *countHook) AfterQuery(context.Context, *momo.QueryEvent) {}

// documentFieldHooks gives Document a Go-side default version of 1 (applied
// only when the column is omitted) and rejects an empty title.
var documentFieldHooks = []momo.FieldHook[suite.Document]{
	{Column: 4, Default: func(*suite.Document) any { return int64(1) }},
	{Column: 2, Validate: func(v any) error {
		if s, ok := v.(string); !ok || s == "" {
			return errors.New("title must not be empty")
		}
		return nil
	}},
}

// TestDefaultsAndValidators is the plan's Task 21 test. The plan inserts the
// default-version model without Omit, which cannot both write a zero and apply
// a default (a Model insert writes every column), so the default case omits
// the version column: omitted columns get the default, zero values are kept.
func TestDefaultsAndValidators(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		hook := &countHook{}
		db = momo.New(db.Driver(), momo.WithHook(hook), momo.WithFieldHooks(documentFieldHooks))
		ctx := context.Background()
		if _, err := momo.Insert[suite.Document](db).Values(suite.DocTenantID.Set(1)).Exec(ctx); err == nil {
			t.Fatal("empty title should fail validation")
		} else if !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("err = %v, want ErrValidation", err)
		}
		if hook.n != 0 {
			t.Fatalf("validation ran after SQL was executed (%d statements)", hook.n)
		}
		d := suite.Document{TenantID: 1, Title: "ok"}
		if _, err := momo.Insert[suite.Document](db).Model(&d).Omit(suite.DocVersion).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := momo.Select[suite.Document](db).Where(suite.DocTitle.Eq("ok")).One(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.Version != 1 {
			t.Fatalf("default version = %d, want 1", got.Version)
		}
		zero := suite.Document{TenantID: 1, Title: "z", Version: 0}
		if _, err := momo.Insert[suite.Document](db).Model(&zero).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		z, err := momo.Select[suite.Document](db).Where(suite.DocTitle.Eq("z")).One(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if z.Version != 0 {
			t.Fatalf("explicit zero version was replaced by a default: %d", z.Version)
		}
	})
}
