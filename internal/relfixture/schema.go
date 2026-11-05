// SPDX-License-Identifier: Apache-2.0

// Package relfixture is the shared relation test fixture: models, descriptors
// (see the momo-06 plan's generator contract) and seeded test databases.
package relfixture

import (
	"context"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// ddl is dialect-neutral: BIGINT, TEXT and BOOLEAN exist on every dialect the
// fixture targets.
var ddl = []string{
	`CREATE TABLE authors (id BIGINT PRIMARY KEY, name TEXT NOT NULL)`,
	`CREATE TABLE documents (id BIGINT PRIMARY KEY, author_id BIGINT REFERENCES authors(id), title TEXT NOT NULL)`,
	`CREATE TABLE comments (id BIGINT PRIMARY KEY, document_id BIGINT NOT NULL REFERENCES documents(id), body TEXT NOT NULL, approved BOOLEAN NOT NULL)`,
	`CREATE TABLE tags (id BIGINT PRIMARY KEY, name TEXT NOT NULL)`,
	`CREATE TABLE taggings (document_id BIGINT NOT NULL, tag_id BIGINT NOT NULL, position BIGINT NOT NULL DEFAULT 0, PRIMARY KEY (document_id, tag_id))`,
	`CREATE TABLE images (id BIGINT PRIMARY KEY, owner_type TEXT NOT NULL, owner_id BIGINT NOT NULL, url TEXT NOT NULL)`,
	`CREATE TABLE employees (id BIGINT PRIMARY KEY, manager_id BIGINT REFERENCES employees(id), name TEXT NOT NULL)`,
	`CREATE TABLE regions (tenant_id BIGINT NOT NULL, code TEXT NOT NULL, name TEXT NOT NULL, PRIMARY KEY (tenant_id, code))`,
	`CREATE TABLE stores (id BIGINT PRIMARY KEY, tenant_id BIGINT NOT NULL, region_code TEXT, name TEXT NOT NULL, FOREIGN KEY (tenant_id, region_code) REFERENCES regions(tenant_id, code))`,
}

// seed rows, in dependency order.
var seed = []struct {
	sql  string
	args []any
}{
	{`INSERT INTO authors (id, name) VALUES (?, ?)`, []any{int64(1), "Ada"}},
	{`INSERT INTO authors (id, name) VALUES (?, ?)`, []any{int64(2), "Grace"}},
	{`INSERT INTO documents (id, author_id, title) VALUES (?, ?, ?)`, []any{int64(1), int64(1), "A1"}},
	{`INSERT INTO documents (id, author_id, title) VALUES (?, ?, ?)`, []any{int64(2), int64(1), "A2"}},
	{`INSERT INTO documents (id, author_id, title) VALUES (?, ?, ?)`, []any{int64(3), int64(2), "G1"}},
	{`INSERT INTO documents (id, author_id, title) VALUES (?, NULL, ?)`, []any{int64(4), "Orphan"}},
	{`INSERT INTO comments (id, document_id, body, approved) VALUES (?, ?, ?, ?)`, []any{int64(1), int64(1), "c1", true}},
	{`INSERT INTO comments (id, document_id, body, approved) VALUES (?, ?, ?, ?)`, []any{int64(2), int64(1), "c2", false}},
	{`INSERT INTO comments (id, document_id, body, approved) VALUES (?, ?, ?, ?)`, []any{int64(3), int64(1), "c3", true}},
	{`INSERT INTO comments (id, document_id, body, approved) VALUES (?, ?, ?, ?)`, []any{int64(4), int64(3), "c4", true}},
	{`INSERT INTO tags (id, name) VALUES (?, ?)`, []any{int64(1), "go"}},
	{`INSERT INTO tags (id, name) VALUES (?, ?)`, []any{int64(2), "db"}},
	{`INSERT INTO taggings (document_id, tag_id, position) VALUES (?, ?, ?)`, []any{int64(1), int64(1), int64(1)}},
	{`INSERT INTO taggings (document_id, tag_id, position) VALUES (?, ?, ?)`, []any{int64(1), int64(2), int64(2)}},
	{`INSERT INTO taggings (document_id, tag_id, position) VALUES (?, ?, ?)`, []any{int64(3), int64(2), int64(1)}},
	{`INSERT INTO images (id, owner_type, owner_id, url) VALUES (?, ?, ?, ?)`, []any{int64(1), "document", int64(1), "u1"}},
	{`INSERT INTO images (id, owner_type, owner_id, url) VALUES (?, ?, ?, ?)`, []any{int64(2), "author", int64(1), "u2"}},
	{`INSERT INTO employees (id, manager_id, name) VALUES (?, NULL, ?)`, []any{int64(1), "boss"}},
	{`INSERT INTO employees (id, manager_id, name) VALUES (?, ?, ?)`, []any{int64(2), int64(1), "dev1"}},
	{`INSERT INTO employees (id, manager_id, name) VALUES (?, ?, ?)`, []any{int64(3), int64(1), "dev2"}},
	{`INSERT INTO regions (tenant_id, code, name) VALUES (?, ?, ?)`, []any{int64(1), "EU", "Europe"}},
	{`INSERT INTO regions (tenant_id, code, name) VALUES (?, ?, ?)`, []any{int64(1), "US", "America"}},
	{`INSERT INTO regions (tenant_id, code, name) VALUES (?, ?, ?)`, []any{int64(2), "EU", "Europe2"}},
	{`INSERT INTO stores (id, tenant_id, region_code, name) VALUES (?, ?, ?, ?)`, []any{int64(1), int64(1), "EU", "Paris"}},
	{`INSERT INTO stores (id, tenant_id, region_code, name) VALUES (?, ?, ?, ?)`, []any{int64(2), int64(1), "US", "NYC"}},
	{`INSERT INTO stores (id, tenant_id, region_code, name) VALUES (?, ?, NULL, ?)`, []any{int64(3), int64(1), "Nowhere"}},
	{`INSERT INTO stores (id, tenant_id, region_code, name) VALUES (?, ?, ?, ?)`, []any{int64(4), int64(2), "EU", "Other"}},
}

// CreateSchema applies the fixture DDL. The database must be empty.
func CreateSchema(t testing.TB, db *momo.DB) {
	t.Helper()
	for _, s := range ddl {
		if _, err := momo.Exec(context.Background(), db, s); err != nil {
			t.Fatalf("ddl %q: %v", s, err)
		}
	}
}

// SeedBasic inserts the deterministic rows that the relation tests assert on:
// authors 1 (Ada) and 2 (Grace); documents 1,2 by Ada, 3 by Grace, 4 with a
// NULL author; comments 1-3 on document 1 and comment 4 on document 3;
// tags go and db; taggings (1,1,pos1), (1,2,pos2), (3,2,pos1); images owned by
// document 1 and author 1; employees boss(1), dev1(2) and dev2(3) reporting to
// boss; regions (1,EU),(1,US),(2,EU); stores 1 (1,EU), 2 (1,US), 3 (1,NULL),
// 4 (2,EU).
func SeedBasic(t testing.TB, db *momo.DB) {
	t.Helper()
	for _, s := range seed {
		if _, err := momo.Exec(context.Background(), db, s.sql, s.args...); err != nil {
			t.Fatalf("seed %q: %v", s.sql, err)
		}
	}
}

// Hook counts statements. Each subtest gets fresh counters. Relation events
// join it when the relation hook lands (momo-06 relhook).
type Hook struct {
	After []string // QueryEvent.Relation of each statement ("" when not a relation load)
}

// BeforeQuery implements momo.Hook.
func (h *Hook) BeforeQuery(ctx context.Context, _ *momo.QueryEvent) context.Context { return ctx }

// AfterQuery implements momo.Hook.
func (h *Hook) AfterQuery(_ context.Context, e *momo.QueryEvent) {
	h.After = append(h.After, e.Relation)
}

// Each runs fn once per dialect available in this build (testopen.Each), on a
// fresh database with the fixture schema applied. The hook is attached to every
// database; its counters are reset for each run and after the schema is built.
func Each(t *testing.T, fn func(t *testing.T, db *momo.DB, h *Hook)) {
	t.Helper()
	h := &Hook{}
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		CreateSchema(t, db)
		h.After = nil
		fn(t, db, h)
	}, momo.WithHook(h))
}

// PostgresDriver returns a pgx-backed driver on a fresh schema from
// testdb.PostgresURL, for tests that need the driver itself (round-trip
// counting). The pool is closed by t.Cleanup.
func PostgresDriver(t testing.TB) momo.Driver {
	t.Helper()
	url := testdb.PostgresURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxdriver.NewPool(ctx, url)
	if err != nil {
		t.Fatalf("relfixture: open postgres pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pgxdriver.New(pool)
}
