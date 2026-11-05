// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// Person exercises irregular pluralisation.
type Person struct{ ent.Schema }

// Fields of Person.
func (Person) Fields() []ent.Field { return []ent.Field{field.String("nick")} }
