package cmdutil

import (
	"fmt"
	"net/url"
	"strings"
)

// ParseHost parses a configured host, which must be a host name or IP
// address with an optional port. git reads URL patterns in a credential
// key built from it, so a wildcard, user name, path, query or fragment would
// widen or shift the scope of a token; such a host is an error.
func ParseHost(host string) (*url.URL, error) {
	invalid := fmt.Errorf("invalid host %q: want a host name with an optional port", host)
	if host == "" || strings.ContainsAny(host, "*@/\\?#% ") {
		return nil, invalid
	}
	u, err := url.Parse("https://" + host)
	if err != nil || u.Hostname() == "" || u.Host != host {
		return nil, invalid
	}
	return u, nil
}

// MatchesHost reports whether rawURL is an https URL whose host and port
// equal the configured host (ASCII case-insensitive, a missing port is 443).
// Only such a URL may get the configured host's token. A non-ASCII host name
// never matches: Unicode case folding joins names such as xσ and xς that IDNA
// turns into two hosts.
func MatchesHost(configured *url.URL, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return false
	}
	return asciiEqualFold(u.Hostname(), configured.Hostname()) && httpsPort(u) == httpsPort(configured)
}

// asciiEqualFold reports whether a and b are equal with only A to Z folded to
// a to z. A string with a byte above tilde equals nothing.
func asciiEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] > '~' || b[i] > '~' || asciiLower(a[i]) != asciiLower(b[i]) {
			return false
		}
	}
	return true
}

func asciiLower(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

func httpsPort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	return "443"
}

// IsPrintableASCII reports whether s holds only characters from space to
// tilde. A credential helper prints a token as one password= line, so a line
// break would add credential lines such as host= or url=.
func IsPrintableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < ' ' || s[i] > '~' {
			return false
		}
	}
	return true
}
