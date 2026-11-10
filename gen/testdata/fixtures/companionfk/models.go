// SPDX-License-Identifier: Apache-2.0

// Package companionfk declares composite foreign keys with Config.ForeignKey.
package companionfk

import "github.com/azrtydxb/momo"

type Document struct {
	momo.Model `momo:"table:document"`
	ID         int64 `momo:"pk"`
	CompanyID  int64
	TemplateID int64
	VersionNo  int64
}

func (d *Document) MomoConfig(c *momo.Config) {
	c.Unique("document_company_id_id_key", &d.CompanyID, &d.ID)
	c.Unique("document_tpl_key", &d.CompanyID, &d.TemplateID, &d.VersionNo)
}

type Note struct {
	momo.Model `momo:"table:note"`
	ID         int64 `momo:"pk"`
	CompanyID  int64
	DocumentID int64
}

func (n *Note) MomoConfig(c *momo.Config) {
	c.ForeignKey("note_document_fk", []any{&n.CompanyID, &n.DocumentID}, "document", []string{"company_id", "id"}).OnDelete("CASCADE").OnUpdate("restrict")
}

type Revision struct {
	momo.Model `momo:"table:revision"`
	ID         int64 `momo:"pk"`
	CompanyID  int64
	TemplateID int64
	VersionNo  int64
}

func (r *Revision) MomoConfig(c *momo.Config) {
	c.ForeignKey("revision_tpl_fk", []any{&r.CompanyID, &r.TemplateID, &r.VersionNo}, "document", []string{"company_id", "template_id", "version_no"})
}
