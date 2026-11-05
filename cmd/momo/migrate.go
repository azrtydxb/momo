// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	iofs "io/fs"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/config"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// Exit codes beyond the shared ones in cmd.go.
const exitDatabase = 3 // database, lock or execution error

const migrateUsage = `Usage: momo migrate [--config momo.yaml] [--dir path] [--dialect name] <command> [flags]

Commands:
  diff <name> [--rename-table a=b]... [--rename-column t.a=b]... [--allow-destructive] [--stage]
              write a migration for the difference between the models and the
              latest snapshot (offline; refuses to overwrite files)
  plan        list the pending migrations (needs a database)
  apply [--to N] [--force-unlock] [--lock-wait 30s]   apply pending migrations (needs a database)
  status      show the state of every migration (needs a database)
  validate [--db]   check the migration directory (offline; --db also checks the database)
  baseline --version N [--force]   record migrations as applied without running them (needs a database)
  drift       compare the live database with the latest applied snapshot (needs a database)
  resolve --version N --applied|--rolled-back   settle a dirty migration (needs a database)

Not available yet: rebase, lint, rollback, new.

The database URL comes from --url, else the environment variable named by
migrate.url_env.<dialect> in momo.yaml, else MOMO_DATABASE_URL. Offline
commands never read it. The URL and its password are never printed.

Exit codes:
  0  success
  1  findings: drift, validate problems, status dirty or mismatch, destructive or staged refusal
  2  usage or configuration error
  3  database, lock or execution error
`

// notAvailable lists the documented subcommands whose engine support has not
// landed; they report a clear error instead of pretending to work.
var notAvailable = map[string]string{
	"rebase":   "migrate rebase needs Engine.Rebase",
	"lint":     "migrate lint needs Engine.Lint",
	"rollback": "migrate rollback needs down migrations",
	"new":      "migrate new --custom needs engine support for custom migrations",
}

// runMigrateCmd adapts runMigrate to the command table.
func runMigrateCmd(args []string, stdout, stderr io.Writer) int {
	return runMigrate(stdout, stderr, args, os.Getenv)
}

// common holds the flags every migrate subcommand accepts.
type common struct{ config, dir, dialect, url string }

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.config, "config", c.config, "path to momo.yaml")
	fs.StringVar(&c.config, "c", c.config, "path to momo.yaml")
	fs.StringVar(&c.dir, "dir", c.dir, "migration directory (overrides migrate.dir)")
	fs.StringVar(&c.dialect, "dialect", c.dialect, "dialect (default: the single configured one)")
	fs.StringVar(&c.url, "url", c.url, "database URL (default: from the environment)")
}

// cli is one invocation: its writers, its environment and the secrets to
// strip from everything it prints.
type cli struct {
	out, errOut io.Writer
	env         func(string) string
	secrets     []string
	c           common
	file        *config.File
	cfgDir, cwd string
	dialectName string
}

var (
	urlUserinfo = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/?#\s]*@`)
	urlInText   = regexp.MustCompile(`(?i)\b(?:postgres(?:ql)?|mysql|mariadb)://[^\s"']+`)
)

// redactURL removes passwords and DSNs from msg: every secret given, the
// userinfo of any URL, and any URL-looking text that carries a query.
func redactURL(msg string, secrets ...string) string {
	for _, s := range secrets {
		if s != "" {
			msg = strings.ReplaceAll(msg, s, "***")
		}
	}
	msg = urlUserinfo.ReplaceAllString(msg, "${1}***@")
	return urlInText.ReplaceAllStringFunc(msg, func(m string) string {
		if i := strings.IndexAny(m, "?"); i >= 0 {
			return m[:i] + "?***"
		}
		return m
	})
}

func (l *cli) say(format string, args ...any) {
	_, _ = fmt.Fprintf(l.out, "%s\n", redactURL(fmt.Sprintf(format, args...), l.secrets...))
}

func (l *cli) errf(code int, format string, args ...any) int {
	_, _ = fmt.Fprintf(l.errOut, "momo: %s\n", redactURL(fmt.Sprintf(format, args...), l.secrets...))
	return code
}

