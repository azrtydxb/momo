// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

func fkTo(name, col string) schema.ForeignKey {
	return schema.ForeignKey{Name: name, Columns: []string{col}, RefTable: "users", RefColumns: []string{"id"}}
}

// inverseSchema has users, messages with two foreign keys to users, profiles
// with a unique one and orders with a plain one.
func inverseSchema() *schema.Schema {
	s := sampleSchema()
	s.Tables = append(s.Tables,
		schema.Table{
			Name: "messages",
			Columns: []schema.Column{
				{Name: "id", Type: "bigint", Identity: "serial"},
				{Name: "sender_id", Type: "bigint"},
				{Name: "recipient_id", Type: "bigint"},
			},
			PrimaryKey:  []string{"id"},
			ForeignKeys: []schema.ForeignKey{fkTo("messages_sender_id_fkey", "sender_id"), fkTo("messages_recipient_id_fkey", "recipient_id")},
		},
		schema.Table{
			Name: "profiles",
			Columns: []schema.Column{
				{Name: "id", Type: "bigint", Identity: "serial"},
				{Name: "user_id", Type: "bigint"},
			},
			PrimaryKey:  []string{"id"},
			Indexes:     []schema.Index{{Name: "profiles_user_id_key", Unique: true, Parts: []schema.IndexPart{{Column: "user_id"}}}},
			ForeignKeys: []schema.ForeignKey{fkTo("profiles_user_id_fkey", "user_id")},
		},
	)
	return s
}

// Catches: the default behaviour changing; existing projects must see no new
// fields unless they opt in.
func TestInverseOffByDefault(t *testing.T) {
	_, files := generate(t, inverseSchema(), Config{})
	if strings.Contains(files["users"], "has_many") || strings.Contains(files["users"], "has_one") {
		t.Fatalf("users has an inverse without introspect.inverse:\n%s", files["users"])
	}
}

// Catches: a two-foreign-key child clashing on one name, a unique foreign key
// getting has_many, or a plain one getting has_one.
func TestInverseNamesAndKinds(t *testing.T) {
	res, files := generate(t, inverseSchema(), Config{Inverse: true})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	mustMatch(t, "users", files["users"],
		"Orders\\s+\\[\\]Order\\s+`momo:\"has_many,fk:UserID\"`",
		"MessagesBySender\\s+\\[\\]Message\\s+`momo:\"has_many,fk:SenderID\"`",
		"MessagesByRecipient\\s+\\[\\]Message\\s+`momo:\"has_many,fk:RecipientID\"`",
		"Profile\\s+\\*Profile\\s+`momo:\"has_one,fk:UserID\"`",
	)
	// the belongs_to side is unchanged
	mustMatch(t, "messages", files["messages"], "Sender\\s+\\*User\\s+`momo:\"belongs_to,fk:SenderID\"`")
	if strings.Contains(files["orders"], "has_many") || strings.Contains(files["profiles"], "has_one") {
		t.Errorf("the child gets no inverse")
	}
	// a second run gives the same bytes
	_, again := generate(t, inverseSchema(), Config{Inverse: true})
	if again["users"] != files["users"] {
		t.Errorf("the output is not deterministic")
	}
}

// Catches: introspect.relations rename and skip being ignored.
func TestInverseRelationRules(t *testing.T) {
	_, files := generate(t, inverseSchema(), Config{Inverse: true, Relations: map[string]RelationRule{
		"User.Orders":              {Name: "Purchases"},
		"User.MessagesBySender":    {Skip: true},
		"User.MessagesByRecipient": {Name: "Inbox"},
	}})
	mustMatch(t, "users", files["users"], "Purchases\\s+\\[\\]Order", "Inbox\\s+\\[\\]Message")
	if strings.Contains(files["users"], "MessagesBySender") {
		t.Errorf("skipped relation written:\n%s", files["users"])
	}
}

// Catches: an inverse written for a referencing table that has no model.
func TestInverseSkipsUnmodeledChild(t *testing.T) {
	_, files := generate(t, inverseSchema(), Config{Inverse: true, Skip: []string{"messages", "profiles"}})
	if strings.Contains(files["users"], "Message") || strings.Contains(files["users"], "Profile") {
		t.Errorf("inverse of an unmodeled table:\n%s", files["users"])
	}
	mustMatch(t, "users", files["users"], "Orders\\s+\\[\\]Order")
}

// Catches: a self-reference clashing with the model's own name.
func TestInverseSelfReference(t *testing.T) {
	s := sampleSchema()
	s.Tables[1].Columns = append(s.Tables[1].Columns, schema.Column{Name: "manager_id", Type: "bigint", Nullable: true})
	s.Tables[1].ForeignKeys = []schema.ForeignKey{fkTo("users_manager_id_fkey", "manager_id")}
	_, files := generate(t, s, Config{Inverse: true})
	mustMatch(t, "users", files["users"], "UsersByManager\\s+\\[\\]User\\s+`momo:\"has_many,fk:ManagerID\"`")
}

func TestInverseRelationRuleValidate(t *testing.T) {
	for _, c := range []Config{
		{Relations: map[string]RelationRule{"Orders": {}}},
		{Relations: map[string]RelationRule{"User.Orders": {Name: "lower"}}},
		{Relations: map[string]RelationRule{"User.Orders": {Name: "X", Skip: true}}},
	} {
		if c.Validate() == nil {
			t.Errorf("%+v validated", c.Relations)
		}
	}
}
