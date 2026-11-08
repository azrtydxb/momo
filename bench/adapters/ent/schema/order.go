// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Order is bench_orders.
type Order struct {
	ent.Schema
}

// Annotations names the table.
func (Order) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "bench_orders"}}
}

// Fields of Order.
func (Order) Fields() []ent.Field {
	return []ent.Field{
		field.Int("customer_id"),
		field.Int64("amount_cents"),
		field.String("status"),
		field.Time("created_at"),
	}
}

// Edges of Order.
func (Order) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("customer", Customer.Type).Ref("orders").Field("customer_id").Unique().Required(),
	}
}
