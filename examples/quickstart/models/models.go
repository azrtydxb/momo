// SPDX-License-Identifier: Apache-2.0

// Package models is the schema of the quickstart: authors and their posts,
// each post carrying an embedding for similarity search.
package models

import (
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/vector"
)

// Author writes posts.
type Author struct {
	momo.Model `momo:"table:authors"`
	ID         int64  `momo:"pk,identity"`
	Name       string `momo:"size:200,notnull,unique"`

	Posts []Post `momo:"has_many,fk:AuthorID"`
}

// Post is one article with a 3-dimensional embedding. Real embeddings are
// larger: change dim to the output size of your model, for example 1536.
type Post struct {
	momo.Model `momo:"table:posts"`
	ID         int64         `momo:"pk,identity"`
	AuthorID   int64         `momo:"column:author_id,notnull,index"`
	Title      string        `momo:"size:300,notnull"`
	Embedding  vector.Vector `momo:"dim:3"`
	CreatedAt  time.Time     `momo:"column:created_at,read_only,default:now()"`

	Author *Author `momo:"belongs_to,fk:AuthorID"`
}

// MomoConfig declares what struct tags cannot express. pgvector's hnsw
// method has no default operator class, so name it. The generator reads this
// method statically; it never runs.
func (p *Post) MomoConfig(c *momo.Config) {
	c.Index("posts_embedding_hnsw", &p.Embedding).
		Method("hnsw").
		Opclass(&p.Embedding, "vector_cosine_ops")
}
