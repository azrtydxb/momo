// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/migrate"
)

// TestRedactURL tests migrate.RedactURLs directly with fake errors that carry raw
// passwords, as an unmasked driver or a wrapped error would. The driver masks
// passwords itself, so end-to-end tests cannot tell migrate.RedactURLs from a no-op.
func TestRedactURL(t *testing.T) {
	cases := []struct {
		name    string
		msg     string
		secrets []string
		leaks   []string // must not survive
		keeps   string   // must survive (context stays readable)
	}{
		{
			name:  "url form",
			msg:   "connect: dial postgres://alice:topsecret@db.internal:5432/app: refused",
			leaks: []string{"topsecret", "alice"},
			keeps: "db.internal",
		},
		{
			name:  "percent-encoded password",
			msg:   "ping postgres://alice:p%40ss%2Fw0rd%23x@db/app failed",
			leaks: []string{"p%40ss%2Fw0rd%23x", "p%40ss"},
			keeps: "failed",
		},
		{
			name:    "decoded password in the message",
			msg:     "FATAL: password authentication failed (tried p@ss/w0rd#x)",
			secrets: []string{"p@ss/w0rd#x"},
			leaks:   []string{"p@ss/w0rd#x", "w0rd"},
			keeps:   "authentication failed",
		},
		{
			// url.Parse fails on this URL, so only the whole-URL secret strips it.
			name:    "raw at, slash and hash in the password",
			msg:     "parse postgres://alice:p@ss/w0rd#x@db/app: invalid",
			secrets: []string{"postgres://alice:p@ss/w0rd#x@db/app"},
			leaks:   []string{"p@ss", "w0rd", "ss/w0rd#x"},
			keeps:   "invalid",
		},
		{
			name:  "password in the query",
			msg:   "dial postgres://db/app?sslmode=disable&password=querysecret&connect_timeout=1 failed",
			leaks: []string{"querysecret"},
			keeps: "failed",
		},
		{
			// Not a password= key: only the URL-with-query rule strips the whole query.
			name:  "other secret in the query",
			msg:   "dial postgres://db/app?sslpassword=keysecret&application_name=x failed",
			leaks: []string{"keysecret"},
			keeps: "failed",
		},
		{
			name:  "key value dsn",
			msg:   "cannot connect: user=alice password=kvsecret host=db dbname=app",
			leaks: []string{"kvsecret"},
			keeps: "host=db",
		},
		{
			name:  "quoted key value dsn",
			msg:   "cannot connect: password='kv sec ret' host=db",
			leaks: []string{"kv sec ret", "sec ret"},
			keeps: "host=db",
		},
		{
			name:  "url inside a longer message",
			msg:   "migrate: apply 20260101000000_init: pgx: failed to connect to `postgres://bob:longsecret@10.0.0.5:6432/app?sslmode=require`: server error",
			leaks: []string{"longsecret", "bob"},
			keeps: "20260101000000_init",
		},
		{
			name:  "other scheme",
			msg:   "dial mysql://root:rootpw@db/app",
			leaks: []string{"rootpw"},
			keeps: "db/app",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wrapped := fmt.Errorf("outer: %w", errors.New(c.msg))
			got := migrate.RedactURLs(wrapped.Error(), c.secrets...)
			for _, leak := range c.leaks {
				if strings.Contains(got, leak) {
					t.Errorf("RedactURLs leaked %q: %s", leak, got)
				}
			}
			if !strings.Contains(got, c.keeps) {
				t.Errorf("RedactURLs lost context %q: %s", c.keeps, got)
			}
		})
	}
}

// TestRedactURLEmptySecretIsIgnored guards the secrets loop: an empty secret
// would otherwise replace the gaps between every rune.
func TestRedactURLEmptySecretIsIgnored(t *testing.T) {
	if got := migrate.RedactURLs("plain message", ""); got != "plain message" {
		t.Fatalf("got %q", got)
	}
}

// TestMigrateUnknownCommandIsRedacted checks that a pasted URL in the command
// position never reaches the output.
func TestMigrateUnknownCommandIsRedacted(t *testing.T) {
	dir := t.TempDir()
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(dir)+"/m\n")
	var out, errOut bytes.Buffer
	code := runMigrate(&out, &errOut, []string{"--config", cfg, "postgres://u:topsecret@h/db"}, noEnv)
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if all := out.String() + errOut.String(); strings.Contains(all, "topsecret") {
		t.Fatalf("unknown command echoed the password: %s", all)
	}
	if !strings.Contains(errOut.String(), "unknown migrate command") {
		t.Fatalf("missing the diagnosis: %s", errOut.String())
	}
}
