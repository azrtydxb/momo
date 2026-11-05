// SPDX-License-Identifier: Apache-2.0

package momo

// WithMaxParams lowers the dialect bind-parameter limit for this DB. It is
// for tests and for drivers or proxies with tighter limits than the dialect
// default. The dialect value is copied; the shared dialect is never mutated.
// The limit applies to relation-load chunking, bulk-insert chunking and the
// raw-SQL bind checks, including inside transactions begun from the DB.
func WithMaxParams(n int) Option {
	return func(db *DB) {
		d := *db.ex.d
		d.MaxParams = n
		db.ex.d = &d
	}
}
