// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"reflect"
	"strconv"
	"sync"
)

// tupleMetas caches one *Meta per tuple instantiation, keyed by the row's
// reflect.Type. Select asks for the meta on every query, so it must be cheap
// and must return the same pointer every time.
var tupleMetas sync.Map // reflect.Type -> *Meta[T]

func loadTupleMeta[T any]() *Meta[T] {
	if v, ok := tupleMetas.Load(reflect.TypeFor[T]()); ok {
		return v.(*Meta[T])
	}
	return nil
}

func storeTupleMeta[T any](n int, ptr func(m *T, i int) any) *Meta[T] {
	cols := make([]ColumnMeta, n)
	for i := range cols {
		cols[i].Name = "c" + strconv.Itoa(i+1)
	}
	m := &Meta[T]{
		Columns: cols,
		Ptr:     ptr,
		Value:   func(*T, int) any { return nil },
		// A tuple has no identity, version, soft-delete or timestamp column.
		Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	}
	v, _ := tupleMetas.LoadOrStore(reflect.TypeFor[T](), m)
	return v.(*Meta[T])
}

// Row1 is a result row of 1 column, scanned positionally into C1. Row1 to
// Row10 are the tuple rows: generic result types for queries that do not read a whole
// model: aggregates, joins and column subsets. They replace a hand-written
// projection struct and Meta per result shape:
//
//	rows, err := momo.Select[momo.Row2[string, int64]](db).
//		From(document.Table).
//		Columns(document.Title, momo.CountAll()).
//		GroupBy(document.Title).
//		All(ctx)
//	// rows[i].C1 is the title, rows[i].C2 the count
//
// Row1 to Row10 take 1 to 10 columns. The Columns list is scanned
// positionally: expression i fills field Ci, so the number of Columns must
// equal the row's arity (ErrValidation otherwise, at SQL or execution time).
// Names play no part. Use a pointer type parameter, such as
// Row2[string, *int64], for a column that can be NULL (a LEFT JOIN side, an
// aggregate over no rows).
//
// A tuple row is a projection: it has no table, so Insert, Update and Delete
// refuse it with ErrValidation, and it carries no soft-delete column of its
// own. As for any projection the live-row filter comes from the From table
// (and WithDeleted or OnlyDeleted adjust it). After works when OrderBy names
// a unique key, because a tuple has no primary key to break ties with.
type Row1[A any] struct {
	Projection
	C1 A
}

// MomoMeta returns the shared metadata of this instantiation.
func (*Row1[A]) MomoMeta() *Meta[Row1[A]] {
	if m := loadTupleMeta[Row1[A]](); m != nil {
		return m
	}
	return storeTupleMeta(1, func(m *Row1[A], i int) any {
		switch i {
		case 0:
			return &m.C1
		}
		return nil
	})
}

// Row2 is a result row of 2 columns, scanned positionally into C1..C2.
// Use a pointer type parameter for a column that can be NULL; see Row1.
type Row2[A, B any] struct {
	Projection
	C1 A
	C2 B
}

// MomoMeta returns the shared metadata of this instantiation.
func (*Row2[A, B]) MomoMeta() *Meta[Row2[A, B]] {
	if m := loadTupleMeta[Row2[A, B]](); m != nil {
		return m
	}
	return storeTupleMeta(2, func(m *Row2[A, B], i int) any {
		switch i {
		case 0:
			return &m.C1
		case 1:
			return &m.C2
		}
		return nil
	})
}

// Row3 is a result row of 3 columns, scanned positionally into C1..C3.
// Use a pointer type parameter for a column that can be NULL; see Row1.
type Row3[A, B, C any] struct {
	Projection
	C1 A
	C2 B
	C3 C
}

// MomoMeta returns the shared metadata of this instantiation.
func (*Row3[A, B, C]) MomoMeta() *Meta[Row3[A, B, C]] {
	if m := loadTupleMeta[Row3[A, B, C]](); m != nil {
		return m
	}
	return storeTupleMeta(3, func(m *Row3[A, B, C], i int) any {
		switch i {
		case 0:
			return &m.C1
		case 1:
			return &m.C2
		case 2:
			return &m.C3
		}
		return nil
	})
}

// Row4 is a result row of 4 columns, scanned positionally into C1..C4.
// Use a pointer type parameter for a column that can be NULL; see Row1.
type Row4[A, B, C, D any] struct {
	Projection
	C1 A
	C2 B
	C3 C
	C4 D
}

// MomoMeta returns the shared metadata of this instantiation.
func (*Row4[A, B, C, D]) MomoMeta() *Meta[Row4[A, B, C, D]] {
	if m := loadTupleMeta[Row4[A, B, C, D]](); m != nil {
		return m
	}
	return storeTupleMeta(4, func(m *Row4[A, B, C, D], i int) any {
		switch i {
		case 0:
			return &m.C1
		case 1:
			return &m.C2
		case 2:
			return &m.C3
		case 3:
			return &m.C4
		}
		return nil
	})
}

// Row5 is a result row of 5 columns, scanned positionally into C1..C5.
// Use a pointer type parameter for a column that can be NULL; see Row1.
type Row5[A, B, C, D, E any] struct {
	Projection
	C1 A
	C2 B
	C3 C
	C4 D
	C5 E
}

// MomoMeta returns the shared metadata of this instantiation.
func (*Row5[A, B, C, D, E]) MomoMeta() *Meta[Row5[A, B, C, D, E]] {
	if m := loadTupleMeta[Row5[A, B, C, D, E]](); m != nil {
		return m
	}
	return storeTupleMeta(5, func(m *Row5[A, B, C, D, E], i int) any {
		switch i {
		case 0:
			return &m.C1
		case 1:
			return &m.C2
		case 2:
			return &m.C3
		case 3:
			return &m.C4
		case 4:
			return &m.C5
		}
		return nil
	})
}

