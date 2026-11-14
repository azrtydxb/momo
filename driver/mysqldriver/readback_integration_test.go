// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysqldriver_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/schema"
)

// These tests run on every server in servers: MySQL 8.4 and 9.7 (no RETURNING:
// keys come from LAST_INSERT_ID, one row per statement) and MariaDB 11.8
// (INSERT ... RETURNING).

type rbParent struct {
	ID   int64
	Name string
	Kids []rbKid
}

func (*rbParent) MomoMeta() *momo.Meta[rbParent] {
	return &momo.Meta[rbParent]{
		Table: "rb_parents", PK: []int{0}, Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
		Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity}, {Name: "name"}},
		Ptr:     func(m *rbParent, i int) any { return [...]any{&m.ID, &m.Name}[i] },
		Value:   func(m *rbParent, i int) any { return [...]any{m.ID, m.Name}[i] },
	}
}

type rbKid struct {
	ID       int64
	ParentID int64
	Body     string
}

func (*rbKid) MomoMeta() *momo.Meta[rbKid] {
	return &momo.Meta[rbKid]{
		Table: "rb_kids", PK: []int{0}, Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
		Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity}, {Name: "parent_id"}, {Name: "body"}},
		Ptr:     func(m *rbKid, i int) any { return [...]any{&m.ID, &m.ParentID, &m.Body}[i] },
		Value:   func(m *rbKid, i int) any { return [...]any{m.ID, m.ParentID, m.Body}[i] },
	}
}

var rbKids = momo.Relation[rbParent, rbKid]{
	Name: "Kids", Kind: momo.HasMany,
	Parent: momo.TableRef{Name: "rb_parents"}, Child: momo.TableRef{Name: "rb_kids"},
	ParentMeta: (*rbParent)(nil).MomoMeta(), ChildMeta: (*rbKid)(nil).MomoMeta(),
	ParentCols: []string{"id"}, ChildCols: []string{"parent_id"}, ChildPK: []string{"id"},
	ParentKey:   func(p *rbParent) ([]any, bool) { return []any{p.ID}, true },
	ChildKey:    func(c *rbKid) ([]any, bool) { return []any{c.ParentID}, true },
	SetChildKey: func(c *rbKid, k []any) { c.ParentID = k[0].(int64) },
	GetMany:     func(p *rbParent) []rbKid { return p.Kids },
	SetMany:     func(p *rbParent, cs []rbKid) { p.Kids = cs },
}

// rbDoc has a soft-delete marker next to a unique slug.
type rbDoc struct {
	ID        int64
	Slug      string
	DeletedAt *time.Time
}

func (*rbDoc) MomoMeta() *momo.Meta[rbDoc] {
	return &momo.Meta[rbDoc]{
		Table: "rb_docs", PK: []int{0}, Identity: 0, Version: -1, SoftDelete: 2, CreatedAt: -1, UpdatedAt: -1,
		Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity}, {Name: "slug"}, {Name: "deleted_at", Flags: momo.FlagSoftDelete | momo.FlagNullable}},
		Ptr:     func(m *rbDoc, i int) any { return [...]any{&m.ID, &m.Slug, &m.DeletedAt}[i] },
		Value:   func(m *rbDoc, i int) any { return [...]any{m.ID, m.Slug, m.DeletedAt}[i] },
	}
}

var rbDocSlug = momo.NewColumn[string]("rb_docs", "slug", 0)

type rbTag struct {
	ID   int64
	Name string
}

func (*rbTag) MomoMeta() *momo.Meta[rbTag] {
	return &momo.Meta[rbTag]{
		Table: "rb_tags", PK: []int{0}, Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
		Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity}, {Name: "name"}},
		Ptr:     func(m *rbTag, i int) any { return [...]any{&m.ID, &m.Name}[i] },
		Value:   func(m *rbTag, i int) any { return [...]any{m.ID, m.Name}[i] },
	}
}

