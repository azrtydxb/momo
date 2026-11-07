// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// TestHooks checks the S-18 hook contract: SQL, duration, rows and errors are
// reported, and bound arguments only with WithArgLogging. The redaction of
// FlagSensitive arguments is asserted where momo-05 delivers it.
func TestHooks(t *testing.T) {
	ctx := context.Background()

	t.Run("select reports sql duration rows and no args", func(t *testing.T) {
		rec := &obstest.Recorder{}
		testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
			obsSeed(t, db)
			rec.Reset()
			docs := obsDocs(t, db)
			ev := rec.Events()
			if len(ev) != 1 {
				t.Fatalf("want 1 event, got %d", len(ev))
			}
			e := ev[0]
			if e.Op != "select" || e.Table != "documents" || !strings.Contains(e.SQL, `"documents"`) {
				t.Fatalf("op/table/sql wrong: %+v", e.QueryEvent)
			}
			if e.Rows != int64(len(docs)) {
				t.Fatalf("Rows = %d, want %d", e.Rows, len(docs))
			}
			if e.Duration <= 0 {
				t.Fatalf("Duration = %v, want > 0", e.Duration)
			}
			if e.Err != nil || e.Args != nil {
				t.Fatalf("Err = %v Args = %v, want nil and nil", e.Err, e.Args)
			}
			if e.CtxSQL != e.SQL {
				t.Fatalf("context from BeforeQuery did not reach AfterQuery: %q", e.CtxSQL)
			}
		}, momo.WithHook(rec))
	})

	t.Run("args are excluded without WithArgLogging", func(t *testing.T) {
		rec := &obstest.Recorder{}
		testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
			obsSeed(t, db)
			rec.Reset()
			if _, err := momo.Select[models.Author](db).Where(author.Name.Eq("Ada")).All(ctx); err != nil {
				t.Fatal(err)
			}
			e := rec.Events()[0]
			if e.Args != nil || e.NumArgs != 1 {
				t.Fatalf("Args = %v NumArgs = %d, want nil and 1", e.Args, e.NumArgs)
			}
		}, momo.WithHook(rec))
	})

	t.Run("args are included with WithArgLogging", func(t *testing.T) {
		rec := &obstest.Recorder{}
		testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
			obsSeed(t, db)
			rec.Reset()
			if _, err := momo.Select[models.Author](db).Where(author.Name.Eq("Ada")).All(ctx); err != nil {
				t.Fatal(err)
			}
			e := rec.Events()[0]
			if len(e.Args) != 1 || e.Args[0] != "Ada" {
				t.Fatalf("Args = %v, want [Ada]", e.Args)
			}
		}, momo.WithHook(rec), momo.WithArgLogging())
	})

	t.Run("exec reports rows affected", func(t *testing.T) {
		rec := &obstest.Recorder{}
		testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
			obsSeed(t, db)
			rec.Reset()
			if _, err := momo.Exec(ctx, db, `INSERT INTO authors (id, name) VALUES (?, ?)`, int64(50), "X"); err != nil {
				t.Fatal(err)
			}
			e := rec.Events()[0]
			if e.Op != "exec" || e.Rows != 1 {
				t.Fatalf("Op = %q Rows = %d, want exec and 1", e.Op, e.Rows)
			}
		}, momo.WithHook(rec))
	})

	t.Run("errors are reported", func(t *testing.T) {
		rec := &obstest.Recorder{}
		testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
			obsSeed(t, db)
			rec.Reset()
			_, err := momo.Exec(ctx, db, `SELECT * FROM no_such_table`)
			if err == nil {
				t.Fatal("want an error for a missing table")
			}
			e := rec.Events()[0]
			if e.Err == nil || !errors.Is(err, e.Err) {
				t.Fatalf("event Err = %v, returned error = %v", e.Err, err)
			}
			if e.Rows != -1 {
				t.Fatalf("Rows = %d, want -1 for a failed statement", e.Rows)
			}
		}, momo.WithHook(rec))
	})
}
