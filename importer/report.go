// SPDX-License-Identifier: Apache-2.0

// Package importer converts models written for another Go ORM into Momo
// models. Readers are static analyzers: they parse source with go/parser and
// never execute user code. Every behaviour that differs between the source
// ORM and Momo is recorded in a Report instead of being converted silently.
package importer

import (
	"cmp"
	"slices"
	"strings"
)

// Kind classifies one semantic change.
type Kind string

// The kinds of semantic change an import can report.
const (
	ZeroValueHandling Kind = "zero-value-handling"
	ImplicitSave      Kind = "implicit-save"
	GoDefault         Kind = "go-default"
	Unsupported       Kind = "unsupported"
	RenamedType       Kind = "renamed-type"
	DroppedField      Kind = "dropped-field"
	DroppedIndex      Kind = "dropped-index"
	Naming            Kind = "naming"
	VersionMapped     Kind = "version-mapped"
	GoSideRule        Kind = "go-side-rule" // a validator, immutable field or hook the source ORM runs in Go
)

// Change is one reported difference between the source ORM and Momo.
type Change struct {
	Kind Kind
	// Source is file:line:col in the source ORM project.
	Source  string
	Model   string
	Field   string
	Message string
}

// Report collects the changes found while reading a project.
type Report struct{ Changes []Change }

// Add records c.
func (r *Report) Add(c Change) { r.Changes = append(r.Changes, c) }

// Has reports whether any change has the given kind.
func (r *Report) Has(kind Kind) bool {
	return slices.ContainsFunc(r.Changes, func(c Change) bool { return c.Kind == kind })
}

// Of returns the changes of the given kind in report order.
func (r *Report) Of(kind Kind) []Change {
	var out []Change
	for _, c := range r.Changes {
		if c.Kind == kind {
			out = append(out, c)
		}
	}
	return out
}

// String renders one line per change, sorted by Source (then kind, model and
// field, so the output is deterministic).
func (r *Report) String() string {
	cs := slices.Clone(r.Changes)
	slices.SortStableFunc(cs, func(a, b Change) int {
		return cmp.Or(
			cmp.Compare(a.Source, b.Source),
			cmp.Compare(a.Kind, b.Kind),
			cmp.Compare(a.Model, b.Model),
			cmp.Compare(a.Field, b.Field),
		)
	})
	var sb strings.Builder
	for _, c := range cs {
		sb.WriteString(c.Source)
		sb.WriteString(": ")
		sb.WriteString(string(c.Kind))
		if c.Model != "" {
			sb.WriteString(": ")
			sb.WriteString(c.Model)
			if c.Field != "" {
				sb.WriteString(".")
				sb.WriteString(c.Field)
			}
		}
		sb.WriteString(": ")
		sb.WriteString(c.Message)
		sb.WriteString("\n")
	}
	return sb.String()
}