var (
	rbTagName  = momo.NewColumn[string]("rb_tags", "name", 0)
	rbUserName = momo.NewColumn[string]("rb_users", "name", 0)
)

// rbUser has an auto-increment primary key and UNIQUE(email); rbPair has a
// second ordinary unique key (name) next to it.
type rbUser struct {
	ID    int64
	Email string
	Name  string
}

type rbPair struct {
	ID    int64
	Email string
	Name  string
}

var (
	rbUserEmail  = momo.NewColumn[string]("rb_users", "email", 0)
	rbUserID     = momo.NewColumn[int64]("rb_users", "id", momo.FlagPK)
	rbUserSchema = &schema.Table{Name: "rb_users", PrimaryKey: []string{"id"},
		Uniques: []schema.Unique{{Name: "rb_users_email", Columns: []string{"email"}}}}
	rbPairSchema = &schema.Table{Name: "rb_pairs", PrimaryKey: []string{"id"},
		Uniques: []schema.Unique{{Name: "rb_pairs_email", Columns: []string{"email"}}, {Name: "rb_pairs_name", Columns: []string{"name"}}}}
	rbPairEmail = momo.NewColumn[string]("rb_pairs", "email", 0)
	rbCols      = []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity}, {Name: "email"}, {Name: "name"}}
)

func (*rbUser) MomoMeta() *momo.Meta[rbUser] {
	return &momo.Meta[rbUser]{
		Table: "rb_users", PK: []int{0}, Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
		TableSchema: rbUserSchema, Columns: rbCols,
		Ptr:   func(m *rbUser, i int) any { return [...]any{&m.ID, &m.Email, &m.Name}[i] },
		Value: func(m *rbUser, i int) any { return [...]any{m.ID, m.Email, m.Name}[i] },
	}
}

func (*rbPair) MomoMeta() *momo.Meta[rbPair] {
	return &momo.Meta[rbPair]{
		Table: "rb_pairs", PK: []int{0}, Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
		TableSchema: rbPairSchema, Columns: rbCols,
		Ptr:   func(m *rbPair, i int) any { return [...]any{&m.ID, &m.Email, &m.Name}[i] },
		Value: func(m *rbPair, i int) any { return [...]any{m.ID, m.Email, m.Name}[i] },
	}
}

func rbSchema(t *testing.T, q momo.Querier) {
	t.Helper()
	mustExecMySQL(t, q, "CREATE TABLE rb_parents (id bigint PRIMARY KEY AUTO_INCREMENT, name varchar(40) NOT NULL)")
	mustExecMySQL(t, q, "CREATE TABLE rb_kids (id bigint PRIMARY KEY AUTO_INCREMENT, parent_id bigint NOT NULL, body varchar(40) NOT NULL)")
	mustExecMySQL(t, q, "CREATE TABLE rb_tags (id bigint PRIMARY KEY AUTO_INCREMENT, name varchar(40) NOT NULL UNIQUE)")
	mustExecMySQL(t, q, "CREATE TABLE rb_users (id bigint PRIMARY KEY AUTO_INCREMENT, email varchar(80) NOT NULL UNIQUE, name varchar(40) NOT NULL)")
	mustExecMySQL(t, q, "CREATE TABLE rb_docs (id bigint PRIMARY KEY AUTO_INCREMENT, slug varchar(40) NOT NULL UNIQUE, deleted_at datetime(6) NULL)")
	mustExecMySQL(t, q, "CREATE TABLE rb_pairs (id bigint PRIMARY KEY AUTO_INCREMENT, email varchar(80) NOT NULL UNIQUE, name varchar(40) NOT NULL UNIQUE)")
}

