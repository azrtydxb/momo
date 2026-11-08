// SPDX-License-Identifier: Apache-2.0

//go:build tools

// Package integration holds the tests that need gqlgen, gRPC, protoc or the
// OpenAPI validator. It is a separate module so the generator packages keep
// those dependencies out of their import graph.
package integration

import (
	_ "github.com/99designs/gqlgen"
	_ "github.com/azrtydxb/momo"
	_ "github.com/santhosh-tekuri/jsonschema/v6"
	_ "google.golang.org/grpc"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
	_ "google.golang.org/protobuf/proto"
)
