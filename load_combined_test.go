// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/tagging"
)

// tagRouter answers the two statements of a taggings-then-tags load: the
// scripted taggings for FROM "taggings", the scripted tags for FROM "tags".
type tagRouter struct {
	*fakedriver.Driver
	taggings, tags [][]any
}

func (r *tagRouter) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	switch {
	case strings.Contains(sql, `FROM "taggings"`):
		r.SetRows(r.taggings)
	case strings.Contains(sql, `FROM "tags"`):
		r.SetRows(r.tags)
	default:
		r.SetRows(nil)
	}
	return r.Driver.Query(ctx, sql, args)
}

func newTagRouter() *tagRouter {
	return &tagRouter{
		Driver:   fakedriver.New(dialect.SQLite),
		taggings: [][]any{{int64(1), int64(2), int64(2)}, {int64(1), int64(1), int64(1)}, {int64(3), int64(2), int64(1)}},
		tags:     [][]any{{int64(1), "go"}, {int64(2), "db"}},
	}
}

func combinedLoader() momo.Loader[models.Document] {
	return document.Taggings.Scope(
		momo.LoadWhere[models.Tagging](tagging.Position.Le(2)),
		momo.LoadOrderBy[models.Tagging](tagging.Position.Desc()),
	).Then(tagging.Tag)
}

// A loader can carry a where, an order and nested loads together: the top level
// is one statement carrying the where and the order, the nested level one
// statement over the keys of the children the top level returned.
func TestScopeThenCombinesOptionsAndNestedLoads(t *testing.T) {
	r := newTagRouter()
	db := momo.New(r)
	docs := []models.Document{{ID: 1}, {ID: 3}}
	if err := momo.LoadAll(context.Background(), db, docs, combinedLoader()); err != nil {
		t.Fatal(err)
	}
	calls := r.Calls()
	if len(calls) != 2 {
		t.Fatalf("want one top-level and one nested statement, got %d: %+v", len(calls), calls)
	}
	top := calls[0].SQL
	if !strings.Contains(top, `"position" <=`) || !strings.Contains(top, `"position" DESC`) {
		t.Fatalf("the top statement must carry the where and the order: %s", top)
	}
	if !strings.Contains(calls[1].SQL, `FROM "tags"`) {
		t.Fatalf("second statement must be the nested tags load: %s", calls[1].SQL)
	}
	if len(docs[0].Taggings) != 2 || docs[0].Taggings[0].Tag == nil || docs[0].Taggings[0].Tag.Name != "db" ||
		docs[0].Taggings[1].Tag == nil || docs[0].Taggings[1].Tag.Name != "go" {
		t.Fatalf("doc 1 taggings with their tags: %+v", docs[0].Taggings)
	}
	if len(docs[1].Taggings) != 1 || docs[1].Taggings[0].Tag == nil || docs[1].Taggings[0].Tag.Name != "db" {
		t.Fatalf("doc 3 taggings with their tags: %+v", docs[1].Taggings)
	}
}

// Scope and Then compose in either order, accumulate, and leave the receiver
// untouched so a base loader can be shared.
func TestScopeThenIsOrderIndependentAndImmutable(t *testing.T) {
	run := func(l momo.Loader[models.Document]) (string, []models.Document) {
		r := newTagRouter()
		docs := []models.Document{{ID: 1}, {ID: 3}}
		if err := momo.LoadAll(context.Background(), momo.New(r), docs, l); err != nil {
			t.Fatal(err)
		}
		return r.Calls()[0].SQL, docs
	}
	where := momo.LoadWhere[models.Tagging](tagging.Position.Le(2))
	sqlA, docsA := run(document.Taggings.Scope(where).Then(tagging.Tag))
	sqlB, docsB := run(document.Taggings.Then(tagging.Tag).Scope(where))
	if sqlA != sqlB || !reflect.DeepEqual(docsA, docsB) {
		t.Fatalf("Scope.Then and Then.Scope must agree:\n%s\n%s", sqlA, sqlB)
	}

	base := document.Taggings.Scope(where)
	_ = base.Then(tagging.Tag).Scope(momo.LoadOrderBy[models.Tagging](tagging.Position.Desc()))
	sqlBase, docsBase := run(base)
	if strings.Contains(sqlBase, "DESC") {
		t.Fatalf("Then/Scope must not change the receiver: %s", sqlBase)
	}
	if docsBase[0].Taggings[0].Tag != nil {
		t.Fatalf("Then must not change the receiver: base loaded nested tags")
	}
}

// With no options and no nested loaders the combined forms behave like the
// plain Relation.
func TestScopeThenEmptyBehavesLikePlainRelation(t *testing.T) {
	run := func(l momo.Loader[models.Document]) (string, []models.Document) {
		r := newTagRouter()
		docs := []models.Document{{ID: 1}, {ID: 3}}
		if err := momo.LoadAll(context.Background(), momo.New(r), docs, l); err != nil {
			t.Fatal(err)
		}
		calls := r.Calls()
		if len(calls) != 1 {
			t.Fatalf("plain relation is one statement, got %d", len(calls))
		}
		return calls[0].SQL, docs
	}
	wantSQL, want := run(document.Taggings)
	for name, l := range map[string]momo.Loader[models.Document]{
		"Scope()":        document.Taggings.Scope(),
		"Then()":         document.Taggings.Then(),
		"Scope().Then()": document.Taggings.Scope().Then(),
		"Then().Scope()": document.Taggings.Then().Scope(),
	} {
		gotSQL, got := run(l)
		if gotSQL != wantSQL || !reflect.DeepEqual(got, want) {
			t.Errorf("%s differs from the plain relation:\n%s\n%s", name, gotSQL, wantSQL)
		}
	}
}

// A failure at the nested level of a combined loader is returned from LoadAll,
// not swallowed, and the level above stays unassigned.
func TestScopeThenNestedLoadErrorPropagates(t *testing.T) {
	r := newTagRouter()
	docs := []models.Document{{ID: 1}, {ID: 3}}
	err := momo.LoadAll(context.Background(), momo.New(r), docs,
		document.Taggings.Scope(momo.LoadWhere[models.Tagging](tagging.Position.Le(2))).
			Then(tagging.Tag.Scope(momo.LoadPerParentLimit[models.Tag](1))))
	var me *momo.Error
	if !errors.As(err, &me) || me.Kind != momo.ErrUnsupported {
		t.Fatalf("nested-level error must propagate as ErrUnsupported, got %v", err)
	}
	if docs[0].Taggings != nil {
		t.Fatalf("a failed nested load must not assign the level above: %+v", docs[0].Taggings)
	}
}

// Cancelling the caller's context as the nested relation starts surfaces
// context.Canceled from a combined loader instead of running the nested statement.
func TestScopeThenNestedLoadHonoursContext(t *testing.T) {
	r := newTagRouter()
	h := &cancelHook{Hook: &relfixture.Hook{}, name: "Tag"}
	db := momo.New(r, momo.WithHook(h))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.cancel = cancel
	docs := []models.Document{{ID: 1}, {ID: 3}}
	err := momo.LoadAll(ctx, db, docs, combinedLoader())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled from the nested level, got %v", err)
	}
	for _, c := range r.Calls() {
		if strings.Contains(c.SQL, `FROM "tags"`) {
			t.Fatalf("nested statement must not run after cancellation: %s", c.SQL)
		}
	}
}
