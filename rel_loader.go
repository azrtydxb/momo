// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
)

// Loader is one relation load request, optionally scoped and nested.
// It is sealed: only Relation, RelationLoader (from Relation.Scope and
// Relation.Then) and Relation.Joined implement it.
type Loader[P any] interface {
	relationName() string
	prepare(q Querier, parents []P) (pendingLoad, error)
}

// pendingLoad is a relation load whose statements are compiled but not yet run.
type pendingLoad interface {
	run(ctx context.Context, q Querier) error
}

// loadConfig holds per-load options. Options never mutate a shared slice:
// each option appends to a clipped copy.
type loadConfig struct {
	where          []Predicate
	order          []Orderable
	perParentLimit int
	withDeleted    bool
	onlyDeleted    bool
}

// LoadOption configures one relation load.
type LoadOption[C any] func(*loadConfig)

// LoadWhere adds predicates on the child table to the child statement.
func LoadWhere[C any](ps ...Predicate) LoadOption[C] {
	return func(c *loadConfig) { c.where = append(c.where[:len(c.where):len(c.where)], ps...) }
}

// LoadOrderBy sets the child order. Stable tie-breakers (relation key columns,
// then child primary key) are always appended after it.
func LoadOrderBy[C any](items ...Orderable) LoadOption[C] {
	return func(c *loadConfig) { c.order = append(c.order[:len(c.order):len(c.order)], items...) }
}

// LoadPerParentLimit keeps at most n children per parent, in load order
// (LoadOrderBy, then the relation key and child primary key), using a window
// function so the load stays one statement per bind-limit chunk. It applies to
// has-many and many-to-many relations; on belongs-to and has-one the load fails
// with ErrUnsupported, and a negative n fails with ErrValidation (both with Op
// "load", before any statement runs). Zero leaves the load unlimited. Nested
// loads (Then) run over the kept children only. A many-to-many relation whose
// child has a primary key ranks with DENSE_RANK, so duplicated join-table rows
// share a rank and cannot use up the limit; without a child primary key
// (Relation.ChildPK empty) it ranks with ROW_NUMBER, so duplicated join rows
// each take a rank and can count against the limit before duplicates collapse.
// Window functions need PostgreSQL, MySQL 8 / MariaDB 10.2 or SQLite 3.25 and
// later.
func LoadPerParentLimit[C any](n int) LoadOption[C] {
	return func(c *loadConfig) { c.perParentLimit = n }
}

// Load loads rel for parents and assigns the result onto each parent. Parent
// order and length are unchanged. Parents with a NULL key get an empty
// association (nil pointer or nil slice).
//
// Children of a model with a soft-delete column are excluded by default, as in
// Select; LoadWithDeleted and LoadOnlyDeleted change that.
//
// Associations are cleared up front, so a load that fails part-way leaves the
// parents with empty associations rather than stale or partial ones. Children
// are copied per parent: a to-one child shared by several parents is copied
// shallowly for each, so pointers inside it (including relations loaded with
// Then) are shared among those copies.
func Load[P, C any](ctx context.Context, q Querier, parents []P, rel Relation[P, C], opts ...LoadOption[C]) error {
	return rel.Scope(opts...).load(ctx, q, parents)
}

// LoadAll loads several relations for the same parents, in order. The top
// level of each relation is one statement per bind-limit chunk; nested loads
// run once over all children of the level above.
//
// On a connection whose driver implements Batcher (pgx), the statements of all
// relations of a level, and every bind-limit chunk, are pipelined in one round
// trip; other drivers load sequentially. Results are the same either way; hook
// event order differs (see rel_pipeline.go). A pipelined level buffers all of its
// child rows in memory before assigning them (roughly twice the peak of
// sequential loading). There is no per-call opt-out: a driver or connection
// without Batcher loads sequentially.
func LoadAll[P any](ctx context.Context, q Querier, parents []P, loaders ...Loader[P]) error {
	if len(parents) == 0 {
		return nil
	}
	pending := make([]pendingLoad, 0, len(loaders))
	for _, l := range loaders {
		p, err := l.prepare(q, parents)
		if err != nil {
			return err
		}
		pending = append(pending, p)
	}
	return runPending(ctx, q, pending)
}

