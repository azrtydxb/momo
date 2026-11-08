// SPDX-License-Identifier: Apache-2.0

//go:build cgo && !windows

package sqlitedriver

import (
	"database/sql"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// openVecForTest opens an in-memory database through Open and closes it with
// t.Cleanup. The database is private to the test and never touches disk.
func openVecForTest(t *testing.T) *sql.DB {
	t.Helper()
	db, _, err := Open(":memory:", Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

func TestSQLiteVecLoaded(t *testing.T) {
	db := openVecForTest(t)
	var v string
	if err := db.QueryRow("SELECT vec_version()").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != SQLiteVecVersion {
		t.Fatalf("vec_version() = %q, want %q (embedded by bindings v0.1.6)", v, SQLiteVecVersion)
	}
}

// TestCheckVecVersion: Open accepts the pinned release and later patch releases
// of its minor version, and refuses everything else with a momo error that
// names both versions.
func TestCheckVecVersion(t *testing.T) {
	for _, got := range []string{SQLiteVecVersion, "v0.1.7", "v0.1.10"} {
		if err := checkVecVersion(got); err != nil {
			t.Fatalf("checkVecVersion(%q) = %v, want it accepted", got, err)
		}
	}
	for _, got := range []string{"", "v0.1.5", "v0.2.0", "v0.2.7", "v1.1.6", "0.1.6", "v0.1.6-alpha.1", "v0.1", "v0.1.x"} {
		err := checkVecVersion(got)
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("checkVecVersion(%q) = %v, want ErrUnsupported", got, err)
		}
		if !strings.Contains(err.Error(), SQLiteVecVersion) || !strings.Contains(err.Error(), `"`+got+`"`) {
			t.Fatalf("diagnostic %q names neither %q nor the pinned %s", err, got, SQLiteVecVersion)
		}
	}
}

func TestSQLiteVecMetrics(t *testing.T) {
	db := openVecForTest(t)
	bitBlob := func(b byte) []byte { return []byte{b} }
	cases := []struct {
		name  string
		query string
		args  []any
		want  float64
	}{
		{"l2", "SELECT vec_distance_l2(vec_f32('[1,2,3]'), vec_f32('[4,5,6]'))", nil, math.Sqrt(27)},
		{"l1", "SELECT vec_distance_l1(vec_f32('[1,2,3]'), vec_f32('[4,5,6]'))", nil, 9},
		{"cosine", "SELECT vec_distance_cosine(vec_f32('[1,2,3]'), vec_f32('[4,5,6]'))", nil, 1 - 32/math.Sqrt(14*77)},
		{"hamming", "SELECT vec_distance_hamming(vec_bit(?), vec_bit(?))", []any{bitBlob(0xB0), bitBlob(0x98)}, 2},
	}
	for _, c := range cases {
		var got float64
		if err := db.QueryRow(c.query, c.args...).Scan(&got); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if math.Abs(got-c.want) > 1e-5 {
			t.Fatalf("%s = %v, want %v", c.name, got, c.want)
		}
	}
	// The metrics the dialect lists are exactly the functions that exist: a
	// function the description names but the library lacks, or the reverse,
	// would make the builders render SQL that fails or hide a metric.
	listed := map[string]bool{}
	for _, m := range dialect.For(dialect.SQLite).Vector.Metrics {
		listed[string(m)] = true
	}
	// A function that exists but rejects these arguments (hamming wants bit
	// vectors) fails with another message; only "no such function" is absence.
	exists := func(fn string) bool {
		var f float64
		err := db.QueryRow("SELECT " + fn + "(vec_f32('[1,2]'), vec_f32('[3,4]'))").Scan(&f)
		return err == nil || !strings.Contains(err.Error(), "no such function")
	}
	for _, m := range []string{"l2", "l1", "cosine", "hamming", "ip", "jaccard"} {
		fn := "vec_distance_" + m
		if m == "ip" {
			fn = "vec_distance_dot"
		}
		if got := exists(fn); got != listed[m] {
			t.Errorf("metric %s: function %s present = %v, dialect lists it = %v", m, fn, got, listed[m])
		}
	}
}

// TestSQLiteANNIndexUnsupported: sqlite-vec compares by brute force, so a
// request for an approximate index is refused wherever it is made, and never
// accepted silently.
func TestSQLiteANNIndexUnsupported(t *testing.T) {
	sq := dialect.For(dialect.SQLite)
	tbl := &schema.Table{Name: "docs", Columns: []schema.Column{{Name: "emb", Type: "vector(3)"}}}
	idx := schema.Index{Name: "docs_emb", Method: "hnsw", Parts: []schema.IndexPart{{Column: "emb", OpClass: "vector_l2_ops"}}}
	if err := schema.ValidateVectorIndex(dialect.SQLite, tbl, idx); !errors.Is(err, schema.ErrInvalidVectorIndex) {
		t.Errorf("ValidateVectorIndex(sqlite) = %v, want ErrInvalidVectorIndex", err)
	}
	if len(sq.Vector.Indexes) != 0 {
		t.Errorf("the SQLite dialect lists vector indexes %v", sq.Vector.Indexes)
	}
}
