package main

import (
	"strings"
	"testing"
)

func TestParseTarget(t *testing.T) {
	valid := map[string]string{
		"sub.example.com":                  "sub.example.com",
		"  Sub.Example.COM. ":              "sub.example.com",
		"https://sub.example.com/a/b?c=d":  "sub.example.com",
		"https://user:pw@example.com:8443": "example.com",
		"example.com/path":                 "example.com",
		"*.example.com":                    "example.com",
		"_dmarc.example.com":               "_dmarc.example.com",
		"example.xn--p1ai":                 "example.xn--p1ai",
	}
	for in, want := range valid {
		got, err := parseTarget(in)
		if err != nil || got != want {
			t.Errorf("parseTarget(%q) = %q, %v; want %q", in, got, err, want)
		}
	}

	invalid := []string{
		"", "localhost", "-u.example.com", "--config=/etc/passwd", "example.com;id",
		"exa mple.com", "example..com", "127.0.0.1", "$(id).example.com",
		strings.Repeat("a", 64) + ".com", "-oU/tmp/x.example.com",
	}
	for _, in := range invalid {
		if got, err := parseTarget(in); err == nil {
			t.Errorf("parseTarget(%q) = %q; want error", in, got)
		}
	}
}

func TestNormalize(t *testing.T) {
	const target = "sub.example.com"
	tests := []struct {
		line string
		subs bool
		want string // empty means rejected
	}{
		{"https://sub.example.com/api/v1?id=1", false, "https://sub.example.com/api/v1?id=1"},
		{"HTTP://SUB.Example.com:80/Path", false, "http://sub.example.com/Path"},
		{"https://sub.example.com:443", false, "https://sub.example.com/"},
		{"https://sub.example.com:8443/x#frag", false, "https://sub.example.com:8443/x"},
		{"https://user:pw@sub.example.com/x", false, "https://sub.example.com/x"},
		{"  https://sub.example.com/x  ", false, "https://sub.example.com/x"},
		{"https://a.sub.example.com/x", false, ""},
		{"https://a.sub.example.com/x", true, "https://a.sub.example.com/x"},
		{"https://example.com/x", true, ""},
		{"https://evilsub.example.com/x", true, ""},
		{"https://sub.example.com.evil.com/x", true, ""},
		{"https://evil.com/?u=https://sub.example.com/", true, ""},
		{"ftp://sub.example.com/x", false, ""},
		{"javascript:alert(1)", false, ""},
		{"sub.example.com/x", false, ""},
		{"https://sub.example.com/\x1b[2Jx", false, ""},
		{"", false, ""},
	}
	for _, tt := range tests {
		u, ok := normalize(tt.line, target, tt.subs)
		got := ""
		if ok {
			got = u.String()
		}
		if got != tt.want {
			t.Errorf("normalize(%q, subs=%v) = %q; want %q", tt.line, tt.subs, got, tt.want)
		}
	}
}

func TestScanLinesSkipsOversized(t *testing.T) {
	in := "a\n" + strings.Repeat("x", 3*maxLine) + "\nb\n"
	var got []string
	if err := scanLines(strings.NewReader(in), func(s string) { got = append(got, s) }); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "a,b" {
		t.Errorf("got %d lines %q; want [a b]", len(got), got)
	}
}
