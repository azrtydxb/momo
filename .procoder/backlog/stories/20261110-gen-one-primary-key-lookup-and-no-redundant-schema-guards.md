# gen: one primary-key lookup and no redundant schema guards

Status: open
Created: 2026-11-10
Epic: -
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

- [ ] One exported helper returns a model's primary key columns, and the four
      plugins call it. Every generator golden and the regenerated integration
      examples are unchanged on CI.
- [ ] No `g.Schema != nil` or `g.Schema == nil` guard is left around a
      `Graph.TableOf` call, and the private `tableOf` is gone.
- [ ] `DescriptorIdents` and `EmitDescriptors` share one helper for the used
      identifiers.
- [ ] `buildPlugins` either loses the unreachable branch or has a test that
      calls it with an unknown name.

## Evidence
