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
