// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net"
	"net/url"
	"strings"
)

// TLSWarning returns a non-empty warning when raw is a URL with sslmode=disable
// and any of its hosts is not localhost, 127.0.0.1, ::1 or a unix socket path.
// It names the hosts only, never the URL, so it carries no credentials.
// Redaction lives in redactURL (migrate.go); this file adds no second copy.
func TLSWarning(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	q := u.Query()
	if q.Get("sslmode") != "disable" {
		return ""
	}
	hosts := u.Host
	if i := strings.LastIndex(hosts, "@"); i >= 0 {
		hosts = hosts[i+1:]
	}
	if h := q.Get("host"); h != "" {
		hosts = h
	}
	var remote []string
	for _, h := range strings.Split(hosts, ",") {
		h = strings.TrimSpace(h)
		if bare, _, err := net.SplitHostPort(h); err == nil {
			h = bare
		}
		h = strings.Trim(h, "[]")
		if h == "" || strings.HasPrefix(h, "/") || strings.HasPrefix(h, "%2F") || strings.HasPrefix(h, "%2f") {
			continue
		}
		switch h {
		case "localhost", "127.0.0.1", "::1":
			continue
		}
		remote = append(remote, h)
	}
	if len(remote) == 0 {
		return ""
	}
	return "TLS is disabled (sslmode=disable) for remote host " + strings.Join(remote, ", ") +
		": credentials and data cross the network unencrypted; use sslmode=require or stronger"
}
