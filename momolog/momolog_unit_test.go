// SPDX-License-Identifier: Apache-2.0

package momolog_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/momolog"
)

const secret = "hunter2-sensitive"

type kid struct{ PID, Seq int64 }

type parent struct {
	ID   int64
	Kids []kid
}

var kidMeta = momo.Meta[kid]{
	Table:      "log_kids",
	SoftDelete: -1,
	Columns:    []momo.ColumnMeta{{Name: "pid"}, {Name: "seq"}},
	Ptr: func(m *kid, i int) any {
		if i == 0 {
			return &m.PID
		}
		return &m.Seq
	},
}

var kids = momo.Relation[parent, kid]{
	Name: "Kids", Kind: momo.HasMany,
	Parent: momo.TableRef{Name: "log_parents"}, Child: momo.TableRef{Name: "log_kids"},
	ChildMeta:  &kidMeta,
	ParentCols: []string{"id"}, ChildCols: []string{"pid"}, ChildPK: []string{"seq"},
	ParentKey: func(p *parent) ([]any, bool) { return []any{p.ID}, true },
	ChildKey:  func(c *kid) ([]any, bool) { return []any{c.PID}, true },
	GetMany:   func(p *parent) []kid { return p.Kids },
	SetMany:   func(p *parent, cs []kid) { p.Kids = cs },
}

func parents(n int) []parent {
	ps := make([]parent, n)
	for i := range ps {
		ps[i].ID = int64(i + 1)
	}
	return ps
}

func logger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func insertSecret(t *testing.T, db *momo.DB) {
	t.Helper()
	_, err := momo.Insert[suite.Document](db).Models([]suite.Document{{TenantID: 1, Title: "t", Secret: secret}}).Exec(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

// failDriver fails every query.
type failDriver struct {
	*fakedriver.Driver
	err error
}

func (d failDriver) Query(context.Context, string, []any) (momo.Rows, error) { return nil, d.err }

func TestHookIsARelationHook(t *testing.T) {
	if _, ok := momolog.New(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))).(momo.RelationHook); !ok {
		t.Fatal("momolog.New must return a hook that also implements momo.RelationHook")
	}
}

func TestNeverLogsSensitiveValue(t *testing.T) {
	var buf bytes.Buffer
	db := momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(momolog.New(logger(&buf), momolog.Values(true))), momo.WithArgLogging())
	insertSecret(t, db)
	out := buf.String()
	if strings.Contains(out, secret) {
		t.Fatalf("log leaks the sensitive value:\n%s", out)
	}
	for _, want := range []string{"[redacted]", "args=", "op=insert", "table=documents", "sql=", "duration=", "rows="} {
		if !strings.Contains(out, want) {
			t.Fatalf("log lacks %q:\n%s", want, out)
		}
	}
}

func TestValuesAreOptIn(t *testing.T) {
	t.Run("WithArgLogging without Values logs no args", func(t *testing.T) {
		var buf bytes.Buffer
		db := momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(momolog.New(logger(&buf))), momo.WithArgLogging())
		insertSecret(t, db)
		out := buf.String()
		if strings.Contains(out, "args=") || strings.Contains(out, "[redacted]") || strings.Contains(out, secret) {
			t.Fatalf("args logged without Values:\n%s", out)
		}
		if !strings.Contains(out, "op=insert") {
			t.Fatalf("no record logged:\n%s", out)
		}
	})
	t.Run("Values without WithArgLogging logs no args", func(t *testing.T) {
		var buf bytes.Buffer
		db := momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(momolog.New(logger(&buf), momolog.Values(true))))
		insertSecret(t, db)
		out := buf.String()
		if strings.Contains(out, "args=") || strings.Contains(out, secret) {
			t.Fatalf("args logged without WithArgLogging:\n%s", out)
		}
		if !strings.Contains(out, "op=insert") {
			t.Fatalf("no record logged:\n%s", out)
		}
	})
}

func TestLogsRelationLoads(t *testing.T) {
	var buf bytes.Buffer
	db := momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(momolog.New(logger(&buf))))
	if err := momo.Load(context.Background(), db, parents(2), kids); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"relation=log_parents.Kids", "msg=momo.relation", "parent=log_parents", "name=Kids", "parents=2", "queries=1", "children=0", "duration="} {
		if !strings.Contains(out, want) {
			t.Fatalf("log lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "err=") {
		t.Fatalf("successful load logged an error:\n%s", out)
	}
}

func TestLogsErrors(t *testing.T) {
	boom := errors.New("boom: connection reset")
	var buf bytes.Buffer
	db := momo.New(failDriver{Driver: fakedriver.New(dialect.Postgres), err: boom}, momo.WithHook(momolog.New(logger(&buf))))
	if err := momo.Load(context.Background(), db, parents(2), kids); err == nil {
		t.Fatal("want a load error")
	}
	var queryErr, relErr bool
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if !strings.Contains(line, "boom: connection reset") {
			continue
		}
		queryErr = queryErr || strings.Contains(line, "msg=momo.query")
		relErr = relErr || strings.Contains(line, "msg=momo.relation")
	}
	if !queryErr || !relErr {
		t.Fatalf("want err on the query record (%v) and the relation record (%v):\n%s", queryErr, relErr, buf.String())
	}
}

func TestLevels(t *testing.T) {
	var buf bytes.Buffer
	h := momolog.New(logger(&buf), momolog.Level(slog.LevelInfo), momolog.SlowOver(time.Second))
	h.AfterQuery(context.Background(), &momo.QueryEvent{Op: "select", SQL: "select 1", Duration: time.Millisecond})
	h.AfterQuery(context.Background(), &momo.QueryEvent{Op: "select", SQL: "select 2", Duration: 2 * time.Second})
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "level=INFO") || !strings.Contains(lines[1], "level=WARN") {
		t.Fatalf("levels:\n%s", buf.String())
	}
}
