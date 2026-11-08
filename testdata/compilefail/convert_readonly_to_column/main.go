// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/azrtydxb/momo"

var ro = momo.NewReadOnlyColumn[string]("documents", "title", 0)

func main() {
	// Converting a read-only column to Column would expose Set.
	_ = momo.Column[string](ro).Set("x")
}
