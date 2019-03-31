package lib

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
