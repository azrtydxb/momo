// SPDX-License-Identifier: Apache-2.0

// Command momo is the momo command line tool. Subcommands are registered in
// cmd.go; see `momo help` for the list and the exit-code contract. The binary
// runs only its built-in plugins; custom generators use the Go API (gen.Emit
// with plugins).
package main

import "os"

func main() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}
