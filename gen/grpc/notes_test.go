// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"strings"
	"testing"
)

// Catches: the message of a model saying nothing about the fields a request
// ignores, so a client fills in the key or a read-only column and wonders why
// it has no effect; and a note leaking into the key messages of Get and Delete,
// where the key is the point of the request.
func TestFieldNotesStateWhatARequestIgnores(t *testing.T) {
	src, ds := Proto(exampleGraph(t), &Lock{}, "momo.example.v1")
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	s := string(src)
	const keyNote = "  // The key of the row. Create ignores it (the database assigns it); Update takes it to find the row.\n"
	const roNote = "  // Read-only: Create and Update ignore it.\n"
	if !strings.Contains(s, keyNote+"  int64 id = 1;") {
		t.Errorf("the key field has no note:\n%s", s)
	}
	if !strings.Contains(s, roNote+"  google.protobuf.Timestamp created_at = ") {
		t.Errorf("the read-only field created_at has no note:\n%s", s)
	}
	// One note per model message (Comment, Document, Tag, Tenant), none in a
	// GetRequest or DeleteRequest.
	if n := strings.Count(s, keyNote); n != 4 {
		t.Errorf("the key note appears %d times, want 4 (once per model message)", n)
	}
	for _, msg := range []string{"message DocumentGetRequest {", "message DocumentDeleteRequest {"} {
		i := strings.Index(s, msg)
		if i < 0 {
			t.Fatalf("no %s", msg)
		}
		body := s[i : i+strings.Index(s[i:], "}")]
		if strings.Contains(body, "//") {
			t.Errorf("%s carries a note:\n%s", msg, body)
		}
	}
}
