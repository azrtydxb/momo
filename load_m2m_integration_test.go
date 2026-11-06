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

type mmTag struct {
	ID        int64
	Name      string
	DeletedAt *time.Time
}

type mmPost struct {
	ID   int64
	Tags []mmTag
}

var mmTagMeta = momo.Meta[mmTag]{
	Table: "mm_tags", SoftDelete: 2, SoftDeleteKind: momo.SoftDeleteTimestamp,
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "name"},
		{Name: "deleted_at", Flags: momo.FlagSoftDelete | momo.FlagNullable},
	},
	Ptr: func(m *mmTag, i int) any { return [...]any{&m.ID, &m.Name, &m.DeletedAt}[i] },
}

var mmTags = momo.Relation[mmPost, mmTag]{
	Name: "Tags", Kind: momo.ManyToMany,
	Parent: momo.TableRef{Name: "mm_posts"}, Child: momo.TableRef{Name: "mm_tags"},
	ChildMeta:  &mmTagMeta,
	ParentCols: []string{"id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
	Through:   &momo.Through{Table: momo.TableRef{Name: "mm_post_tags"}, ParentCols: []string{"post_id"}, ChildCols: []string{"tag_id"}},
	ParentKey: func(p *mmPost) ([]any, bool) { return []any{p.ID}, true },
	ChildKey:  func(c *mmTag) ([]any, bool) { return []any{c.ID}, true },
	GetMany:   func(p *mmPost) []mmTag { return p.Tags },
	SetMany:   func(p *mmPost, cs []mmTag) { p.Tags = cs },
}

func mmNames(p mmPost) string {
	var out []string
	for _, k := range p.Tags {
		out = append(out, k.Name)
	}
	return fmt.Sprint(out)
}

// TestLoadManyToManySoftDeleteAndDuplicateJoinRows pins, on a real database:
// soft-deleted far rows are hidden (and revealed by LoadWithDeleted), and a
// duplicated join row yields one child.
func TestLoadManyToManySoftDeleteAndDuplicateJoinRows(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		ctx := context.Background()
		for _, s := range []string{
			"DROP TABLE IF EXISTS mm_post_tags",
			"DROP TABLE IF EXISTS mm_tags",
			"DROP TABLE IF EXISTS mm_posts",
			"CREATE TABLE mm_posts (id BIGINT PRIMARY KEY)",
			"CREATE TABLE mm_tags (id BIGINT PRIMARY KEY, name TEXT NOT NULL, deleted_at TIMESTAMP NULL)",
			"CREATE TABLE mm_post_tags (post_id BIGINT NOT NULL, tag_id BIGINT NOT NULL)", // no PK: duplicates possible
			"INSERT INTO mm_posts (id) VALUES (1), (2), (3)",
			"INSERT INTO mm_tags (id, name, deleted_at) VALUES (1, 'a', NULL), (2, 'b', NULL), (3, 'gone', CURRENT_TIMESTAMP)",
			"INSERT INTO mm_post_tags (post_id, tag_id) VALUES (1, 1), (1, 1), (1, 2), (1, 3), (2, 2), (3, 3)",
		} {
			if _, err := momo.Exec(ctx, db, s); err != nil {
				t.Fatalf("%s: %v", s, err)
			}
		}
		t.Cleanup(func() {
			for _, s := range []string{"mm_post_tags", "mm_tags", "mm_posts"} {
				_, _ = momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS "+s)
			}
		})
		posts := func() []mmPost { return []mmPost{{ID: 1}, {ID: 2}, {ID: 3}} }

		for _, tc := range []struct {
			name string
			opts []momo.LoadOption[mmTag]
			want string
		}{
			{"default hides deleted", nil, "[a b] [b] []"},
			{"with deleted", []momo.LoadOption[mmTag]{momo.LoadWithDeleted[mmTag]()}, "[a b gone] [b] [gone]"},
			{"only deleted", []momo.LoadOption[mmTag]{momo.LoadOnlyDeleted[mmTag]()}, "[gone] [] [gone]"},
		} {
			ps := posts()
			if err := momo.Load(ctx, db, ps, mmTags, tc.opts...); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if got := mmNames(ps[0]) + " " + mmNames(ps[1]) + " " + mmNames(ps[2]); got != tc.want {
				t.Fatalf("%s: got %s, want %s", tc.name, got, tc.want)
			}
		}
	})
}
