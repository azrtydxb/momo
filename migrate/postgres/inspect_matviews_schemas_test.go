// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"slices"
	"testing"

	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// TestPostgresInspectsMatviewsAcrossSchemas reads materialized views of two
// schemas in one pass: dependencies across schemas keep their (schema, name)
// order and spelling, and an invalid index stays with the one view it belongs
// to without disturbing the others.
func TestPostgresInspectsMatviewsAcrossSchemas(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	d := db.Driver()
	cur := testdb.Schema(t)
	aux := cur + "_aux"
	mustExec(t, d, `CREATE SCHEMA `+aux)
	t.Cleanup(func() { _, _ = d.Exec(context.Background(), `DROP SCHEMA `+aux+` CASCADE`, nil) })
	mustExec(t, d, `CREATE TABLE base (id bigint PRIMARY KEY, v int NOT NULL)`)
	mustExec(t, d, `CREATE TABLE `+aux+`.src (id bigint PRIMARY KEY, v int NOT NULL)`)
	mustExec(t, d, `INSERT INTO base VALUES (1, 7), (2, 7)`)
	mustExec(t, d, `INSERT INTO `+aux+`.src VALUES (1, 7), (2, 7)`)
	mustExec(t, d, `CREATE MATERIALIZED VIEW mv_a AS SELECT v, count(*) AS n FROM base GROUP BY v`)
	mustExec(t, d, `CREATE INDEX mv_a_ok ON mv_a (v)`)
	mustExec(t, d, `CREATE MATERIALIZED VIEW `+aux+`.mv_b AS SELECT b.v FROM base b JOIN `+aux+`.src s ON s.id = b.id`)
	mustExec(t, d, `CREATE INDEX mv_b_ok ON `+aux+`.mv_b (v)`)
	mustExec(t, d, `CREATE MATERIALIZED VIEW mv_c AS SELECT v FROM `+aux+`.src`)
	mustExec(t, d, `CREATE INDEX mv_c_ok ON mv_c (v)`)
	// A CONCURRENTLY build over duplicate data fails and leaves an invalid index.
	if _, err := d.Exec(ctx, `CREATE UNIQUE INDEX CONCURRENTLY mv_c_v_key ON mv_c (v)`, nil); err == nil {
		t.Fatal("expected the unique build to fail")
	}
	got, err := inspectOnce(ctx, t, db, migrate.InspectOptions{Schemas: []string{cur, aux}})
	if err != nil {
		t.Fatal(err)
	}
	type key struct{ schema, name string }
	byKey := map[key]schema.View{}
	var order []key
	for _, v := range got.Views {
		k := key{v.Schema, v.Name}
		byKey[k] = v
		order = append(order, k)
	}
	if want := []key{{"", "mv_a"}, {"", "mv_c"}, {aux, "mv_b"}}; !slices.Equal(order, want) {
		t.Fatalf("view order = %v, want %v", order, want)
	}
	for k, want := range map[key][]string{
		{"", "mv_a"}:  {"base"},
		{"", "mv_c"}:  {aux + ".src"},
		{aux, "mv_b"}: {"base", aux + ".src"},
	} {
		if g := byKey[k].Depends; !slices.Equal(g, want) {
			t.Errorf("%v depends = %v, want %v", k, g, want)
		}
	}
	idx := func(k key) []string {
		var out []string
		for _, ix := range byKey[k].Indexes {
			out = append(out, ix.Name)
		}
		return out
	}
	for k, want := range map[key][]string{
		{"", "mv_a"}:  {"mv_a_ok"},
		{"", "mv_c"}:  {"mv_c_ok", "mv_c_v_key"},
		{aux, "mv_b"}: {"mv_b_ok"},
	} {
		if g := idx(k); !slices.Equal(g, want) {
			t.Errorf("%v indexes = %v, want %v", k, g, want)
		}
	}
	for _, ix := range byKey[key{"", "mv_c"}].Indexes {
		if ix.Name == "mv_c_v_key" && !ix.Unique {
			t.Errorf("mv_c_v_key is not reported unique: %+v", ix)
		}
	}
}
