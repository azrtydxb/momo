// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"fmt"
	"regexp"
)

// PluginAPIVersion is the generator plugin API version this momo supports.
const PluginAPIVersion = 1

// Plugin extends generation. Emit runs the plugins of Graph.Plugins, in
// order, after the built-in emitters. A plugin receives the normalized
// schema and relation graph and must emit files only through w, so its
// output gets the same collision and handwritten-file protection as the
// built-in emitters. Name must match ^[a-z][a-z0-9_]*$.
type Plugin interface {
	Name() string
	Generate(g *Graph, w *Writer) error
}

// VersionedPlugin is implemented by a plugin that targets a specific plugin
// API version. A plugin without it targets version 1.
type VersionedPlugin interface {
	APIVersion() int
}

var pluginNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// validatePlugin checks the name and API version of p.
func validatePlugin(p Plugin) error {
	name := p.Name()
	if !pluginNameRE.MatchString(name) {
		return fmt.Errorf("invalid plugin name %q", name)
	}
	if vp, ok := p.(VersionedPlugin); ok {
		if v := vp.APIVersion(); v != PluginAPIVersion {
			return fmt.Errorf("plugin %s targets API v%d; this momo supports v%d", name, v, PluginAPIVersion)
		}
	}
	return nil
}
