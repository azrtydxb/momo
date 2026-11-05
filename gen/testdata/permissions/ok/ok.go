// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/gen/testdata/permissions/item"
)

var (
	_ momo.Assignment       = item.Name.Set("x")
	_ momo.InsertAssignment = item.Owner.Set("x")
	_ momo.InsertAssignment = item.Serial.Set("x")
	_ momo.Predicate        = item.Code.Eq("x")
)

func main() {}