// rel renders p relative to the working directory, with forward slashes.
func (l *cli) rel(p string) string { return filepath.ToSlash(relPath(p, l.cwd)) }

// parseInterspersed parses flags that may follow positional arguments.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// pairs is a repeatable key=value flag.
type pairs map[string]string

func (p pairs) String() string { return fmt.Sprint(map[string]string(p)) }
func (p pairs) Set(v string) error {
	k, val, ok := strings.Cut(v, "=")
	if !ok || k == "" || val == "" {
		return fmt.Errorf("want old=new, got %q", v)
	}
	p[k] = val
	return nil
}

// runMigrate implements `momo migrate`. env looks up environment variables;
// only commands that need a database call it.
func runMigrate(out, errOut io.Writer, args []string, env func(string) string) int {
	l := &cli{out: out, errOut: errOut, env: env}
	gfs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	gfs.SetOutput(io.Discard)
	l.c.register(gfs)
	if err := gfs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = fmt.Fprint(out, migrateUsage)
			return exitOK
		}
		_, _ = fmt.Fprintf(errOut, "momo: %v\n\n%s", err, migrateUsage)
		return exitUsage
	}
	rest := gfs.Args()
	if len(rest) == 0 {
		_, _ = fmt.Fprint(errOut, migrateUsage)
		return exitUsage
	}
	name, sub := rest[0], rest[1:]
	if name == "help" {
		_, _ = fmt.Fprint(out, migrateUsage)
		return exitOK
	}
	if why, ok := notAvailable[name]; ok {
		return l.errf(exitUsage, "migrate %s is not available yet (%s)", name, why)
	}
	run := map[string]func(*flag.FlagSet) func([]string) int{
		"diff":     l.diffCmd,
		"plan":     l.planCmd,
		"apply":    l.applyCmd,
		"status":   l.statusCmd,
		"validate": l.validateCmd,
		"baseline": l.baselineCmd,
		"drift":    l.driftCmd,
		"resolve":  l.resolveCmd,
	}[name]
	if run == nil {
		_, _ = fmt.Fprintf(errOut, "momo: unknown migrate command %q\n\n%s", name, migrateUsage)
		return exitUsage
	}
	fs := flag.NewFlagSet("migrate "+name, flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() { _, _ = fmt.Fprint(errOut, migrateUsage) }
	l.c.register(fs)
	body := run(fs)
	// body parses its own flags after registering them; see parse below.
	return body(sub)
}

// setup parses the subcommand flags and loads the configuration. It returns a
// non-nil code pointer when the command must stop.
func (l *cli) setup(fs *flag.FlagSet, args []string) ([]string, *int) {
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		code := exitUsage
		if errors.Is(err, flag.ErrHelp) {
			code = exitOK
		}
		return nil, &code
	}
	if l.cwd, err = os.Getwd(); err != nil {
		code := l.errf(exitUsage, "%v", err)
		return nil, &code
	}
	file, dir, err := loadConfigFile(l.c.config, l.cwd)
	if err != nil {
		code := l.errf(exitUsage, "%v", err)
		return nil, &code
	}
	l.file, l.cfgDir = file, dir
	if l.dialectName, err = l.chooseDialect(); err != nil {
		code := l.errf(exitUsage, "%v", err)
		return nil, &code
	}
	return pos, nil
}

// chooseDialect picks the dialect: --dialect, else the only one configured,
// else postgres when nothing is configured.
func (l *cli) chooseDialect() (string, error) {
	name := l.c.dialect
	if name == "" {
		set := map[string]bool{}
		for _, d := range l.file.Dialects {
			set[d] = true
		}
		if len(set) == 0 {
			for d := range l.file.Migrate.Dir {
				set[d] = true
			}
			for d := range l.file.Migrate.URLEnv {
				set[d] = true
			}
		}
		names := make([]string, 0, len(set))
		for d := range set {
			names = append(names, d)
		}
		sort.Strings(names)
		switch len(names) {
		case 0:
			name = string(dialect.Postgres)
		case 1:
			name = names[0]
		default:
			return "", fmt.Errorf("several dialects are configured (%s): pass --dialect", strings.Join(names, ", "))
		}
	}
	if dialect.For(dialect.Name(name)) == nil {
		return "", fmt.Errorf("unknown dialect %q", name)
	}
	return name, nil
}

