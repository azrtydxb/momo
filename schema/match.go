// SPDX-License-Identifier: Apache-2.0

package schema

import "path"

// MatchObject reports whether any glob matches the object name, or
// schema.name when schemaName is not empty (the current schema is spelled with
// an empty one, so a glob such as public.users never matches a table in the
// current schema: use its bare name). It is the one matching rule behind
// migrate.exclude, introspect.skip and introspect.only. A malformed glob never
// matches; the callers validate globs separately.
func MatchObject(globs []string, schemaName, name string) bool {
	for _, g := range globs {
		if ok, _ := path.Match(g, name); ok {
			return true
		}
		if schemaName != "" {
			if ok, _ := path.Match(g, schemaName+"."+name); ok {
				return true
			}
		}
	}
	return false
}
