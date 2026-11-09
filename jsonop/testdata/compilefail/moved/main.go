// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/types"
)

var doc = momo.NewColumn[types.JSON[map[string]any]]("events", "after", 0)

func main() {
	var _ momo.JSONTextExpr
	_ = momo.JSONText(doc, "via")
	_ = momo.JSONPath(doc, "a", "b")
	_ = momo.JSONContains(doc, map[string]any{"via": "api"})
	_ = momo.JSONHasKey(doc, "via")
}
