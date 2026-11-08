// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
)

// Audit is a local mixin: the fields it declares are part of every schema
// that lists it.
type Audit struct{ mixin.Schema }

// Fields of the Audit mixin.
func (Audit) Fields() []ent.Field {
	return []ent.Field{
		field.String("created_by").Optional().Nillable(),
		field.Time("seen_at").Optional().Default(time.Now),
	}
}

// User belongs to an Org and owns Pets and a Card.
type User struct{ ent.Schema }

// Mixin lists the local mixin.
func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{Audit{}}
}

// Annotations names the table.
func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "app_users"}}
}

// Fields of User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("email").Unique().Immutable(),
		field.String("name").Optional().Nillable(),
		field.Bool("active").Default(true),
		field.Int("score").Default(0),
		field.Time("joined_at").Default(time.Now),
	}
}

// Edges of User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("org", Org.Type).Ref("users").Unique().Required(),
		edge.To("pets", Pet.Type),
		edge.To("card", Card.Type).Unique(),
	}
}

// Indexes of User.
func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
		index.Fields("score", "active"),
		index.Fields("active").StorageKey("app_users_active_idx"),
	}
}
