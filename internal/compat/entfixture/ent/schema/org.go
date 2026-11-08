// SPDX-License-Identifier: Apache-2.0

// Package schema is the Ent schema of the importer fixture. The importer
// reads these files with go/parser, and the fixture test compares the schema
// Ent's own migration creates from them with the schema Momo derives from the
// imported models.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// Org is a tenant.
type Org struct{ ent.Schema }

// Mixin adds created_at and updated_at.
func (Org) Mixin() []ent.Mixin {
	return []ent.Mixin{mixin.Time{}}
}

// Fields of Org.
func (Org) Fields() []ent.Field {
	return []ent.Field{
		field.String("slug").Unique(),
		field.String("display_name").MaxLen(120).Optional(),
	}
}

// Edges of Org.
func (Org) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("users", User.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}
