// SPDX-License-Identifier: Apache-2.0

// Package tagvocab exercises the full field tag vocabulary: every tag that
// changes the schema appears at least once.
package tagvocab

import (
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/vector"
)

type Org struct {
	momo.Model `momo:"table:orgs"`
	ID         int64  `momo:"pk,identity"`
	Name       string `momo:"notnull,size:80,unique,collate:C,comment:the org name"`
}

type Article struct {
	momo.Model `momo:"table:articles"`
	ID         int64               `momo:"pk,identity"`
	OrgID      int64               `momo:"column:org_id,notnull,index"`
	Title      string              `momo:"notnull,size:200,index:articles_title_sorted,sort:desc,nulls:last"`
	Slug       string              `momo:"unique:articles_slug_uniq,default:'untitled'"`
	Summary    *string             `momo:"comment:short summary"`
	Notes      string              `momo:"nullable,type:text"`
	Price      float64             `momo:"notnull,precision:10,scale:2"`
	Rating     int32               `momo:"check:rating BETWEEN 0 AND 5,default:0"`
	State      string              `momo:"enum:article_state,default:'draft'"`
	Embedding  vector.Vector       `momo:"dim:3,nullable"`
	Half       vector.HalfVector   `momo:"dim:4,nullable"`
	Sparse     vector.SparseVector `momo:"dim:5,nullable"`
	Flags      vector.Bits         `momo:"dim:8,nullable"`
	CreatedAt  time.Time           `momo:"created_at,notnull,default:now()"`
	Tags       []Tag               `momo:"many2many,join:article_tags"`

	Org *Org `momo:"belongs_to,fk:OrgID,on_delete:cascade,on_update:restrict"`
}

type Tag struct {
	momo.Model `momo:"table:tags"`
	ID         int64  `momo:"pk,identity"`
	Label      string `momo:"notnull,unique"`
}

func (a *Article) MomoConfig(c *momo.Config) {
	c.Enum("article_state", "draft", "live")
	c.Index("articles_emb_hnsw", &a.Embedding).Method("hnsw").Opclass(&a.Embedding, "vector_cosine_ops").With("m", "16")
}
