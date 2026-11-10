// SPDX-License-Identifier: Apache-2.0

package sqlitepure

// The directive below declares this package's database status. internal/gendocs
// reads it as a comment, so the generator links neither this package nor a
// database driver, and writes the status into the README and the docs. Fields:
// dialect (postgres, sqlite or mysql), usable (the package runs queries and
// writes for that dialect), cgo (the package needs cgo), windows (the package
// builds on Windows) and vectors (yes, no, or server when the answer depends on
// the server version). internal/gendocs tests the declaration against this
// package's source.

//momo:driver dialect=sqlite usable=true cgo=false windows=true vectors=no
