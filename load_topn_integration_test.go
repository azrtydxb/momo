// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// A duplicated join row must not consume the per-parent limit: post 1 has tag
// 1 twice, and the top-2 must still be two distinct tags. Soft-deleted far
// rows are excluded before ranking.
func TestTopNManyToManyDuplicateJoinRows(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		ctx := context.Background()
		for _, s := range []string{
			"DROP TABLE IF EXISTS mm_post_tags",
			"DROP TABLE IF EXISTS mm_tags",
			"DROP TABLE IF EXISTS mm_posts",
			"CREATE TABLE mm_posts (id BIGINT PRIMARY KEY)",
			"CREATE TABLE mm_tags (id BIGINT PRIMARY KEY, name TEXT NOT NULL, deleted_at TIMESTAMP NULL)",
			"CREATE TABLE mm_post_tags (post_id BIGINT NOT NULL, tag_id BIGINT NOT NULL)",
			"INSERT INTO mm_posts (id) VALUES (1), (2)",
			"INSERT INTO mm_tags (id, name, deleted_at) VALUES (1, 'a', NULL), (2, 'b', NULL), (3, 'c', NULL), (4, 'gone', CURRENT_TIMESTAMP)",
			"INSERT INTO mm_post_tags (post_id, tag_id) VALUES (1, 1), (1, 1), (1, 2), (1, 3), (1, 4), (2, 3), (2, 3), (2, 2)",
		} {
			mustExec(t, db, s)
		}
		t.Cleanup(func() {
			for _, s := range []string{"mm_post_tags", "mm_tags", "mm_posts"} {
				_, _ = momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS "+s)
			}
		})
		ps := []mmPost{{ID: 1}, {ID: 2}}
		if err := momo.Load(ctx, db, ps, mmTags, momo.LoadPerParentLimit[mmTag](2)); err != nil {
			t.Fatal(err)
		}
		if got := mmNames(ps[0]) + " " + mmNames(ps[1]); got != "[a b] [b c]" {
			t.Fatalf("top-2 distinct tags per post: %s", got)
		}
	})
}
