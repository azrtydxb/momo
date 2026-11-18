// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

const (
	// Indexes that back a unique, primary-key or exclusion constraint are
	// reported by the constraint, not here. An index attached to a parent
	// partitioned index (a pg_inherits row on the index) belongs to the parent:
	// Postgres creates, names and drops it.
	// $1 is the set of relation oids, passed as int8[] so the driver encodes it
	// without an oid[] codec; the rows of each relation come in relname order.
	qIndexes = `SELECT i.indrelid, ci.oid, ci.relname, am.amname, i.indisunique, i.indisvalid, i.indnkeyatts,
       pg_get_expr(i.indpred, i.indrelid), i.indoption::text, i.indclass::text, ci.reloptions
FROM pg_index i
JOIN pg_class ci ON ci.oid = i.indexrelid
JOIN pg_am am ON am.oid = ci.relam
WHERE i.indrelid = ANY($1::int8[]::oid[]) AND NOT i.indisprimary
  AND NOT EXISTS (SELECT 1 FROM pg_constraint c WHERE c.conindid = i.indexrelid AND c.contype IN ('u','p','x'))
  AND NOT EXISTS (SELECT 1 FROM pg_inherits h WHERE h.inhrelid = i.indexrelid)
ORDER BY i.indrelid, ci.relname`
	// One row per key column. attnum 0 in indkey marks an expression key; the
	// column name comes from pg_attribute so quoting is never parsed back.
	qIndexKeys = `SELECT k.n, a.attname::text, pg_get_indexdef(i.indexrelid, k.n, true), oc.opcname::text, oc.opcdefault
FROM pg_index i
CROSS JOIN LATERAL generate_series(1, i.indnkeyatts::int) k(n)
LEFT JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = (string_to_array(i.indkey::text, ' '))[k.n]::int2
LEFT JOIN pg_opclass oc ON oc.oid = (string_to_array(i.indclass::text, ' '))[k.n]::oid
WHERE i.indexrelid = $1 ORDER BY k.n`
)

type rawIndex struct {
	rel              uint32
	oid              uint32
	name, method     string
	unique, valid    bool
	nkeys            int16
	pred             *string
	options, classes string
	reloptions       []string
}

// relIndexError is an index-reading error that remembers which relation's
// index failed, so a caller that read several relations at once can name it.
// Its text is the wrapped error's.
type relIndexError struct {
	rel uint32
	err error
}

func (e *relIndexError) Error() string { return e.err.Error() }
func (e *relIndexError) Unwrap() error { return e.err }

// inspectIndexesOf reads the standalone indexes of several relations with one
// catalog query, keyed by relation oid. Each relation's indexes and invalid
// names come in relname order; an index that is not valid (a failed CREATE
// INDEX CONCURRENTLY) is still listed with the others and also named in the
// invalid map.
func (Dialect) inspectIndexesOf(ctx context.Context, c momo.PinnedConn, relOIDs []uint32) (map[uint32][]schema.Index, map[uint32][]string, error) {
	ids := make([]int64, len(relOIDs))
	for i, o := range relOIDs {
		ids[i] = int64(o)
	}
	var raws []rawIndex
	err := forEach(ctx, c, qIndexes, []any{ids}, func(r momo.Rows) error {
		var x rawIndex
		if err := r.Scan(&x.rel, &x.oid, &x.name, &x.method, &x.unique, &x.valid, &x.nkeys, &x.pred, &x.options, &x.classes, &x.reloptions); err != nil {
			return err
		}
		raws = append(raws, x)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	out := map[uint32][]schema.Index{}
	invalid := map[uint32][]string{}
	for _, x := range raws {
		ix, err := buildIndex(ctx, c, x)
		if err != nil {
			return nil, nil, &relIndexError{rel: x.rel, err: fmt.Errorf("index %s: %w", x.name, err)}
		}
		out[x.rel] = append(out[x.rel], ix)
		if !x.valid {
			invalid[x.rel] = append(invalid[x.rel], x.name)
		}
	}
	return out, invalid, nil
}

func buildIndex(ctx context.Context, c momo.PinnedConn, x rawIndex) (schema.Index, error) {
	ix := schema.Index{Name: x.name, Unique: x.unique}
	if x.method != "btree" {
		ix.Method = x.method
	}
	if x.pred != nil {
		ix.Where = *x.pred
	}
	for _, kv := range x.reloptions {
		k, v, _ := strings.Cut(kv, "=")
		ix.With = append(ix.With, schema.KV{Key: k, Value: v})
	}
	// Known gaps: INCLUDE columns (indnkeyatts excludes them, so they are not
	// read) and NULLS NOT DISTINCT on unique indexes are not inspected, so
	// the schema model cannot declare them yet.
	opts := strings.Fields(x.options)
	err := forEach(ctx, c, qIndexKeys, []any{x.oid}, func(r momo.Rows) error {
		var (
			n            int
			col          *string
			def          string
			opc          *string
			opcIsDefault *bool
		)
		if err := r.Scan(&n, &col, &def, &opc, &opcIsDefault); err != nil {
			return err
		}
		part := schema.IndexPart{}
		if col != nil {
			part.Column = *col
		} else {
			part.Expr = schema.NormalizeExpr(dialect.Postgres, def)
		}
		if n <= len(opts) {
			flags, err := strconv.Atoi(opts[n-1])
			if err != nil {
				return fmt.Errorf("indoption %q: %w", opts[n-1], err)
			}
			desc, nullsFirst := flags&1 != 0, flags&2 != 0
			part.Desc = desc
			// NULLS FIRST is the default for DESC and NULLS LAST for ASC.
			switch {
			case desc && !nullsFirst:
				part.Nulls = "last"
			case !desc && nullsFirst:
				part.Nulls = "first"
			}
		}
		if opc != nil && opcIsDefault != nil && !*opcIsDefault {
			part.OpClass = *opc
		}
		ix.Parts = append(ix.Parts, part)
		return nil
	})
	return ix, err
}
