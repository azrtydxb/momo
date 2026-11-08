// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/azrtydxb/momo"

var im = momo.NewImmutableColumn[string]("documents", "title", 0)

func main() {
	// Converting an immutable column to Column would make it updatable.
	_ = momo.Column[string](im).Set("x")
}
