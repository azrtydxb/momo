// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/sqlfn"
)

var published = momo.NewColumn[bool]("documents", "published", 0)

func main() {
	// A string CASE cannot yield an int branch value.
	_ = sqlfn.Case[string]().When(published.Eq(true), 42)
}
