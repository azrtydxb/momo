// SPDX-License-Identifier: Apache-2.0

// Package models holds the relation fixture's model structs. In a real
// project these are the user-written types; momo-02's generator emits the
// MomoMeta methods in momo_gen.go and the subpackages. The fixture's
// descriptors are hand-written because the generator's tag grammar cannot yet
// express composite foreign keys.
package models

import "github.com/azrtydxb/momo"

// Author writes documents.
type Author struct {
	momo.Model    `momo:"table:authors"`
	ID            int64 `momo:"pk"`
	Name          string
	Images        []Image
	FirstDocument *Document
}

// Document belongs to an optional author.
type Document struct {
	momo.Model `momo:"table:documents"`
	ID         int64 `momo:"pk"`
	AuthorID   *int64
	Title      string
	Author     *Author
	Comments   []Comment
	Notes      []Note
	Tags       []Tag
	Taggings   []Tagging
	Images     []Image
}

// Comment belongs to a document.
type Comment struct {
	momo.Model `momo:"table:comments"`
	ID         int64 `momo:"pk"`
	DocumentID int64
	Body       string
	Approved   bool
}

// Note is a soft-deletable child of a document (boolean marker).
type Note struct {
	momo.Model `momo:"table:notes"`
	ID         int64 `momo:"pk"`
	DocumentID int64
	Body       string
	Deleted    bool `momo:"softdelete"`
}

// Tag is joined to documents through Tagging.
type Tag struct {
	momo.Model `momo:"table:tags"`
	ID         int64 `momo:"pk"`
	Name       string
}

// Tagging is the explicit join model with an extra Position column.
type Tagging struct {
	momo.Model `momo:"table:taggings"`
	DocumentID int64 `momo:"pk"`
	TagID      int64 `momo:"pk"`
	Position   int64
	Tag        *Tag
}

// Image is polymorphic: OwnerType names the owner's kind, OwnerID its key.
type Image struct {
	momo.Model `momo:"table:images"`
	ID         int64 `momo:"pk"`
	OwnerType  string
	OwnerID    int64
	URL        string
}

// Employee is self-referencing through ManagerID.
type Employee struct {
	momo.Model `momo:"table:employees"`
	ID         int64 `momo:"pk"`
	ManagerID  *int64
	Name       string
	Manager    *Employee
	Reports    []Employee
}

// Region has a composite primary key.
type Region struct {
	momo.Model `momo:"table:regions"`
	TenantID   int64  `momo:"pk"`
	Code       string `momo:"pk"`
	Name       string
	Stores     []Store
}

// Store references a region by a composite, nullable-in-part key.
type Store struct {
	momo.Model `momo:"table:stores"`
	ID         int64 `momo:"pk"`
	TenantID   int64
	RegionCode *string
	Name       string
	Region     *Region
}
