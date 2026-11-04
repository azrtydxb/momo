// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/azrtydxb/momo"

var title = momo.NewColumn[string]("documents", "title", 0)

func f(v any) momo.Predicate {
	return title.Eq(v)
}

func main() { _ = f }
