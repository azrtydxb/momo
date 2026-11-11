# gen: one primary-key lookup and no redundant schema guards

Status: done 2026-11-10
Created: 2026-11-10
Epic: -
Sprint: 008-sprint-8-fix-the-kuvryn-orbit-port-issues-73-to-79-and-the
Type: story

## Description

The v0.6.0 pre-PR review's simplify lens found duplicated and dead code in the
generator plugins. Nothing it found changes behaviour, so the cuts were kept
out of the release.

- The "primary key columns of the model's table" lookup is written four times:
  `singleIntPK` in gen/graphql/sdl.go, `SinglePK` in gen/internal/gosrc/gosrc.go,
  `writeModel` in gen/grpc/proto.go and `propertiesOf` in gen/openapi/doc.go.
- `Graph.TableOf` already returns false for a nil schema, but gen/grpc/proto.go
  (twice), gen/openapi/doc.go, gen/graphql/sdl.go and gen/internal/gosrc/gosrc.go
  still guard it with `g.Schema != nil` or `g.Schema == nil`.
- The private `tableOf` in gen/emit_meta.go only unwraps `Graph.TableOf`.
- `DescriptorIdents` and `EmitDescriptors` in gen/emit_descriptors.go both build
  the same set of used identifiers.
- The `default: unknown plugin` branch of `buildPlugins` in cli/plugins.go
  cannot be reached from `selectPlugins`.

## Acceptance criteria

- [x] One exported helper returns a model's primary key columns, and the four
      plugins call it. Every generator golden and the regenerated integration
      examples are unchanged on CI.
- [x] No `g.Schema != nil` or `g.Schema == nil` guard is left around a
      `Graph.TableOf` call, and the private `tableOf` is gone.
- [x] `DescriptorIdents` and `EmitDescriptors` share one helper for the used
      identifiers.
- [x] `buildPlugins` either loses the unreachable branch or has a test that
      calls it with an unknown name.

## Evidence

Built as a pure refactor (commits 08fa364, 60c8348, 32f76ff). The test for a
refactor is that nothing generated changes, so the guard is the existing
goldens and the integration examples, not a new test.

- gen.Graph.PrimaryKey(m) returns the primary key columns of m's table (nil
  without one). singleIntPK (gen/graphql/sdl.go), SinglePK
  (gen/internal/gosrc/gosrc.go), writeModel (gen/grpc/proto.go) and
  propertiesOf (gen/openapi/doc.go) call it.
- Every g.Schema != nil / == nil guard around Graph.TableOf is gone, and so is
  the private tableOf in gen/emit_meta.go (its caller uses g.TableOf).
- usedIdents in gen/emit_descriptors.go builds the used-identifier set for both
  DescriptorIdents and EmitDescriptors.
- buildPlugins keeps its default branch: cli/plugins_test.go
  TestPluginRegistration already calls it with the unknown name "soap" and
  asserts the error (line 50). That test passes in the unit job below. It was
  written before this story and was not made to fail here.

CI, ci-agent run 42907768963, SHA 32f76ff203064e3991d87af737199e54366d3fe9: all
nine jobs green. lint (job 130150332310) runs gen-check, then make
gen-integration followed by a diff that must be empty and a clean status, so
the regenerated integration examples are byte-identical; unit (130150332162)
runs every generator golden. An earlier run, 42905678676 on 60c8348, was
cancelled after its lint and unit jobs failed to compile (usedIdents took the
wrong map type) and apidiff-docs asked for a docs escape; both were fixed in
the following commits.
