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
	Ptr:   func(m *mmTag, i int) any { return [...]any{&m.ID, &m.Name, &m.DeletedAt}[i] },
	Value: func(m *mmTag, i int) any { return [...]any{m.ID, m.Name, m.DeletedAt}[i] },
	PK:    []int{0},
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

type mmCoded struct {
	ID   int64
	Code string
}

var mmCodedMeta = momo.Meta[mmCoded]{
	Table: "mm_coded", SoftDelete: -1,
	Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "code"}},
	Ptr:     func(m *mmCoded, i int) any { return [...]any{&m.ID, &m.Code}[i] },
	Value:   func(m *mmCoded, i int) any { return [...]any{m.ID, m.Code}[i] },
	PK:      []int{0},
}

type mmCodedPost struct {
	ID    int64
	Coded []mmCoded
}

// The relation targets the non-PK unique column "code"; duplicated join rows
// must still collapse to one child per (parent, child) pair.
var mmCodedRel = momo.Relation[mmCodedPost, mmCoded]{
	Name: "Coded", Kind: momo.ManyToMany,
	Parent: momo.TableRef{Name: "mm_posts"}, Child: momo.TableRef{Name: "mm_coded"},
	ChildMeta:  &mmCodedMeta,
	ParentCols: []string{"id"}, ChildCols: []string{"code"}, ChildPK: []string{"id"},
	Through:   &momo.Through{Table: momo.TableRef{Name: "mm_post_codes"}, ParentCols: []string{"post_id"}, ChildCols: []string{"code"}},
	ParentKey: func(p *mmCodedPost) ([]any, bool) { return []any{p.ID}, true },
	ChildKey:  func(c *mmCoded) ([]any, bool) { return []any{c.Code}, true },
	GetMany:   func(p *mmCodedPost) []mmCoded { return p.Coded },
	SetMany:   func(p *mmCodedPost, cs []mmCoded) { p.Coded = cs },
}

func TestLoadManyToManyDuplicateJoinRowsOnNonPKTarget(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		ctx := context.Background()
		for _, s := range []string{
			"DROP TABLE IF EXISTS mm_post_codes",
			"DROP TABLE IF EXISTS mm_coded",
			"DROP TABLE IF EXISTS mm_posts",
			"CREATE TABLE mm_posts (id BIGINT PRIMARY KEY)",
			"CREATE TABLE mm_coded (id BIGINT PRIMARY KEY, code TEXT NOT NULL UNIQUE)",
			"CREATE TABLE mm_post_codes (post_id BIGINT NOT NULL, code TEXT NOT NULL)",
			"INSERT INTO mm_posts (id) VALUES (1), (2)",
			"INSERT INTO mm_coded (id, code) VALUES (1, 'x'), (2, 'y')",
			"INSERT INTO mm_post_codes (post_id, code) VALUES (1, 'x'), (1, 'x'), (1, 'y'), (2, 'x')",
		} {
			if _, err := momo.Exec(ctx, db, s); err != nil {
				t.Fatalf("%s: %v", s, err)
			}
		}
		t.Cleanup(func() {
			for _, s := range []string{"mm_post_codes", "mm_coded", "mm_posts"} {
				_, _ = momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS "+s)
			}
		})
		ps := []mmCodedPost{{ID: 1}, {ID: 2}}
		if err := momo.Load(ctx, db, ps, mmCodedRel); err != nil {
			t.Fatal(err)
		}
		if len(ps[0].Coded) != 2 || len(ps[1].Coded) != 1 {
			t.Fatalf("duplicate join rows not collapsed: %+v %+v", ps[0].Coded, ps[1].Coded)
		}
	})
}

type mcHook struct{ args []int }

func (h *mcHook) BeforeQuery(ctx context.Context, _ *momo.QueryEvent) context.Context { return ctx }
func (h *mcHook) AfterQuery(_ context.Context, e *momo.QueryEvent) {
	if e.Relation != "" {
		h.args = append(h.args, e.NumArgs)
	}
}

type mcPost struct {
	Tenant, Code int64
	Kids         []mmCoded
}

var mcRel = momo.Relation[mcPost, mmCoded]{
	Name: "Kids", Kind: momo.ManyToMany,
	Parent: momo.TableRef{Name: "mc_posts"}, Child: momo.TableRef{Name: "mm_coded"},
	ChildMeta:  &mmCodedMeta,
	ParentCols: []string{"tenant", "code"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
	Through:   &momo.Through{Table: momo.TableRef{Name: "mc_join"}, ParentCols: []string{"tenant", "code"}, ChildCols: []string{"kid_id"}},
	ParentKey: func(p *mcPost) ([]any, bool) { return []any{p.Tenant, p.Code}, true },
	ChildKey:  func(c *mmCoded) ([]any, bool) { return []any{c.ID}, true },
	GetMany:   func(p *mcPost) []mmCoded { return p.Kids },
	SetMany:   func(p *mcPost, cs []mmCoded) { p.Kids = cs },
}

// A composite join key (two parent columns, one child column) is chunked on
// the parent key width: with room for four arguments, five parents need three
// statements binding 4, 4 and 2 arguments.
func TestLoadManyToManyCompositeJoinKeyChunking(t *testing.T) {
	h := &mcHook{}
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		ctx := context.Background()
		for _, s := range []string{
			"DROP TABLE IF EXISTS mc_join",
			"DROP TABLE IF EXISTS mc_posts",
			"DROP TABLE IF EXISTS mm_coded",
			"CREATE TABLE mc_posts (tenant BIGINT NOT NULL, code BIGINT NOT NULL, PRIMARY KEY (tenant, code))",
			"CREATE TABLE mm_coded (id BIGINT PRIMARY KEY, code TEXT NOT NULL UNIQUE)",
			"CREATE TABLE mc_join (tenant BIGINT NOT NULL, code BIGINT NOT NULL, kid_id BIGINT NOT NULL)",
			"INSERT INTO mc_posts (tenant, code) VALUES (1, 1), (1, 2), (2, 1), (2, 2), (3, 1)",
			"INSERT INTO mm_coded (id, code) VALUES (1, 'a'), (2, 'b')",
			"INSERT INTO mc_join (tenant, code, kid_id) VALUES (1, 1, 1), (1, 2, 2), (2, 1, 1), (2, 1, 2), (3, 1, 2)",
		} {
			if _, err := momo.Exec(ctx, db, s); err != nil {
				t.Fatalf("%s: %v", s, err)
			}
		}
		t.Cleanup(func() {
			for _, s := range []string{"mc_join", "mc_posts", "mm_coded"} {
				_, _ = momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS "+s)
			}
		})
		ps := []mcPost{{Tenant: 1, Code: 1}, {Tenant: 1, Code: 2}, {Tenant: 2, Code: 1}, {Tenant: 2, Code: 2}, {Tenant: 3, Code: 1}}
		h.args = nil
		if err := momo.Load(ctx, db, ps, mcRel); err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(h.args) != "[4 4 2]" {
			t.Fatalf("per-statement argument counts %v, want [4 4 2]", h.args)
		}
		if len(ps[0].Kids) != 1 || len(ps[1].Kids) != 1 || len(ps[2].Kids) != 2 || len(ps[3].Kids) != 0 || len(ps[4].Kids) != 1 {
			t.Fatalf("kids: %+v", ps)
		}
	}, momo.WithHook(h), momo.WithMaxParams(16+4))
}
