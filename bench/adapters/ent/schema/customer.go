// SPDX-License-Identifier: Apache-2.0

// Package schema defines the Ent schema of the benchmark tables.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Customer is bench_customers.
type Customer struct {
	ent.Schema
}

// Annotations names the table.
func (Customer) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "bench_customers"}}
}

// Fields of Customer.
func (Customer) Fields() []ent.Field {
	return []ent.Field{
		field.String("email"),
		field.String("country"),
		field.String("status"),
		field.Time("created_at"),
	}
}

// Edges of Customer.
func (Customer) Edges() []ent.Edge {
	return []ent.Edge{edge.To("orders", Order.Type)}
}
