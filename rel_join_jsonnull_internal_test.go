// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"reflect"
	"testing"

	"github.com/azrtydxb/momo/types"
)

type jjDoc struct{ A int }

type jjChild struct {
	ID  int64
	Doc types.JSON[jjDoc]
}

type jjParent struct {
	ID    int64
	Child *jjChild
}

var jjChildMeta = Meta[jjChild]{
	Table:   "jj_child",
	Columns: []ColumnMeta{{Name: "id"}, {Name: "doc"}},
	Ptr: func(m *jjChild, i int) any {
		return []any{&m.ID, &m.Doc}[i]
	},
	Value:    func(m *jjChild, i int) any { return nil },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

var jjRel = Relation[jjParent, jjChild]{
	Name: "Child", Kind: BelongsTo,
	Parent: TableRef{Name: "jj_parent"}, Child: TableRef{Name: "jj_child"},
	ChildMeta:  &jjChildMeta,
	ParentCols: []string{"child_id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
	SetOne: func(p *jjParent, c *jjChild) { p.Child = c },
}

// jjScan mimics a driver that, like the pgx driver, scans a JSON null into a
// **JSON[T] holder as a non-nil pointer to a value that reports IsNull: id gets
// 7 and the holder in dst[1] is filled from src (nil means SQL NULL).
func jjScan(t *testing.T, dst []any, src any) {
	t.Helper()
	id := reflect.ValueOf(dst[0]).Elem()
	id.Set(reflect.New(id.Type().Elem()))
	id.Elem().SetInt(7)
	if src == nil {
		return
	}
	h := reflect.ValueOf(dst[1]).Elem()
	v := reflect.New(h.Type().Elem())
	if err := v.Interface().(interface{ Scan(any) error }).Scan(src); err != nil {
		t.Fatal(err)
	}
	h.Set(v)
}

// Catches: a non-pointer types.JSON[T] column of a joined child failing, or
// losing its JSON null state, when the driver hands the **JSON holder a
// non-nil pointer for a JSON null (it used to be nil, which assign treated as
// SQL NULL and rejected).
func TestJoinBinderJSONNullInNonPointerColumn(t *testing.T) {
	j := joinedLoader[jjParent, jjChild]{jjRel}
	bind := j.joinBinder()
	dst := make([]any, j.joinWidth())

	bind.bind(dst)
	jjScan(t, dst, "null")
	var p jjParent
	if err := bind.assign(&p); err != nil || p.Child == nil {
		t.Fatalf("JSON null: err=%v child=%+v", err, p.Child)
	}
	if !p.Child.Doc.IsNull() {
		t.Fatalf("joined JSON null lost: %+v", p.Child.Doc)
	}
	if v, _ := p.Child.Doc.Value(); v != "null" {
		t.Fatalf("Value = %v, want null", v)
	}

	bind.bind(dst)
	jjScan(t, dst, `{"A":3}`)
	var q jjParent
	if err := bind.assign(&q); err != nil || q.Child == nil || q.Child.Doc.IsNull() || q.Child.Doc.V.A != 3 {
		t.Fatalf("object: err=%v child=%+v", err, q.Child)
	}
	if !p.Child.Doc.IsNull() {
		t.Fatal("a later row changed the earlier child")
	}

	// SQL NULL in a non-nullable column of a matched row stays an error.
	bind.bind(dst)
	jjScan(t, dst, nil)
	var r jjParent
	if err := bind.assign(&r); err == nil || r.Child != nil {
		t.Fatalf("SQL NULL: err=%v child=%+v", err, r.Child)
	}
}
