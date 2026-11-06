// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type jbChild struct {
	ID    int64
	Name  string
	Note  *string
	Count int64
	OK    bool
	Tag   string
}

type jbParent struct {
	ID    int64
	Child *jbChild
}

var jbChildMeta = Meta[jbChild]{
	Table: "jb_child",
	Columns: []ColumnMeta{{Name: "id", Flags: FlagPK}, {Name: "name"}, {Name: "note", Flags: FlagNullable},
		{Name: "count"}, {Name: "ok"}, {Name: "tag"}},
	Ptr: func(m *jbChild, i int) any {
		return []any{&m.ID, &m.Name, &m.Note, &m.Count, &m.OK, &m.Tag}[i]
	},
	Value:    func(m *jbChild, i int) any { return nil },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

var jbRel = Relation[jbParent, jbChild]{
	Name: "Child", Kind: BelongsTo,
	Parent: TableRef{Name: "jb_parent"}, Child: TableRef{Name: "jb_child"},
	ChildMeta:  &jbChildMeta,
	ParentCols: []string{"child_id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
	SetOne: func(p *jbParent, c *jbChild) { p.Child = c },
}

// fillScan mimics a driver scanning one non-NULL row into dst: a **E
// destination gets a fresh E, a *E destination (a nullable pointer field)
// gets a fresh pointee.
func fillScan(dst []any) {
	for _, d := range dst {
		v := reflect.ValueOf(d).Elem()
		v.Set(reflect.New(v.Type().Elem()))
		if e := v.Elem(); e.Kind() == reflect.Int64 {
			e.SetInt(7)
		}
	}
}

// BenchmarkJoinBinderRow measures one joined row: bind, scan, assign. Record
// the allocs/op before and after changing the binder.
func BenchmarkJoinBinderRow(b *testing.B) {
	j := joinedLoader[jbParent, jbChild]{jbRel}
	bind := j.joinBinder()
	dst := make([]any, j.joinWidth())
	b.ReportAllocs()
	for b.Loop() {
		bind.bind(dst)
		fillScan(dst)
		var p jbParent
		if err := bind.assign(&p); err != nil || p.Child == nil {
			b.Fatal("assign", err)
		}
	}
}

// TestJoinBinderNullColumnIsNamed checks that a NULL in a non-nullable column of
// a matched row fails with an error naming the child column.
func TestJoinBinderNullColumnIsNamed(t *testing.T) {
	j := joinedLoader[jbParent, jbChild]{jbRel}
	bind := j.joinBinder()
	dst := make([]any, j.joinWidth())
	bind.bind(dst)
	fillScan(dst)
	reflect.ValueOf(dst[3]).Elem().SetZero() // "count" is NULL, the key matched
	var p jbParent
	err := bind.assign(&p)
	var me *Error
	if !errors.As(err, &me) || me.Kind != ErrValidation {
		t.Fatalf("want ErrValidation, got %v", err)
	}
	if me.Column != "count" || me.Table != "jb_child" || !strings.Contains(err.Error(), "column=count") {
		t.Fatalf("error must name the column: %v (column=%q table=%q)", err, me.Column, me.Table)
	}
	if p.Child != nil {
		t.Fatalf("a failed row must not set the child: %+v", p.Child)
	}
}

// TestJoinBinderRowsDoNotShare checks that consecutive rows scanned through
// one binder yield independent children: no shared struct and no shared
// pointee behind a nullable field.
func TestJoinBinderRowsDoNotShare(t *testing.T) {
	j := joinedLoader[jbParent, jbChild]{jbRel}
	bind := j.joinBinder()
	dst := make([]any, j.joinWidth())
	var ps [3]jbParent
	for i := range ps {
		bind.bind(dst)
		fillScan(dst)
		if err := bind.assign(&ps[i]); err != nil {
			t.Fatal(err)
		}
		ps[i].Child.Name = string(rune('a' + i))
	}
	for i := range ps {
		for k := i + 1; k < len(ps); k++ {
			if ps[i].Child == ps[k].Child {
				t.Fatalf("rows %d and %d share a child", i, k)
			}
			if ps[i].Child.Note == ps[k].Child.Note {
				t.Fatalf("rows %d and %d share a Note pointee", i, k)
			}
		}
		if ps[i].Child.Name != string(rune('a'+i)) {
			t.Fatalf("row %d was changed by a later row: %q", i, ps[i].Child.Name)
		}
	}
}
