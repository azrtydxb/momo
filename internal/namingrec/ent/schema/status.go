// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// Status exercises words that look plural.
type Status struct{ ent.Schema }

// Fields of Status.
func (Status) Fields() []ent.Field { return []ent.Field{field.String("label")} }
