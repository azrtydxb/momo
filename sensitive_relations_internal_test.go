// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

const relSecret = "owner-secret-key"

type secParent struct{ ID string }
type secChild struct {
	ID    int64
	Owner string
}

// relDriver records statements and returns no rows.
type relDriver struct {
	d     *dialect.Dialect
	mu    sync.Mutex
	calls map[string][]any
}

func (r *relDriver) Dialect() *dialect.Dialect { return r.d }
func (r *relDriver) note(sql string, args []any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.calls == nil {
		r.calls = map[string][]any{}
	}
	r.calls[sql] = slices.Clone(args)
}
func (r *relDriver) Query(_ context.Context, sql string, args []any) (Rows, error) {
	r.note(sql, args)
	return emptyRows{}, nil
}
func (r *relDriver) Exec(_ context.Context, sql string, args []any) (int64, error) {
	r.note(sql, args)
	return 0, nil
}
func (r *relDriver) Begin(context.Context, TxOptions) (DriverTx, error) { return relTx{r}, nil }

type relTx struct{ r *relDriver }

func (t relTx) Query(ctx context.Context, sql string, args []any) (Rows, error) {
	return t.r.Query(ctx, sql, args)
}
func (t relTx) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	return t.r.Exec(ctx, sql, args)
}
func (relTx) Commit(context.Context) error   { return nil }
func (relTx) Rollback(context.Context) error { return nil }

type emptyRows struct{}

func (emptyRows) Next() bool        { return false }
func (emptyRows) Scan(...any) error { return nil }
func (emptyRows) Err() error        { return nil }
func (emptyRows) Close() error      { return nil }

type relRecorder struct {
	mu     sync.Mutex
	events []QueryEvent
}

func (h *relRecorder) BeforeQuery(ctx context.Context, e *QueryEvent) context.Context { return ctx }
func (h *relRecorder) AfterQuery(_ context.Context, e *QueryEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, *e)
}

// secRelation builds a relation whose join columns are sensitive on both sides.
func secRelation(kind RelationKind) Relation[secParent, secChild] {
	return secRelationWith(kind, FlagSensitive, FlagSensitive)
}

// secRelationWith sets the flags of the parent key column and of the child
// join column, so a test can make only one side sensitive.
func secRelationWith(kind RelationKind, parentFlag, childFlag uint16) Relation[secParent, secChild] {
	pm := &Meta[secParent]{Table: "parents", Columns: []ColumnMeta{{Name: "id", Flags: FlagPK | parentFlag}}, SoftDelete: -1,
		Value: func(p *secParent, _ int) any { return p.ID }}
	cm := &Meta[secChild]{Table: "kids", SoftDelete: -1, Columns: []ColumnMeta{{Name: "id", Flags: FlagPK}, {Name: "owner", Flags: childFlag}},
		Value: func(c *secChild, i int) any {
			if i == 0 {
				return c.ID
			}
			return c.Owner
		}}
	r := Relation[secParent, secChild]{
		Name: "Kids", Kind: kind,
		Parent: TableRef{Name: "parents"}, Child: TableRef{Name: "kids"},
		ParentMeta: pm, ChildMeta: cm,
		ParentCols: []string{"id"}, ChildCols: []string{"owner"}, ChildPK: []string{"id"},
		ParentKey:   func(p *secParent) ([]any, bool) { return []any{p.ID}, true },
		ChildKey:    func(c *secChild) ([]any, bool) { return []any{c.Owner}, true },
		SetChildKey: func(c *secChild, k []any) {},
		GetMany:     func(p *secParent) []secChild { return nil },
		SetMany:     func(p *secParent, cs []secChild) {},
	}
	if kind == ManyToMany {
		r.ChildCols = []string{"owner"}
		r.Through = &Through{Table: TableRef{Name: "jt"}, ParentCols: []string{"p"}, ChildCols: []string{"c"}}
	}
	return r
}

func TestRelationLoadQueriesMarkSensitiveKeys(t *testing.T) {
	cases := map[string]struct {
		kind  RelationKind
		limit int
	}{
		"has many":                {HasMany, 0},
		"has many per-parent cap": {HasMany, 2},
		"many to many":            {ManyToMany, 0},
		"many to many per-parent": {ManyToMany, 2},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := secRelation(c.kind)
			cfg := &loadConfig{perParentLimit: c.limit}
			qs, err := r.childQueries(limitedQuerier{d: dialect.For(dialect.Postgres)}, [][]any{{relSecret}}, cfg)
			if err != nil || len(qs) == 0 {
				t.Fatalf("childQueries: %v %d", err, len(qs))
			}
			for _, q := range qs {
				for i, a := range q.Args {
					masked := slices.Contains(q.sens, i)
					if a == relSecret && !masked {
						t.Errorf("arg %d holds the key but is not marked: %s", i, q.SQL)
					}
					if a != relSecret && masked {
						t.Errorf("arg %d is marked but is not the key", i)
					}
				}
				if !slices.Contains(q.Args, any(relSecret)) {
					t.Errorf("key missing from args: %v", q.Args)
				}
			}
		})
	}
}

