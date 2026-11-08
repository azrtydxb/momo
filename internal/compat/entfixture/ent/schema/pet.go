// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Pet is owned by a User through an edge field and lives in an Org.
type Pet struct{ ent.Schema }

// Fields of Pet.
func (Pet) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.Int("owner_id").Optional(),
	}
}

// Edges of Pet.
func (Pet) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", User.Type).Ref("pets").Unique().Field("owner_id"),
		edge.To("home", Org.Type).Unique().StorageKey(edge.Column("home_org"), edge.Symbol("pets_home_fk")),
	}
}
