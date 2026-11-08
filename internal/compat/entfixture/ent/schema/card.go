// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Card is held by one User.
type Card struct{ ent.Schema }

// Fields of Card.
func (Card) Fields() []ent.Field {
	return []ent.Field{
		field.String("number").Unique(),
	}
}

// Edges of Card.
func (Card) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", User.Type).Ref("card").Unique().Required(),
	}
}
