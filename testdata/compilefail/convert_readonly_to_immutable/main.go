// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/azrtydxb/momo"

var ro = momo.NewReadOnlyColumn[string]("documents", "title", 0)

func main() {
	// Converting a read-only column to ImmutableColumn would make it insertable.
	_ = momo.ImmutableColumn[string](ro)
}
