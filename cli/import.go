// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/importer"
	importbun "github.com/azrtydxb/momo/importer/bun"
	importent "github.com/azrtydxb/momo/importer/ent"
	importgorm "github.com/azrtydxb/momo/importer/gorm"
)

const importUsage = `Usage: momo import <gorm|bun|ent> --src <dir> --out <dir> [--package <name>] [--report <file>]

Reads the models of a GORM, Bun or Ent project statically (the source is
parsed, never compiled or run) and writes the equivalent Momo structs to
<out>/models.go. The import report is printed: every behaviour the source ORM
had that Momo does not reproduce is listed with its file position.

--src      directory with the GORM or Bun structs, or the Ent schema package
--out      directory the Momo models.go is written to
--package  Go package name of the output (default: derived from --out)
--report   also write the report to this file

Nothing is written when the report holds an "unsupported" change.

Exit codes: 0 models written, 1 the report holds an unsupported change
(nothing written), 2 usage or input error.
`

// runImport implements `momo import gorm|bun|ent`: it dispatches to the
// reader, prints the report to out and writes the models with importer.Emit.
// It returns 0 when the models are written, 1 when the report has an
// unsupported change (nothing is written) and 2 for a usage or input error.
func runImport(out, errOut io.Writer, args []string) int {
	// -h and --help mean help only in a flag position, not as the value of a
	// flag such as --src.
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if a == "-h" || a == "-help" || a == "--help" {
			_, _ = fmt.Fprint(out, importUsage)
			return exitOK
		}
		switch strings.TrimLeft(a, "-") {
		case "src", "out", "package", "report":
			if strings.HasPrefix(a, "-") && !strings.Contains(a, "=") {
				i++ // the next argument is this flag's value
			}
		}
	}
	if len(args) == 0 || len(args[0]) == 0 || args[0][0] == '-' {
		_, _ = fmt.Fprint(errOut, "momo import: name the source ORM first: gorm, bun or ent\n\n"+importUsage)
		return exitUsage
	}
	tool := args[0]
	read := map[string]func(string) ([]importer.Model, *importer.Report, error){
		"gorm": importgorm.Read, "bun": importbun.Read, "ent": importent.Read,
	}[tool]
	if read == nil {
		_, _ = fmt.Fprintf(errOut, "momo import: unknown source %q (want gorm, bun or ent)\n\n%s", tool, importUsage)
		return exitUsage
	}
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	var src, outDir, pkg, report string
	fs.StringVar(&src, "src", "", "source directory")
	fs.StringVar(&outDir, "out", "", "output directory")
	fs.StringVar(&pkg, "package", "", "output package name")
	fs.StringVar(&report, "report", "", "report file")
	if err := fs.Parse(args[1:]); err != nil {
		code := fail(errOut, err)
		_, _ = fmt.Fprintf(errOut, "\n%s", importUsage)
		return code
	}
	if fs.NArg() > 0 {
		return fail(errOut, fmt.Errorf("import: unexpected argument %q", fs.Arg(0)))
	}
	if src == "" || outDir == "" {
		return fail(errOut, errors.New("import: --src and --out are required"))
	}
	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return fail(errOut, err)
	}
	if pkg == "" {
		pkg = packageNameFor(absOut)
	}
	models, rep, err := read(src)
	if err != nil {
		return fail(errOut, err)
	}
	if rep == nil {
		rep = &importer.Report{}
	}
	_, _ = fmt.Fprint(out, rep.String())
	if report != "" {
		if err := os.WriteFile(report, []byte(rep.String()), 0o644); err != nil { // #nosec G306 -- a report is meant to be committed and read, like generated source
			return fail(errOut, err)
		}
	}
	w := gen.NewWriter(gen.WriterOptions{ModuleRoot: absOut})
	if err := importer.Emit(models, rep, w, absOut, pkg); err != nil {
		if errors.Is(err, importer.ErrUnsupported) {
			_, _ = fmt.Fprintf(errOut, "momo: %s\nmomo: nothing was written\n", err)
			return exitFindings
		}
		return fail(errOut, err)
	}
	if _, err := w.Commit(); err != nil {
		return fail(errOut, err)
	}
	return exitOK
}

// runImportCmd adapts runImport to the command table.
func runImportCmd(args []string, stdout, stderr io.Writer) int {
	return runImport(stdout, stderr, args)
}