// rbCheckGraph fails unless every kid holds the key the database stored for the
// row with its body, and points at the parent.
func rbCheckGraph(t *testing.T, ctx context.Context, q momo.Querier, p *rbParent) {
	t.Helper()
	if p.ID == 0 {
		t.Fatal("parent key was not read back")
	}
	stored, err := momo.Select[rbKid](q).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]rbKid{}
	for _, k := range stored {
		byID[k.ID] = k
	}
	seen := map[int64]bool{}
	for i, k := range p.Kids {
		if k.ID == 0 || seen[k.ID] {
			t.Fatalf("kid %d: key %d is zero or repeated", i, k.ID)
		}
		seen[k.ID] = true
		row, ok := byID[k.ID]
		if !ok || row.Body != k.Body || row.ParentID != p.ID || k.ParentID != p.ID {
			t.Fatalf("kid %d: read-back key %d does not point at its own row: stored=%+v ok=%v, memory=%+v, parent=%d", i, k.ID, row, ok, k, p.ID)
		}
	}
}

// Catches: CreateGraph on MySQL or MariaDB failing with ErrUnsupported, or a
// child ending up with a key that is not its own row's. The concurrent graphs
// give the server a chance to interleave their inserts, but the interleaving is
// not deterministic, so this does not prove that guessed keys would fail; the
// unit tests pin the one-row-per-statement rule.
func TestCreateGraphReadBackMySQLFamily(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			ctx := context.Background()
			db := s.open(t)
			rbSchema(t, db)
			rows, err := momo.Scalars[int64](ctx, db, "SELECT @@auto_increment_increment")
			if err != nil || len(rows) != 1 {
				t.Fatal(err)
			}
			if modes, err := momo.Scalars[int64](ctx, db, "SELECT @@innodb_autoinc_lock_mode"); err == nil && len(modes) == 1 {
				t.Logf("%s: innodb_autoinc_lock_mode=%d auto_increment_increment=%d", s.name, modes[0], rows[0])
			}

			// One graph on the pool: a transaction of its own.
			p := &rbParent{Name: "p"}
			for i := range 12 {
				p.Kids = append(p.Kids, rbKid{Body: fmt.Sprintf("solo-%d", i)})
			}
			if err := momo.CreateGraph(ctx, db, p, rbKids.Include()); err != nil {
				t.Fatalf("CreateGraph: %v", err)
			}
			rbCheckGraph(t, ctx, db, p)

			// The same inside a caller's transaction (a savepoint).
			q := &rbParent{Name: "q", Kids: []rbKid{{Body: "tx-0"}, {Body: "tx-1"}, {Body: "tx-2"}}}
			err = db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
				return momo.CreateGraph(ctx, tx, q, rbKids.Include())
			})
			if err != nil {
				t.Fatalf("CreateGraph in a Tx: %v", err)
			}
			rbCheckGraph(t, ctx, db, q)

			// Graphs inserting at once (the interleaving is up to the server).
			var wg sync.WaitGroup
			graphs := make([]*rbParent, 3)
			errs := make([]error, len(graphs))
			for g := range graphs {
				graphs[g] = &rbParent{Name: fmt.Sprintf("c%d", g)}
				for i := range 15 {
					graphs[g].Kids = append(graphs[g].Kids, rbKid{Body: fmt.Sprintf("c%d-%d", g, i)})
				}
				wg.Add(1)
				go func() {
					defer wg.Done()
					errs[g] = momo.CreateGraph(ctx, db, graphs[g], rbKids.Include())
				}()
			}
			wg.Wait()
			for g, err := range errs {
				if err != nil {
					t.Fatalf("concurrent graph %d: %v", g, err)
				}
				rbCheckGraph(t, ctx, db, graphs[g])
			}

			// A failure undoes the whole graph.
			bad := &rbParent{Name: "bad", Kids: []rbKid{{Body: "ok"}, {Body: "this body is much longer than the forty characters its column holds"}}}
			if err := momo.CreateGraph(ctx, db, bad, rbKids.Include()); err == nil {
				t.Fatal("an over-long body inserted")
			}
			n, err := momo.Select[rbParent](db).Count(ctx)
			if err != nil || n != 5 {
				t.Fatalf("parents after the failed graph = %d, %v; want 5 (the failed graph is rolled back)", n, err)
			}
		})
	}
}

