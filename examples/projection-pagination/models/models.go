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
// The generator does not emit projections yet, so the metadata is written by
// hand. It is short, and the compiler checks every line of it: Ptr is how a
// row is scanned into the struct, Value is only for writes, which a
// projection never does.
type Card struct {
	momo.Projection
	ID    int64
	Title string
	Score int64
}

// MomoMeta describes Card to Momo. Identity, Version, SoftDelete, CreatedAt
// and UpdatedAt are -1 because a projection has none of those columns.
func (*Card) MomoMeta() *momo.Meta[Card] { return &cardMeta }

var cardMeta = momo.Meta[Card]{
	Columns: []momo.ColumnMeta{{Name: "id"}, {Name: "title"}, {Name: "score"}},
	Ptr: func(m *Card, i int) any {
		switch i {
		case 0:
			return &m.ID
		case 1:
			return &m.Title
		default:
			return &m.Score
		}
	},
	Value:    func(*Card, int) any { return nil },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

// ScoreCount is a projection of an aggregate: how many articles have each
// score.
type ScoreCount struct {
	momo.Projection
	Score    int64
	Articles int64
}

// MomoMeta describes ScoreCount to Momo.
func (*ScoreCount) MomoMeta() *momo.Meta[ScoreCount] { return &scoreCountMeta }

var scoreCountMeta = momo.Meta[ScoreCount]{
	Columns: []momo.ColumnMeta{{Name: "score"}, {Name: "articles"}},
	Ptr: func(m *ScoreCount, i int) any {
		if i == 0 {
			return &m.Score
		}
		return &m.Articles
	},
	Value:    func(*ScoreCount, int) any { return nil },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}
