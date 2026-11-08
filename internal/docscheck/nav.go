// SPDX-License-Identifier: Apache-2.0

// Package docscheck holds the checks that keep the documentation site honest:
// the navigation lists real pages, every page is reachable, the Go code blocks
// compile, and the agent guide carries its rules. The package has no runtime
// users; its tests read docs/, mkdocs.yml and the built site.
package docscheck

import "errors"

// NavPages returns every .md path listed in the nav of <root>/mkdocs.yml, in
// document order, relative to docs/.
func NavPages(root string) ([]string, error) {
	return nil, errors.New("docscheck: NavPages is not implemented")
}
