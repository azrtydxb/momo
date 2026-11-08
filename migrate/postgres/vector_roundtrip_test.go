// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func vectorSchema() *schema.Schema {
	return &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "vector"}},
		Tables: []schema.Table{
			{
				Name:       "docs",
				PrimaryKey: []string{"id"},
				Columns: []schema.Column{
					{Name: "id", Type: "bigint", Identity: "by_default"},
					{Name: "title", Type: "text", Nullable: true},
					{Name: "emb", Type: "vector(1536)"},
					{Name: "half", Type: "halfvec(3072)"},
					{Name: "sp", Type: "sparsevec(1000)"},
					{Name: "bits", Type: "bit(64)"},
				},
				Indexes: []schema.Index{{
					Name:   "docs_emb_hnsw",
					Method: "hnsw",
					Parts:  []schema.IndexPart{{Column: "emb", OpClass: "vector_cosine_ops"}},
					With:   []schema.KV{{Key: "m", Value: "16"}, {Key: "ef_construction", Value: "64"}},
				}},
			},
			{
				Name:       "docs_ivf",
				PrimaryKey: []string{"id"},
				Columns: []schema.Column{
					{Name: "id", Type: "bigint"},
					{Name: "half", Type: "halfvec(64)"},
				},
				Indexes: []schema.Index{{
					Name:   "docs_half_ivf",
					Method: "ivfflat",
					Parts:  []schema.IndexPart{{Column: "half", OpClass: "halfvec_l2_ops"}},
					With:   []schema.KV{{Key: "lists", Value: "100"}},
				}},
			},
		},
	}
}

// TestVectorMigrationRoundTrip applies the vector schema through Diff,
// OrderWithViews, Render and exec (not through the engine), inspects it back,
// requires schema.Equal and requires an empty second diff.
func TestVectorMigrationRoundTrip(t *testing.T) {
	db := testopen.Postgres(t)
	got := roundTrip(t, db, vectorSchema())
	docs, ok := got.Table("docs")
	if !ok {
		t.Fatal("docs missing after round trip")
	}
	wantTypes := map[string]string{"emb": "vector(1536)", "half": "halfvec(3072)", "sp": "sparsevec(1000)", "bits": "bit(64)"}
	for name, typ := range wantTypes {
		c, ok := docs.Column(name)
		if !ok || c.Type != typ {
			t.Errorf("column %s = %+v, want type %s", name, c, typ)
		}
	}
	if len(docs.Indexes) != 1 || docs.Indexes[0].Method != "hnsw" {
		t.Fatalf("hnsw index lost: %+v", docs.Indexes)
	}
}

// TestDimensionlessVectorRoundTrip creates a table whose embedding columns
// are declared without dimensions, with a dims column and a plain index on it
// (the layout of a store whose embedding size follows the model), inspects it
// back as the bare types and requires an empty second diff. roundTrip does the
// diff, apply, inspect, equal and second-diff steps.
func TestDimensionlessVectorRoundTrip(t *testing.T) {
	desired := &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "vector"}},
		Tables: []schema.Table{{
			Name:       "kb_chunks",
			PrimaryKey: []string{"id"},
			Columns: []schema.Column{
				{Name: "id", Type: "bigint", Identity: "by_default"},
				{Name: "dims", Type: "integer"},
				{Name: "embedding", Type: "vector"},
				{Name: "maybe", Type: "vector", Nullable: true},
				{Name: "half", Type: "halfvec"},
			},
			Indexes: []schema.Index{{Name: "kb_chunks_dims", Parts: []schema.IndexPart{{Column: "dims"}}}},
		}},
	}
	db := testopen.Postgres(t)
	got := roundTrip(t, db, desired)
	tbl, ok := got.Table("kb_chunks")
	if !ok {
		t.Fatal("kb_chunks missing after round trip")
	}
	for col, typ := range map[string]string{"embedding": "vector", "maybe": "vector", "half": "halfvec"} {
		c, ok := tbl.Column(col)
		if !ok || c.Type != typ {
			t.Errorf("column %s = %+v, want type %s", col, c, typ)
		}
	}
	// Rows of different sizes share the column.
	mustExec(t, db.Driver(), "INSERT INTO kb_chunks (dims, embedding, half) VALUES (3, '[1,2,3]', '[1,2,3]'), (5, '[1,2,3,4,5]', '[1,2,3,4,5]')")
}

