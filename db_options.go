// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"fmt"
	"slices"
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
