# Refactor: one comparison method set and consistent error construction

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Maintainability items 5 and 7: the comparison method set is written seven times (Column, Agg, exprOps, permission wrappers) and Agg already lacks methods; errors are built through 10 helpers and 28 inline literals, and some bare errors.New values cannot be matched with errors.Is. Audit report: .procoder/research/security-audit-2026-11-07.md and .procoder/research/maintainability-audit-2026-11-07.md.

## Acceptance criteria

- [x] One shared comparison implementation backs Column, Agg, expressions and permission wrappers; Agg gains the missing methods (tests)
- [x] Every library error is matchable with errors.Is against a sentinel (test sweeping error sites)
- [x] No public API removed without a deprecation

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

- Criterion 1: CI run 42414213760 (ci-agent, e02c5fc) green. Column, Agg, expressions and the four permission wrappers share exprOps through colCore; Agg gains EqExpr, In, NotIn, Between, IsNull, IsNotNull, IsDistinctFrom, InQuery, NotInQuery. comparison_shared_test.go asserts the method set at compile time and renders an Agg comparison.
- Criterion 2: run 42414213760. TestLibraryErrorsAreMatchable sweeps the sources for bare momo errors; bare errors now go through newError, validation and noQuerier with ErrValidation. The two select-cancelled errors still wrap the context error without a Kind.
- Criterion 3: no public API removed; the delta below is additive. The wrappers keep Excluded returning Expression, and Agg could never be built from positional fields outside the package (unexported fn).

## Public API delta (additive) and behaviour notes

- Agg gains EqExpr, NeExpr, LtExpr, LeExpr, GtExpr, GeExpr, In, NotIn, Between, IsNull, IsNotNull, IsDistinctFrom, IsNotDistinctFrom, InQuery and NotInQuery.
- Every host of the shared comparison set (FuncExpr, CaseExpr, ArithExpr, CastExpr, ConcatExpr, LengthExpr, ExcludedExpr) gains IsDistinctFrom, IsNotDistinctFrom, InQuery and NotInQuery.
- Errors that were bare (Commit and Rollback on a savepoint scope, a query without a Querier, Scan without a current row, ScanModel into nil, no dialect) now carry ErrValidation, so errors.Is matches them. The driver batch-result-count mismatch carries ErrUnsupported (a driver fault, not a validation failure).
- The zero value Column[T]{} no longer renders an empty reference: using it in a comparison fails the build with ErrValidation. Documented on Column.
- The permission wrappers carry distinct unexported marker fields, so no conversion between Column and a wrapper, or between wrappers, compiles. TestPermissionWrappersAreNotConvertible asserts it through reflect ConvertibleTo, which follows the compiler's conversion rule.
- TestLibraryErrorsAreMatchable parses the sources with go/ast and flags errors.New or fmt.Errorf calls with a "momo:" message and no %w.

