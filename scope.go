package main

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// hostname accepts DNS names only. Requiring a leading alphanumeric also
// guarantees a target can never be parsed as a flag by the tools we exec.
var hostname = regexp.MustCompile(`^([a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9_])?\.)+[a-z]([a-z0-9-]{0,61}[a-z0-9])$`)

// parseTarget reduces user input (a host, or a URL containing one) to a
// validated lowercase hostname.
func parseTarget(input string) (string, error) {
	host := strings.ToLower(strings.TrimSpace(input))
	if strings.Contains(host, "://") {
		u, err := url.Parse(host)
		if err != nil {
			return "", fmt.Errorf("invalid target %q", input)
		}
		host = u.Hostname()
	} else if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "*."), ".")

	if len(host) > 253 || !hostname.MatchString(host) {
		return "", fmt.Errorf("invalid target %q: expected a domain such as sub.example.com", input)
	}
	return host, nil
}

// normalize parses one line of tool output and returns its canonical URL if it
// is an http(s) URL in scope for target. Canonical form lowercases scheme and
// host and drops default ports and fragments, so the same endpoint reported by
// different tools de-duplicates. url.Parse rejects control characters, which
// keeps hostile archived URLs from reaching the terminal.
func normalize(line, target string, subs bool) (*url.URL, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, false
	}
	u, err := url.Parse(line)
	if err != nil {
		return nil, false
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, false
	}

	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host != target && !(subs && strings.HasSuffix(host, "."+target)) {
		return nil, false
	}

	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	u.Host = host
	if port != "" {
		u.Host = host + ":" + port
	}
	u.User = nil
	u.Fragment = ""
	u.RawFragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	return u, true
}
