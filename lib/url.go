package lib

import (
	"path"
	"strings"
)

// [RFC 1738](https://www.ietf.org/rfc/rfc1738.txt)
// §2.2
func shouldEscape(c byte) bool {
	// alphanum
	if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' {
		return false
	}

	switch c {
	case '$', '-', '_', '.', '+', '!', '*', '\'', '(', ')', ',': // Special characters
		return false

	case '/', '?', ':', '@', '=', '&': // Reserved characters
		return c == '?'
	}
	// Everything else must be escaped.
	return true
}

// pathEscapeRFC1738 escapes the string so it can be safely placed
// inside a URL path segment according to RFC1738.
// Based on golang native implementation of `url.PathEscape`
// https://golang.org/src/net/url/url.go?s=7976:8008#L276
func pathEscapeRFC1738(s string) string {
	spaceCount, hexCount := 0, 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if shouldEscape(c) {
			hexCount++
		}
	}

	if spaceCount == 0 && hexCount == 0 {
		return s
	}

	var buf [64]byte
	var t []byte

	required := len(s) + 2*hexCount
	if required <= len(buf) {
		t = buf[:required]
	} else {
		t = make([]byte, required)
	}

	if hexCount == 0 {
		copy(t, s)
		for i := 0; i < len(s); i++ {
			if s[i] == ' ' {
				t[i] = '+'
			}
		}
		return string(t)
	}

	j := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case shouldEscape(c):
			t[j] = '%'
			t[j+1] = "0123456789ABCDEF"[c>>4]
			t[j+2] = "0123456789ABCDEF"[c&15]
			j += 3
		default:
			t[j] = s[i]
			j++
		}
	}
	return string(t)
}

// Like path.Join, but preserves trailing slash..
func pathJoin(elem ...string) string {
	if len(elem) == 0 {
		return ""
	}
	hadSlash := strings.HasSuffix(elem[len(elem)-1], "/")
	p := path.Join(elem...)
	if hadSlash {
		p += "/"
	}
	return p
}

// pathClean works like path.Clean but will always  preserve a trailing slash.
func pathClean(p string) string {
	hadSlash := strings.HasSuffix(p, "/")
	p = path.Clean(p)
	if hadSlash && !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}

// trimIndexHTML remaps paths matching "<dir>/index.html" to "<dir>/".
func trimIndexHTML(p string) string {
	const suffix = "/index.html"
	if strings.HasSuffix(p, suffix) {
		return p[:len(p)-len(suffix)+1]
	}
	return p
}
