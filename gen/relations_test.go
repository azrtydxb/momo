// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"path/filepath"
	"testing"
)

func TestRelationMetadata(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := Load([]string{"./rel"}, Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	var author, post *Model
	for _, m := range g.Models {
		switch m.GoName {
		case "Author":
			author = m
		case "Post":
			post = m
		}
	}
	if author == nil || post == nil {
		t.Fatal("models missing")
	}
	if len(author.Relations) != 1 || author.Relations[0].Kind != HasMany || author.Relations[0].FK != "AuthorID" {
		t.Fatalf("author relations: %+v", author.Relations)
	}
	if len(post.Relations) != 1 || post.Relations[0].Kind != BelongsTo {
		t.Fatalf("post relations: %+v", post.Relations)
	}
}

// TestRelationTagsReachSchema loads a model whose belongs-to relation carries
// on_delete and on_update tags, and checks they reach the schema's foreign
// key through Load (discovery, relation resolution and schema construction).
func TestRelationTagsReachSchema(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := Load([]string{"./rel"}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diags: %v", diags)
	}
	posts, ok := g.Schema.Table("posts")
	if !ok || len(posts.ForeignKeys) != 1 {
		t.Fatalf("posts fks: %+v", posts)
	}
	fk := posts.ForeignKeys[0]
	if fk.OnDelete != "CASCADE" || fk.OnUpdate != "RESTRICT" {
		t.Fatalf("fk actions lost between tags and schema: %+v", fk)
	}
}
