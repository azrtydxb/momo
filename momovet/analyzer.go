// SPDX-License-Identifier: Apache-2.0

// Package momovet provides a go/analysis analyzer that reports SQL text
// built at run time and passed to Momo's raw-SQL functions.
//
// Momo binds every value as a parameter; the statement text of a raw-SQL
// function must therefore be a constant. A string built with concatenation or
// a format call from variable input is an injection point.
//
// Run it with the command in cmd/momovet, as a vet tool:
//
//	go install github.com/azrtydxb/momo/cmd/momovet@latest
//	go vet -vettool="$(which momovet)" ./...
package momovet

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/types/typeutil"
)

// momoPath is the import path of the package whose functions are checked.
const momoPath = "github.com/azrtydxb/momo"

// rawSQLFuncs maps each raw-SQL function of the momo package to the index of
// its SQL text argument.
var rawSQLFuncs = map[string]int{
	"Exec":        2, // Exec(ctx, q, sql, args...)
	"Scalars":     2, // Scalars(ctx, q, sql, args...)
	"SQL":         0, // SQL(sql, args...)
	"TypedSQL":    0, // TypedSQL(sql, args...)
	"RawSubquery": 0, // RawSubquery(sql, args...)
}

// Analyzer flags non-constant SQL strings passed to Momo's raw-SQL entry
// points: momo.Exec and momo.Scalars (argument index 2), momo.SQL,
// momo.TypedSQL and momo.RawSubquery (argument index 0). A string literal, a
// constant, or an expression of constants is accepted.
var Analyzer = &analysis.Analyzer{
	Name:     "momovet",
	Doc:      "report non-constant SQL passed to momo raw-SQL functions",
	URL:      "https://github.com/azrtydxb/momo/blob/main/docs/momovet.md",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	in := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	in.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		name, idx, ok := rawSQLCall(pass.TypesInfo, call)
		if !ok || idx >= len(call.Args) {
			return
		}
		arg := call.Args[idx]
		if tv, found := pass.TypesInfo.Types[arg]; found && tv.Value != nil {
			return // a constant expression
		}
		pass.Reportf(arg.Pos(), "non-constant SQL string passed to momo.%s", name)
	})
	return nil, nil
}

// rawSQLCall reports whether call invokes one of the raw-SQL functions, with
// the function's name and the index of its SQL argument.
func rawSQLCall(info *types.Info, call *ast.CallExpr) (name string, idx int, ok bool) {
	fn, isFunc := typeutil.Callee(info, call).(*types.Func)
	if !isFunc || fn.Pkg() == nil || fn.Pkg().Path() != momoPath {
		return "", 0, false
	}
	if sig, isSig := fn.Type().(*types.Signature); isSig && sig.Recv() != nil {
		return "", 0, false // a method, not a package-level function
	}
	idx, ok = rawSQLFuncs[fn.Name()]
	return fn.Name(), idx, ok
}
