// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/graphql"
	"github.com/azrtydxb/momo/gen/grpc"
	"github.com/azrtydxb/momo/gen/openapi"
	"github.com/azrtydxb/momo/gen/viz"
)

// pluginOptions reads the options of one plugin and remembers which it has
// used, so an option the plugin does not know is an error.
type pluginOptions struct {
	plugin string
	out    string
	opts   map[string]any
	used   map[string]bool
	err    error
}

func newPluginOptions(plugin, out string, opts map[string]any) *pluginOptions {
	return &pluginOptions{plugin: plugin, out: out, opts: opts, used: map[string]bool{}}
}

func (o *pluginOptions) fail(format string, args ...any) {
	if o.err == nil {
		o.err = fmt.Errorf(format, args...)
	}
}

// str returns the string option key, or "" when it is unset.
func (o *pluginOptions) str(key string) string {
	o.used[key] = true
	v, ok := o.opts[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		o.fail("option %q of plugin %q must be a string", key, o.plugin)
	}
	return s
}

// boolean returns the boolean option key, or false when it is unset.
func (o *pluginOptions) boolean(key string) bool {
	o.used[key] = true
	v, ok := o.opts[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		o.fail("option %q of plugin %q must be true or false", key, o.plugin)
	}
	return b
}

// dir returns the output directory option: required, and resolved against
// the output base when it is relative.
func (o *pluginOptions) dir() string {
	d := o.str("dir")
	if d == "" {
		o.fail("plugin %q needs the option \"dir\"", o.plugin)
		return ""
	}
	return o.resolve(d)
}

func (o *pluginOptions) resolve(d string) string {
	if o.out != "" && !filepath.IsAbs(d) {
		return filepath.Join(o.out, d)
	}
	return d
}

// finish reports an option nobody asked for, else the first option error.
func (o *pluginOptions) finish() error {
	keys := make([]string, 0, len(o.opts))
	for k := range o.opts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !o.used[k] {
			return fmt.Errorf("unknown option %q for plugin %q", k, o.plugin)
		}
	}
	return o.err
}

// builtinPlugins names the plugins built into the binary.
var builtinPlugins = []string{"graphql", "grpc", "openapi", "viz"}

// buildPlugins returns the built-in plugins configured by the plugins section
// of momo.yaml, in the order of the sorted plugin names, so the output order
// is deterministic. A relative dir is resolved against out. An unknown plugin
// name or option is an error; there is no registry, so a custom generator uses
// the Go API (gen.Emit with plugins).
func buildPlugins(cfg map[string]map[string]any, out string) ([]gen.Plugin, error) {
	names := make([]string, 0, len(cfg))
	for n := range cfg {
		names = append(names, n)
	}
	sort.Strings(names)
	var ps []gen.Plugin
	for _, n := range names {
		o := newPluginOptions(n, out, cfg[n])
		var p gen.Plugin
		switch n {
		case "graphql":
			p = graphql.Plugin{
				Dir:           o.dir(),
				Package:       o.str("package"),
				ModelsPackage: o.str("models_package"),
				ImportPath:    o.str("import_path"),
			}
		case "grpc":
			p = grpc.Plugin{
				Dir:          o.dir(),
				Package:      o.str("package"),
				ProtoPackage: o.str("proto_package"),
				GoPackage:    o.str("go_package"),
			}
		case "openapi":
			p = openapi.Plugin{
				Dir:     o.dir(),
				Package: o.str("package"),
				Options: openapi.Options{
					Title:    o.str("title"),
					Version:  o.str("version"),
					BasePath: o.str("base_path"),
					Handlers: o.boolean("handlers"),
				},
			}
		case "viz":
			format := o.str("format")
			if format == "" {
				format = "mermaid"
			}
			if format != "mermaid" && format != "dot" {
				o.fail("option \"format\" of plugin \"viz\" must be mermaid or dot, got %q", format)
			}
			dir := o.out
			if d := o.str("dir"); d != "" {
				dir = o.resolve(d)
			}
			p = viz.Plugin{Format: format, Out: dir}
		default:
			return nil, fmt.Errorf("unknown plugin %q: the momo binary only runs its built-in plugins (%s); "+
				"for custom generators call gen.Emit with your plugins from Go code", n, strings.Join(builtinPlugins, ", "))
		}
		if err := o.finish(); err != nil {
			return nil, err
		}
		ps = append(ps, p)
	}
	return ps, nil
}
