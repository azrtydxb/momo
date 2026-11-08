// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Catches: a field renumbered on regeneration, a retired number reused by a
// later column, or numbers lost in a save and load round trip.
func TestProtoLockStableNumbers(t *testing.T) {
	l := &Lock{Version: 1, Messages: map[string]*MessageLock{}}
	first, diags := l.Assign("Document", []string{"id", "title", "body"})
	if len(diags) != 0 {
		t.Fatalf("assign: %v", diags)
	}
	if first["id"] != 1 || first["title"] != 2 || first["body"] != 3 {
		t.Fatalf("first=%v", first)
	}
	// rename of the Go field keeps the column, so the number is kept
	again, diags := l.Assign("Document", []string{"id", "title", "body"})
	if len(diags) != 0 {
		t.Fatalf("assign: %v", diags)
	}
	if again["title"] != 2 {
		t.Fatalf("renumbered: %v", again)
	}
	// removing a column reserves its number and it is never reused
	dropped, diags := l.Assign("Document", []string{"id", "title"})
	if len(diags) != 0 {
		t.Fatalf("assign: %v", diags)
	}
	if _, ok := dropped["body"]; ok {
		t.Fatal("removed column still assigned")
	}
	added, diags := l.Assign("Document", []string{"id", "title", "summary"})
	if len(diags) != 0 {
		t.Fatalf("assign: %v", diags)
	}
	if added["summary"] != 4 {
		t.Fatalf("new column reused a reserved number: %v", added)
	}
	path := filepath.Join(t.TempDir(), "momo.proto.lock")
	data, err := l.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Messages["Document"].Fields["title"] != 2 {
		t.Fatalf("lock round trip lost numbers: %+v", loaded.Messages["Document"])
	}
	if r := loaded.Messages["Document"].Reserved; len(r) != 1 || r[0] != 3 {
		t.Fatalf("reserved = %v, want [3]", r)
	}
}

// Catches: a missing lock treated as an error, and a damaged, newer or
// self-contradicting lock being trusted instead of refused.
func TestLoadLockFailsClosed(t *testing.T) {
	dir := t.TempDir()
	l, err := LoadLock(filepath.Join(dir, "absent.lock"))
	if err != nil || l.Version != 1 || len(l.Messages) != 0 {
		t.Fatalf("missing lock = %+v, %v", l, err)
	}
	for name, body := range map[string]string{
		"not json":       `{`,
		"newer version":  `{"version":2,"messages":{}}`,
		"no version":     `{"messages":{}}`,
		"null message":   `{"version":1,"messages":{"M":null}}`,
		"zero number":    `{"version":1,"messages":{"M":{"fields":{"a":0}}}}`,
		"too large":      `{"version":1,"messages":{"M":{"fields":{"a":536870912}}}}`,
		"protobuf range": `{"version":1,"messages":{"M":{"fields":{"a":19500}}}}`,
		"repeated":       `{"version":1,"messages":{"M":{"fields":{"a":1,"b":1}}}}`,
		"reserved clash": `{"version":1,"messages":{"M":{"fields":{"a":1},"reserved":[1]}}}`,
	} {
		p := filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".lock")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadLock(p); err == nil {
			t.Errorf("%s: lock was accepted", name)
		}
	}
}

// Catches: a new field landing in the range protobuf reserves, and a repeated
// column silently sharing a number.
func TestAssignSkipsProtobufRangeAndFlagsRepeats(t *testing.T) {
	l := &Lock{Version: 1, Messages: map[string]*MessageLock{"M": {Fields: map[string]int{"a": 18999}}}}
	got, diags := l.Assign("M", []string{"a", "b"})
	if len(diags) != 0 || got["b"] != 20000 {
		t.Fatalf("got %v, diags %v; want b=20000", got, diags)
	}
	_, diags = l.Assign("M", []string{"a", "b", "b"})
	if len(diags) != 1 || !strings.Contains(diags[0].Msg, "listed twice") {
		t.Fatalf("diags = %v", diags)
	}
}
