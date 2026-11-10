// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"path/filepath"

	"github.com/azrtydxb/momo/gen"
)

// Plugin is the gen.Plugin that writes the .proto file, its field-number lock
// and the Go server adapters.
type Plugin struct {
	// Dir is the output directory.
	Dir string
	// Package is the Go package name of the server adapters; it defaults to
	// the base name of Dir.
	Package string
	// ProtoPackage is the protobuf package, for example momo.example.v1.
	ProtoPackage string
	// GoPackage is the import path of the Go package protoc-gen-go generates
	// from the .proto file. It becomes the go_package option and the import of
	// the adapters.
	GoPackage string
}

// Name returns "grpc".
func (Plugin) Name() string { return "grpc" }

// APIVersion returns gen.PluginAPIVersion.
func (Plugin) APIVersion() int { return gen.PluginAPIVersion }

// Generate writes <Dir>/momo.proto and <Dir>/momo.proto.lock through w.Text
// and <Dir>/server_gen.go through w.File. The lock is read from an existing
// <Dir>/momo.proto.lock, so field numbers never change between runs.
func (p Plugin) Generate(g *gen.Graph, w *gen.Writer) error {
	lockPath := filepath.Join(p.Dir, "momo.proto.lock")
	lock, err := LoadLock(lockPath)
	if err != nil {
		return err
	}
	src, diags := protoSource(g, lock, p.ProtoPackage, p.GoPackage)
	if len(diags) > 0 {
		return &gen.ErrDiagnostics{Diagnostics: diags}
	}
	raw, err := lock.Marshal()
	if err != nil {
		return err
	}
	if err := w.Text(filepath.Join(p.Dir, "momo.proto"), src); err != nil {
		return err
	}
	// The lock is JSON, which has no comment syntax, so it is not a generated
	// file the writer can tell from a handwritten one by a header. It goes
	// through the writer all the same: see lockFile.
	if err := w.Text(lockPath, lockFile(raw)); err != nil {
		return err
	}
	pkg := p.Package
	if pkg == "" {
		pkg = filepath.Base(p.Dir)
	}
	srv, diags := ServerSource(g, lock, pkg, p.GoPackage)
	if len(diags) > 0 {
		return &gen.ErrDiagnostics{Diagnostics: diags}
	}
	return w.File(filepath.Join(p.Dir, "server_gen.go"), srv)
}
