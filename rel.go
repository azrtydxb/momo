// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"
)

// RelationKind is the shape of a relation.
type RelationKind uint8

const (
	// BelongsTo has its foreign key on the parent; the child is matched by its key.
	BelongsTo RelationKind = iota + 1
	// HasOne has its foreign key on the child; at most one child per parent.
	HasOne
	// HasMany has its foreign key on the child; any number of children per parent.
	HasMany
	// ManyToMany goes through a join table between parent and child.
	ManyToMany
)

// String returns the lower-case name of the relation kind.
func (k RelationKind) String() string {
	switch k {
	case BelongsTo:
		return "belongs-to"
	case HasOne:
		return "has-one"
	case HasMany:
		return "has-many"
	case ManyToMany:
		return "many-to-many"
	}
	return fmt.Sprintf("RelationKind(%d)", uint8(k))
}

// Through names the join table of a ManyToMany relation.
type Through struct {
	Table      TableRef
	ParentCols []string // join columns referencing Relation.ParentCols, same order
	ChildCols  []string // join columns referencing Relation.ChildCols, same order
}

// Discriminator restricts a polymorphic has-one/has-many to rows whose type
// column equals Value.
type Discriminator struct {
	Column string
	Value  any
}

// Relation describes relation Name from parent P to child C. Generated code
// emits one value per relation field. Relation values are immutable and safe
// for concurrent use.
type Relation[P, C any] struct {
	Name          string
	Kind          RelationKind
	Parent        TableRef
	Child         TableRef
	ParentMeta    *Meta[P]
	ChildMeta     *Meta[C]
	ParentCols    []string
	ChildCols     []string
	ChildPK       []string
	Through       *Through
	Discriminator *Discriminator
	ParentKey     func(p *P) ([]any, bool)
	ChildKey      func(c *C) ([]any, bool)
	SetParentKey  func(p *P, key []any)
	SetChildKey   func(c *C, key []any)
	GetOne        func(p *P) *C
	SetOne        func(p *P, c *C)
	GetMany       func(p *P) []C
	SetMany       func(p *P, cs []C)
}

func (r *Relation[P, C]) invalid(msg string) error {
	return newError(ErrValidation, "relation", r.Child.Name, errors.New(r.Name+": "+msg))
}

// validate checks the descriptor's shape. Every loader calls it before SQL is built.
func (r *Relation[P, C]) validate() error {
	switch {
	case r.Name == "":
		return r.invalid("relation has no name")
	case len(r.ParentCols) == 0:
		return r.invalid("no parent columns")
	case r.ParentKey == nil || r.ChildKey == nil:
		return r.invalid("ParentKey and ChildKey are required")
	case r.Kind == ManyToMany && r.Through == nil:
		return r.invalid("many-to-many requires Through")
	case r.Kind != ManyToMany && r.Through != nil:
		return r.invalid("Through is only valid for many-to-many")
	case r.Discriminator != nil && (r.Kind == BelongsTo || r.Kind == ManyToMany):
		return r.invalid("Discriminator is only valid for has-one and has-many")
	case r.Through != nil && (len(r.Through.ParentCols) != len(r.ParentCols) || len(r.Through.ChildCols) != len(r.ChildCols)):
		return r.invalid("Through column arity must match the relation key arity")
	case r.Through == nil && len(r.ChildCols) != len(r.ParentCols):
		return r.invalid("parent and child key arity differ")
	}
	switch r.Kind {
	case BelongsTo, HasOne:
		if r.GetOne == nil || r.SetOne == nil {
			return r.invalid("to-one relations require GetOne and SetOne")
		}
	case HasMany, ManyToMany:
		if r.GetMany == nil || r.SetMany == nil {
			return r.invalid("to-many relations require GetMany and SetMany")
		}
	default:
		return r.invalid("unknown relation kind")
	}
	return nil
}
