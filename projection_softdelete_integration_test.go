// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// titleRow is a projection of one column.
type titleRow struct {
	momo.Projection
	Title string
}

var titleRowMeta = momo.Meta[titleRow]{
	Columns:    []momo.ColumnMeta{{Name: "title"}},
	Ptr:        func(m *titleRow, _ int) any { return &m.Title },
	Value:      func(m *titleRow, _ int) any { return m.Title },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
}

func (*titleRow) MomoMeta() *momo.Meta[titleRow] { return &titleRowMeta }

// TestProjectionSoftDelete reads a projection over the timestamp, unix and
// bool soft-delete tables: soft-deleted rows are hidden by default,
// WithDeleted returns both, OnlyDeleted only the deleted one.
func TestProjectionSoftDelete(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		createSoftVariants(t, db)
		ctx := context.Background()
		for _, ins := range []string{
			"INSERT INTO documents (tenant_id, title) VALUES (7, 'live'), (7, 'gone')",
			"INSERT INTO docs_unix (tenant_id, title) VALUES (7, 'live'), (7, 'gone')",
			"INSERT INTO docs_bool (tenant_id, title) VALUES (7, 'live'), (7, 'gone')",
		} {
			if _, err := momo.Exec(ctx, db, ins); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := momo.SoftDelete[suite.Document](db).Where(suite.DocTitle.Eq("gone")).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.SoftDelete[sdUnix](db).Where(sdUnixTitle.Eq("gone")).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.SoftDelete[sdBool](db).Where(sdBoolTitle.Eq("gone")).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		for _, c := range []struct {
			name string
			from momo.TableRef
		}{
			{"timestamp", momo.TableRef{Name: "documents", SoftDelete: "deleted_at"}},
			{"unix", momo.TableRef{Name: "docs_unix", SoftDelete: "deleted_unix", SoftDeleteKind: momo.SoftDeleteUnix}},
			{"bool", momo.TableRef{Name: "docs_bool", SoftDelete: "deleted", SoftDeleteKind: momo.SoftDeleteBool}},
		} {
			t.Run(c.name, func(t *testing.T) {
				title := momo.NewColumn[string](c.from.Name, "title", 0)
				q := func() momo.SelectQuery[titleRow] {
					return momo.Select[titleRow](db).From(c.from).Columns(title).OrderBy(title.Asc())
				}
				titles := func(sq momo.SelectQuery[titleRow]) []string {
					rows, err := sq.All(ctx)
					if err != nil {
						t.Fatal(err)
					}
					var out []string
					for _, r := range rows {
						out = append(out, r.Title)
					}
					return out
				}
				if got := titles(q()); len(got) != 1 || got[0] != "live" {
					t.Errorf("default = %v, want [live]", got)
				}
				if got := titles(q().WithDeleted()); len(got) != 2 {
					t.Errorf("WithDeleted = %v, want both rows", got)
				}
				if got := titles(q().OnlyDeleted()); len(got) != 1 || got[0] != "gone" {
					t.Errorf("OnlyDeleted = %v, want [gone]", got)
				}
				if n, err := q().Count(ctx); err != nil || n != 1 {
					t.Errorf("Count = %d, %v, want 1", n, err)
				}
			})
		}
	})
}