// Catches: FindOrCreate on MySQL or MariaDB reporting created for a row that
// existed (or the reverse), not returning the stored row, or letting
// concurrent callers of one key create it twice or fail.
func TestFindOrCreateMySQLFamily(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			ctx := context.Background()
			db := s.open(t)
			rbSchema(t, db)
			key := []momo.ColumnRef{rbTagName}

			first := &rbTag{Name: "go"}
			created, err := momo.FindOrCreate[rbTag, *rbTag](ctx, db, key, first)
			if err != nil || !created || first.ID == 0 {
				t.Fatalf("first call: created=%v id=%d err=%v, want created with a key", created, first.ID, err)
			}
			again := &rbTag{Name: "go"}
			created, err = momo.FindOrCreate[rbTag, *rbTag](ctx, db, key, again)
			if err != nil || created || again.ID != first.ID {
				t.Fatalf("second call: created=%v id=%d (want %d) err=%v, want found", created, again.ID, first.ID, err)
			}

			// Inside a caller's transaction.
			err = db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
				a := &rbTag{Name: "tx"}
				if c, err := momo.FindOrCreate[rbTag, *rbTag](ctx, tx, key, a); err != nil || !c || a.ID == 0 {
					return fmt.Errorf("created in Tx: created=%v id=%d err=%v", c, a.ID, err)
				}
				b := &rbTag{Name: "tx"}
				if c, err := momo.FindOrCreate[rbTag, *rbTag](ctx, tx, key, b); err != nil || c || b.ID != a.ID {
					return fmt.Errorf("found in Tx: created=%v id=%d (want %d) err=%v", c, b.ID, a.ID, err)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}

			// Callers racing for one new key: exactly one creates it.
			const racers = 6
			var wg sync.WaitGroup
			var mu sync.Mutex
			createdBy, ids, failures := 0, map[int64]bool{}, []error{}
			for range racers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					m := &rbTag{Name: "race"}
					c, err := momo.FindOrCreate[rbTag, *rbTag](ctx, db, key, m)
					mu.Lock()
					defer mu.Unlock()
					if err != nil {
						failures = append(failures, err)
						return
					}
					if c {
						createdBy++
					}
					ids[m.ID] = true
				}()
			}
			wg.Wait()
			if len(failures) > 0 || createdBy != 1 || len(ids) != 1 {
				t.Fatalf("race: created by %d callers, %d distinct keys, failures %v; want 1, 1, none", createdBy, len(ids), failures)
			}
			n, err := momo.Select[rbTag](db).Count(ctx)
			if err != nil || n != 3 {
				t.Fatalf("tags = %d, %v; want 3 (go, tx, race)", n, err)
			}
		})
	}
}

// Catches: an upsert on UNIQUE(email) with an auto-increment primary key
// refused on MySQL or MariaDB, writing a second row instead of updating the
// first, and the unsafe targets (the primary key next to UNIQUE(email), a
// second ordinary unique key) being accepted.
func TestUpsertSecondaryTargetMySQLFamily(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			ctx := context.Background()
			db := s.open(t)
			rbSchema(t, db)

			u := rbUser{Email: "a@example.com", Name: "first"}
			if _, err := momo.Upsert[rbUser](db).Model(&u).OnConflict(rbUserEmail).DoUpdate().Exec(ctx); err != nil {
				t.Fatalf("insert branch: %v", err)
			}
			w := rbUser{Email: "a@example.com", Name: "second"}
			if _, err := momo.Upsert[rbUser](db).Model(&w).OnConflict(rbUserEmail).DoUpdate().Exec(ctx); err != nil {
				t.Fatalf("update branch: %v", err)
			}
			all, err := momo.Select[rbUser](db).All(ctx)
			if err != nil || len(all) != 1 || all[0].Name != "second" || all[0].Email != "a@example.com" {
				t.Fatalf("rows = %+v, %v; want the one row updated to second", all, err)
			}

			// A different email inserts a new row.
			x := rbUser{Email: "b@example.com", Name: "b"}
			if _, err := momo.Upsert[rbUser](db).Model(&x).OnConflict(rbUserEmail).DoUpdate().Exec(ctx); err != nil {
				t.Fatal(err)
			}
			if n, err := momo.Select[rbUser](db).Count(ctx); err != nil || n != 2 {
				t.Fatalf("rows = %d, %v; want 2", n, err)
			}

			// Refused: the primary key as target while UNIQUE(email) exists.
			pk := rbUser{ID: 1, Email: "a@example.com", Name: "z"}
			_, err = momo.Upsert[rbUser](db).Model(&pk).OnConflict(rbUserID).DoUpdate().Exec(ctx)
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("primary key target next to UNIQUE(email): err = %v, want ErrUnsupported", err)
			}
			// Refused: a second ordinary unique key the insert can reach.
			pr := rbPair{Email: "a@example.com", Name: "n"}
			_, err = momo.Upsert[rbPair](db).Model(&pr).OnConflict(rbPairEmail).DoUpdate().Exec(ctx)
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("second ordinary unique key: err = %v, want ErrUnsupported", err)
			}
			if n, err := momo.Select[rbPair](db).Count(ctx); err != nil || n != 0 {
				t.Fatalf("a refused upsert wrote %d rows, %v", n, err)
			}
		})
	}
}

