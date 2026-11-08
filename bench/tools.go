// SPDX-License-Identifier: Apache-2.0

//go:build tools

package bench

// Tool dependencies pinned in go.mod: the Momo generator and the Ent code
// generator, both run by make bench-generate.
import (
	_ "entgo.io/ent/cmd/ent"
	_ "github.com/azrtydxb/momo/cmd/momo"
)
