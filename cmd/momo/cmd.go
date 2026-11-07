// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"

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
  generate  generate code for model packages (flags: -c momo.yaml, --check)
  migrate   manage schema migrations (momo migrate help)
  help      print this help ("momo help generate" for a command)

Plugins: momo generate runs only the plugins built into this binary. For
custom generator plugins use the Go API (gen.Emit with plugins).

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

func commands() []command {
	return []command{
		{"generate", runGenerate},
		{"migrate", runMigrateCmd},
	}
}

// plugins returns the built-in generator plugins as a fresh slice, so no
// global state is kept. It is empty until plugins ship with the binary.
// External plugin registration is not supported by the binary; custom
// generators use the Go API (gen.Emit with plugins).
func plugins() []gen.Plugin {
	return []gen.Plugin{}
}

// Run executes the momo CLI with args (without the program name) and returns
// the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage)
		return exitUsage
	}
	switch args[0] {
	case "help", "-h", "--help":
		if args[0] == "help" && len(args) > 1 {
			for _, c := range commands() {
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
	for _, c := range commands() {
		if c.name == args[0] {
			return c.run(args[1:], stdout, stderr)
		}
	}
	_, _ = fmt.Fprintf(stderr, "momo: unknown command %q\n\n%s", migrate.RedactURLs(args[0]), usage)
	return exitUsage
}
