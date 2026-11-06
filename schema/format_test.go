// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestSchemaFormatVersion(t *testing.T) {
	s := &Schema{FormatVersion: FormatVersion, Dialect: "postgres", Tables: []Table{{Name: "t"}}}
	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"format_version": 1`) {
		t.Fatalf("snapshot does not carry format_version: %s", buf.String())
	}
	future := strings.Replace(buf.String(), `"format_version": 1`, `"format_version": 99`, 1)
	_, err := Load(strings.NewReader(future))
	if !errors.Is(err, ErrNewerFormat) {
		t.Fatalf("want ErrNewerFormat, got %v", err)
	}
	if !strings.Contains(err.Error(), "upgrade momo") {
		t.Fatalf("error must name the required Momo version action: %v", err)
	}
}
