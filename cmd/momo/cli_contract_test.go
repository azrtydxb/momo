// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLIExitCodes(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Run([]string{"help"}, &out, &errb); code != 0 {
		t.Fatalf("help: exit %d", code)
	}
	for _, want := range []string{
		"Exit codes:",
		"  0  success, or generated code is current",
		"  1  stale generated files, or generator or viz diagnostics",
		"  2  usage, configuration or infrastructure error",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help lacks %q:\n%s", want, out.String())
		}
	}
	out.Reset()
	errb.Reset()
	if code := Run([]string{"nosuch"}, &out, &errb); code != 2 {
		t.Fatalf("unknown command: exit %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "Usage:") {
		t.Fatalf("unknown command prints no usage to stderr: %q", errb.String())
	}
	if code := Run([]string{"generate", "--no-such-flag"}, &out, &errb); code != 2 {
		t.Fatalf("bad flag: exit %d, want 2", code)
	}
	if code := Run([]string{"viz", "--snapshot", "/nonexistent/x.json"}, &out, &errb); code != 2 {
		t.Fatalf("missing snapshot: exit %d, want 2", code)
	}
	if code := Run(nil, &out, &errb); code != 2 {
		t.Fatalf("no args: exit %d, want 2", code)
	}
}
