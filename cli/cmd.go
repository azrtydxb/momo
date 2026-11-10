// SPDX-License-Identifier: Apache-2.0

// Package cli is the momo command line tool as a library. The momo binary
// (cmd/momo) is a thin caller of Main; a team that writes its own gen.Plugin
// builds a binary with the same behavior and its plugins selectable from
// momo.yaml:
//
//	package main
//
//	import "github.com/azrtydxb/momo/cli"
//
//	func main() { cli.Main(myPlugin{}) }
//
// Build it with go build and run it where you would run momo: the commands,
// flags, exit codes and diagnostics are the ones of `momo generate`, and a
// plugin passed to Main is enabled by naming it in the plugins section of
// momo.yaml. A plugin that is not passed to Main cannot be enabled: naming it
// is a configuration error (exit 2).
package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/migrate"
)

// Exit codes of the CLI contract, documented in `momo help`.
const (
	exitOK       = 0 // success, or generated code is current
	exitFindings = 1 // stale generated files, or generator diagnostics
	exitUsage    = 2 // usage, configuration or infrastructure error
)

const usage = `Usage: momo <command> [flags]

Commands:
  generate  generate code for model packages, or models from a database (flags: -c momo.yaml, --check, --from-db)
  import    convert GORM, Bun or Ent models to Momo models (momo import gorm|bun|ent --src <dir> --out <dir>)
  migrate   manage schema migrations (momo migrate help)
  viz       draw an ER diagram, Mermaid or DOT (flags: --format, --snapshot, --out)
  help      print this help ("momo help generate" for a command)

Plugins: momo generate runs the plugins built into this binary (graphql, grpc,
openapi, viz), enabled and configured in the plugins section of momo.yaml, and
any plugin you pass to cli.Main(plugins...) from github.com/azrtydxb/momo/cli
(see the package documentation). Or call gen.Emit with your plugins from Go
code.

Exit codes:
  0  success, or generated code is current
  1  stale generated files, or generator or viz diagnostics
  2  usage, configuration or infrastructure error
  3  database, lock or execution error (momo migrate)
`

// command is one subcommand. Further subcommands (for example migrate) are
// added to commands without touching Run.
type command struct {
	name string
	run  func(args []string, stdout, stderr io.Writer) int
}

func commands(plugins []gen.Plugin) []command {
	return []command{
		{"generate", func(args []string, stdout, stderr io.Writer) int {
			return runGenerate(args, stdout, stderr, plugins...)
		}},
		{"import", runImportCmd},
		{"migrate", runMigrateCmd},
		{"viz", runViz},
	}
}

// Main is the entry point of a custom momo binary: put it in a main function
// and the binary behaves as `momo` does (commands, flags, exit codes,
// diagnostics) with the given generator plugins selectable by name from the
// plugins section of momo.yaml. It runs Run with the process arguments, standard
// output and standard error, then exits with the code Run returns. Main does
// not return, so write nothing after it.
func Main(plugins ...gen.Plugin) {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr, plugins...))
}

// Run executes the momo CLI with args (without the program name), writes its
// output to stdout and its diagnostics to stderr, and returns the process exit
// code: 0 for success or current generated code, 1 for stale generated files or
// generator diagnostics, 2 for a usage, configuration or infrastructure error.
// The plugins are the generator plugins this run offers; momo.yaml enables them
// by name, and naming one that is not passed here is a configuration error. Run
// is the testable form of Main: a custom binary calls Main, a test calls Run
// with its own writers.
func Run(args []string, stdout, stderr io.Writer, plugins ...gen.Plugin) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage)
		return exitUsage
	}
	cmds := commands(plugins)
	switch args[0] {
	case "help", "-h", "--help":
		if args[0] == "help" && len(args) > 1 {
			for _, c := range cmds {
				if c.name == args[1] {
					return c.run([]string{"-h"}, stdout, stderr)
				}
			}
			_, _ = fmt.Fprintf(stderr, "momo: no help for %q\n\n%s", migrate.RedactURLs(args[1]), usage)
			return exitUsage
		}
		_, _ = fmt.Fprint(stdout, usage)
		return exitOK
	}
	for _, c := range cmds {
		if c.name == args[0] {
			return c.run(args[1:], stdout, stderr)
		}
	}
	_, _ = fmt.Fprintf(stderr, "momo: unknown command %q\n\n%s", migrate.RedactURLs(args[0]), usage)
	return exitUsage
}
