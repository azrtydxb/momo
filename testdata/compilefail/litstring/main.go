// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/azrtydxb/momo"

func main() {
	// Lit accepts numbers, bool and time.Time only: a string could carry SQL.
	_ = momo.Lit("x")
}
