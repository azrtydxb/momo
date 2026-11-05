// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
)

// Loader is one relation load request, optionally scoped and nested.
// It is sealed: only Relation.Scope, Relation.Then and Relation itself implement it.
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

// LoadPerParentLimit keeps at most n children per parent (implemented with a
// window function; see the top-N task).
func LoadPerParentLimit[C any](n int) LoadOption[C] {
	return func(c *loadConfig) { c.perParentLimit = n }
}

// Load loads rel for parents and assigns the result onto each parent. Parent
// order and length are unchanged. Parents with a NULL key get an empty
// association (nil pointer or nil slice).
func Load[P, C any](ctx context.Context, q Querier, parents []P, rel Relation[P, C], opts ...LoadOption[C]) error {
	return rel.scoped(opts, nil).load(ctx, q, parents)
}

// LoadAll loads several relations for the same parents, in order. The top
// level of each relation is one statement per bind-limit chunk; nested loads
// run once over all children of the level above.
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
	for _, p := range pending {
		if err := p.run(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

// Scope returns a loader for r with per-load options.
func (r Relation[P, C]) Scope(opts ...LoadOption[C]) Loader[P] {
	return r.scoped(opts, nil)
}

// Then returns a loader for r that also loads nested relations on the children.
func (r Relation[P, C]) Then(nested ...Loader[C]) Loader[P] {
	return r.scoped(nil, nested)
}

// scopedLoader is the sealed Loader implementation for a relation with options and nested loads.
type scopedLoader[P, C any] struct {
	rel    Relation[P, C]
	cfg    loadConfig
	nested []Loader[C]
}

func (r Relation[P, C]) scoped(opts []LoadOption[C], nested []Loader[C]) scopedLoader[P, C] {
	s := scopedLoader[P, C]{rel: r, nested: nested}
	for _, o := range opts {
		o(&s.cfg)
	}
	return s
}

func (s scopedLoader[P, C]) relationName() string { return s.rel.Name }

func (s scopedLoader[P, C]) prepare(q Querier, parents []P) (pendingLoad, error) {
	return s.rel.prepareWith(q, parents, &s.cfg, s.nested)
}

func (s scopedLoader[P, C]) load(ctx context.Context, q Querier, parents []P) error {
	return LoadAll(ctx, q, parents, Loader[P](s))
}

// Relation implements Loader for the plain relation (no options, no nesting).
func (r Relation[P, C]) relationName() string { return r.Name }

func (r Relation[P, C]) prepare(q Querier, parents []P) (pendingLoad, error) {
	return r.prepareWith(q, parents, &loadConfig{}, nil)
}

func (r Relation[P, C]) load(ctx context.Context, q Querier, parents []P) error {
	return LoadAll(ctx, q, parents, Loader[P](r))
}
