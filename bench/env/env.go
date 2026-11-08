// SPDX-License-Identifier: Apache-2.0

// Package env captures the machine, toolchain and pinned module versions a
// benchmark run happened on, so published results can be reproduced.
package env

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Environment is the recorded context of one benchmark run.
type Environment struct {
	CapturedAtUTC   string            `json:"captured_at_utc"`
	GoVersion       string            `json:"go_version"`
	GOOS            string            `json:"goos"`
	GOARCH          string            `json:"goarch"`
	NumCPU          int               `json:"num_cpu"`
	GOMAXPROCS      int               `json:"gomaxprocs"`
	CPUModel        string            `json:"cpu_model"`
	Kernel          string            `json:"kernel"`
	Hostname        string            `json:"hostname,omitempty"`
	VCSRevision     string            `json:"vcs_revision"`
	VCSModified     bool              `json:"vcs_modified"`
	ModuleVersions  map[string]string `json:"module_versions"`
	PostgresVersion string            `json:"postgres_version"`
	PgvectorVersion string            `json:"pgvector_version"`
}

// Capture reads the environment. The PostgreSQL fields stay empty; fill them
// with CapturePostgres.
func Capture() (Environment, error) {
	// The hostname identifies the machine that ran the benchmark, so it is
	// recorded only when BENCH_RECORD_HOSTNAME=1; published results leave it out.
	host := ""
	if os.Getenv("BENCH_RECORD_HOSTNAME") == "1" {
		h, err := os.Hostname()
		if err != nil {
			return Environment{}, err
		}
		host = h
	}
	e := Environment{
		CapturedAtUTC:  time.Now().UTC().Format(time.RFC3339),
		GoVersion:      runtime.Version(),
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		NumCPU:         runtime.NumCPU(),
		GOMAXPROCS:     runtime.GOMAXPROCS(0),
		CPUModel:       cpuModel(),
		Kernel:         kernel(),
		Hostname:       host,
		ModuleVersions: map[string]string{},
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if bi.Main.Path != "" {
			e.ModuleVersions[bi.Main.Path] = bi.Main.Version
		}
		for _, d := range bi.Deps {
			v := d.Version
			if d.Replace != nil {
				v = d.Replace.Version
				if v == "" {
					v = "(devel) => " + d.Replace.Path
				}
			}
			e.ModuleVersions[d.Path] = v
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				e.VCSRevision = s.Value
			case "vcs.modified":
				e.VCSModified = s.Value == "true"
			}
		}
	}
	if len(e.ModuleVersions) <= 1 {
		// Test binaries carry no dependency list in their build info; ask the
		// go command, which reads the pinned go.mod of the bench module.
		for _, line := range strings.Split(output(exec.Command("go", "list", "-m", "all")), "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 {
				e.ModuleVersions[f[0]] = f[1]
			}
		}
	}
	return e, nil
}

// CapturePostgres fills the server and pgvector versions from dsn.
func CapturePostgres(ctx context.Context, dsn string, e *Environment) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	if err := conn.QueryRow(ctx, "SHOW server_version").Scan(&e.PostgresVersion); err != nil {
		return err
	}
	err = conn.QueryRow(ctx, "SELECT extversion FROM pg_extension WHERE extname = 'vector'").Scan(&e.PgvectorVersion)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return nil
}

func cpuModel() string {
	switch runtime.GOOS {
	case "linux":
		f, err := os.Open("/proc/cpuinfo")
		if err != nil {
			return ""
		}
		defer func() { _ = f.Close() }()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "model name") {
				if _, v, ok := strings.Cut(line, ":"); ok {
					return strings.TrimSpace(v)
				}
			}
		}
	case "darwin":
		return output(exec.Command("sysctl", "-n", "machdep.cpu.brand_string"))
	case "windows":
		out := output(exec.Command("wmic", "cpu", "get", "name"))
		lines := strings.Split(out, "\n")
		if len(lines) > 1 {
			return strings.TrimSpace(lines[1])
		}
	}
	return ""
}

func kernel() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	return output(exec.Command("uname", "-sr"))
}

func output(cmd *exec.Cmd) string {
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
