// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"net"
	"net/url"
	"strings"
)

// TLSWarning returns a non-empty warning when the connection string raw may
// reach a remote host without TLS. That is the case when the effective sslmode
// is disable, allow or prefer, or is not set at all (libpq then defaults to
// prefer, which falls back to plaintext). raw is a postgres:// URL or a
// key=value string; the sslmode comes from it, else from PGSSLMODE, and a host
// missing from it is taken from PGHOST and PGHOSTADDR (hostaddr counts as a
// host too). The URL scheme is matched without regard to case. env looks up an environment variable.
// A host that is localhost, 127.0.0.1, ::1 or a unix socket path, or no host at
// all, stays quiet. require and verify-ca stay quiet too, but the warning
// recommends verify-full, the only mode that checks the server's name.
// It names the hosts only, never the URL, so it carries no credentials.
// Redaction lives in migrate.RedactURLs; this file adds no second copy.
func TLSWarning(raw string, env func(string) string) string {
	host, hostaddr, mode, ok := connHostsAndMode(raw)
	if !ok {
		return ""
	}
	if env == nil {
		env = func(string) string { return "" }
	}
	if mode == "" {
		mode = env("PGSSLMODE")
	}
	var shown string
	switch strings.ToLower(mode) {
	case "":
		shown = "sslmode is not set; libpq defaults to prefer"
	case "disable", "allow", "prefer":
		shown = "sslmode=" + mode
	default:
		return ""
	}
	if host == "" {
		host = env("PGHOST")
	}
	if hostaddr == "" {
		hostaddr = env("PGHOSTADDR")
	}
	// libpq connects to hostaddr when it is set, and to host otherwise, so a
	// remote address in either one counts.
	hosts := host
	if hostaddr != "" {
		hosts += "," + hostaddr
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
	return "TLS is not enforced (" + shown + ") for remote host " + strings.Join(remote, ", ") +
		": credentials and data can cross the network unencrypted; use sslmode=verify-full"
}

// connHostsAndMode extracts the host, hostaddr and sslmode from a postgres URL
// or a key=value string. ok is false when raw is neither.
func connHostsAndMode(raw string) (host, hostaddr, mode string, ok bool) {
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", "", "", false
		}
		q := u.Query()
		host = u.Host
		if i := strings.LastIndex(host, "@"); i >= 0 {
			host = host[i+1:]
		}
		if h := q.Get("host"); h != "" {
			host = h
		}
		return host, q.Get("hostaddr"), q.Get("sslmode"), true
	}
	kv, ok := parseKeyValue(raw)
	if !ok {
		return "", "", "", false
	}
	return kv["host"], kv["hostaddr"], kv["sslmode"], true
}

// parseKeyValue parses a libpq key=value connection string: values may be
// single-quoted, with backslash escapes. It reports false for anything else.
func parseKeyValue(s string) (map[string]string, bool) {
	out := map[string]string{}
	s = strings.TrimSpace(s)
	for s != "" {
		eq := strings.IndexByte(s, '=')
		if eq <= 0 {
			return nil, false
		}
		key := strings.TrimSpace(s[:eq])
		if key == "" || strings.ContainsAny(key, " \t\r\n") {
			return nil, false
		}
		s = strings.TrimLeft(s[eq+1:], " \t\r\n")
		val, rest, ok := readValue(s)
		if !ok {
			return nil, false
		}
		out[key] = val
		s = strings.TrimLeft(rest, " \t\r\n")
	}
	return out, len(out) > 0
}

// readValue reads one key=value value from the start of s and returns it with
// the rest of s. ok is false for an unterminated quote.
func readValue(s string) (val, rest string, ok bool) {
	var b strings.Builder
	if strings.HasPrefix(s, "'") {
		for i := 1; i < len(s); i++ {
			switch s[i] {
			case '\\':
				if i+1 < len(s) {
					i++
					b.WriteByte(s[i])
				}
			case '\'':
				return b.String(), s[i+1:], true
			default:
				b.WriteByte(s[i])
			}
		}
		return "", "", false
	}
	i := 0
	for i < len(s) && !strings.ContainsRune(" \t\r\n", rune(s[i])) {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String(), s[i:], true
}
