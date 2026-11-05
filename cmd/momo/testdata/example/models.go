// SPDX-License-Identifier: Apache-2.0

// Package example is a small model package the CLI tests copy into a temporary
// module and generate code for.
package example

import "github.com/azrtydxb/momo"

//go:generate go run github.com/azrtydxb/momo/cmd/momo generate

// Tenant owns documents.
type Tenant struct {
	momo.Model `momo:"table:tenants"`
	ID         int64  `momo:"pk,identity"`
	Name       string `momo:"size:200,notnull,unique"`
}

// Document is the main model.
type Document struct {
	momo.Model `momo:"table:documents"`
	ID         int64     `momo:"pk,identity"`
	TenantID   int64     `momo:"column:tenant_id,notnull,index"`
	Title      string    `momo:"size:300,notnull"`
	Body       string    `momo:"type:text"`
	Comments   []Comment `momo:"has_many,fk:DocumentID"`
	Tenant     *Tenant   `momo:"belongs_to,fk:TenantID"`
}

// MomoConfig declares the complex configuration.
func (d *Document) MomoConfig(c *momo.Config) {
	c.Enum("document_status", "draft", "published", "archived")
	c.Check("documents_title_nonempty", "length(title) > 0")
	c.Comment(&d.Body, "Markdown body, with commas")
}

// Comment belongs to a document.
type Comment struct {
	momo.Model `momo:"table:comments"`
	ID         int64 `momo:"pk,identity"`
	DocumentID int64 `momo:"index"`
	Body       string
}