// Catches: FindOrCreate hiding a unique violation on another key than the one
// it was given (returning ErrNotFound or a created row), or leaving a row
// behind.
func TestFindOrCreateOtherKeyViolationMySQLFamily(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			ctx := context.Background()
			db := s.open(t)
			rbSchema(t, db)
			first := rbUser{Email: "a@example.com", Name: "x"}
			if _, err := momo.Insert[rbUser](db).Model(&first).Exec(ctx); err != nil {
				t.Fatal(err)
			}
			// The key is name, which is not unique; the insert collides on email.
			clash := rbUser{Email: "a@example.com", Name: "y"}
			created, err := momo.FindOrCreate[rbUser, *rbUser](ctx, db, []momo.ColumnRef{rbUserName}, &clash)
			if !errors.Is(err, momo.ErrUniqueViolation) || created {
				t.Fatalf("created=%v err=%v, want ErrUniqueViolation", created, err)
			}
			if n, err := momo.Select[rbUser](db).Count(ctx); err != nil || n != 1 {
				t.Fatalf("users = %d, %v; want the one original row", n, err)
			}
		})
	}
}

// Catches: FindOrCreate returning or resurrecting a soft-deleted row on MySQL
// or MariaDB instead of failing with ErrUniqueViolation.
func TestFindOrCreateSoftDeletedMySQLFamily(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			ctx := context.Background()
			db := s.open(t)
			rbSchema(t, db)
			key := []momo.ColumnRef{rbDocSlug}
			d := &rbDoc{Slug: "intro"}
			if created, err := momo.FindOrCreate[rbDoc, *rbDoc](ctx, db, key, d); err != nil || !created || d.ID == 0 {
				t.Fatalf("create: created=%v id=%d err=%v", created, d.ID, err)
			}
			mustExecMySQL(t, db, "UPDATE rb_docs SET deleted_at = NOW(6) WHERE id = ?", d.ID)
			again := &rbDoc{Slug: "intro"}
			created, err := momo.FindOrCreate[rbDoc, *rbDoc](ctx, db, key, again)
			if !errors.Is(err, momo.ErrUniqueViolation) || created {
				t.Fatalf("soft-deleted key: created=%v err=%v, want ErrUniqueViolation", created, err)
			}
			if again.ID != 0 {
				t.Fatalf("the model was overwritten with the deleted row: %+v", again)
			}
			rows, err := momo.Scalars[int64](ctx, db, "SELECT COUNT(*) FROM rb_docs")
			if err != nil || len(rows) != 1 || rows[0] != 1 {
				t.Fatalf("rows = %v, %v; want the one soft-deleted row untouched", rows, err)
			}
		})
	}
}
