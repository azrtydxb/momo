# Export the expression-extension API from the core

Status: done
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

Expression features cannot move out of the root today because `Expression[T]` has the unexported method `resultType`, and the features call unexported builder helpers (see section 1 of `.procoder/research/package-split.md`). Add the smallest exported surface that lets a subpackage build typed expressions, with no behaviour change: export `exprOps[T]` as `Ops[T]` (embedding it satisfies `Expression[T]` and gives Eq, Ne, Lt, Le, Gt, Ge, IsNull, IsNotNull, Between, In, NotIn, Asc and Desc), add `Typed[T]` to wrap any `Expr` as `Expression[T]`, `PredicateFunc` to wrap a render function as a `Predicate`, `FlagsOf` and an exported `Flagged` interface for sensitive-column propagation, and the builder methods `ArgFor`, `RenderSensitive`, `ExprFor`, `Unsupported` and `WriteByte` (exported spelling of the existing helpers). Add the `VectorSource` accessor needed by `vecop` as an exported method. Each exported name carries a doc comment that states it is an extension point and that using it correctly means writing identifiers through `Ident` and values through `Arg`. This change is additive and non-breaking: existing code keeps compiling, and the internal callers switch to the exported names so there is one implementation.

## Acceptance criteria

- [x] a new external test package `momo_test` defines a predicate type and a typed expression type that embed `momo.Ops[bool]` and `momo.Ops[string]`, pass them to `Where` and `Columns`, and assert the rendered SQL and args (unit test, would not compile before)
- [x] a sensitive column passed through a custom expression built with `FlagsOf` and `ArgFor` is masked as `RedactedArg` in a recording hook (unit test; the same property `Flags` has for the JSON expressions today)
- [x] `Unsupported` called from a custom expression on a dialect that lacks it fails the build with `ErrUnsupported` (unit test)
- [x] every existing root golden, unit and compile-fail test is unchanged and green, proving no behaviour moved
- [x] every new exported identifier appears in `docs/ai-guide.md` under a section 'Writing your own expression', with a runnable example in `example_expression_test.go`
- [x] `apidiff` against main reports compatible additions only (CI output attached)

## Evidence

- Red (criteria 1-3): ci-agent run 42494391921 on throwaway branch `worktree-agent-a06d0f826f0fcb7e9-red` (only `expression_extension_test.go`, package `momo_test`); every job failed with `undefined: momo.Ops`, `undefined: momo.NewOps`, `undefined: momo.FlagsOf`. Branch deleted.
- Criterion 1: `TestExtensionOpsCustomExpressions` (custom `momo.Ops[bool]` predicate and `momo.Ops[string]` expression through `Columns` and `Where`, SQL and args asserted) and `TestExtensionTypedAndPredicateFunc`, green in ci-agent run 42505016647 (headSha 321e194).
- Criterion 2: `TestExtensionMasksSensitiveArgs` (FlagsOf, ArgFor, RenderSensitive, ExprFor, Ops.Eq; sensitive masked as `RedactedArg`, plain visible, driver gets the real value), same run.
- Criterion 3: `TestExtensionUnsupported` (ErrUnsupported on SQLite and MySQL, PostgreSQL builds), same run.
- Criterion 4: no existing test file changed; unit, race, compile-fail, fuzz-smoke, bench-module, lint and the PostgreSQL, MySQL 8.4, MySQL 9.7 and MariaDB integration jobs all green in run 42505016647.
- Criterion 5: section "Writing your own expression" in `docs/ai-guide.md` (a complete program, passes `TestDocsCodeBlocks`; docs run 42505016530 green); runnable `ExampleOps` and `ExamplePredicateFunc` in `example_expression_test.go`.
- Criterion 6: one-off apidiff run 42494703288 (throwaway branch, deleted) against origin/sprint-6 and origin/main printed "Compatible changes" only (`(*Builder).ArgFor`, `ExprFor`, `RenderSensitive`, `Unsupported`, `Flagged`, `FlagsOf`, `NewOps`, `Ops`, `PredicateFunc`, `Typed`, `VectorColumnOf` added) and an empty "incompatible only" section.
- Deviations: the `VectorSource` accessor is the generic function `VectorColumnOf`, not an interface method, because a new interface method is apidiff-incompatible. `NewOps` is added because `Ops.self` is unexported. `Ops` also defines `resultType` so embedders satisfy `Expression[T]`. `exprOps` stays as a generic alias of `Ops`, and the old unexported `argFor`, `renderSens`, `exprFor`, `unsupported`, `flagsOf` forward to the exported names, so call sites in files owned by the parallel internal-extraction work were not touched.
