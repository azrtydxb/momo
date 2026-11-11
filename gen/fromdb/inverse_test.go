// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"regexp"
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
	if strings.Contains(files["users"], "[]Message") || strings.Contains(files["users"], "*Profile") {
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

func warnings(res *Result, sub string) []Finding {
	var out []Finding
	for _, f := range res.Findings {
		if f.Severity == SevWarn && strings.Contains(f.Object+" "+f.Msg, sub) {
			out = append(out, f)
		}
	}
	return out
}

// Catches: a typo in an introspect.relations key being silently ignored.
func TestInverseUnmatchedRuleWarns(t *testing.T) {
	res, _ := generate(t, inverseSchema(), Config{Inverse: true, Relations: map[string]RelationRule{
		"User.Ordrs":  {Name: "Purchases"},
		"User.Orders": {Name: "Buys"},
	}})
	typo := warnings(res, "User.Ordrs")
	if len(typo) != 1 || !strings.Contains(typo[0].Msg, "matched no generated inverse") {
		t.Errorf("want one warning for the typo key: %v", res.Findings)
	}
	if len(warnings(res, "User.Orders")) != 0 {
		t.Errorf("a matched key must not warn: %v", res.Findings)
	}
}

// Catches: a collision suffix (Orders2) written without telling the user which
// field clashed and how to rename it.
func TestInverseCollisionWarns(t *testing.T) {
	s := inverseSchema()
	s.Tables[1].Columns = append(s.Tables[1].Columns, schema.Column{Name: "orders", Type: "text", Nullable: true})
	res, files := generate(t, s, Config{Inverse: true})
	mustMatch(t, "users", files["users"], "Orders2\\s+\\[\\]Order")
	w := warnings(res, "User.Orders")
	if len(w) != 1 || !strings.Contains(w[0].Msg, "Orders2") || !strings.Contains(w[0].Msg, "introspect.relations") {
		t.Errorf("collision warning: %v", res.Findings)
	}
}

// Catches: the inverse kind being wrong for a unique, partial, expression or
// composite key, and an inverse written for a composite foreign key.
func TestInverseKindMatrix(t *testing.T) {
	ix := func(unique bool, where string, parts ...schema.IndexPart) schema.Index {
		return schema.Index{Name: "items_ix", Unique: unique, Where: where, Parts: parts}
	}
	col := func(c string) schema.IndexPart { return schema.IndexPart{Column: c} }
	cases := []struct {
		name  string
		build func(tb *schema.Table)
		want  string // has_one, has_many or none
	}{
		{"plain", func(tb *schema.Table) {}, "has_many"},
		{"nullable", func(tb *schema.Table) { tb.Columns[1].Nullable = true }, "has_many"},
		{"nullable unique", func(tb *schema.Table) {
			tb.Columns[1].Nullable = true
			tb.Indexes = []schema.Index{ix(true, "", col("user_id"))}
		}, "has_one"},
		{"whole pk", func(tb *schema.Table) {
			tb.Columns = tb.Columns[1:]
			tb.PrimaryKey = []string{"user_id"}
		}, "has_one"},
		{"unique constraint", func(tb *schema.Table) {
			tb.Uniques = []schema.Unique{{Name: "items_user_key", Columns: []string{"user_id"}}}
		}, "has_one"},
		{"unique index", func(tb *schema.Table) { tb.Indexes = []schema.Index{ix(true, "", col("user_id"))} }, "has_one"},
		{"unique index with storage params", func(tb *schema.Table) {
			i := ix(true, "", col("user_id"))
			i.With = []schema.KV{{Key: "fillfactor", Value: "70"}}
			tb.Indexes = []schema.Index{i}
		}, "has_one"},
		{"non-unique index", func(tb *schema.Table) { tb.Indexes = []schema.Index{ix(false, "", col("user_id"))} }, "has_many"},
		{"partial unique index", func(tb *schema.Table) { tb.Indexes = []schema.Index{ix(true, "user_id > 0", col("user_id"))} }, "has_many"},
		{"expression unique index", func(tb *schema.Table) {
			tb.Indexes = []schema.Index{ix(true, "", schema.IndexPart{Expr: "(user_id + 1)"})}
		}, "has_many"},
		{"composite unique index", func(tb *schema.Table) {
			tb.Indexes = []schema.Index{ix(true, "", col("user_id"), col("note"))}
		}, "has_many"},
		{"composite unique constraint", func(tb *schema.Table) {
			tb.Uniques = []schema.Unique{{Name: "items_two_key", Columns: []string{"user_id", "note"}}}
		}, "has_many"},
		{"composite foreign key", func(tb *schema.Table) {
			tb.ForeignKeys = []schema.ForeignKey{{Name: "items_fk", Columns: []string{"user_id", "note"}, RefTable: "users", RefColumns: []string{"id", "email"}}}
		}, "none"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := sampleSchema()
			tb := schema.Table{
				Name: "items",
				Columns: []schema.Column{
					{Name: "id", Type: "bigint", Identity: "serial"},
					{Name: "user_id", Type: "bigint"},
					{Name: "note", Type: "text"},
				},
				PrimaryKey:  []string{"id"},
				ForeignKeys: []schema.ForeignKey{fkTo("items_user_id_fkey", "user_id")},
			}
			c.build(&tb)
			s.Tables = append(s.Tables, tb)
			_, files := generate(t, s, Config{Inverse: true})
			users := files["users"]
			got := "none"
			switch {
			case regexp.MustCompile("\\*Item\\s+`momo:\"has_one,fk:UserID\"`").MatchString(users):
				got = "has_one"
			case regexp.MustCompile("\\[\\]Item\\s+`momo:\"has_many,fk:UserID\"`").MatchString(users):
				got = "has_many"
			}
			if got != c.want {
				t.Errorf("inverse = %s, want %s:\n%s", got, c.want, users)
			}
		})
	}
}
