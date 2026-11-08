// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
)

// Sized has one field of every column type the importer maps.
type Sized struct{ ent.Schema }

// Fields of Sized.
func (Sized) Fields() []ent.Field {
	return []ent.Field{
		field.Int32("rank"),
		field.Uint16("level"),
		field.Int8("flag"),
		field.Int64("big"),
		field.Uint("count").Optional(),
		field.Float("ratio"),
		field.Float32("weight"),
		field.Float("price").SchemaType(map[string]string{dialect.Postgres: "numeric(10,2)"}),
		field.Bytes("blob").Optional(),
		field.Text("body").Optional(),
		field.Enum("kind").Values("draft", "live").Default("draft"),
		field.String("label").Default("it's"),
	}
}
