// SPDX-License-Identifier: Apache-2.0

// Package docscheck holds the checks that keep the documentation site honest:
// the navigation lists real pages, every page is reachable, the Go code blocks
// compile, and the agent guide carries its rules. The package has no runtime
// users; its tests read docs/, mkdocs.yml and the built site.
package docscheck

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// NavPages returns every .md path listed in the nav of <root>/mkdocs.yml, in
// document order, relative to docs/. Entries that are not Markdown pages, such
// as external links, are skipped.
func NavPages(root string) ([]string, error) {
	b, err := os.ReadFile(filepath.Join(root, "mkdocs.yml")) // #nosec G304 -- fixed file name under the repository root
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Nav []any `yaml:"nav"`
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("mkdocs.yml: %w", err)
	}
	if len(cfg.Nav) == 0 {
		return nil, fmt.Errorf("mkdocs.yml has no nav")
	}
	var pages []string
	walkNav(cfg.Nav, &pages)
	return pages, nil
}

func walkNav(v any, pages *[]string) {
	switch x := v.(type) {
	case string:
		if strings.HasSuffix(x, ".md") {
			*pages = append(*pages, x)
		}
	case []any:
		for _, e := range x {
			walkNav(e, pages)
		}
	case map[string]any:
		// A nav entry is a one-key map: title -> page or nested list.
		for _, e := range x {
			walkNav(e, pages)
		}
	}
}
