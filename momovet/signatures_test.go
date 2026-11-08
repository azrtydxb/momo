// SPDX-License-Identifier: Apache-2.0

package momovet

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestRawSQLFuncsMatchMomo checks the analyzer's table against the real momo
// package: every listed function exists with a string parameter named sql at
// the listed index, and every exported function of momo with a string
// parameter named sql is listed, so a new raw-SQL entry point cannot go
// unchecked.
func TestRawSQLFuncsMatchMomo(t *testing.T) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes | packages.NeedImports | packages.NeedDeps}
	pkgs, err := packages.Load(cfg, momoPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 || len(pkgs[0].Errors) > 0 || pkgs[0].Types == nil {
		t.Fatalf("loading %s failed: %v", momoPath, pkgs)
	}
	scope := pkgs[0].Types.Scope()

	for name, idx := range rawSQLFuncs {
		fn, ok := scope.Lookup(name).(*types.Func)
		if !ok {
			t.Errorf("rawSQLFuncs lists %s, which is not a function of %s", name, momoPath)
			continue
		}
		params := fn.Type().(*types.Signature).Params()
		if idx >= params.Len() {
			t.Errorf("%s has %d parameters, the table says the SQL is parameter %d", name, params.Len(), idx)
			continue
		}
		if p := params.At(idx); p.Name() != "sql" || !types.Identical(p.Type(), types.Typ[types.String]) {
			t.Errorf("%s parameter %d is %s %s, want sql string", name, idx, p.Name(), p.Type())
		}
	}

	for _, name := range scope.Names() {
		fn, ok := scope.Lookup(name).(*types.Func)
		if !ok || !fn.Exported() {
			continue
		}
		params := fn.Type().(*types.Signature).Params()
		for i := 0; i < params.Len(); i++ {
			p := params.At(i)
			if p.Name() != "sql" || !types.Identical(p.Type(), types.Typ[types.String]) {
				continue
			}
			if idx, listed := rawSQLFuncs[name]; !listed || idx != i {
				t.Errorf("%s takes SQL text as parameter %d but is not in rawSQLFuncs at that index", name, i)
			}
		}
	}
}
