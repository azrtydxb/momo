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
	"go/token"
	"go/types"
	"sort"
	"strings"

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
	"MapRows":     2, // MapRows(ctx, q, sql, args...)
	"Query":       2, // Query[T](ctx, q, sql, args...)
	"QueryRows":   2, // QueryRows[T](ctx, q, sql, args...)
	"RawQuery":    1, // RawQuery(q, sql, args...)
	"SQL":         0, // SQL(sql, args...)
	"TypedSQL":    0, // TypedSQL(sql, args...)
	"RawSubquery": 0, // RawSubquery(sql, args...)
}

// directivePrefix starts the line directive that marks a call as vetted.
const directivePrefix = "//momovet:ok"

// directive is one //momovet:ok comment.
type directive struct {
	pos        token.Pos
	reason     string
	standalone bool // alone on its line, so it may cover the call below it
	used       bool
}

type lineKey struct {
	file string
	line int
}

// collectDirectives indexes every //momovet:ok comment by file and line, and
// returns them in source order. Only the exact line-comment form counts:
// "//momovet:ok" followed by the end of the comment or a space or tab.
func collectDirectives(pass *analysis.Pass) (map[lineKey]*directive, []*directive) {
	byLine := map[lineKey]*directive{}
	var all []*directive
	for _, f := range pass.Files {
		var code map[int]bool // lines that hold code, built on first use
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				rest, ok := strings.CutPrefix(c.Text, directivePrefix)
				if !ok || (rest != "" && rest[0] != ' ' && rest[0] != '\t') {
					continue
				}
				if code == nil {
					code = codeLines(pass.Fset, f)
				}
				p := pass.Fset.Position(c.Slash)
				d := &directive{pos: c.Slash, reason: strings.TrimSpace(rest), standalone: !code[p.Line]}
				byLine[lineKey{p.Filename, p.Line}] = d
				all = append(all, d)
			}
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].pos < all[j].pos })
	return byLine, all
}

// codeLines returns the lines on which a syntax node of f starts or ends. A
// comment that shares such a line trails code; any other comment line is
// standalone.
func codeLines(fset *token.FileSet, f *ast.File) map[int]bool {
	lines := map[int]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n.(type) {
		case nil:
			return false
		case *ast.Comment, *ast.CommentGroup:
			return false
		}
		lines[fset.Position(n.Pos()).Line] = true
		lines[fset.Position(n.End()).Line] = true
		return true
	})
	return lines
}

// find returns the directive that covers a call: a trailing comment on the
// line of the call or of its SQL argument, or a standalone comment on the line
// directly above the call. A trailing comment never covers the next line.
func find(pass *analysis.Pass, dirs map[lineKey]*directive, call *ast.CallExpr, arg ast.Expr) *directive {
	c := pass.Fset.Position(call.Pos())
	a := pass.Fset.Position(arg.Pos())
	for _, k := range []lineKey{{c.Filename, c.Line}, {a.Filename, a.Line}} {
		if d, ok := dirs[k]; ok {
			return d
		}
	}
	if d, ok := dirs[lineKey{c.Filename, c.Line - 1}]; ok && d.standalone {
		return d
	}
	return nil
}

// Analyzer flags non-constant SQL strings passed to Momo's raw-SQL entry
// points: momo.Exec, momo.Scalars, momo.MapRows, momo.Query and momo.QueryRows (argument index 2), momo.RawQuery (index 1), momo.SQL,
// momo.TypedSQL and momo.RawSubquery (argument index 0). A string literal, a
// constant, or an expression of constants is accepted. A //momovet:ok <reason>
// comment on the call's line, or alone on the line above, suppresses the
// report for that call; the directive is itself reported when it has no reason or
// suppresses nothing.
var Analyzer = &analysis.Analyzer{
	Name:     "momovet",
	Doc:      "report non-constant SQL passed to momo raw-SQL functions",
	URL:      "https://github.com/azrtydxb/momo/blob/main/docs/momovet.md",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	dirs, ordered := collectDirectives(pass)
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
		if d := find(pass, dirs, call, arg); d != nil {
			d.used = true
			return
		}
		pass.Reportf(arg.Pos(), "non-constant SQL string passed to momo.%s", name)
	})
	for _, d := range ordered {
		switch {
		case d.reason == "":
			pass.Reportf(d.pos, "%s needs a reason: //momovet:ok <reason>", directivePrefix)
		case !d.used:
			pass.Reportf(d.pos, "unused %s directive: no non-constant momo SQL call here", directivePrefix)
		}
	}
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
