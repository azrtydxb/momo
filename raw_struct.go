// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"database/sql"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/azrtydxb/momo/internal/snake"
)

// metaFor returns the metadata Query and QueryRows scan through: the
// generated metadata when T has it (a model, a projection or Row1..Row10), and
// otherwise a description of the plain struct T built by reflection once per
// type and cached. A DB with a naming preset (WithNaming) keeps its own cache,
// so the preset never has to be a map key: any value works, and the plans of a
// short-lived preset go away with its DB.
func metaFor[T any](db *DB) (*Meta[T], error) {
	if m, ok := any((*T)(nil)).(interface{ MomoMeta() *Meta[T] }); ok {
		return m.MomoMeta(), nil
	}
	if db != nil && db.naming != nil {
		return structMeta[T](db.naming, db.namingPlans)
	}
	return structMeta[T](nil, nil)
}

// structMetas caches the plans built with the default snake_case naming.
var structMetas sync.Map // reflect.Type -> *structMetaEntry

type structMetaEntry struct {
	meta any // *Meta[T]
	err  error
}

// structMeta returns the plan of T under namer, cached in cache, or in the
// process-wide cache when cache is nil.
func structMeta[T any](namer FieldNamer, cache *sync.Map) (*Meta[T], error) {
	if cache == nil {
		cache = &structMetas
	}
	rt := reflect.TypeFor[T]()
	v, ok := cache.Load(rt)
	if !ok {
		e := &structMetaEntry{}
		if m, err := buildStructMeta[T](rt, namer); err != nil {
			e.err = err
		} else {
			e.meta = m
		}
		v, _ = cache.LoadOrStore(rt, e)
	}
	e := v.(*structMetaEntry)
	if e.err != nil {
		return nil, e.err
	}
	return e.meta.(*Meta[T]), nil
}

type structField struct {
	name  string
	path  []int
	depth int
}

func buildStructMeta[T any](rt reflect.Type, namer FieldNamer) (*Meta[T], error) {
	if rt.Kind() != reflect.Struct {
		return nil, &Error{Kind: ErrValidation, Op: "raw", Err: fmt.Errorf("%s is not a struct and has no momo metadata; Query and QueryRows read into a model, a projection, Row1..Row10 or a plain struct", rt)}
	}
	var fields []structField
	if err := collectStructFields(rt, namer, nil, 0, nil, &fields); err != nil {
		return nil, err
	}
	byName := map[string]int{}
	var out []structField
	for _, f := range fields {
		i, dup := byName[f.name]
		switch {
		case !dup:
			byName[f.name] = len(out)
			out = append(out, f)
		case out[i].depth > f.depth:
			out[i] = f
		case out[i].depth == f.depth:
			return nil, &Error{Kind: ErrValidation, Op: "raw", Err: fmt.Errorf("%s: two fields map to the column %q; give one a momo:\"column:x\" tag or momo:\"-\"", rt, f.name)}
		}
	}
	if len(out) == 0 {
		return nil, &Error{Kind: ErrValidation, Op: "raw", Err: fmt.Errorf("%s has no exported fields to scan into", rt)}
	}
	cols := make([]ColumnMeta, len(out))
	paths := make([][]int, len(out))
	for i, f := range out {
		cols[i].Name = f.name
		paths[i] = f.path
	}
	return &Meta[T]{
		Columns: cols,
		Ptr: func(m *T, i int) any {
			v := reflect.ValueOf(m).Elem()
			p := paths[i]
			for k, idx := range p {
				v = v.Field(idx)
				if k < len(p)-1 && v.Kind() == reflect.Pointer {
					if v.IsNil() {
						v.Set(reflect.New(v.Type().Elem()))
					}
					v = v.Elem()
				}
			}
			return v.Addr().Interface()
		},
		Value:    func(*T, int) any { return nil },
		Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	}, nil
}

var (
	scannerType = reflect.TypeFor[sql.Scanner]()
	timeType    = reflect.TypeFor[time.Time]()
)

// collectStructFields lists the scannable fields of rt: exported, not tagged
// momo:"-", with an embedded struct flattened unless it is itself a column
// (a time.Time, a sql.Scanner, or one with a column tag).
func collectStructFields(rt reflect.Type, namer FieldNamer, prefix []int, depth int, stack []reflect.Type, out *[]structField) error {
	stack = append(stack, rt)
	for i := range rt.NumField() {
		f := rt.Field(i)
		col := ""
		if tag, ok := f.Tag.Lookup("momo"); ok {
			if tag == "-" {
				continue
			}
			for _, part := range strings.Split(tag, ",") {
				if v, ok := strings.CutPrefix(strings.TrimSpace(part), "column:"); ok {
					col = v
				}
			}
		}
		path := append(append([]int(nil), prefix...), i)
		if f.Anonymous && col == "" {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				if !f.IsExported() {
					continue // reflection cannot allocate an unexported embedded pointer
				}
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct && !isLeafStruct(ft) {
				if slices.Contains(stack, ft) {
					return &Error{Kind: ErrValidation, Op: "raw", Err: fmt.Errorf("%s: embedded cycle through %s; tag the embedded field momo:\"-\"", stack[0], ft)}
				}
				if err := collectStructFields(ft, namer, path, depth+1, stack, out); err != nil {
					return err
				}
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if col == "" {
			if namer != nil {
				col = namer.ColumnName(f.Name)
			} else {
				col = snake.ToSnake(f.Name)
			}
		}
		*out = append(*out, structField{name: col, path: path, depth: depth})
	}
	return nil
}

func isLeafStruct(t reflect.Type) bool {
	return t == timeType || reflect.PointerTo(t).Implements(scannerType)
}
