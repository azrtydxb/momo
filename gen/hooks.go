// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"go/token"
	"go/types"
)

// hookNames are the model methods that fill momo.HookSet, in the fixed order
// the generator emits them.
var hookNames = []string{"BeforeInsert", "BeforeUpdate", "BeforeDelete", "AfterLoad"}

// DetectHooks finds the hook methods of a model statically. A method named
// BeforeInsert, BeforeUpdate, BeforeDelete or AfterLoad in the method set of
// *T is a hook when its signature is exactly func(context.Context) error. A
// method with a hook name and another signature yields a diagnostic at the
// method. The result maps each detected hook name to true.
func DetectHooks(named *types.Named, fset *token.FileSet) (map[string]bool, []Diagnostic) {
	found := map[string]bool{}
	var diags []Diagnostic
	if named == nil {
		return found, nil
	}
	ms := types.NewMethodSet(types.NewPointer(named))
	for _, name := range hookNames {
		sel := ms.Lookup(nil, name)
		if sel == nil {
			continue
		}
		fn, ok := sel.Obj().(*types.Func)
		if !ok {
			continue
		}
		if isHookSignature(fn.Type().(*types.Signature)) {
			found[name] = true
			continue
		}
		diags = append(diags, Diagnostic{
			Pos: fset.Position(fn.Pos()),
			Msg: "hook " + name + " on " + named.Obj().Name() + " must have signature func(context.Context) error",
		})
	}
	return found, diags
}

// isHookSignature reports whether sig is func(context.Context) error.
func isHookSignature(sig *types.Signature) bool {
	if sig.Variadic() || sig.Params().Len() != 1 || sig.Results().Len() != 1 {
		return false
	}
	p, ok := sig.Params().At(0).Type().(*types.Named)
	if !ok || p.Obj().Pkg() == nil || p.Obj().Pkg().Path() != "context" || p.Obj().Name() != "Context" {
		return false
	}
	return types.Identical(sig.Results().At(0).Type(), types.Universe.Lookup("error").Type())
}

// hooksLiteral renders the Hooks field of a model's momo.Meta literal, or ""
// when the model has no hooks. The runtime hook takes (ctx, *T), so each hook
// is a small closure over the model method.
func hooksLiteral(goName string, hooks map[string]bool) string {
	if len(hooks) == 0 {
		return ""
	}
	s := "Hooks: momo.HookSet[" + goName + "]{"
	for _, n := range hookNames {
		if hooks[n] {
			s += n + ": func(ctx context.Context, m *" + goName + ") error { return m." + n + "(ctx) }, "
		}
	}
	return s[:len(s)-1] + "},\n"
}

// anyHooks reports whether any model declares a hook.
func anyHooks(models []*Model) bool {
	for _, m := range models {
		if len(m.Hooks) > 0 {
			return true
		}
	}
	return false
}
