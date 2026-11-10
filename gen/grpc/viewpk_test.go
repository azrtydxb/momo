// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/apiquery"
)

// Catches: a plugin that looks a model up in Graph.Schema by table name and so
// misses a view model, which the schema does not hold as a table. A view model
// with a pk field must keep its key: Get and Delete address it by that key.
func TestViewModelKeepsItsKey(t *testing.T) {
	dir, err := filepath.Abs("../testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./viewpk"}, gen.Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	_, ds := Proto(g, &Lock{}, "view.v1")
	for _, d := range ds {
		if strings.Contains(d.Msg, "primary key") {
			t.Errorf("unexpected diagnostic: %v", d)
		}
	}
}

// Catches: a view model getting Create, Update and Delete rpcs with the
// default ops, which the runtime refuses on every call.
func TestViewModelServiceIsReadOnly(t *testing.T) {
	dir, err := filepath.Abs("../testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./viewpk"}, gen.Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	src, ds := Proto(g, &Lock{}, "view.v1")
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	s := string(src)
	svc := func(name string) string {
		i := strings.Index(s, "service "+name+" {")
		if i < 0 {
			t.Fatalf("no service %s", name)
		}
		return s[i : i+strings.Index(s[i:], "}\n")]
	}
	view := svc("ActiveAccountService")
	for _, no := range []string{"rpc Create", "rpc Update", "rpc Delete"} {
		if strings.Contains(view, no) {
			t.Errorf("the view service has %q", no)
		}
	}
	if !strings.Contains(view, "rpc Get") || !strings.Contains(view, "rpc List") {
		t.Errorf("the view service lost a read rpc:\n%s", view)
	}
	if !strings.Contains(svc("AccountService"), "rpc Create") {
		t.Error("the table service lost Create")
	}
}

// Catches: an explicit write operation for a view model being dropped or
// accepted instead of reported.
func TestViewModelExplicitWriteOpIsADiagnostic(t *testing.T) {
	dir, err := filepath.Abs("../testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./viewpk"}, gen.Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	_, ds := protoSource(g, &Lock{}, "view.v1", "", apiquery.Ops{"ActiveAccount": {"get", "delete"}})
	found := false
	for _, d := range ds {
		found = found || strings.Contains(d.Msg, "ActiveAccount is read-only")
	}
	if !found {
		t.Errorf("no read-only diagnostic: %v", ds)
	}
}
