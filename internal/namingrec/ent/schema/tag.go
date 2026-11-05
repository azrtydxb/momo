// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Tag is the many-to-many target.
type Tag struct{ ent.Schema }

// Fields of Tag.
func (Tag) Fields() []ent.Field { return []ent.Field{field.String("name")} }

// Edges of Tag: the inverse side makes the tags edge many-to-many.
func (Tag) Edges() []ent.Edge {
	return []ent.Edge{edge.From("user_profiles", UserProfile.Type).Ref("tags")}
}
