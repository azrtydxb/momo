#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Regenerates the committed example output of the ecosystem generators under
# internal/integration: the plugin output (momo generate with
# internal/integration/momo.yaml), the protoc stubs of the gRPC example and the
# gqlgen executor of the GraphQL example. Needs protoc (make protoc).
set -euo pipefail

GO=${GO:-go}
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

$GO run ./cmd/momo generate -c internal/integration/momo.yaml

tools=$(mktemp -d)
trap 'rm -rf "$tools"' EXIT
cd "$root/internal/integration"
$GO build -o "$tools/protoc-gen-go" google.golang.org/protobuf/cmd/protoc-gen-go
$GO build -o "$tools/protoc-gen-go-grpc" google.golang.org/grpc/cmd/protoc-gen-go-grpc
mkdir -p grpc/example/pb
protoc-dist/bin/protoc \
	--plugin=protoc-gen-go="$tools/protoc-gen-go" --plugin=protoc-gen-go-grpc="$tools/protoc-gen-go-grpc" \
	--proto_path=grpc/example \
	--go_out=grpc/example/pb --go_opt=paths=source_relative \
	--go-grpc_out=grpc/example/pb --go-grpc_opt=paths=source_relative \
	momo.proto

cd "$root/internal/integration/graphql/example/graph"
$GO run github.com/99designs/gqlgen generate