// Scope returns a loader for r with per-load options. The result can be scoped
// further and given nested loads: r.Scope(opts...).Then(nested...) filters and
// orders the relation and loads nested relations over exactly the children
// that remain.
func (r Relation[P, C]) Scope(opts ...LoadOption[C]) RelationLoader[P, C] {
	return RelationLoader[P, C]{rel: r}.Scope(opts...)
}

// Then returns a loader for r that also loads nested relations on the children.
// The result can be scoped: r.Then(nested...).Scope(opts...) is the same loader
// as r.Scope(opts...).Then(nested...).
func (r Relation[P, C]) Then(nested ...Loader[C]) RelationLoader[P, C] {
	return RelationLoader[P, C]{rel: r}.Then(nested...)
}

// RelationLoader is a loader for one relation that carries per-load options
// (Scope) and nested loads (Then) together, so "approved comments, newest first,
// each with its author" is one loader:
//
//	document.Comments.Scope(
//		momo.LoadWhere[models.Comment](comment.Approved.Eq(true)),
//		momo.LoadOrderBy[models.Comment](comment.ID.Desc()),
//	).Then(comment.Author)
//
// It is a value: Scope and Then return a new loader and leave the receiver
// unchanged, so a base loader can be shared. Scope may be called several times
// (predicates AND together, order items append, a later LoadPerParentLimit
// replaces an earlier one). Then appends nested loaders. The options apply to
// this relation's children only; nested loaders carry their own options. Nested
// loads run once over exactly the children that survive the options, so
// the statement count stays one per bind-limit chunk at the top and one per
// level below it.
//
// A RelationLoader with no options and no nested loaders behaves like the plain
// Relation. It implements Loader[P]; Loader stays sealed.
type RelationLoader[P, C any] struct {
	rel    Relation[P, C]
	cfg    loadConfig
	nested []Loader[C]
}

// Scope returns a copy of l with opts applied after its existing options.
func (l RelationLoader[P, C]) Scope(opts ...LoadOption[C]) RelationLoader[P, C] {
	for _, o := range opts {
		o(&l.cfg)
	}
	return l
}

// Then returns a copy of l with nested appended after its existing nested loads.
func (l RelationLoader[P, C]) Then(nested ...Loader[C]) RelationLoader[P, C] {
	l.nested = append(l.nested[:len(l.nested):len(l.nested)], nested...)
	return l
}

func (l RelationLoader[P, C]) relationName() string { return l.rel.Name }

func (l RelationLoader[P, C]) prepare(q Querier, parents []P) (pendingLoad, error) {
	return l.rel.prepareWith(q, parents, &l.cfg, l.nested)
}

func (l RelationLoader[P, C]) load(ctx context.Context, q Querier, parents []P) error {
	return LoadAll(ctx, q, parents, Loader[P](l))
}

// Relation implements Loader for the plain relation (no options, no nesting).
func (r Relation[P, C]) relationName() string { return r.Name }

func (r Relation[P, C]) prepare(q Querier, parents []P) (pendingLoad, error) {
	return r.prepareWith(q, parents, &loadConfig{}, nil)
}

// LoadWithDeleted includes soft-deleted children for child models with a
// soft-delete column, like SelectQuery.WithDeleted. They are excluded by default.
func LoadWithDeleted[C any]() LoadOption[C] {
	return func(c *loadConfig) { c.withDeleted, c.onlyDeleted = true, false }
}

// LoadOnlyDeleted loads only soft-deleted children, like SelectQuery.OnlyDeleted.
func LoadOnlyDeleted[C any]() LoadOption[C] {
	return func(c *loadConfig) { c.onlyDeleted, c.withDeleted = true, false }
}
