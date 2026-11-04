// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"encoding/json"
	"math/rand"
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