// TestAssociationWritesMaskSensitiveKeys runs every association statement and
// checks hook events against what the driver received.
func TestAssociationWritesMaskSensitiveKeys(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []RelationKind{HasMany, ManyToMany} {
		kindName := map[RelationKind]string{HasMany: "has many", ManyToMany: "many to many"}[kind]
		ops := map[string]func(r Relation[secParent, secChild], db *DB) error{
			"link": func(r Relation[secParent, secChild], db *DB) error {
				return r.Link(ctx, db, &secParent{ID: relSecret}, &secChild{ID: 7, Owner: relSecret})
			},
			"unlink": func(r Relation[secParent, secChild], db *DB) error {
				return r.Unlink(ctx, db, &secParent{ID: relSecret}, &secChild{ID: 7, Owner: relSecret})
			},
			"replace": func(r Relation[secParent, secChild], db *DB) error {
				return r.Replace(ctx, db, &secParent{ID: relSecret}, &secChild{ID: 7, Owner: relSecret})
			},
			"count": func(r Relation[secParent, secChild], db *DB) error {
				_, err := r.Count(ctx, db, &secParent{ID: relSecret})
				return err
			},
		}
		for opName, op := range ops {
			t.Run(kindName+"/"+opName, func(t *testing.T) {
				drv := &relDriver{d: dialect.For(dialect.Postgres)}
				rec := &relRecorder{}
				db := New(drv, WithHook(rec), WithArgLogging())
				if err := op(secRelation(kind), db); err != nil {
					t.Fatal(err)
				}
				secrets := 0
				for _, e := range rec.events {
					sent, ok := drv.calls[e.SQL]
					if !ok || len(sent) == 0 {
						continue
					}
					for i, a := range sent {
						if a == relSecret {
							secrets++
							if e.Args[i] != RedactedArg {
								t.Errorf("%s: hook saw %v at %d, want the placeholder", e.SQL, e.Args[i], i)
							}
						} else if e.Args[i] != a {
							t.Errorf("%s: hook saw %v at %d, driver got %v", e.SQL, e.Args[i], i, a)
						}
					}
				}
				if secrets == 0 {
					t.Fatal("no statement carried the key to the driver")
				}
			})
		}
	}
}

// TestJoinTableAsymmetricKeysMaskOnlyTheSensitiveSide uses a sensitive column
// on one side of a many-to-many relation only: each statement must mask the
// value at the sensitive column's position and leave the other visible.
func TestJoinTableAsymmetricKeysMaskOnlyTheSensitiveSide(t *testing.T) {
	ctx := context.Background()
	const parentVal, childVal = "parent-key-value", "child-key-value"
	sides := map[string]struct {
		parentFlag, childFlag uint16
		masked, visible       string
	}{
		"plain parent, sensitive child": {0, FlagSensitive, childVal, parentVal},
		"sensitive parent, plain child": {FlagSensitive, 0, parentVal, childVal},
	}
	ops := map[string]func(r Relation[secParent, secChild], db *DB) error{
		"link": func(r Relation[secParent, secChild], db *DB) error {
			return r.Link(ctx, db, &secParent{ID: parentVal}, &secChild{ID: 7, Owner: childVal})
		},
		"unlink": func(r Relation[secParent, secChild], db *DB) error {
			return r.Unlink(ctx, db, &secParent{ID: parentVal}, &secChild{ID: 7, Owner: childVal})
		},
		"replace": func(r Relation[secParent, secChild], db *DB) error {
			return r.Replace(ctx, db, &secParent{ID: parentVal}, &secChild{ID: 7, Owner: childVal})
		},
	}
	for sideName, side := range sides {
		for opName, op := range ops {
			t.Run(sideName+"/"+opName, func(t *testing.T) {
				drv := &relDriver{d: dialect.For(dialect.Postgres)}
				rec := &relRecorder{}
				db := New(drv, WithHook(rec), WithArgLogging())
				if err := op(secRelationWith(ManyToMany, side.parentFlag, side.childFlag), db); err != nil {
					t.Fatal(err)
				}
				maskedSeen := false
				for _, e := range rec.events {
					sent := drv.calls[e.SQL]
					for i, a := range sent {
						switch a {
						case side.masked:
							maskedSeen = true
							if e.Args[i] != RedactedArg {
								t.Errorf("%s: %v leaked at %d", e.SQL, e.Args[i], i)
							}
						case side.visible:
							if e.Args[i] != a {
								t.Errorf("%s: the plain key was masked at %d", e.SQL, i)
							}
						}
					}
				}
				if !maskedSeen {
					t.Fatal("the sensitive key never reached the driver")
				}
			})
		}
	}
}
