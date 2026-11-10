// SPDX-License-Identifier: Apache-2.0

// Command momo is the momo command line tool. It is a thin caller of
// cli.Main; see `momo help` for the commands and the exit-code contract. The
// stock binary ships no generator plugins; to run your own, build a binary
// with cli.Main(plugins...) (see package cli).
package main

import "github.com/azrtydxb/momo/cli"

func main() {
	cli.Main()
}