// migrationDir is the directory of the chosen dialect's migrations.
func (l *cli) migrationDir() string {
	d := l.c.dir
	if d == "" {
		d = l.file.Migrate.Dir[l.dialectName]
	}
	if d == "" {
		d = filepath.Join("migrations", l.dialectName)
	}
	if !filepath.IsAbs(d) {
		base := l.cfgDir
		if l.c.dir != "" {
			base = l.cwd
		}
		d = filepath.Join(base, d)
	}
	return d
}

// dirFS opens the migration directory; one that does not exist yet reads as empty.
func dirFS(dir string) iofs.FS {
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return fstest.MapFS{}
	}
	return os.DirFS(dir)
}

// dialectImpl returns the migration dialect, or an error when none exists yet.
func (l *cli) dialectImpl() (migrate.Dialect, error) {
	if l.dialectName == string(dialect.Postgres) {
		return mpg.New(), nil
	}
	return nil, fmt.Errorf("migrations for dialect %q are not available yet", l.dialectName)
}

// engineOptions maps the migrate: section of momo.yaml to engine options.
func (l *cli) engineOptions(d migrate.Dialect, extra ...migrate.Option) ([]migrate.Option, error) {
	m := l.file.Migrate
	cfg := migrate.Config{Schemas: m.Schemas, Exclude: m.Exclude}
	var err error
	if m.LockTimeout != "" {
		if cfg.LockTimeout, err = time.ParseDuration(m.LockTimeout); err != nil {
			return nil, err
		}
	}
	if m.StatementTimeout != "" {
		if cfg.StatementTimeout, err = time.ParseDuration(m.StatementTimeout); err != nil {
			return nil, err
		}
	}
	return append([]migrate.Option{migrate.WithDialect(d), migrate.WithConfig(cfg)}, extra...), nil
}

// databaseURL resolves the URL of a command that needs a database.
func (l *cli) databaseURL() (string, error) {
	u := l.c.url
	if u == "" {
		if name := l.file.Migrate.URLEnv[l.dialectName]; name != "" {
			u = l.env(name)
		}
	}
	if u == "" {
		u = l.env("MOMO_DATABASE_URL")
	}
	if u == "" {
		return "", errors.New("no database URL: pass --url, set migrate.url_env in momo.yaml, or set MOMO_DATABASE_URL")
	}
	l.secrets = append(l.secrets, u)
	if parsed, err := url.Parse(u); err == nil && parsed.User != nil {
		if pw, ok := parsed.User.Password(); ok {
			l.secrets = append(l.secrets, pw)
		}
	}
	return u, nil
}

// openEngine builds an engine on a live database. The returned close func
// releases the pool.
func (l *cli) openEngine(ctx context.Context, extra ...migrate.Option) (*migrate.Engine, func(), int) {
	dsn, err := l.databaseURL()
	if err != nil {
		return nil, nil, l.errf(exitUsage, "%v", err)
	}
	d, err := l.dialectImpl()
	if err != nil {
		return nil, nil, l.errf(exitUsage, "%v", err)
	}
	opts, err := l.engineOptions(d, extra...)
	if err != nil {
		return nil, nil, l.errf(exitUsage, "%v", err)
	}
	if l.dialectName != string(dialect.Postgres) {
		return nil, nil, l.errf(exitUsage, "no driver for dialect %q", l.dialectName)
	}
	pool, err := pgxdriver.NewPool(ctx, dsn)
	if err != nil {
		return nil, nil, l.errf(exitDatabase, "connect: %v", err)
	}
	var drv momo.Driver = pgxdriver.New(pool)
	e, err := migrate.NewEngine(drv, dirFS(l.migrationDir()), opts...)
	if err != nil {
		pool.Close()
		return nil, nil, l.errf(exitUsage, "%v", err)
	}
	return e, pool.Close, -1
}

