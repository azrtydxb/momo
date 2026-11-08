// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen/tags"
)

func idModel(f *Field, cfg map[string]string) *Model {
	return &Model{Fields: []*Field{f}, Config: &CompanionResult{IDGens: cfg}}
}

func TestIDGensOfProblems(t *testing.T) {
	str := func(tg tags.Tags) *Field {
		return &Field{GoName: "ID", Column: "id", GoType: "string", BaseType: "string", Tags: tg}
	}
	cases := []struct {
		name string
		m    *Model
		want string // empty: valid
	}{
		{"tag", idModel(str(tags.Tags{IDGen: "cuid"}), nil), ""},
		{"config", idModel(str(tags.Tags{}), map[string]string{"id": "uuid"}), ""},
		{"identity", idModel(str(tags.Tags{IDGen: "cuid", Identity: true}), nil), "cannot be combined with identity or serial"},
		{"serial", idModel(str(tags.Tags{IDGen: "cuid", Serial: true}), nil), "cannot be combined with identity or serial"},
		{"config on int", idModel(&Field{GoName: "ID", Column: "id", GoType: "int64", BaseType: "int64"}, map[string]string{"id": "cuid"}), "needs a string field, not int64"},
		{"both", idModel(str(tags.Tags{IDGen: "cuid"}), map[string]string{"id": "ulid"}), "declared by both the tag and MomoConfig"},
		{"bad name", idModel(str(tags.Tags{IDGen: "Not-Ok"}), nil), "must be a lowercase name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gens, diags := idGensOf(tc.m)
			if tc.want == "" {
				if len(diags) != 0 || len(gens) != 1 || gens[0].Column != 0 {
					t.Fatalf("gens %+v diags %+v", gens, diags)
				}
				return
			}
			if len(gens) != 0 || len(diags) != 1 || !strings.Contains(diags[0].Msg, tc.want) {
				t.Fatalf("gens %+v diags %+v, want %q", gens, diags, tc.want)
			}
		})
	}
}
