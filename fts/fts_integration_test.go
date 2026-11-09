// SPDX-License-Identifier: Apache-2.0

//go:build integration

package fts_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/fts"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/sqlfn"
	"github.com/azrtydxb/momo/types"
	"github.com/azrtydxb/momo/vector"
)

// ftsHit is a projection of an article id, a score and a text.
type ftsHit struct {
	ID   int64
	Rank float64
	Text string
}

var ftsHitMeta = momo.Meta[ftsHit]{
	Columns:  []momo.ColumnMeta{{Name: "id"}, {Name: "rank"}, {Name: "text"}},
	Ptr:      func(m *ftsHit, i int) any { return [...]any{&m.ID, &m.Rank, &m.Text}[i] },
	Value:    func(m *ftsHit, i int) any { return [...]any{m.ID, m.Rank, m.Text}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*ftsHit) MomoMeta() *momo.Meta[ftsHit] { return &ftsHitMeta }

// ftsStored reads the stored tsvector back.
type ftsStored struct {
	ID  int64
	Tsv types.TSVector
}

var ftsStoredMeta = momo.Meta[ftsStored]{
	Columns:  []momo.ColumnMeta{{Name: "id"}, {Name: "search_tsv"}},
	Ptr:      func(m *ftsStored, i int) any { return [...]any{&m.ID, &m.Tsv}[i] },
	Value:    func(m *ftsStored, i int) any { return [...]any{m.ID, m.Tsv}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*ftsStored) MomoMeta() *momo.Meta[ftsStored] { return &ftsStoredMeta }

var (
	kbID    = momo.ColumnOf[int64]("kb_articles", "id")
	kbTitle = momo.ColumnOf[string]("kb_articles", "title")
	kbBody  = momo.ColumnOf[string]("kb_articles", "body")
	kbTsv   = momo.ColumnOf[types.TSVector]("kb_articles", "search_tsv")
	kbEmb   = momo.NewVectorColumn[vector.Vector]("kb_articles", "emb", 3, 0)
)

// createKB makes a knowledge-base table whose tsvector column is maintained by
// a trigger, the way a MomoConfig function and trigger declare it.
func createKB(t *testing.T, db *momo.DB) {
	t.Helper()
	for _, s := range []string{
		"DROP TABLE IF EXISTS kb_articles",
		`CREATE TABLE kb_articles (id bigint primary key, title text not null, body text not null,
			emb vector(3) not null, search_tsv tsvector)`,
		`CREATE OR REPLACE FUNCTION kb_articles_tsv() RETURNS trigger LANGUAGE plpgsql AS $f$
			BEGIN
				NEW.search_tsv := setweight(to_tsvector('english', coalesce(NEW.title, '')), 'A') ||
					to_tsvector('english', coalesce(NEW.body, ''));
				RETURN NEW;
			END; $f$`,
		`CREATE TRIGGER kb_articles_tsv_trg BEFORE INSERT OR UPDATE ON kb_articles
			FOR EACH ROW EXECUTE FUNCTION kb_articles_tsv()`,
		"CREATE INDEX kb_articles_search_gin ON kb_articles USING gin (search_tsv)",
		`INSERT INTO kb_articles (id, title, body, emb) VALUES
			(1, 'Reset your password', 'Click the reset link in the email to reset a forgotten password', '[1,0,0]'),
			(2, 'Billing questions', 'Invoices are listed under billing. You can reset the payment method', '[0,1,0]'),
			(3, 'Shipping', 'We ship worldwide', '[0,0,1]')`,
	} {
		if _, err := db.Driver().Exec(context.Background(), s, nil); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func ftsIDs(hits []ftsHit) []int64 {
	ids := make([]int64, len(hits))
	for i, h := range hits {
		ids[i] = h.ID
	}
	return ids
}

// Catches: a wrong operator or argument order in the builders, a regconfig
// bound without its cast, ranking that does not order best first, a headline
// that ignores its options string, and a trigger-maintained tsvector that
// cannot be scanned into types.TSVector.
func TestFullTextSearch(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		q := fts.WebsearchToQuery("english", "reset")
		if d != dialect.Postgres {
			_, err := momo.Select[ftsHit](db).From(momo.TableRef{Name: "kb_articles"}).
				Columns(kbID, fts.RankCD(kbTsv, q), kbTitle).Where(fts.Match(kbTsv, q)).SQL()
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("full-text on %s = %v, want ErrUnsupported", d, err)
			}
			return
		}
		createKB(t, db)
		ctx := context.Background()
		from := momo.TableRef{Name: "kb_articles"}
		rank := fts.RankCD(kbTsv, q)

		// Where, Columns and OrderBy together: best match first.
		hits, err := momo.Select[ftsHit](db).From(from).
			Columns(kbID, rank, kbTitle).Where(fts.Match(kbTsv, q)).OrderBy(rank.Desc()).All(ctx)
		if err != nil || !slices.Equal(ftsIDs(hits), []int64{1, 2}) || hits[0].Rank <= hits[1].Rank || hits[1].Rank <= 0 {
			t.Fatalf("ranked search = %+v, %v; want articles 1 then 2 with falling positive ranks", hits, err)
		}

		// Web-search syntax: a negated term, then a quoted phrase.
		neg, err := momo.Select[ftsHit](db).From(from).Columns(kbID, rank, kbTitle).
			Where(fts.Match(kbTsv, fts.WebsearchToQuery("english", "reset -password"))).All(ctx)
		if err != nil || !slices.Equal(ftsIDs(neg), []int64{2}) {
			t.Fatalf("negation = %+v, %v; want article 2", neg, err)
		}
		phrase, err := momo.Select[ftsHit](db).From(from).Columns(kbID, rank, kbTitle).
			Where(fts.Match(kbTsv, fts.WebsearchToQuery("english", `"reset link"`))).All(ctx)
		if err != nil || !slices.Equal(ftsIDs(phrase), []int64{1}) {
			t.Fatalf("phrase = %+v, %v; want article 1", phrase, err)
		}

		// plainto, phraseto and to_tsquery with a to_tsvector over a text column.
		for name, query := range map[string]momo.Expression[types.TSQuery]{
			"plain":  fts.PlainToQuery("english", "ships worldwide"),
			"phrase": fts.PhraseToQuery("english", "ship worldwide"),
			"tsq":    fts.ToQuery("english", "ship & world:*"),
		} {
			got, err := momo.Select[ftsHit](db).From(from).Columns(kbID, rank, kbTitle).
				Where(fts.Match(fts.ToVector("english", kbBody), query)).All(ctx)
			if err != nil || !slices.Equal(ftsIDs(got), []int64{3}) {
				t.Fatalf("%s = %+v, %v; want article 3", name, got, err)
			}
		}

		// ts_rank with a normalization flag stays positive.
		norm, err := momo.Select[ftsHit](db).From(from).
			Columns(kbID, fts.Rank(kbTsv, q, 1), kbTitle).Where(fts.Match(kbTsv, q)).All(ctx)
		if err != nil || len(norm) != 2 || norm[0].Rank <= 0 {
			t.Fatalf("ts_rank = %+v, %v", norm, err)
		}

		// ts_headline with an options string.
		head, err := momo.Select[ftsHit](db).From(from).
			Columns(kbID, rank, fts.Headline("english", kbBody, q, "StartSel=<b>, StopSel=</b>")).
			Where(kbID.Eq(1)).All(ctx)
		if err != nil || len(head) != 1 || !strings.Contains(head[0].Text, "<b>reset</b>") {
			t.Fatalf("headline = %+v, %v; want marked-up reset", head, err)
		}

		// The stored tsvector scans into types.TSVector.
		stored, err := momo.Select[ftsStored](db).From(from).Columns(kbID, kbTsv).Where(kbID.Eq(3)).One(ctx)
		if err != nil || !strings.Contains(string(stored.Tsv), "'ship'") || !strings.Contains(string(stored.Tsv), "'worldwid'") {
			t.Fatalf("stored tsvector = %q, %v", stored.Tsv, err)
		}

		// A whole tsvector value bound as a parameter round-trips.
		for _, s := range []string{"DROP TABLE IF EXISTS fts_tmp", "CREATE TABLE fts_tmp (v tsvector)"} {
			if _, err := db.Driver().Exec(ctx, s, nil); err != nil {
				t.Fatal(err)
			}
		}
		in := types.TSVector("'cat':3 'fat':2")
		if _, err := momo.Exec(ctx, db, "INSERT INTO fts_tmp (v) VALUES (?)", in); err != nil {
			t.Fatal(err)
		}
		back, err := momo.Scalars[string](ctx, db, "SELECT CAST(v AS text) FROM fts_tmp")
		if err != nil || len(back) != 1 || back[0] != string(in) {
			t.Fatalf("tsvector bind = %v, %v; want %q", back, err, in)
		}
	})
}

// Catches: the documented hybrid ranking pattern drifting from what the
// builders produce: a cosine distance and a text rank combined in one score,
// best first, over rows that match either signal.
func TestFullTextHybridRanking(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skipf("full-text and vector search need PostgreSQL, not %s", d)
		}
		createKB(t, db)
		ctx := context.Background()

		q := fts.WebsearchToQuery("english", "reset")
		dist := momo.CosineDistance(kbEmb, vector.Vector{1, 0, 0})
		text := fts.RankCD(kbTsv, q)
		// Lower is better: the distance, reduced by half the text rank.
		score := sqlfn.SubExpr[float64](dist, sqlfn.Mul[float64](text, 0.5))

		hits, err := momo.Select[ftsHit](db).From(momo.TableRef{Name: "kb_articles"}).
			Columns(kbID, score.As("score"), kbTitle).
			OrderBy(score.Asc(), kbID.Asc()).All(ctx)
		if err != nil || !slices.Equal(ftsIDs(hits), []int64{1, 2, 3}) {
			t.Fatalf("hybrid order = %+v, %v; want 1, 2, 3", hits, err)
		}
		if !(hits[0].Rank < hits[1].Rank && hits[1].Rank < hits[2].Rank) {
			t.Fatalf("scores must rise down the list: %+v", hits)
		}
	})
}
