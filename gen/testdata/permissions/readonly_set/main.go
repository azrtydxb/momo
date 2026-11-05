// SPDX-License-Identifier: Apache-2.0

// compile-fail fixture: must not build
package main

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/gen/testdata/permissions/item"
)

var _ momo.Assignment = item.Code.Set("x")

func main() {}
