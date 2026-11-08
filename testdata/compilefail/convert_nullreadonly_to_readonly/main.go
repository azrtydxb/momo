// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/azrtydxb/momo"

var nro = momo.NewNullReadOnlyColumn[string]("documents", "title", 0)

func main() {
	// The nullable and non-nullable read-only wrappers are distinct types.
	_ = momo.ReadOnlyColumn[string](nro)
}
