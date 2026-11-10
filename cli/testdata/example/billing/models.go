// SPDX-License-Identifier: Apache-2.0

// Package billing is a nested model package for go generate tests.
package billing

import "github.com/azrtydxb/momo"

//go:generate go run github.com/azrtydxb/momo/cmd/momo generate

// Invoice is a billing model.
type Invoice struct {
	momo.Model `momo:"table:invoices"`
	ID         int64 `momo:"pk,identity"`
	Total      int64
}