// TestVectorIndexMatrixRoundTrip creates one table per column type and per
// index method and operator class that pgvector accepts, then round-trips
// all of them in one schema.
func TestVectorIndexMatrixRoundTrip(t *testing.T) {
	type combo struct {
		typ, method, opclass string
		with                 []schema.KV
	}
	hnswWith := []schema.KV{{Key: "ef_construction", Value: "64"}, {Key: "m", Value: "16"}}
	ivfWith := []schema.KV{{Key: "lists", Value: "10"}}
	var combos []combo
	for _, f := range []struct{ typ, prefix string }{{"vector(8)", "vector"}, {"halfvec(8)", "halfvec"}} {
		for _, m := range []string{"l2", "ip", "cosine", "l1"} {
			combos = append(combos, combo{f.typ, "hnsw", f.prefix + "_" + m + "_ops", hnswWith})
		}
		for _, m := range []string{"l2", "ip", "cosine"} {
			combos = append(combos, combo{f.typ, "ivfflat", f.prefix + "_" + m + "_ops", ivfWith})
		}
	}
	for _, m := range []string{"l2", "ip", "cosine", "l1"} {
		combos = append(combos, combo{"sparsevec(8)", "hnsw", "sparsevec_" + m + "_ops", hnswWith})
	}
	for _, m := range []string{"hamming", "jaccard"} {
		combos = append(combos, combo{"bit(8)", "hnsw", "bit_" + m + "_ops", hnswWith})
	}
	combos = append(combos, combo{"bit(8)", "ivfflat", "bit_hamming_ops", ivfWith})

	desired := &schema.Schema{Dialect: "postgres", Extensions: []schema.Extension{{Name: "vector"}}}
	for i, c := range combos {
		base := strings.NewReplacer("(", "_", ")", "").Replace(c.typ)
		name := fmt.Sprintf("m%02d_%s_%s", i, base, c.method)
		desired.Tables = append(desired.Tables, schema.Table{
			Name:    name,
			Columns: []schema.Column{{Name: "id", Type: "bigint"}, {Name: "v", Type: c.typ}},
			Indexes: []schema.Index{{
				Name:   name + "_idx",
				Method: c.method,
				Parts:  []schema.IndexPart{{Column: "v", OpClass: c.opclass}},
				With:   c.with,
			}},
		})
	}
	db := testopen.Postgres(t)
	got := roundTrip(t, db, desired)
	want := schema.Normalized(desired)
	for i, c := range combos {
		tbl := desired.Tables[i]
		wt, _ := want.Table(tbl.Name)
		gt, ok := got.Table(tbl.Name)
		if !ok || len(gt.Indexes) != 1 {
			t.Fatalf("%s: table or index missing", tbl.Name)
		}
		ix := gt.Indexes[0]
		// Inspection reports a default operator class as empty, so compare
		// with the normalized declaration (ivfflat on vector, l2).
		if ix.Method != c.method || ix.Parts[0].OpClass != wt.Indexes[0].Parts[0].OpClass {
			t.Errorf("%s: got %s/%s, want %s/%s", tbl.Name, ix.Method, ix.Parts[0].OpClass, c.method, wt.Indexes[0].Parts[0].OpClass)
		}
	}
}

// scratchURL returns raw pointed at database name, without the per-test
// search_path.
func scratchURL(t *testing.T, raw, name string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	q := u.Query()
	q.Del("search_path")
	u.RawQuery = q.Encode()
	return u.String()
}

