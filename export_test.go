// SPDX-License-Identifier: Apache-2.0

package momo

// ScopeThen is a test hook that combines per-load options with nested loads,
// which the public Scope and Then cannot express together.
func ScopeThen[P, C any](r Relation[P, C], opts []LoadOption[C], nested ...Loader[C]) Loader[P] {
	return r.scoped(opts, nested)
}
