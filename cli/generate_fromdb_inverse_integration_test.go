// SPDX-License-Identifier: Apache-2.0

//go:build integration

package cli

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	testdb "github.com/azrtydxb/momo/internal/testdb"
)

// inverseDDL has a plain foreign key (books), a unique one (author_profiles)
// and two to the same table (messages).
var inverseDDL = []string{
	`CREATE TABLE authors (id bigserial PRIMARY KEY, name text NOT NULL)`,
	`CREATE TABLE books (
		id bigserial PRIMARY KEY,
		author_id bigint NOT NULL REFERENCES authors (id),
		title text NOT NULL)`,
	`CREATE TABLE author_profiles (
		id bigserial PRIMARY KEY,
		author_id bigint NOT NULL UNIQUE REFERENCES authors (id),
		bio text NOT NULL)`,
	`CREATE TABLE messages (
		id bigserial PRIMARY KEY,
		sender_id bigint NOT NULL REFERENCES authors (id),
		recipient_id bigint NOT NULL REFERENCES authors (id),
		body text NOT NULL)`,
	`INSERT INTO authors (name) VALUES ('ann'), ('bob')`,
	`INSERT INTO books (author_id, title) VALUES (1, 'one'), (1, 'two'), (2, 'three')`,
	`INSERT INTO author_profiles (author_id, bio) VALUES (1, 'ann bio')`,
	`INSERT INTO messages (sender_id, recipient_id, body) VALUES (1, 2, 'hi bob'), (2, 1, 'hi ann'), (1, 2, 'again')`,
}

const inverseMain = `package main

import (
	"context"
	"fmt"
	"os"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"

	"example.test/example/models"
	"example.test/example/models/author"
)

func main() {
	ctx := context.Background()
	pool, err := pgxdriver.NewPool(ctx, os.Getenv("MOMO_TEST_URL"))
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	db := momo.New(pgxdriver.New(pool))
	rows, err := momo.Select[models.Author](db).
		Preload(author.Books, author.AuthorProfile, author.MessagesBySender, author.MessagesByRecipient).
		OrderBy(author.ID.Asc()).All(ctx)
	if err != nil {
		panic(err)
	}
	for _, a := range rows {
		profile := "none"
		if a.AuthorProfile != nil {
			profile = a.AuthorProfile.Bio
		}
		fmt.Printf("%s books=%d profile=%s sent=%d received=%d\n", a.Name, len(a.Books), profile, len(a.MessagesBySender), len(a.MessagesByRecipient))
	}
}
`

// TestGenerateFromDBInverse is issue 91 against a real PostgreSQL: with
// introspect.inverse the models carry has_many, has_one and the two-foreign-key
// names, and a program built on them preloads each inverse.
func TestGenerateFromDBInverse(t *testing.T) {
	dsn := testdb.PostgresURL(t)
	execAll(t, dsn, inverseDDL)
	root := copyModule(t, t.TempDir())
	writeFile(t, filepath.Join(root, "momo.yaml"), `version: 1
packages:
  - ./models
dialects: [postgres]
migrate:
  exclude: [vector, pgcrypto]
introspect:
  inverse: true
`)
	writeFile(t, filepath.Join(root, "main.go"), inverseMain)
	chdir(t, root)
	t.Setenv("MOMO_TEST_URL", dsn)
	if code, out, errOut := fromDB(t, "--no-snapshot"); code != 0 {
		t.Fatalf("generate --from-db: exit %d\n%s%s", code, out, errOut)
	}
	var o, e bytes.Buffer
	if c := Run([]string{"generate"}, &o, &e); c != 0 {
		t.Fatalf("generate: exit %d\n%s%s", c, o.String(), e.String())
	}
	cmd := exec.Command("go", "run", "-mod=mod", ".") //nolint:gosec // fixed arguments
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run: %v\n%s", err, out)
	}
	want := "ann books=2 profile=ann bio sent=2 received=1\nbob books=1 profile=none sent=1 received=2\n"
	if string(out) != want {
		t.Fatalf("output:\n%s\nwant:\n%s", out, want)
	}
	if code, out2, errOut := fromDB(t, "--no-snapshot", "--check"); code != 0 || strings.Contains(out2, "stale") {
		t.Fatalf("a re-run must be quiet: exit %d\n%s%s", code, out2, errOut)
	}
}
