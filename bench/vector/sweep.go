// SPDX-License-Identifier: Apache-2.0

package vector

import (
	"context"
	"errors"
)

// DB is the minimal search interface both the momo and the raw pgx paths
// satisfy.
type DB interface {
	// TopK returns the ids of the k nearest vectors to query, nearest first.
	TopK(ctx context.Context, query []float32, k int) ([]int64, error)
	// WithSearchEf runs fn with hnsw.ef_search set to ef.
	WithSearchEf(ctx context.Context, ef int, fn func() error) error
	// WithExact runs fn with index scans disabled, so TopK is exact.
	WithExact(ctx context.Context, fn func() error) error
}

// Sweep returns the smallest ef_search from efs whose mean recall@k against the
// exact result is at least target, together with the recall achieved. If no
// value reaches target it returns the largest value and its recall.
func Sweep(ctx context.Context, db DB, ds Dataset, k int, target float64, efs []int) (ef int, recall float64, err error) {
	if len(efs) == 0 {
		return 0, 0, errors.New("vector: Sweep needs at least one ef_search value")
	}
	exact := make([][]int64, len(ds.Queries))
	err = db.WithExact(ctx, func() error {
		for i, q := range ds.Queries {
			ids, err := db.TopK(ctx, q, k)
			if err != nil {
				return err
			}
			exact[i] = ids
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	for _, candidate := range efs {
		approx := make([][]int64, len(ds.Queries))
		err = db.WithSearchEf(ctx, candidate, func() error {
			for i, q := range ds.Queries {
				ids, err := db.TopK(ctx, q, k)
				if err != nil {
					return err
				}
				approx[i] = ids
			}
			return nil
		})
		if err != nil {
			return 0, 0, err
		}
		ef, recall = candidate, Recall(exact, approx)
		if recall >= target {
			return ef, recall, nil
		}
	}
	return ef, recall, nil
}
