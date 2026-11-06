// SPDX-License-Identifier: Apache-2.0

// Package models is the schema of the relations example: authors write
// documents, documents have comments and are tagged with many tags.
//
// A relation is a field whose tag names its kind. The field's Go type has to
// match the kind: a pointer for belongs_to and has_one, a slice for has_many
// and many2many. The generator turns each one into a descriptor such as
// document.Comments, and `momo migrate diff` adds the foreign keys and the
// join table.
package models

import "github.com/azrtydxb/momo"

// Author writes documents. Name is unique so FindOrCreate can key on it.
type Author struct {
	momo.Model `momo:"table:authors"`
	ID         int64  `momo:"pk,identity"`
	Name       string `momo:"size:100,notnull,unique"`

	// has_many: the foreign key (AuthorID) lives on the child, Document.
	Documents []Document `momo:"has_many,fk:AuthorID"`
}

// Document belongs to an Author, has many Comments and many Tags.
type Document struct {
	momo.Model `momo:"table:documents"`
	ID         int64  `momo:"pk,identity"`
	AuthorID   *int64 `momo:"index"` // nullable: a document may have no author
	Title      string `momo:"size:200,notnull"`

	// belongs_to: the foreign key (AuthorID) lives on this model.
	Author *Author `momo:"belongs_to,fk:AuthorID"`
	// has_many: the foreign key (DocumentID) lives on Comment.
	Comments []Comment `momo:"has_many,fk:DocumentID"`
	// many2many: both sides name the same join table.
	Tags []Tag `momo:"many2many,join:taggings"`
}

// Comment is a reader's remark on a document.
type Comment struct {
	momo.Model `momo:"table:comments"`
	ID         int64  `momo:"pk,identity"`
	DocumentID int64  `momo:"notnull,index"`
	Body       string `momo:"size:500,notnull"`
	Approved   bool   `momo:"notnull"`
}

// Tag labels documents. The other side of Document.Tags.
type Tag struct {
	momo.Model `momo:"table:tags"`
	ID         int64  `momo:"pk,identity"`
	Name       string `momo:"size:50,notnull,unique"`

	Documents []Document `momo:"many2many,join:taggings"`
}
