// SPDX-License-Identifier: Apache-2.0

package momotest

import "github.com/azrtydxb/momo"

// MomoOptionsOf returns the options that WithMomoOptions collected from opts,
// in order. It is for helper packages such as sqlitetest that run momotest
// options against another database: they pass these to momo.New and ignore the
// PostgreSQL-only options. It needs no server.
func MomoOptionsOf(opts ...Option) []momo.Option {
	return newConfig(opts).momoOpts
}
