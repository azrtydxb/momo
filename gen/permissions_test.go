// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen/tags"
)

func TestFieldPermissions(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	ok := exec.Command("go", "build", "-o", os.DevNull, "./gen/testdata/permissions/ok")
	ok.Dir = root
	if out, err := ok.CombinedOutput(); err != nil {
		t.Fatalf("valid fixture must build: %v\n%s", err, out)
	}
	want := map[string]string{
		"readonly_set":      "has no field or method Set",
		"createonly_update": "InsertAssignment",
		"immutable_update":  "InsertAssignment",
	}
	for fixture, reason := range want {
		cmd := exec.Command("go", "build", "-o", os.DevNull, "./gen/testdata/permissions/"+fixture)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("%s must not compile", fixture)
		}
		if !strings.Contains(string(out), reason) {
			t.Fatalf("%s failed without %q:\n%s", fixture, reason, out)
		}
	}
}

func TestAccessFor(t *testing.T) {
	cases := []struct {
		name    string
		tg      tags.Tags
		pointer bool
		want    Access
		diag    string
		typ     string
	}{
		{"plain", tags.Tags{}, false, Writable, "", "Column"},
		{"plain pointer", tags.Tags{}, true, Writable, "", "NullColumn"},
		{"read_only", tags.Tags{ReadOnly: true}, false, ReadOnly, "", "ReadOnlyColumn"},
		{"read_only pointer", tags.Tags{ReadOnly: true}, true, ReadOnly, "", "NullReadOnlyColumn"},
		{"create_only", tags.Tags{CreateOnly: true}, false, CreateOnly, "", "CreateOnlyColumn"},
		{"immutable", tags.Tags{Immutable: true}, false, Immutable, "", "ImmutableColumn"},
		{"create_only pointer", tags.Tags{CreateOnly: true}, true, Writable, "create_only and immutable fields must not be pointers", ""},
		{"immutable pointer", tags.Tags{Immutable: true}, true, Writable, "create_only and immutable fields must not be pointers", ""},
		{"conflict", tags.Tags{ReadOnly: true, Immutable: true}, false, Writable, "conflicting write permissions on F", ""},
	}
	for _, c := range cases {
		got, diags := AccessFor(c.tg, "F", c.pointer)
		if got != c.want {
			t.Errorf("%s: access=%v want %v", c.name, got, c.want)
		}
		if c.diag == "" && len(diags) != 0 || c.diag != "" && (len(diags) != 1 || diags[0].Msg != c.diag) {
			t.Errorf("%s: diags=%v want %q", c.name, diags, c.diag)
		}
		if c.typ != "" {
			if typ, ctor := DescriptorType(got, c.pointer); typ != c.typ || ctor != "New"+c.typ {
				t.Errorf("%s: descriptor %s/%s want %s", c.name, typ, ctor, c.typ)
			}
		}
	}
}

func TestFieldPermissionsDiscovery(t *testing.T) {
	dir, err := filepath.Abs("testdata/permissions")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := Load([]string{"./model"}, Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: err=%v diags=%v", err, diags)
	}
	got := map[string]Access{}
	for _, f := range g.Models[0].Fields {
		got[f.GoName] = f.Access
	}
	want := map[string]Access{"ID": Writable, "Code": ReadOnly, "Owner": CreateOnly, "Serial": Immutable, "Name": Writable}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s access=%v want %v", k, got[k], v)
		}
	}
}
