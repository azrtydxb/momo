// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen/naming"
)

const joinSrc = relHeader + `
type Tag struct {
	momo.Model ` + "`momo:\"table:tags\"`" + `
	ID   int64 ` + "`momo:\"pk\"`" + `
	Name string
}

type APIDoc struct {
	momo.Model ` + "`momo:\"table:api_docs\"`" + `
	UID  int64 ` + "`momo:\"pk,column:doc_pk\"`" + `
	Tags []Tag ` + "`momo:\"many2many,join:doc_tags\"`" + `
}
`

// prefixNamer prefixes every column, so a hardcoded preset cannot match it.
type prefixNamer struct{ naming.Momo }

func (prefixNamer) ColumnName(f string) string { return "x" + naming.Momo{}.ColumnName(f) }

// joinColumns loads joinSrc with namer and returns the Through line of the
// descriptor and the join table columns of the schema.
func joinColumns(t *testing.T, namer naming.Namer) (desc string, tbl []string) {
	t.Helper()
	dir, pat := scratchModels(t, joinSrc)
	cfg := Config{Dir: dir, Naming: namer}
	g, diags, err := Load([]string{pat}, cfg)
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	var doc *Model
	for _, m := range g.Models {
		if m.GoName == "APIDoc" {
			doc = m
		}
	}
	src, ds := EmitDescriptors(doc, g)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	for _, line := range strings.Split(string(src), "\n") {
		if strings.Contains(line, "Through:") {
			desc = strings.TrimSpace(line)
		}
	}
	s, sd := BuildSchema(g.Models, cfg)
	if len(sd) != 0 {
		t.Fatal(sd)
	}
	for _, tb := range s.Tables {
		if tb.Name == "doc_tags" {
			for _, c := range tb.Columns {
				tbl = append(tbl, c.Name)
			}
		}
	}
	return desc, tbl
}

func TestJoinColumnsFollowNamingAndPKColumn(t *testing.T) {
	cases := []struct {
		name   string
		namer  naming.Namer
		parent string
		child  string
	}{
		{"default momo", nil, "api_doc_doc_pk", "tag_id"},
		{"gorm preset", naming.GORM{}, "api_doc_doc_pk", "tag_id"},
		{"custom namer", prefixNamer{}, "xapi_doc_doc_pk", "xtag_xid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desc, tbl := joinColumns(t, tc.namer)
			want := `ParentCols: []string{"` + tc.parent + `"}, ChildCols: []string{"` + tc.child + `"}`
			if !strings.Contains(desc, want) {
				t.Fatalf("descriptor %q lacks %s", desc, want)
			}
			if len(tbl) != 2 || tbl[0] != tc.parent || tbl[1] != tc.child {
				t.Fatalf("schema join columns %v, want %s and %s", tbl, tc.parent, tc.child)
			}
		})
	}
}
