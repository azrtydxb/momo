// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"slices"
	"testing"
)

func TestSortCoversAllCollections(t *testing.T) {
	s := &Schema{
		Extensions: []Extension{{Name: "vector"}, {Name: "citext"}},
		Enums:      []Enum{{Name: "b"}, {Name: "a"}},
		Tables: []Table{{
			Name:        "t",
			Uniques:     []Unique{{Name: "u2"}, {Name: "u1"}},
			Checks:      []Check{{Name: "c2"}, {Name: "c1"}},
			Indexes:     []Index{{Name: "i2"}, {Name: "i1"}},
			ForeignKeys: []ForeignKey{{Name: "f2"}, {Name: "f1"}},
		}, {Name: "a"}},
	}
	s.Sort()
	tb := s.Tables[1]
	if s.Extensions[0].Name != "citext" || s.Enums[0].Name != "a" || s.Tables[0].Name != "a" ||
		tb.Uniques[0].Name != "u1" || tb.Checks[0].Name != "c1" || tb.Indexes[0].Name != "i1" || tb.ForeignKeys[0].Name != "f1" {
		t.Fatalf("Sort left a collection unsorted: %+v", s)
	}
}

// FuzzSchemaSort: shuffling the collections of a schema and then sorting it
// must give the same bytes as sorting the original.
func FuzzSchemaSort(f *testing.F) {
	f.Add(int64(1))
	f.Add(int64(42))
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))
		base := populated()
		for i := 0; i < 4; i++ {
			base.Tables = append(base.Tables, Table{Name: string(rune('a' + i)), Columns: []Column{{Name: "c", Type: "int"}}})
		}
		shuffled := populated()
		for i := 0; i < 4; i++ {
			shuffled.Tables = append(shuffled.Tables, base.Tables[len(base.Tables)-4+i])
		}
		r.Shuffle(len(shuffled.Tables), func(i, j int) { shuffled.Tables[i], shuffled.Tables[j] = shuffled.Tables[j], shuffled.Tables[i] })
		r.Shuffle(len(shuffled.Extensions), func(i, j int) {
			shuffled.Extensions[i], shuffled.Extensions[j] = shuffled.Extensions[j], shuffled.Extensions[i]
		})

		base.Sort()
		shuffled.Sort()
		a, err := json.Marshal(base)
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(shuffled)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("Sort is order-sensitive:\n%s\n%s", a, b)
		}
	})
}

func TestSortCoversManagedObjects(t *testing.T) {
	s := &Schema{
		Schemas:   []SchemaNamespace{{Name: "b"}, {Name: "a"}},
		Tables:    []Table{{Schema: "b", Name: "t"}, {Schema: "a", Name: "t"}},
		Views:     []View{{Name: "v2", Depends: []string{"z", "a", "z"}, Indexes: []Index{{Name: "i2"}, {Name: "i1"}}}, {Name: "v1"}},
		Functions: []Function{{Name: "f", Args: "b"}, {Name: "f", Args: "a"}},
		Triggers:  []Trigger{{Table: "t", Name: "b", Events: []string{"UPDATE", "INSERT"}}, {Table: "t", Name: "a"}},
		Policies:  []Policy{{Table: "t", Name: "q", Roles: []string{"y", "x"}}, {Table: "t", Name: "p"}},
	}
	s.Sort()
	if s.Schemas[0].Name != "a" || s.Tables[0].Schema != "a" || s.Views[0].Name != "v1" ||
		s.Functions[0].Args != "a" || s.Triggers[0].Name != "a" || s.Policies[0].Name != "p" {
		t.Fatalf("Sort left a managed-object collection unsorted: %+v", s)
	}
	v := s.Views[1]
	if !slices.Equal(v.Depends, []string{"a", "z"}) || v.Indexes[0].Name != "i1" {
		t.Fatalf("view depends/indexes not sorted and deduplicated: %+v", v)
	}
	if !slices.Equal(s.Triggers[1].Events, []string{"INSERT", "UPDATE"}) || !slices.Equal(s.Policies[1].Roles, []string{"x", "y"}) {
		t.Fatalf("events/roles not sorted: %+v", s)
	}
}

func TestEqualNormalizesManagedObjects(t *testing.T) {
	a := &Schema{Dialect: "postgres",
		Views:    []View{{Name: "v", Query: "SELECT  A FROM t"}},
		Triggers: []Trigger{{Table: "t", Name: "g", When: "(NEW.x > 1)"}},
		Policies: []Policy{{Table: "t", Name: "p", Using: "(Tenant = 1)", Check: "tenant  = 1"}}}
	b := &Schema{Dialect: "postgres",
		Views:    []View{{Name: "v", Query: "select a from t"}},
		Triggers: []Trigger{{Table: "t", Name: "g", When: "new.x > 1"}},
		Policies: []Policy{{Table: "t", Name: "p", Using: "tenant = 1", Check: "tenant = 1"}}}
	if !Equal(a, b) {
		t.Fatal("equivalent managed objects compared unequal")
	}
	b.Policies[0].Using = "tenant = 2"
	if Equal(a, b) {
		t.Fatal("different policy compared equal")
	}
}
