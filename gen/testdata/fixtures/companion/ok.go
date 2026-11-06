// SPDX-License-Identifier: Apache-2.0

package companion

import (
	"github.com/azrtydxb/momo"
)

// Vector stands in for vector.Vector, which lands with the vectors plan.
type Vector []float32

type Doc struct {
	momo.Model `momo:"table:docs"`
	ID         int64
	Title      string
	Body       string
	Embedding  Vector `momo:"dim:3"`
}

func (d *Doc) MomoConfig(c *momo.Config) {
	c.Index("docs_title_idx", &d.Title).Where("title <> ''").Desc(&d.Title)
	c.Unique("docs_title_key", &d.Title)
	c.IndexExpr("docs_lower_title", "lower(title)")
	c.Check("docs_title_nonempty", "length(title) > 0")
	c.Enum("doc_status", "draft", "published")
	c.Default(&d.Title, "'untitled', 'x'")
	c.Comment(&d.Body, "markdown, with commas")
	c.Index("docs_emb_hnsw", &d.Embedding).Method("hnsw").Opclass(&d.Embedding, "vector_cosine_ops").With("m", "16")
}
