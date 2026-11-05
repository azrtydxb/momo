// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestFormatVersions(t *testing.T) {
	cur, err := json.Marshal(populated())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bytes.NewReader(cur)); err != nil {
		t.Fatalf("current format rejected: %v", err)
	}

	future := bytes.Replace(cur, []byte(`"format_version":1`), []byte(`"format_version":2`), 1)
	_, err = Load(bytes.NewReader(future))
	if !errors.Is(err, ErrNewerFormat) {
		t.Fatalf("format 2 error = %v, want ErrNewerFormat", err)
	}
	if !strings.Contains(err.Error(), "format_version 2") {
		t.Errorf("error does not name the format: %v", err)
	}

	missing := bytes.Replace(cur, []byte(`"format_version":1,`), []byte(``), 1)
	if _, err := Load(bytes.NewReader(missing)); !errors.Is(err, ErrMissingFormatVersion) {
		t.Fatalf("missing format_version error = %v, want ErrMissingFormatVersion", err)
	}
}