// failure maps an engine error to the documented exit code.
func (l *cli) failure(err error) int {
	switch {
	case errors.Is(err, migrate.ErrDestructive), errors.Is(err, migrate.ErrStagedRequired),
		errors.Is(err, migrate.ErrBaselineMismatch), errors.Is(err, migrate.ErrBranchConflict):
		return l.errf(exitFindings, "%v", err)
	case errors.Is(err, migrate.ErrNoSnapshot):
		return l.errf(exitUsage, "%v", err)
	default:
		return l.errf(exitDatabase, "%v", err)
	}
}

func interruptContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

// withEngine runs fn with a database engine and maps its error to an exit code.
func (l *cli) withEngine(fs *flag.FlagSet, extra func() ([]migrate.Option, error), fn func(context.Context, *migrate.Engine, []string) int) func([]string) int {
	return func(args []string) int {
		pos, stop := l.setup(fs, args)
		if stop != nil {
			return *stop
		}
		var opts []migrate.Option
		if extra != nil {
			var err error
			if opts, err = extra(); err != nil {
				return l.errf(exitUsage, "%v", err)
			}
		}
		ctx, cancel := interruptContext()
		defer cancel()
		e, closeFn, code := l.openEngine(ctx, opts...)
		if e == nil {
			return code
		}
		defer closeFn()
		return fn(ctx, e, pos)
	}
}

func (l *cli) planCmd(fs *flag.FlagSet) func([]string) int {
	return l.withEngine(fs, nil, func(ctx context.Context, e *migrate.Engine, _ []string) int {
		pending, err := e.Plan(ctx)
		if err != nil {
			return l.failure(err)
		}
		if len(pending) == 0 {
			l.say("no pending migrations")
		}
		for _, p := range pending {
			l.say("pending %s %s (%d statements)", p.Version, p.Name, len(p.Statements))
		}
		return exitOK
	})
}

func (l *cli) applyCmd(fs *flag.FlagSet) func([]string) int {
	var to string
	var force bool
	var wait string
	fs.StringVar(&to, "to", "", "apply up to and including this version")
	fs.BoolVar(&force, "force-unlock", false, "take over a lock held by a dead process")
	fs.StringVar(&wait, "lock-wait", "", "how long to wait for the migration lock (default 30s)")
	var waitD time.Duration
	extra := func() ([]migrate.Option, error) {
		if wait == "" {
			return nil, nil
		}
		d, err := time.ParseDuration(wait)
		if err != nil || d < 0 {
			return nil, fmt.Errorf("--lock-wait: %q is not a duration", wait)
		}
		waitD = d
		return []migrate.Option{migrate.WithLockWait(d)}, nil
	}
	return l.withEngine(fs, extra, func(ctx context.Context, e *migrate.Engine, _ []string) int {
		rep, err := e.Apply(ctx, migrate.ApplyOptions{To: to, ForceUnlock: force, LockWait: waitD})
		if err != nil {
			return l.failure(err)
		}
		if rep == nil || len(rep.Applied) == 0 {
			l.say("no pending migrations")
			return exitOK
		}
		for _, a := range rep.Applied {
			l.say("applied %s (%d statements, %s)", a.Version, a.Statements, a.Duration.Round(time.Millisecond))
		}
		return exitOK
	})
}

// printStatus prints every version and reports whether any is not healthy.
func (l *cli) printStatus(rows []migrate.VersionStatus) (bad bool) {
	for _, r := range rows {
		line := fmt.Sprintf("%s %s %s", r.State, r.Version, r.Name)
		if r.Error != "" {
			line += ": " + r.Error
		}
		l.say("%s", line)
		if r.State != "applied" && r.State != "pending" {
			bad = true
		}
	}
	return bad
}

func (l *cli) statusCmd(fs *flag.FlagSet) func([]string) int {
	return l.withEngine(fs, nil, func(ctx context.Context, e *migrate.Engine, _ []string) int {
		rows, err := e.Status(ctx)
		if err != nil {
			return l.failure(err)
		}
		if l.printStatus(rows) {
			return exitFindings
		}
		return exitOK
	})
}

