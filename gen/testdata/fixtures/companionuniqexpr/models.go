// SPDX-License-Identifier: Apache-2.0

// Package companionuniqexpr declares unique expression indexes.
package companionuniqexpr

import "github.com/azrtydxb/momo"

type Principal struct {
	momo.Model `momo:"table:principal"`
	ID         int64 `momo:"pk"`
	Email      string
}

func (p *Principal) MomoConfig(c *momo.Config) {
	c.IndexExpr("principal_email_key", "lower(email)").Unique()
}

type Template struct {
	momo.Model `momo:"table:template"`
	ID         int64 `momo:"pk"`
	CompanyID  int64
	Name       string
	DeletedAt  *int64
}

func (t *Template) MomoConfig(c *momo.Config) {
	c.IndexExpr("template_company_name_key", "company_id", "lower(name)").Unique()
	c.IndexExpr("template_live_name_key", "lower(name)").Unique().Where("deleted_at IS NULL")
	c.IndexExpr("template_plain_expr_idx", "lower(name)")
}
