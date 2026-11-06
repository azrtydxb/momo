// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

type sdKid struct {
	ID        int64
	ParentID  int64
	Name      string
	DeletedAt *time.Time
}

type sdParent struct {
	ID   int64
	Kids []sdKid
}

var sdKidMeta = momo.Meta[sdKid]{
	Table:          "sd_kids",
	SoftDelete:     3,
	SoftDeleteKind: momo.SoftDeleteTimestamp,
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "parent_id"},
		{Name: "name"},
		{Name: "deleted_at", Flags: momo.FlagSoftDelete | momo.FlagNullable},
	},
	Ptr: func(m *sdKid, i int) any { return [...]any{&m.ID, &m.ParentID, &m.Name, &m.DeletedAt}[i] },
}

var sdKids = momo.Relation[sdParent, sdKid]{
	Name: "Kids", Kind: momo.HasMany,
	Parent: momo.TableRef{Name: "sd_parents"}, Child: momo.TableRef{Name: "sd_kids"},
	ChildMeta:  &sdKidMeta,
	ParentCols: []string{"id"}, ChildCols: []string{"parent_id"}, ChildPK: []string{"id"},
	ParentKey: func(p *sdParent) ([]any, bool) { return []any{p.ID}, true },
	ChildKey:  func(c *sdKid) ([]any, bool) { return []any{c.ParentID}, true },
	GetMany:   func(p *sdParent) []sdKid { return p.Kids },
	SetMany:   func(p *sdParent, cs []sdKid) { p.Kids = cs },
}

func sdNames(p sdParent) []string {
	var out []string
	for _, k := range p.Kids {
		out = append(out, k.Name)
	}
	return out
}

// TestLoadSoftDeletedChildren pins that Load hides soft-deleted children like
// Select does, and that LoadWithDeleted / LoadOnlyDeleted mirror WithDeleted /
// OnlyDeleted.
func TestLoadSoftDeletedChildren(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		ctx := context.Background()
		for _, s := range []string{
			"DROP TABLE IF EXISTS sd_kids",
			"DROP TABLE IF EXISTS sd_parents",
			"CREATE TABLE sd_parents (id BIGINT PRIMARY KEY)",
			"CREATE TABLE sd_kids (id BIGINT PRIMARY KEY, parent_id BIGINT NOT NULL, name TEXT NOT NULL, deleted_at TIMESTAMP NULL)",
			"INSERT INTO sd_parents (id) VALUES (1)",
			"INSERT INTO sd_kids (id, parent_id, name, deleted_at) VALUES (1, 1, 'live', NULL)",
			"INSERT INTO sd_kids (id, parent_id, name, deleted_at) VALUES (2, 1, 'gone', CURRENT_TIMESTAMP)",
		} {
			if _, err := momo.Exec(ctx, db, s); err != nil {
				t.Fatalf("%s: %v", s, err)
			}
		}
		t.Cleanup(func() {
			_, _ = momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS sd_kids")
			_, _ = momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS sd_parents")
		})

		for _, tc := range []struct {
			name string
			opts []momo.LoadOption[sdKid]
			want string
		}{
			{"default hides deleted", nil, "[live]"},
			{"with deleted", []momo.LoadOption[sdKid]{momo.LoadWithDeleted[sdKid]()}, "[live gone]"},
			{"only deleted", []momo.LoadOption[sdKid]{momo.LoadOnlyDeleted[sdKid]()}, "[gone]"},
		} {
			ps := []sdParent{{ID: 1}}
			if err := momo.Load(ctx, db, ps, sdKids, tc.opts...); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if got := fmt.Sprint(sdNames(ps[0])); got != tc.want {
				t.Fatalf("%s: got %s, want %s", tc.name, got, tc.want)
			}
		}
	})
}
