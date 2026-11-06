// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/note"
	"github.com/azrtydxb/momo/internal/relfixture/tag"
)

// graphDriver scripts the RETURNING rows of the documents and comments
// inserts and counts transactions begun and statements per table.
type graphDriver struct {
	*fakedriver.Driver
	begins  int
	inserts map[string]int // INSERT statements by table
	nextID  int64
}

func newGraphDriver(d dialect.Name) *graphDriver {
	return &graphDriver{Driver: fakedriver.New(d), inserts: map[string]int{}}
}

func (g *graphDriver) script(sql string, args []any) {
	if !strings.HasPrefix(sql, "INSERT INTO") {
		g.Driver.SetRows(nil)
		return
	}
	switch {
	case strings.Contains(sql, `"comments"`):
		g.inserts["comments"]++
		rows := make([][]any, 0, len(args)/4)
		for range len(args) / 4 {
			g.nextID++
			rows = append(rows, []any{g.nextID, int64(10), "b", true})
		}
		g.Driver.SetRows(rows)
	case strings.Contains(sql, `"documents"`):
		g.inserts["documents"]++
		g.Driver.SetRows([][]any{{int64(10), nil, "t"}})
	default:
		g.Driver.SetRows(nil)
	}
}

func (g *graphDriver) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	g.script(sql, args)
	return g.Driver.Query(ctx, sql, args)
}

func (g *graphDriver) Begin(ctx context.Context, o momo.TxOptions) (momo.DriverTx, error) {
	g.begins++
	tx, err := g.Driver.Begin(ctx, o)
	return &graphTx{DriverTx: tx, g: g}, err
}

type graphTx struct {
	momo.DriverTx
	g *graphDriver
}

func (t *graphTx) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	t.g.script(sql, args)
	return t.DriverTx.Query(ctx, sql, args)
}

// TestCreateGraphInsertsChildrenInBulk pins the statement count of a graph:
// 1000 has-many children are one multi-row statement, not 1000, and a graph
// over the bind limit (9000 rows x 4 columns > 32766) is one statement per
// chunk. The generated ids come back into the children in order.
func TestCreateGraphInsertsChildrenInBulk(t *testing.T) {
	for _, tc := range []struct{ children, statements int }{{1000, 1}, {9000, 2}} {
		g := newGraphDriver(dialect.SQLite)
		db := momo.New(g)
		doc := &models.Document{ID: 10, Title: "t", Comments: make([]models.Comment, tc.children)}
		for i := range doc.Comments {
			doc.Comments[i].Body = "b"
		}
		if err := momo.CreateGraph(context.Background(), db, doc, document.Comments.Include()); err != nil {
			t.Fatalf("%d children: %v", tc.children, err)
		}
		if got := g.inserts["comments"]; got != tc.statements {
			t.Fatalf("%d children: %d comment INSERT statements, want %d", tc.children, got, tc.statements)
		}
		if g.inserts["documents"] != 1 {
			t.Fatalf("documents inserts: %d", g.inserts["documents"])
		}
		for i, c := range doc.Comments {
			if c.ID != int64(i+1) || c.DocumentID != 10 {
				t.Fatalf("child %d: id=%d document=%d; keys must be written and read back in order", i, c.ID, c.DocumentID)
			}
		}
	}
}

// TestGraphWritesUnsupportedBeforeTransaction checks that a dialect without
// RETURNING or ON CONFLICT is rejected before a transaction is opened.
func TestGraphWritesUnsupportedBeforeTransaction(t *testing.T) {
	g := newGraphDriver(dialect.MySQL)
	db := momo.New(g)
	ctx := context.Background()

	doc := &models.Document{ID: 10, Title: "t", Comments: []models.Comment{{ID: 1, Body: "b"}}}
	err := momo.CreateGraph(ctx, db, doc, document.Comments.Include())
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("CreateGraph on MySQL: want ErrUnsupported, got %v", err)
	}
	_, err = momo.FindOrCreate[models.Tag, *models.Tag](ctx, db, []momo.ColumnRef{tag.ID}, &models.Tag{ID: 1})
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("FindOrCreate on MySQL: want ErrUnsupported, got %v", err)
	}
	if g.begins != 0 || len(g.Calls()) != 0 {
		t.Fatalf("no transaction or statement may run: begins=%d calls=%d", g.begins, len(g.Calls()))
	}
}

// findDriver answers the statements of FindOrCreate: the INSERT conflicts (no
// row), a COUNT reports deletedRows, any other SELECT finds nothing.
type findDriver struct {
	*fakedriver.Driver
	deletedRows int64
}

func (f *findDriver) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	if strings.HasPrefix(sql, "SELECT COUNT") {
		f.Driver.SetRows([][]any{{f.deletedRows}})
	} else {
		f.Driver.SetRows(nil)
	}
	return f.Driver.Query(ctx, sql, args)
}

// TestFindOrCreateSoftDeletedMatchIsNotFound pins what FindOrCreate does when
// the key is held by a soft-deleted row: the row is neither returned nor
// restored, the call fails with ErrUniqueViolation, and the re-select excludes
// deleted rows. With no row at all behind the conflict (a snapshot that cannot
// see the winner) the error stays ErrNotFound.
func TestFindOrCreateSoftDeletedMatchIsNotFound(t *testing.T) {
	for _, tc := range []struct {
		deleted int64
		want    error
	}{{1, momo.ErrUniqueViolation}, {0, momo.ErrNotFound}} {
		f := &findDriver{Driver: fakedriver.New(dialect.Postgres), deletedRows: tc.deleted}
		db := momo.New(f)
		m := &models.Note{ID: 1, DocumentID: 1, Body: "new"}
		created, err := momo.FindOrCreate[models.Note, *models.Note](context.Background(), db, []momo.ColumnRef{note.ID}, m)
		if created || !errors.Is(err, tc.want) {
			t.Fatalf("deleted rows %d: created=%v err=%v, want %v", tc.deleted, created, err, tc.want)
		}
		if m.Body != "new" {
			t.Fatalf("the soft-deleted row must not overwrite m: %+v", m)
		}
		var reselect string
		for _, c := range f.Calls() {
			if strings.HasPrefix(c.SQL, "SELECT") && !strings.HasPrefix(c.SQL, "SELECT COUNT") {
				reselect = c.SQL
			}
		}
		if !strings.Contains(reselect, `"deleted" = `) {
			t.Fatalf("the re-select must exclude soft-deleted rows, got %q", reselect)
		}
	}
}
