// SPDX-License-Identifier: Apache-2.0

// Package schema holds the Ent schemas of the naming fixture models.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// UserProfile is the many-to-many owner.
type UserProfile struct{ ent.Schema }

// Fields of UserProfile.
func (UserProfile) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("http_url"),
		field.Time("created_at"),
	}
}

// Edges of UserProfile.
func (UserProfile) Edges() []ent.Edge {
	return []ent.Edge{edge.To("tags", Tag.Type)}
}
