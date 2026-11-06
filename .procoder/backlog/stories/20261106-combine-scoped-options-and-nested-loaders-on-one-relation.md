# Combine scoped options and nested loaders on one relation

Status: open
Created: 2026-11-06
Epic: momo-06-relations
Sprint: -

## Description

As a momo user loading associations eagerly, I want to filter and order a relation and also load nested relations on its children in one loader, so that "approved comments, newest first, each with its author" (spec S-81) is expressible.

Today a loader is built with either `Relation.Scope(opts...)` or `Relation.Then(loaders...)` (`loader.go`, around lines 92-99). `Scope` calls `scoped(opts, nil)` and `Then` calls `scoped(nil, nested)`, and both return a `Loader[P]`, which has no `Scope` or `Then` method. So the two cannot be chained or combined. A caller who needs a scoped relation with a nested load has no way to say it.

Proposal, pick one in the design step:

- Add `Then` (and `Scope`) to the returned loader type, so `document.Comments.Scope(opts...).Then(comment.Author)` works. `Scope` and `Then` would then return a public generic builder (for example `RelationLoader[P, C]`) that still implements `Loader[P]`.
- Or add a `LoadThen` load option that takes the nested loaders, so `document.Comments.Scope(LoadWhere(...), LoadOrderBy(...), LoadThen(comment.Author))` composes inside one option list.

Whichever is chosen must keep existing `Scope(...)` and `Then(...)` calls compiling and behaving as they do now. It must also keep the batching guarantees: one statement per bind-limit chunk at the top level and one nested statement per level over the whole pool of children.

Done means a single loader can carry a where, an order and nested loaders, with the scope applied to the relation's children and nested loads run over exactly those filtered children.

## Acceptance criteria

- [ ] An integration test loads `document.Comments` with `LoadWhere(approved = true)`, `LoadOrderBy(id desc)` and a nested load of each comment's parent document (or another nested relation) in one loader. It asserts the filtered, ordered comments and the nested value on every returned comment.
- [ ] The same test asserts the statement count: one for the top level plus one for the nested level, however many comments match.
- [ ] Nested loads run only over the filtered children: a comment excluded by `LoadWhere` is never used as a nested parent (assert via a recording `ParentKey` or the statement's bound keys).
- [ ] Combining works at a nested level too: `document.Author.Then(author.FirstDocument.<combined form>(...))`, with the scope applying to the deepest relation only.
- [ ] Existing `Scope(opts...)` and `Then(loaders...)` call sites compile unchanged and all current load tests still pass.
- [ ] Calling the combined form with no options or no nested loaders behaves like the plain `Relation`, `Scope` or `Then`.
- [ ] An error or cancelled context at the nested level of a combined loader is returned from `LoadAll`, not swallowed (mutation check as in `TestNestedLoadErrorPropagates` and `TestNestedLoadHonoursContext`).
- [ ] `procoder check` gate is clean and the change is committed.

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