// Row6 is a result row of 6 columns, scanned positionally into C1..C6.
// Use a pointer type parameter for a column that can be NULL; see Row1.
type Row6[A, B, C, D, E, F any] struct {
	Projection
	C1 A
	C2 B
	C3 C
	C4 D
	C5 E
	C6 F
}

// MomoMeta returns the shared metadata of this instantiation.
func (*Row6[A, B, C, D, E, F]) MomoMeta() *Meta[Row6[A, B, C, D, E, F]] {
	if m := loadTupleMeta[Row6[A, B, C, D, E, F]](); m != nil {
		return m
	}
	return storeTupleMeta(6, func(m *Row6[A, B, C, D, E, F], i int) any {
		switch i {
		case 0:
			return &m.C1
		case 1:
			return &m.C2
		case 2:
			return &m.C3
		case 3:
			return &m.C4
		case 4:
			return &m.C5
		case 5:
			return &m.C6
		}
		return nil
	})
}

// Row7 is a result row of 7 columns, scanned positionally into C1..C7.
// Use a pointer type parameter for a column that can be NULL; see Row1.
type Row7[A, B, C, D, E, F, G any] struct {
	Projection
	C1 A
	C2 B
	C3 C
	C4 D
	C5 E
	C6 F
	C7 G
}

// MomoMeta returns the shared metadata of this instantiation.
func (*Row7[A, B, C, D, E, F, G]) MomoMeta() *Meta[Row7[A, B, C, D, E, F, G]] {
	if m := loadTupleMeta[Row7[A, B, C, D, E, F, G]](); m != nil {
		return m
	}
	return storeTupleMeta(7, func(m *Row7[A, B, C, D, E, F, G], i int) any {
		switch i {
		case 0:
			return &m.C1
		case 1:
			return &m.C2
		case 2:
			return &m.C3
		case 3:
			return &m.C4
		case 4:
			return &m.C5
		case 5:
			return &m.C6
		case 6:
			return &m.C7
		}
		return nil
	})
}

// Row8 is a result row of 8 columns, scanned positionally into C1..C8.
// Use a pointer type parameter for a column that can be NULL; see Row1.
type Row8[A, B, C, D, E, F, G, H any] struct {
	Projection
	C1 A
	C2 B
	C3 C
	C4 D
	C5 E
	C6 F
	C7 G
	C8 H
}

// MomoMeta returns the shared metadata of this instantiation.
func (*Row8[A, B, C, D, E, F, G, H]) MomoMeta() *Meta[Row8[A, B, C, D, E, F, G, H]] {
	if m := loadTupleMeta[Row8[A, B, C, D, E, F, G, H]](); m != nil {
		return m
	}
	return storeTupleMeta(8, func(m *Row8[A, B, C, D, E, F, G, H], i int) any {
		switch i {
		case 0:
			return &m.C1
		case 1:
			return &m.C2
		case 2:
			return &m.C3
		case 3:
			return &m.C4
		case 4:
			return &m.C5
		case 5:
			return &m.C6
		case 6:
			return &m.C7
		case 7:
			return &m.C8
		}
		return nil
	})
}

// Row9 is a result row of 9 columns, scanned positionally into C1..C9.
// Use a pointer type parameter for a column that can be NULL; see Row1.
type Row9[A, B, C, D, E, F, G, H, I any] struct {
	Projection
	C1 A
	C2 B
	C3 C
	C4 D
	C5 E
	C6 F
	C7 G
	C8 H
	C9 I
}

// MomoMeta returns the shared metadata of this instantiation.
func (*Row9[A, B, C, D, E, F, G, H, I]) MomoMeta() *Meta[Row9[A, B, C, D, E, F, G, H, I]] {
	if m := loadTupleMeta[Row9[A, B, C, D, E, F, G, H, I]](); m != nil {
		return m
	}
	return storeTupleMeta(9, func(m *Row9[A, B, C, D, E, F, G, H, I], i int) any {
		switch i {
		case 0:
			return &m.C1
		case 1:
			return &m.C2
		case 2:
			return &m.C3
		case 3:
			return &m.C4
		case 4:
			return &m.C5
		case 5:
			return &m.C6
		case 6:
			return &m.C7
		case 7:
			return &m.C8
		case 8:
			return &m.C9
		}
		return nil
	})
}

// Row10 is a result row of 10 columns, scanned positionally into C1..C10.
// Use a pointer type parameter for a column that can be NULL; see Row1.
type Row10[A, B, C, D, E, F, G, H, I, J any] struct {
	Projection
	C1  A
	C2  B
	C3  C
	C4  D
	C5  E
	C6  F
	C7  G
	C8  H
	C9  I
	C10 J
}

// MomoMeta returns the shared metadata of this instantiation.
func (*Row10[A, B, C, D, E, F, G, H, I, J]) MomoMeta() *Meta[Row10[A, B, C, D, E, F, G, H, I, J]] {
	if m := loadTupleMeta[Row10[A, B, C, D, E, F, G, H, I, J]](); m != nil {
		return m
	}
	return storeTupleMeta(10, func(m *Row10[A, B, C, D, E, F, G, H, I, J], i int) any {
		switch i {
		case 0:
			return &m.C1
		case 1:
			return &m.C2
		case 2:
			return &m.C3
		case 3:
			return &m.C4
		case 4:
			return &m.C5
		case 5:
			return &m.C6
		case 6:
			return &m.C7
		case 7:
			return &m.C8
		case 8:
			return &m.C9
		case 9:
			return &m.C10
		}
		return nil
	})
}