// TestVectorExtensionMissing applies a migration that declares vector(3) in a
// scratch database that has no pgvector extension, as a role that may not
// create it, and requires the clear unsupported-capability error. The shared
// test database is never touched: dropping the extension there is database
// wide and would break every concurrent test.
//
// The plan connects as the administrator, but an administrator's CREATE
// EXTENSION installs the extension (the package is on the server), so there
// is no failure to classify. The application role cannot install pgvector (it
// is not a trusted extension), which is the real missing-extension failure.
func TestVectorExtensionMissing(t *testing.T) {
	ctx := context.Background()
	adminURL := testdb.PostgresURL(t)
	appURL := testdb.PostgresAppRoleURL(t)
	scratch := testdb.Schema(t) + "_noext"
	admin, err := pgxpool.New(ctx, scratchURL(t, adminURL, "postgres"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	if _, err := admin.Exec(ctx, `CREATE DATABASE "`+scratch+`"`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `DROP DATABASE IF EXISTS "`+scratch+`" WITH (FORCE)`); err != nil {
			t.Error(err)
		}
	})
	role := mustUser(t, appURL)
	adm, err := pgxpool.New(ctx, scratchURL(t, adminURL, scratch))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(adm.Close)
	// The role may create tables (the history table) but not extensions.
	if _, err := adm.Exec(ctx, `GRANT CREATE ON SCHEMA public TO "`+role+`"`); err != nil {
		t.Fatal(err)
	}
	var has bool
	if err := adm.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector')`).Scan(&has); err != nil || has {
		t.Fatalf("scratch database must have no vector extension: %v %v", has, err)
	}
	pool, err := pgxdriver.NewPool(ctx, scratchURL(t, appURL, scratch))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	sch := &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "vector"}},
		Tables: []schema.Table{{
			Name:    "t",
			Columns: []schema.Column{{Name: "emb", Type: "vector(3)"}},
		}},
	}
	eng := migratetest.NewEngine(t, mpg.New(), pgxdriver.New(pool), migratetest.EmptyFS())
	m, err := eng.Diff(ctx, sch, "needs_vector", migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	eng = migratetest.NewEngine(t, mpg.New(), pgxdriver.New(pool), migratetest.FS(m.Files))
	_, err = eng.Apply(ctx, migrate.ApplyOptions{})
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want momo.ErrUnsupported", err)
	}
	if !strings.Contains(err.Error(), `extension "vector" is not installed`) {
		t.Fatalf("err = %v, want the missing-extension message", err)
	}
}

func mustUser(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		t.Fatalf("no user in %q", raw)
	}
	return u.User.Username()
}

// pgErr is a driver error carrying a SQLSTATE.
type pgErr struct{ state, msg string }

func (e pgErr) Error() string    { return e.msg }
func (e pgErr) SQLState() string { return e.state }

// TestClassifyMissingExtension: the failures a missing or uninstallable
// extension produces all become ErrUnsupported naming the extension; other
// errors pass through unchanged.
func TestClassifyMissingExtension(t *testing.T) {
	cases := []struct {
		name string
		err  error
		ext  string // empty: unchanged
	}{
		{"not available", pgErr{"58P01", `extension "vector" is not available`}, "vector"},
		{"permission", pgErr{"42501", `permission denied to create extension "vector"`}, "vector"},
		{"type missing", pgErr{"42704", `type "halfvec" does not exist`}, "vector"},
		{"unrelated type", pgErr{"42704", `type "color" does not exist`}, ""},
		{"unrelated", pgErr{"23505", "duplicate key"}, ""},
	}
	var c momo.ErrorClassifier = mpg.New()
	for _, tc := range cases {
		got := c.ClassifyError(tc.err)
		if tc.ext == "" {
			if got != tc.err {
				t.Errorf("%s: changed to %v", tc.name, got)
			}
			continue
		}
		if !errors.Is(got, momo.ErrUnsupported) || !strings.Contains(got.Error(), `extension "`+tc.ext+`" is not installed`) {
			t.Errorf("%s: got %v", tc.name, got)
		}
		if !errors.Is(got, tc.err) {
			t.Errorf("%s: cause lost: %v", tc.name, got)
		}
	}
}
