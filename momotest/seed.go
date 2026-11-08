// SPDX-License-Identifier: Apache-2.0

package momotest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
)

const seedTimeout = time.Minute

// Step is one part of a Seed: it writes fixtures through q.
type Step func(ctx context.Context, q momo.Querier) error

// Insert returns a step that inserts rows one by one, in the order given,
// reading generated columns (identity keys, defaults) back into each row. The
// pointers stay valid after Seed, so a later step or the test can use a key
// that an earlier row received:
//
//	org := &models.Org{Name: "acme"}
//	momotest.Seed(t, db,
//	    momotest.Insert(org),
//	    momotest.Insert(&models.User{OrgID: org.ID, Email: "ada@acme.test"}),
//	)
func Insert[T any, PT momo.Modeler[T]](rows ...*T) Step {
	return func(ctx context.Context, q momo.Querier) error {
		for _, r := range rows {
			if err := momo.CreateGraph[T, PT](ctx, q, r); err != nil {
				return err
			}
		}
		return nil
	}
}

// Graph returns a step that inserts root with the related rows it includes in
// one unit, as momo.CreateGraph does: parents first, then root, then children,
// with foreign keys filled in.
func Graph[T any, PT momo.Modeler[T]](root *T, includes ...momo.Association[T]) Step {
	return func(ctx context.Context, q momo.Querier) error {
		return momo.CreateGraph[T, PT](ctx, q, root, includes...)
	}
}

// Seed runs the steps in order, so a table that others reference comes first.
// All of them run in one transaction (a savepoint when q is a *momo.Tx, such
// as the one from NewTx): if a step fails, nothing is kept and the test fails
// with the error. q is a *momo.DB or a *momo.Tx.
func Seed(t testing.TB, q momo.Querier, steps ...Step) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), seedTimeout)
	defer cancel()
	run := func(tx *momo.Tx) error {
		for _, s := range steps {
			if err := s(ctx, tx); err != nil {
				return err
			}
		}
		return nil
	}
	var err error
	switch qq := q.(type) {
	case *momo.DB:
		err = qq.Tx(ctx, momo.TxOptions{}, run)
	case *momo.Tx:
		err = qq.Tx(ctx, run)
	default:
		err = errors.New("momotest: Seed needs a *momo.DB or a *momo.Tx")
	}
	if err != nil {
		t.Fatalf("momotest: seed: %v", err)
	}
}
