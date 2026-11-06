// SPDX-License-Identifier: Apache-2.0

// Package models is the schema of the projection-pagination example: a table
// of articles, plus two small result types that read only some of its
// columns.
package models

import "github.com/azrtydxb/momo"

// Article is a row of the articles table. Body is large, which is why the
// list views below do not select it.
type Article struct {
	momo.Model `momo:"table:articles"`
	ID         int64  `momo:"pk,identity"`
	Title      string `momo:"size:200,notnull"`
	Body       string `momo:"notnull"`
	Score      int64  `momo:"notnull"`
}

// MomoConfig declares the index that serves the keyset order of the example,
// score descending then id ascending. A keyset query is only as fast as an
// index that matches its ORDER BY. The generator reads this method
// statically; it never runs.
func (a *Article) MomoConfig(c *momo.Config) {
	c.Index("articles_score_id_idx", &a.Score, &a.ID).Desc(&a.Score)
}

// Card is a projection: the columns a list page needs, nothing else. A
// projection embeds momo.Projection and has no table of its own; the query
// names the table (From) and the columns (Columns), in field order.
//
// The generator writes its MomoMeta into momo_gen.go, so there is nothing to
// maintain by hand. A field maps to the column its name gives under the
// naming rule, or to the column of a `momo:"column:..."` tag.
type Card struct {
	momo.Projection
	ID    int64
	Title string
	Score int64
}

// ScoreCount is a projection of an aggregate: how many articles have each
// score.
type ScoreCount struct {
	momo.Projection
	Score    int64
	Articles int64
}
