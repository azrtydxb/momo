# Export the expression-extension API from the core

Status: open
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: -

## Description

Expression features cannot move out of the root today because `Expression[T]` has the unexported method `resultType`, and the features call unexported builder helpers (see section 1 of `.procoder/research/package-split.md`). Add the smallest exported surface that lets a subpackage build typed expressions, with no behaviour change: export `exprOps[T]` as `Ops[T]` (embedding it satisfies `Expression[T]` and gives Eq, Ne, Lt, Le, Gt, Ge, IsNull, IsNotNull, Between, In, NotIn, Asc and Desc), add `Typed[T]` to wrap any `Expr` as `Expression[T]`, `PredicateFunc` to wrap a render function as a `Predicate`, `FlagsOf` and an exported `Flagged` interface for sensitive-column propagation, and the builder methods `ArgFor`, `RenderSensitive`, `ExprFor`, `Unsupported` and `WriteByte` (exported spelling of the existing helpers). Add the `VectorSource` accessor needed by `vecop` as an exported method. Each exported name carries a doc comment that states it is an extension point and that using it correctly means writing identifiers through `Ident` and values through `Arg`. This change is additive and non-breaking: existing code keeps compiling, and the internal callers switch to the exported names so there is one implementation.

## Acceptance criteria

- [ ] a new external test package `momo_test` defines a predicate type and a typed expression type that embed `momo.Ops[bool]` and `momo.Ops[string]`, pass them to `Where` and `Columns`, and assert the rendered SQL and args (unit test, would not compile before)
- [ ] a sensitive column passed through a custom expression built with `FlagsOf` and `ArgFor` is masked as `RedactedArg` in a recording hook (unit test; the same property `Flags` has for the JSON expressions today)
- [ ] `Unsupported` called from a custom expression on a dialect that lacks it fails the build with `ErrUnsupported` (unit test)
- [ ] every existing root golden, unit and compile-fail test is unchanged and green, proving no behaviour moved
- [ ] every new exported identifier appears in `docs/ai-guide.md` under a section 'Writing your own expression', with a runnable example in `example_expression_test.go`
- [ ] `apidiff` against main reports compatible additions only (CI output attached)

## Evidence