func (l *cli) driftCmd(fs *flag.FlagSet) func([]string) int {
	return l.withEngine(fs, nil, func(ctx context.Context, e *migrate.Engine, _ []string) int {
		rep, err := e.Drift(ctx)
		if err != nil {
			return l.failure(err)
		}
		for _, d := range rep.Changed {
			l.say("changed: %s %s: %s", d.Object.Kind, qualifiedName(d.Object), d.Detail)
		}
		for _, o := range rep.Missing {
			l.say("missing: %s %s", o.Kind, qualifiedName(o))
		}
		for _, o := range rep.Unmanaged {
			l.say("unmanaged: %s %s", o.Kind, qualifiedName(o))
		}
		for _, o := range rep.Unverified {
			l.say("unverified: %s %s", o.Kind, qualifiedName(o))
		}
		if !rep.Clean() {
			return exitFindings
		}
		l.say("no drift")
		return exitOK
	})
}

func qualifiedName(o migrate.ObjectRef) string {
	if o.Schema == "" {
		return o.Name
	}
	return o.Schema + "." + o.Name
}

func (l *cli) baselineCmd(fs *flag.FlagSet) func([]string) int {
	var version string
	var force bool
	fs.StringVar(&version, "version", "", "last migration version to record as applied")
	fs.BoolVar(&force, "force", false, "record even when the database differs from the snapshot")
	inner := l.withEngine(fs, nil, func(ctx context.Context, e *migrate.Engine, _ []string) int {
		if err := e.Baseline(ctx, version, force); err != nil {
			return l.failure(err)
		}
		l.say("baselined through %s", version)
		return exitOK
	})
	return func(args []string) int {
		// The version is checked before anything touches the database.
		if _, err := parseInterspersed(fs, args); err != nil {
			return exitUsage
		}
		if version == "" {
			return l.errf(exitUsage, "baseline needs --version")
		}
		return inner(args)
	}
}

func (l *cli) resolveCmd(fs *flag.FlagSet) func([]string) int {
	var version string
	var applied, rolledBack bool
	fs.StringVar(&version, "version", "", "the dirty migration version")
	fs.BoolVar(&applied, "applied", false, "the migration was in fact applied")
	fs.BoolVar(&rolledBack, "rolled-back", false, "the migration was rolled back")
	inner := l.withEngine(fs, nil, func(ctx context.Context, e *migrate.Engine, _ []string) int {
		state := migrate.ResolveApplied
		if rolledBack {
			state = migrate.ResolveRolledBack
		}
		if err := e.Resolve(ctx, version, state); err != nil {
			return l.failure(err)
		}
		l.say("resolved %s as %s", version, state)
		return exitOK
	})
	return func(args []string) int {
		if _, err := parseInterspersed(fs, args); err != nil {
			return exitUsage
		}
		if version == "" || applied == rolledBack {
			return l.errf(exitUsage, "resolve needs --version and exactly one of --applied or --rolled-back")
		}
		return inner(args)
	}
}

func (l *cli) validateCmd(fs *flag.FlagSet) func([]string) int {
	var withDB bool
	fs.BoolVar(&withDB, "db", false, "also check the database history against the files")
	return func(args []string) int {
		if _, stop := l.setup(fs, args); stop != nil {
			return *stop
		}
		rep, err := migrate.ValidateOffline(dirFS(l.migrationDir()))
		if err != nil {
			return l.errf(exitUsage, "%v", err)
		}
		for _, p := range rep.Problems {
			l.say("problem: %s", p)
		}
		code := exitOK
		if len(rep.Problems) > 0 {
			code = exitFindings
		}
		if withDB {
			ctx, cancel := interruptContext()
			defer cancel()
			e, closeFn, c := l.openEngine(ctx)
			if e == nil {
				return c
			}
			defer closeFn()
			rows, err := e.Status(ctx)
			if err != nil {
				return l.failure(err)
			}
			if l.printStatus(rows) {
				code = exitFindings
			}
		}
		if code == exitOK {
			l.say("migration directory is valid")
		}
		return code
	}
}

