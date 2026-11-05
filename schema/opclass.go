// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"slices"
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

// defaultOpClasses lists the PostgreSQL default operator class per access
// method and normalized column type (parameters such as vector(3) removed).
// Inspection leaves OpClass empty for a default class, so normalization does
// the same for a declared one. Verified against pg_opclass.opcdefault.
// pgvector's hnsw has no default class, and ivfflat has one only for vector,
// so those other combinations always keep their declared class. Types not
// listed keep a declared class, which can only produce a reported difference,
// never hide one.
var defaultOpClasses = map[string]map[string]string{
	"btree": {
		"boolean": "bool_ops", "smallint": "int2_ops", "integer": "int4_ops", "bigint": "int8_ops",
		"text": "text_ops", "uuid": "uuid_ops", "jsonb": "jsonb_ops",
		"timestamp with time zone": "timestamptz_ops", "timestamp without time zone": "timestamp_ops",
		"date": "date_ops", "numeric": "numeric_ops",
	},
	"gin":     {"anyarray": "array_ops", "jsonb": "jsonb_ops", "tsvector": "tsvector_ops"},
	"ivfflat": {"vector": "vector_l2_ops"},
}

// normalizeIndexes rewrites indexes to the form inspection reports: btree is
// the empty method, default operator classes are empty, storage parameters
// are sorted by key and expressions are canonical. cols are the columns the
// index parts refer to (nil for a materialized view, which keeps declared
// classes).
func normalizeIndexes(d dialect.Name, cols []Column, ixs []Index) {
	for i := range ixs {
		ix := &ixs[i]
		ix.Concurrently = false
		ix.Where = NormalizeExpr(d, ix.Where)
		if d != dialect.Postgres {
			for k := range ix.Parts {
				ix.Parts[k].Expr = NormalizeExpr(d, ix.Parts[k].Expr)
			}
			continue
		}
		if ix.Method == "btree" {
			ix.Method = ""
		}
		slices.SortStableFunc(ix.With, func(a, b KV) int { return strings.Compare(a.Key, b.Key) })
		method := ix.Method
		if method == "" {
			method = "btree"
		}
		for k := range ix.Parts {
			p := &ix.Parts[k]
			p.Expr = NormalizeExpr(d, p.Expr)
			if p.OpClass == "" || p.Column == "" {
				continue
			}
			ci := slices.IndexFunc(cols, func(c Column) bool { return c.Name == p.Column })
			if ci < 0 {
				continue
			}
			if p.OpClass == defaultOpClasses[method][opClassType(cols[ci].Type)] {
				p.OpClass = ""
			}
		}
	}
}

// opClassType maps a column type to its key in defaultOpClasses: the
// normalized base type with parameters removed, or "anyarray" for any array.
func opClassType(typ string) string {
	t := NormalizeType(dialect.Postgres, typ)
	if strings.HasSuffix(t, "[]") {
		return "anyarray"
	}
	if i := strings.IndexByte(t, '('); i >= 0 {
		t = strings.TrimSpace(t[:i]) + t[strings.IndexByte(t, ')')+1:]
	}
	return t
}
