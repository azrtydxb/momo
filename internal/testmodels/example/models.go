// SPDX-License-Identifier: Apache-2.0

// Package example is the reference schema for momo: a multi-tenant document
// store with comments, tags and a pgvector embedding. Copy it as a starting
// point. Run `go generate ./...` after changing a model; `momo generate
// --check` fails CI when the committed output is stale.
package example

import (
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/vector"
)

// Tenant owns documents.
type Tenant struct {
	momo.Model `momo:"table:tenants"`
	ID         int64  `momo:"pk,identity"`
	Name       string `momo:"size:200,notnull,unique"`
}

// Document is a tenant's document with a semantic-search embedding.
type Document struct {
	momo.Model `momo:"table:documents"`
	ID         int64         `momo:"pk,identity"`
	TenantID   int64         `momo:"column:tenant_id,notnull,index"`
	Title      string        `momo:"size:300,notnull"`
	Body       string        `momo:"type:text"`
	Embedding  vector.Vector `momo:"dim:1536"`
	Status     string        `momo:"column:status,enum:document_status,default:'draft'"`
	CreatedAt  time.Time     `momo:"column:created_at,read_only,default:now()"`

	Comments []Comment `momo:"has_many,fk:DocumentID"`
	Tags     []Tag     `momo:"many2many,join:document_tags"`
	Tenant   *Tenant   `momo:"belongs_to,fk:TenantID"`
}

// MomoConfig declares what struct tags cannot express: the hnsw index with its
// operator class (hnsw has no default), the status enum, a check and a comment.
// The generator reads it statically; it never runs.
func (d *Document) MomoConfig(c *momo.Config) {
	c.Index("documents_embedding_hnsw", &d.Embedding).
		Method("hnsw").
		Opclass(&d.Embedding, "vector_cosine_ops").
		With("m", "16").
		With("ef_construction", "64")
	c.Enum("document_status", "draft", "published", "archived")
	c.Check("documents_title_nonempty", "length(title) > 0")
	c.Comment(&d.Body, "Markdown body, with commas")
}

// Comment is a remark on a document.
type Comment struct {
	momo.Model `momo:"table:comments"`
	ID         int64 `momo:"pk,identity"`
	DocumentID int64 `momo:"index"`
	Body       string
}

// Tag labels documents through the document_tags join table.
type Tag struct {
	momo.Model `momo:"table:tags"`
	ID         int64  `momo:"pk,identity"`
	Name       string `momo:"unique"`
}
