// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"testing"
	"time"
)

func TestTimestampValueAndUnset(t *testing.T) {
	now := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	i64 := int64(7)
	var nilI64 *int64
	var nilTime *time.Time
	set := now.Add(-time.Hour)

	cases := []struct {
		name      string
		sample    any
		wantValue any // compared with ==, or via deref for pointers
		unsetZero bool
		setValue  any
		setIsSet  bool
	}{
		{"time.Time", time.Time{}, now, true, set, true},
		{"*time.Time", nilTime, now, true, &set, true},
		{"int64", int64(0), now.Unix(), true, int64(5), true},
		{"*int64", nilI64, now.Unix(), true, &i64, true},
	}
	for _, c := range cases {
		got, ok := timestampValue(now, c.sample)
		if !ok {
			t.Errorf("%s: timestampValue not supported", c.name)
			continue
		}
		switch g := got.(type) {
		case *time.Time:
			if !g.Equal(now) {
				t.Errorf("%s: value %v, want %v", c.name, *g, now)
			}
		case *int64:
			if *g != now.Unix() {
				t.Errorf("%s: value %d, want %d", c.name, *g, now.Unix())
			}
		default:
			if got != c.wantValue {
				t.Errorf("%s: value %v, want %v", c.name, got, c.wantValue)
			}
		}
		if unset := timestampUnset(c.sample); unset != c.unsetZero {
			t.Errorf("%s: unset(zero) = %v, want %v", c.name, unset, c.unsetZero)
		}
		if unset := timestampUnset(c.setValue); unset == c.setIsSet {
			t.Errorf("%s: unset(assigned) = %v, want %v", c.name, unset, !c.setIsSet)
		}
	}

	type named int64
	if _, ok := timestampValue(now, named(0)); ok {
		t.Error("named int64 must be unsupported")
	}
}
