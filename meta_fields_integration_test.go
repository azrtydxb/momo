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

// TestFieldHooksLive proves against a real database that a Go-side default
// beats the column's DB default, that OnUpdate lands in the guarded UPDATE, and
// that a validation failure leaves the table untouched, chunked bulk included.
func TestFieldHooksLive(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		hook := &countHook{}
		hooks := append([]momo.FieldHook[suite.Document]{
			{Column: 4, Default: func(*suite.Document) any { return int64(5) }},
			{Column: 3, OnUpdate: func(*suite.Document) any { s := "edited"; return &s }},
		}, documentFieldHooks[1])
		db = momo.New(db.Driver(), momo.WithHook(hook), momo.WithFieldHooks(hooks))
		ctx := context.Background()

		// Values: the omitted version takes the Go default (5), not the DB default (1).
		if _, err := momo.Insert[suite.Document](db).Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("v")).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		// ValuesRows: per-row default, explicit zero kept.
		if _, err := momo.Insert[suite.Document](db).ValuesRows(
			[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("r1")},
			[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("r2"), suite.DocVersion.Set(0)},
		).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		want := map[string]int64{"v": 5, "r1": 5, "r2": 0}
		for title, ver := range want {
			got, err := momo.Select[suite.Document](db).Where(suite.DocTitle.Eq(title)).One(ctx)
			if err != nil || got.Version != ver {
				t.Fatalf("%s: version = %d (%v), want %d", title, got.Version, err, ver)
			}
		}

		// Update: OnUpdate fills body; an update matching no row changes nothing.
		if _, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("v2")).Where(suite.DocTitle.Eq("v")).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := momo.Select[suite.Document](db).Where(suite.DocTitle.Eq("v2")).One(ctx)
		if err != nil || got.Body == nil || *got.Body != "edited" {
			t.Fatalf("OnUpdate body = %v (%v)", got.Body, err)
		}

		// A failing validator executes nothing (hook count unchanged) and writes nothing.
		before := hook.n
		bad := []suite.Document{{TenantID: 1, Title: "ok-a"}, {TenantID: 1, Title: ""}}
		if _, err := momo.Insert[suite.Document](db).Models(bad).Exec(ctx); !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("bulk err = %v", err)
		}
		if _, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("")).Where(suite.DocTitle.Eq("v2")).Exec(ctx); !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("update err = %v", err)
		}
		if hook.n != before {
			t.Fatalf("validation failures executed %d statements", hook.n-before)
		}
		if n, err := momo.Select[suite.Document](db).Where(suite.DocTitle.Eq("ok-a")).Count(ctx); err != nil || n != 0 {
			t.Fatalf("rejected bulk wrote %d rows (%v)", n, err)
		}
		if n, err := momo.Select[suite.Document](db).Where(suite.DocTitle.Eq("v2")).Count(ctx); err != nil || n != 1 {
			t.Fatalf("rejected update changed the row: %d (%v)", n, err)
		}

		before = hook.n // the verification counts above are statements too

		// Chunked bulk (over the bind limit) with a bad last row writes nothing.
		docs := make([]suite.Document, 9000)
		for i := range docs {
			docs[i] = suite.Document{TenantID: 7, Title: "bulk"}
		}
		docs[len(docs)-1].Title = ""
		if _, err := momo.Insert[suite.Document](db).Models(docs).Exec(ctx); !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("chunked err = %v", err)
		}
		if hook.n != before {
			t.Fatalf("chunked validation failure executed %d statements", hook.n-before)
		}
		// All valid: the chunked insert applies the default to every row.
		docs[len(docs)-1].Title = "bulk"
		if _, err := momo.Insert[suite.Document](db).Models(docs).Omit(suite.DocVersion).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if n, err := momo.Select[suite.Document](db).Where(suite.DocTenantID.Eq(7), suite.DocVersion.Eq(5)).Count(ctx); err != nil || n != 9000 {
			t.Fatalf("chunked defaults: %d rows with version 5 (%v)", n, err)
		}
	})
}
