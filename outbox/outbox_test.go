// SPDX-License-Identifier: Apache-2.0

package outbox_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/outbox"
	"github.com/azrtydxb/momo/schema"
	"github.com/jackc/pgx/v5"
)

var (
	declaredMu    sync.Mutex
	declaredCache = map[dialect.Name]*schema.Schema{}
)

// loadDeclared loads the outbox model of testdata/model with the generator and
// returns the schema it declares for d. It is the schema `momo migrate diff`
// writes a migration from.
func loadDeclared(t testing.TB, d dialect.Name) *schema.Schema {
	t.Helper()
	declaredMu.Lock()
	defer declaredMu.Unlock()
	if s, ok := declaredCache[d]; ok {
		return s
	}
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./testdata/model"}, gen.Config{Dir: dir, Dialects: []dialect.Name{d}})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	declaredCache[d] = g.Schema
	return g.Schema
}

// Catches: a declared model that drifts from the columns the relay reads and
// writes, which would only show as a SQL error at run time.
func TestDeclaredModelMatchesRelayColumns(t *testing.T) {
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		t.Run(string(d), func(t *testing.T) { checkDeclared(t, loadDeclared(t, d)) })
	}
}

func checkDeclared(t *testing.T, s *schema.Schema) {
	t.Helper()
	var tbl *schema.Table
	for i := range s.Tables {
		if s.Tables[i].Name == outbox.DefaultTable {
			tbl = &s.Tables[i]
		}
	}
	if tbl == nil {
		t.Fatalf("the model declares no table %q: %+v", outbox.DefaultTable, s.Tables)
	}
	want := map[string]bool{ // column -> nullable
		"id": false, "topic": false, "key": true, "payload": false, "headers": true,
		"created_at": false, "available_at": false, "attempts": false,
		"published_at": true, "dead_at": true, "last_error": true,
	}
	if len(tbl.Columns) != len(want) {
		t.Fatalf("columns = %d, want %d: %+v", len(tbl.Columns), len(want), tbl.Columns)
	}
	for _, c := range tbl.Columns {
		nullable, ok := want[c.Name]
		if !ok {
			t.Errorf("unexpected column %q", c.Name)
			continue
		}
		if c.Nullable != nullable {
			t.Errorf("column %q nullable = %v, want %v", c.Name, c.Nullable, nullable)
		}
	}
	if len(tbl.PrimaryKey) != 1 || tbl.PrimaryKey[0] != "id" {
		t.Errorf("primary key = %v, want [id]", tbl.PrimaryKey)
	}
	if len(tbl.Indexes) != 2 {
		t.Errorf("indexes = %d, want the due and the key index: %+v", len(tbl.Indexes), tbl.Indexes)
	}
}

// Catches: a table name that reaches the SQL text unquoted or unchecked.
func TestNewValidatesTableName(t *testing.T) {
	for _, bad := range []string{"", "a-b", "1events", "ev ents", `a"b`, "a.b", strings.Repeat("x", 64)} {
		if _, err := outbox.New(outbox.WithTable(bad)); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("WithTable(%q): err = %v, want ErrValidation", bad, err)
		}
	}
	for _, ok := range []string{"outbox_events", "Events2", "_x", strings.Repeat("x", 63)} {
		o, err := outbox.New(outbox.WithTable(ok))
		if err != nil {
			t.Errorf("WithTable(%q): %v", ok, err)
			continue
		}
		if o.Table() != ok {
			t.Errorf("Table() = %q, want %q", o.Table(), ok)
		}
	}
	o, err := outbox.New()
	if err != nil || o.Table() != outbox.DefaultTable {
		t.Fatalf("New() = %v, %v; want the default table", o, err)
	}
}

// Catches: Add running outside a transaction, or on a database that cannot
// keep the outbox contract, instead of refusing.
func TestAddRefusals(t *testing.T) {
	ctx := context.Background()
	o, err := outbox.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.Add(ctx, nil, outbox.Event{Topic: "t"}); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("nil tx: err = %v, want ErrValidation", err)
	}

	pg := momo.New(fakedriver.New(dialect.Postgres))
	tx, err := pg.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.Add(ctx, tx, outbox.Event{}); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("empty topic: err = %v, want ErrValidation", err)
	}

	other := momo.New(fakedriver.NewDialect(&dialect.Dialect{Name: "other"}))
	otx, err := other.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.Add(ctx, otx, outbox.Event{Topic: "t"}); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("unknown dialect tx: err = %v, want ErrUnsupported", err)
	}
}

// Catches: a relay that starts on a dialect it cannot claim rows on, or with
// settings that would spin or never retry.
func TestNewRelayRefusals(t *testing.T) {
	o, err := outbox.New()
	if err != nil {
		t.Fatal(err)
	}
	pub := outbox.PublisherFunc(func(context.Context, outbox.Event) error { return nil })
	pg := momo.New(fakedriver.New(dialect.Postgres))

	if _, err := o.NewRelay(momo.New(fakedriver.NewDialect(&dialect.Dialect{Name: "other"})), pub); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("unknown dialect: err = %v, want ErrUnsupported", err)
	}
	my := momo.New(fakedriver.New(dialect.MySQL))
	if _, err := o.NewRelay(my, pub); err != nil {
		t.Errorf("mysql defaults: %v", err)
	}
	if _, err := o.NewRelay(my, pub, outbox.WithListen(&pgx.ConnConfig{})); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("listen on mysql: err = %v, want ErrUnsupported", err)
	}
	if _, err := o.NewRelay(pg, nil); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("nil publisher: err = %v, want ErrValidation", err)
	}
	if _, err := o.NewRelay(nil, pub); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("nil db: err = %v, want ErrValidation", err)
	}
	for name, opt := range map[string]outbox.RelayOption{
		"batch":    outbox.WithBatchSize(0),
		"lease":    outbox.WithLease(0),
		"poll":     outbox.WithPollInterval(-1),
		"backoff":  outbox.WithBackoff(0, 1),
		"backoff2": outbox.WithBackoff(2, 1),
		"attempts": outbox.WithMaxAttempts(0),
		"retain":   outbox.WithRetention(-1),
	} {
		if _, err := o.NewRelay(pg, pub, opt); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		}
	}
	lite := momo.New(fakedriver.New(dialect.SQLite))
	if _, err := o.NewRelay(lite, pub, outbox.WithListen(&pgx.ConnConfig{})); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("listen on sqlite: err = %v, want ErrUnsupported", err)
	}
	if _, err := o.NewRelay(lite, pub); err != nil {
		t.Errorf("sqlite defaults: %v", err)
	}
	if _, err := o.NewRelay(pg, pub, outbox.WithListen(nil)); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("nil listen config: err = %v, want ErrValidation", err)
	}
	if _, err := o.NewRelay(pg, pub); err != nil {
		t.Errorf("defaults: %v", err)
	}
}
