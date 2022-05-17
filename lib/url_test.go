package lib

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestPathEscapeRFC1738(t *testing.T) {
	c := qt.New(t)

	testCases := []struct {
		input    string
		expected string
	}{
		// should NOT encode
		{"/path/", "/path/"},
		{"/path/-/", "/path/-/"},
		{"/path/_/", "/path/_/"},
		{"/path/*", "/path/*"},
		{"/path*", "/path*"},
		{"/path/*.ext", "/path/*.ext"},
		{"/path/filename*", "/path/filename*"},

		// should encode
		{"/path/tilde~file", "/path/tilde%7Efile"}, // https://github.com/bep/s3deploy/issues/46
		{"/path/世界", "/path/%E4%B8%96%E7%95%8C"},   // non-ascii
	}

	for _, tc := range testCases {
		actual := pathEscapeRFC1738(tc.input)
		c.Assert(actual, qt.Equals, tc.expected)
	}
}
