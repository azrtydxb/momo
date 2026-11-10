// SPDX-License-Identifier: Apache-2.0

// Package companionfkbad holds composite foreign keys the builder refuses.
package companionfkbad

import "github.com/azrtydxb/momo"

type Parent struct {
	momo.Model `momo:"table:parent"`
	ID         int64 `momo:"pk"`
	CompanyID  int64
	Code       string
}

type Child struct {
	momo.Model `momo:"table:child"`
	ID         int64 `momo:"pk"`
	CompanyID  int64
	ParentID   int64
}

func (c *Child) MomoConfig(cfg *momo.Config) {
	// (company_id, id) is not unique on parent: id alone is the primary key.
	cfg.ForeignKey("child_not_unique_fk", []any{&c.CompanyID, &c.ParentID}, "parent", []string{"company_id", "id"})
	cfg.ForeignKey("child_ghost_fk", []any{&c.CompanyID, &c.ParentID}, "ghost", []string{"company_id", "id"})
	cfg.ForeignKey("child_col_fk", []any{&c.CompanyID, &c.ParentID}, "parent", []string{"company_id", "nope"})
}
