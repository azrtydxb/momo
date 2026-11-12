// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"fmt"
	"slices"
	"sync"
)

// WithMaxParams lowers the dialect bind-parameter limit for this DB to n. It
// is for tests and for drivers or proxies with tighter limits than the
// dialect default.
//
// n must be positive: zero or a negative n would read as "no limit" in every
// guard, so WithMaxParams panics at New with a message naming the value (a
// programmer error, like a bad regexp.MustCompile). An n above the dialect's
// own limit is clamped to it, because the wire protocol rejects more
// parameters than that; a dialect that declares no limit takes n as given.
//
// The limit applies to everything the DB runs: relation-load chunking
// (Load, LoadAll), bulk-insert chunking, raw SQL bind checks including In,
// transactions begun from the DB (Tx), and handles from OnConn. The dialect
// value is copied; the shared dialect is never mutated.
func WithMaxParams(n int) Option {
	return func(db *DB) {
		if n <= 0 {
			panic(fmt.Sprintf("momo: WithMaxParams(%d): the limit must be positive", n))
		}
		d := *db.ex.d
		if d.MaxParams > 0 && n > d.MaxParams {
			n = d.MaxParams
		}
		d.MaxParams = n
		// The struct copy is shallow; clone the slices so a caller that edits
		// this DB's dialect cannot corrupt the process-wide one.
		d.Vector.Metrics = slices.Clone(d.Vector.Metrics)
		d.Vector.Indexes = slices.Clone(d.Vector.Indexes)
		db.ex.d = &d
	}
}

// FieldNamer names the column of a Go struct field. Every preset of
// gen/naming (naming.Momo{}, naming.Prisma{}, ...) is one.
type FieldNamer interface {
	// ColumnName returns the column name for a Go field name.
	ColumnName(goField string) string
}

// WithNaming sets the naming preset that Query and QueryRows apply to the
// fields of a plain struct, the ones with no generated metadata. A field with
// no `momo:"column:x"` tag is then matched to the column the preset gives its
// name, so under naming.Prisma{} the field NextAttemptAt reads the column
// nextAttemptAt. Use the same preset as the naming: key of momo.yaml. Without
// the option, or with a nil n, the column is the snake_case of the field name
// (naming.ToSnake). Models, projections and tuple rows carry the names the
// generator derived, so they are not affected.
//
// The preset may be any value, comparable or not, a preset or your own type,
// and it is called once per field when a type is first scanned. It must be
// stable: the plan of a type is built once and kept for the life of the DB (a
// DB with a preset keeps its own plans, so a preset built per request, with a
// DB per request, costs a plan per type and DB and is freed with the DB).
// momo reads no preset from momo.yaml or from a generated registration: this
// option is the only way to set one.
func WithNaming(n FieldNamer) Option {
	return func(db *DB) {
		db.naming = n
		db.namingPlans = nil
		if n != nil {
			db.namingPlans = new(sync.Map)
		}
	}
}