func (l *cli) diffCmd(fs *flag.FlagSet) func([]string) int {
	renameTables, renameColumns := pairs{}, pairs{}
	var allow, stage bool
	fs.Var(renameTables, "rename-table", "old=new table rename (repeatable)")
	fs.Var(renameColumns, "rename-column", "table.old=new column rename (repeatable)")
	fs.BoolVar(&allow, "allow-destructive", false, "allow drops and rewrites")
	fs.BoolVar(&stage, "stage", false, "stage NOT NULL columns without a default")
	return func(args []string) int {
		pos, stop := l.setup(fs, args)
		if stop != nil {
			return *stop
		}
		if len(pos) != 1 {
			return l.errf(exitUsage, "diff needs exactly one migration name")
		}
		d, err := l.dialectImpl()
		if err != nil {
			return l.errf(exitUsage, "%v", err)
		}
		opts, err := l.engineOptions(d)
		if err != nil {
			return l.errf(exitUsage, "%v", err)
		}
		dir := l.migrationDir()
		// Diff never touches the database: the engine gets no driver.
		e, err := migrate.NewEngine(nil, dirFS(dir), opts...)
		if err != nil {
			return l.errf(exitUsage, "%v", err)
		}
		desired, code := l.desiredSchema()
		if desired == nil {
			return code
		}
		ctx, cancel := interruptContext()
		defer cancel()
		m, err := e.Diff(ctx, desired, pos[0], migrate.DiffOptions{
			RenameTables: renameTables, RenameColumns: renameColumns, AllowDestructive: allow, Stage: stage,
		})
		if err != nil {
			return l.failure(err)
		}
		if m == nil {
			l.say("no changes")
			return exitOK
		}
		written, err := writeNewFiles(dir, m.Files)
		for _, p := range written {
			l.say("%s", l.rel(p))
		}
		if err != nil {
			if errors.Is(err, iofs.ErrExist) {
				return l.errf(exitFindings, "%v", err)
			}
			return l.errf(exitUsage, "%v", err)
		}
		for _, h := range m.Hints {
			l.say("hint: %s", h)
		}
		return exitOK
	}
}

// desiredSchema loads the models and builds the schema they declare.
func (l *cli) desiredSchema() (*schema.Schema, int) {
	patterns, base := []string(nil), l.cwd
	if len(l.file.Packages) > 0 {
		patterns, base = l.file.Packages, l.cfgDir
	}
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	if err := checkPatterns(patterns, base); err != nil {
		return nil, l.errf(exitUsage, "%v", err)
	}
	cfg, err := config.ToConfig(l.file, base)
	if err != nil {
		return nil, l.errf(exitUsage, "%v", err)
	}
	g, diags, err := gen.Load(patterns, cfg)
	if err != nil {
		return nil, l.errf(exitUsage, "%v", err)
	}
	if len(diags) > 0 {
		printDiagnostics(l.errOut, diags, l.cwd)
		return nil, exitFindings
	}
	return g.Schema, exitOK
}

// writeNewFiles writes files (keyed by relative name) into dir atomically:
// each goes to a temporary file in dir and is renamed into place, and no
// existing file is overwritten except momo.sum, which a migration always
// replaces. It checks every name before it writes anything. It returns the
// paths it wrote, in a stable order.
func writeNewFiles(dir string, files map[string][]byte) ([]string, error) {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if filepath.Base(n) != n || n == "." || n == ".." {
			return nil, fmt.Errorf("refusing to write %q outside the migration directory", n)
		}
		if n == migrate.SumFileName {
			continue
		}
		if _, err := os.Lstat(filepath.Join(dir, n)); err == nil {
			return nil, fmt.Errorf("refusing to overwrite %s: %w", filepath.Join(dir, n), iofs.ErrExist)
		}
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	var written []string
	for _, n := range names {
		final := filepath.Join(dir, n)
		if err := writeAtomic(dir, final, files[n], n != migrate.SumFileName); err != nil {
			return written, err
		}
		written = append(written, final)
	}
	return written, nil
}

// writeAtomic writes data to final through a temporary file in dir. With
// exclusive set the file must not exist when it is renamed into place.
func writeAtomic(dir, final string, data []byte, exclusive bool) (err error) {
	tmp, err := os.CreateTemp(dir, ".momo-*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp.Name(), 0o644); err != nil { //nolint:gosec // migration files are meant to be committed and read by the team
		return err
	}
	if exclusive {
		if _, statErr := os.Lstat(final); statErr == nil {
			return fmt.Errorf("refusing to overwrite %s: %w", final, iofs.ErrExist)
		}
	}
	return os.Rename(tmp.Name(), final)
}
