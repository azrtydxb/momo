// SPDX-License-Identifier: Apache-2.0

package momo

import "context"

// Scalars renders sql through the raw path (see Exec: '?' binds, "??" is a
// literal '?', Expr arguments render inline, an argument-count mismatch is a
// build error; on PostgreSQL native $n placeholders bind argument n, see
// SQL) and returns the first column of every row as V. An empty
// result returns a nil slice and a nil error.
func Scalars[V any](ctx context.Context, q Querier, sql string, args ...any) ([]V, error) {
	c, err := compile(q.Dialect(), func(b *Builder) { b.appendRaw(sql, args) })
	if err != nil {
		return nil, err
	}
	r, err := q.conn().query(ctx, "raw", "", "", c)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var out []V
	for r.Next() {
		var v V
		if err := r.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return out, r.Close()
}
