// SPDX-License-Identifier: Apache-2.0

package tags

import "testing"

// TestParseHostile covers malformed, duplicated, escaped and oddly spaced input.
// Each case pins the exact diagnostic list so no input is silently accepted.
func TestParseHostile(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty tag is not an error", "", nil},
		{"whitespace only", "   ", nil},
		{"trailing comma", "pk,", []string{`unknown tag option ""`}},
		{"double comma", "pk,,notnull", []string{`unknown tag option ""`}},
		{"leading comma", ",pk", []string{`unknown tag option ""`}},
		{"colon only", ":", []string{`unknown tag option ""`}},
		{"dash among others", "-,pk", []string{`unknown tag option "-"`}},
		{"upper case key", "PK", []string{`unknown tag option "PK"`}},
		{"mixed case key", "Column:a", []string{`unknown tag option "Column"`}},
		{"duplicate valued", "column:a,column:b", []string{`duplicate tag option "column"`}},
		{"duplicate bare", "pk,pk", []string{`duplicate tag option "pk"`}},
		{"duplicate index", "index,index:x", []string{`duplicate tag option "index"`}},
		{"empty value", "column:", []string{"column requires a value"}},
		{"empty default", "default:", []string{"default requires a value"}},
		{"empty index name", "index:", []string{"index requires a value"}},
		{"bare key with value", "pk:1", []string{"pk does not take a value"}},
		{"negative integer", "size:-1", []string{"size must not be negative"}},
		{"integer with space", "dim: 3", []string{"dim must be an integer"}},
		{"integer overflow", "dim:99999999999999999999", []string{"dim must be an integer"}},
		{"float integer", "scale:1.5", []string{"scale must be an integer"}},
		{"bad sort", "sort:up", []string{"sort must be asc or desc"}},
		{"bad nulls", "nulls:middle", []string{"nulls must be first or last"}},
		{"bad soft delete", "soft_delete:maybe", []string{"soft_delete must be timestamp, unix or bool"}},
		{"bad rel", "rel:belongs", []string{"rel must be belongs_to, has_one, has_many or many2many"}},
		{"bad time unit", "created_at:hours", []string{"created_at must be unix, unixmilli or unixnano"}},
		{"empty updated unit", "updated_at:", []string{"updated_at must be unix, unixmilli or unixnano"}},
		{"retired halfvec key", "halfvec:3", []string{`unknown tag option "halfvec"`}},
		{"several problems accumulate", "bogus,size:x,dim", []string{`unknown tag option "bogus"`, "size must be an integer", "dim requires a value"}},
		{"backslash is not an escape", `comment:a\,b`, []string{`unknown tag option "b"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, probs := Parse(tc.raw)
			var got []string
			for _, p := range probs {
				got = append(got, p.Msg)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("Parse(%q) problems = %q, want %q", tc.raw, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("Parse(%q) problem %d = %q, want %q", tc.raw, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestParseValues(t *testing.T) {
	g, probs := Parse(" default:a:b:c , check:x > 0 ,comment:hi there,rel:many2many,polytype:owner_type,polyvalue:doc,soft_delete:bool,updated_at ")
	if len(probs) != 0 {
		t.Fatalf("problems %+v", probs)
	}
	if g.Default != "a:b:c" || g.Check != "x > 0" || g.Comment != "hi there" || g.Rel != "many2many" ||
		g.PolyType != "owner_type" || g.PolyValue != "doc" || g.SoftDelete != "bool" || !g.UpdatedAt || g.TimeUnit != "" {
		t.Fatalf("got %+v", g)
	}
	g, probs = Parse("unique:uq_x,fk:AuthorID,join:t,on_delete:cascade,prefix:p_,table:t,schema:s,enum:e,collate:C,type:citext,pk,identity,version,nullable,read_only,create_only,immutable,sensitive,embedded,no_constraint,belongs_to,has_one,has_many,many2many,precision:10,scale:2")
	if len(probs) != 0 {
		t.Fatalf("problems %+v", probs)
	}
	if !g.Unique || g.UniqueName != "uq_x" || g.Fk != "AuthorID" || g.Join != "t" || g.OnDelete != "cascade" ||
		g.Prefix != "p_" || g.Table != "t" || g.Schema != "s" || g.Enum != "e" || g.Collate != "C" || g.Type != "citext" ||
		!g.PK || !g.Identity || !g.Version || !g.Nullable || !g.ReadOnly || !g.CreateOnly || !g.Immutable || !g.Sensitive ||
		!g.Embedded || !g.NoConstraint || !g.BelongsTo || !g.HasOne || !g.HasMany || !g.ManyToMany || g.Precision != 10 || g.Scale != 2 {
		t.Fatalf("got %+v", g)
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{"", "-", "pk,notnull", "default:now(),x", ":,,:", "size:-1"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		g, probs := Parse(raw)
		if g.Skip && len(probs) != 0 {
			t.Fatalf("skip with problems %+v", probs)
		}
		if g.Size < 0 || g.Dim < 0 || g.Precision < 0 || g.Scale < 0 {
			t.Fatalf("negative integer accepted: %+v", g)
		}
	})
}
