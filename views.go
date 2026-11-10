// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

// RefreshMaterializedView runs REFRESH MATERIALIZED VIEW on q. The name is
// the view's name, or schema.name; each part is quoted as an identifier, so a
// name cannot inject SQL. With concurrently set the statement is
// REFRESH MATERIALIZED VIEW CONCURRENTLY: readers are not blocked, and
// PostgreSQL requires a unique index on the view (declare it in the view's
// Indexes) and a view that already holds data.
//
// PostgreSQL only: other dialects fail with ErrUnsupported, and a name that
// is empty or has more than two parts fails with ErrValidation, both before
// any statement is sent. Declare the view in the model's MomoConfig with
// c.View and Materialized set; a migration creates it, empty or filled as the
// query returns at that moment, and this helper brings it up to date.
func RefreshMaterializedView(ctx context.Context, q Querier, name string, concurrently bool) error {
	d := q.Dialect()
	if d.Name != dialect.Postgres {
		return newError(ErrUnsupported, "refresh", name, errors.New("materialized views are only available on PostgreSQL"))
	}
	parts := strings.Split(name, ".")
	if len(parts) > 2 {
		return validation("refresh", name, "view name %q has more than two parts", name)
	}
	quoted := make([]string, len(parts))
	for i, p := range parts {
		if p == "" {
			return validation("refresh", name, "view name %q has an empty part", name)
		}
		quoted[i] = d.QuoteIdent(p)
	}
	sql := "REFRESH MATERIALIZED VIEW "
	if concurrently {
		sql += "CONCURRENTLY "
	}
	sql += strings.Join(quoted, ".")
	// The statement is sent as is: a quoted name may hold '?' or '$1', which
	// the raw-SQL placeholder rewriting of Exec would take for parameters.
	_, err := q.conn().exec(ctx, "exec", name, Compiled{SQL: sql})
	return err
}
