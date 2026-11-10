// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/apiquery"
)

// Guards the primary key of a view model in propertiesOf, which reads it
// through Graph.TableOf. The pk tag on the field marks it too, so this test
// cannot tell the two sources apart and does not fail with the old lookup;
// it pins the behavior that a view model's key is not a required write
// property.
func TestViewModelPrimaryKeyIsNotRequired(t *testing.T) {
	dir, err := filepath.Abs("../testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./viewpk"}, gen.Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	for _, m := range g.Models {
		props, ds := propertiesOf(g, m)
		if len(ds) != 0 {
			t.Fatalf("%s: %v", m.GoName, ds)
		}
		for _, p := range props {
			if p.column == "id" && p.required {
				t.Errorf("%s: the primary key id is marked required on write", m.GoName)
			}
		}
	}
}

func loadViewpk(t *testing.T) *gen.Graph {
	t.Helper()
	dir, err := filepath.Abs("../testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./viewpk"}, gen.Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	return g
}

// Catches: a view model getting create, update and delete routes with the
// default ops, which the runtime refuses with ErrValidation on every call.
func TestViewModelGetsOnlyReadOperations(t *testing.T) {
	g := loadViewpk(t)
	opts := Options{Title: "t", Version: "1", BasePath: "/v1"}
	doc, ds := Document(g, opts)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	var d struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(doc, &d); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Paths["/v1/accounts"]["post"]; !ok {
		t.Error("the table model lost POST /v1/accounts")
	}
	for path, methods := range map[string][]string{
		"/v1/active_accounts":      {"post"},
		"/v1/active_accounts/{id}": {"put", "delete"},
	} {
		for _, m := range methods {
			if _, ok := d.Paths[path][m]; ok {
				t.Errorf("the document lists %s %s for a view model", m, path)
			}
		}
	}
	if _, ok := d.Paths["/v1/active_accounts"]["get"]; !ok {
		t.Error("the view model lost GET /v1/active_accounts")
	}
	if _, ok := d.Paths["/v1/active_accounts/{id}"]["get"]; !ok {
		t.Error("the view model lost GET /v1/active_accounts/{id}")
	}
	src, ds := Handlers(g, "handlers", opts)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	s := string(src)
	for _, no := range []string{`"POST /v1/active_accounts"`, `"PUT /v1/active_accounts/{id}"`, `"DELETE /v1/active_accounts/{id}"`, "func (h *ActiveAccountHandler) create", "func (h *ActiveAccountHandler) update", "func (h *ActiveAccountHandler) delete"} {
		if strings.Contains(s, no) {
			t.Errorf("handlers contain %s for a view model", no)
		}
	}
	if !strings.Contains(s, `"GET /v1/active_accounts"`) || !strings.Contains(s, `"POST /v1/accounts"`) {
		t.Error("handlers lost a read route of the view model or the write route of the table model")
	}
}

// Catches: an explicit write operation for a view model being silently
// dropped, or accepted, instead of reported as a configuration error.
func TestViewModelExplicitWriteOpIsADiagnostic(t *testing.T) {
	g := loadViewpk(t)
	opts := Options{Title: "t", Version: "1", Ops: apiquery.Ops{"ActiveAccount": {"list", "create"}}}
	_, ds := Document(g, opts)
	found := false
	for _, d := range ds {
		found = found || strings.Contains(d.Msg, "ActiveAccount is read-only")
	}
	if !found {
		t.Errorf("no read-only diagnostic from Document: %v", ds)
	}
	_, ds = Handlers(g, "handlers", opts)
	found = false
	for _, d := range ds {
		found = found || strings.Contains(d.Msg, "ActiveAccount is read-only")
	}
	if !found {
		t.Errorf("no read-only diagnostic from Handlers: %v", ds)
	}
}
