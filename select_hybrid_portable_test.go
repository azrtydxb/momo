// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/schema"
	"github.com/azrtydxb/momo/vector"
)

type searchDoc = hybridDoc

func init() {
	hybridDocMeta.TableSchema = &schema.Table{
		Name:       "docs",
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "docs_search", Method: "fulltext",
			Parts:    []schema.IndexPart{{Column: "body"}},
			FullText: &schema.FullText{Columns: []string{"body"}}}},
	}
}

func mariaDBFake() *fakedriver.Driver {
	return fakedriver.NewDialect(dialect.MySQLFlavor(dialect.FlavorMariaDB))
}

// Catches: the SQL shape or argument order of either portable statement
// changing, and the PostgreSQL statement being touched.
func TestHybridSearchGoldenPortable(t *testing.T) {
	spec := hybridSpec()
	spec.TextConfig = "" // ignored off PostgreSQL
	spec.Query = "vector databases"
	sq, err := momo.HybridSearch[searchDoc](momo.New(fakedriver.New(dialect.SQLite)), spec).SQL()
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{
		`"docs_fts" MATCH ?`,
		`bm25("docs_fts")`,
		`vec_distance_cosine(t."emb", ?)`,
		`UNION ALL`,
		`GROUP BY pk`,
		`ORDER BY f.score DESC, f.pk ASC`,
	} {
		if !strings.Contains(sq.SQL, frag) {
			t.Errorf("sqlite statement lacks %q:\n%s", frag, sq.SQL)
		}
	}
	// FTS5 query, text candidates, query vector, vector candidates, RRFK, RRFK, limit.
	if len(sq.Args) != 7 || sq.Args[0] != `"vector" "databases"` || sq.Args[1] != 100 || sq.Args[3] != 100 || sq.Args[4] != 60 || sq.Args[5] != 60 || sq.Args[6] != 10 {
		t.Errorf("sqlite args = %#v", sq.Args)
	}
	mq, err := momo.HybridSearch[searchDoc](momo.New(mariaDBFake()), spec).SQL()
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{
		"MATCH(`body`) AGAINST (? IN BOOLEAN MODE)",
		"VEC_DISTANCE_COSINE(t.`emb`, VEC_FromText(?))",
		"UNION ALL",
	} {
		if !strings.Contains(mq.SQL, frag) {
			t.Errorf("mariadb statement lacks %q:\n%s", frag, mq.SQL)
		}
	}
	if !reflect.DeepEqual(mq.Args[0], "+vector +databases") || !reflect.DeepEqual(mq.Args[1], "+vector +databases") {
		t.Errorf("mariadb text arguments = %#v", mq.Args[:2])
	}
}

// Catches: a query without a positive word sending a MATCH or binding the query
// text, and the empty leg changing the argument list of one dialect only.
func TestHybridSearchEmptyTextLegGolden(t *testing.T) {
	spec := hybridSpec()
	spec.Query = "-vector"
	sq, err := momo.HybridSearch[searchDoc](momo.New(fakedriver.New(dialect.SQLite)), spec).SQL()
	if err != nil {
		t.Fatal(err)
	}
	mq, err := momo.HybridSearch[searchDoc](momo.New(mariaDBFake()), spec).SQL()
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]momo.Compiled{"sqlite": sq, "mariadb": mq} {
		if strings.Contains(c.SQL, "MATCH") || !strings.Contains(c.SQL, "1 = 0") {
			t.Errorf("%s: empty text leg sends a MATCH or lacks 1 = 0:\n%s", name, c.SQL)
		}
		if len(c.Args) != 6 || c.Args[0] != 100 || c.Args[2] != 100 || c.Args[3] != 60 || c.Args[4] != 60 || c.Args[5] != 10 {
			t.Errorf("%s: args = %#v", name, c.Args)
		}
	}
}

// Catches: a sensitive text column leaving the query text visible in a hook
// event, at either of the two places MariaDB binds it.
func TestHybridSearchSensitiveTextMariaDB(t *testing.T) {
	rec := &obstest.Recorder{}
	db := momo.New(mariaDBFake(), momo.WithHook(rec), momo.WithArgLogging())
	spec := hybridSpec()
	spec.Text = momo.NewColumn[string]("docs", "body", momo.FlagSensitive)
	spec.Query = "secret words"
	if _, err := momo.HybridSearch[searchDoc](db, spec).All(t.Context()); err != nil {
		t.Fatal(err)
	}
	var ev *obstest.Event
	for _, e := range rec.Events() {
		if e.Op == "select" {
			e := e
			ev = &e
		}
	}
	if ev == nil {
		t.Fatalf("no select event recorded: %v", rec.Events())
	}
	// MATCH is rendered twice (the score and the filter), so the query text is
	// bound at positions 0 and 1.
	if len(ev.Args) < 2 || ev.Args[0] != momo.RedactedArg || ev.Args[1] != momo.RedactedArg {
		t.Fatalf("hook saw args %v, want both query-text binds as RedactedArg", ev.Args)
	}
	for _, a := range ev.Args {
		if s, ok := a.(string); ok && strings.Contains(s, "secret") {
			t.Errorf("hook saw the query text: %v", ev.Args)
		}
	}
}

// Catches: MySQL silently degrading to a text-only search, and SQL sent.
func TestHybridSearchMySQLUnsupported(t *testing.T) {
	drv := fakedriver.NewDialect(dialect.MySQLFlavor(dialect.FlavorMySQL))
	db := momo.New(drv)
	_, err := momo.HybridSearch[searchDoc](db, hybridSpec()).All(t.Context())
	if !errors.Is(err, momo.ErrUnsupported) || !strings.Contains(err.Error(), "cosine") {
		t.Fatalf("err = %v, want ErrUnsupported naming the cosine distance", err)
	}
	if len(drv.Calls()) != 0 {
		t.Fatalf("SQL was sent: %v", drv.Calls())
	}
}

// Catches: a text column without a declaration accepted, and a spec validation
// that differs from PostgreSQL.
func TestHybridSpecValidationPortable(t *testing.T) {
	for _, d := range []*fakedriver.Driver{fakedriver.New(dialect.SQLite), mariaDBFake()} {
		db := momo.New(d)
		bad := hybridSpec()
		bad.Text = momo.NewColumn[string]("docs", "id", 0)
		if _, err := momo.HybridSearch[searchDoc](db, bad).SQL(); !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), `"id"`) {
			t.Errorf("undeclared column: err = %v", err)
		}
		for name, mod := range map[string]func(*momo.HybridSpec){
			"candidates": func(s *momo.HybridSpec) { s.TextCandidates = 1001 },
			"rrfk":       func(s *momo.HybridSpec) { s.RRFK = -1 },
			"limit":      func(s *momo.HybridSpec) { s.Limit = 1001 },
			"dimension":  func(s *momo.HybridSpec) { s.QueryVector = vector.Vector{1, 2} },
		} {
			s := hybridSpec()
			mod(&s)
			if _, err := momo.HybridSearch[searchDoc](db, s).SQL(); !errors.Is(err, momo.ErrValidation) {
				t.Errorf("%s: err = %v, want ErrValidation", name, err)
			}
		}
	}
}
