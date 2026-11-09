// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"fmt"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testmodels/example"
	"github.com/azrtydxb/momo/vecop"
	"github.com/azrtydxb/momo/vector"
)

// Bit vectors take the Hamming and Jaccard distances.
func Example_vectorHamming() {
	db := momo.New(pgxdriver.New(nil))
	col := momo.NewVectorColumn[vector.Bits]("documents", "bits", 8, 0)

	expr := vecop.Hamming(col, vector.Bits{Len: 8, Data: []byte{0xF0}})
	c, err := momo.Select[example.Document](db).
		OrderBy(expr.Asc()).Limit(3).SQL()
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(strings.Contains(c.SQL, `("documents"."bits" <~> $1) ASC`))
	// Output:
	// true
}
