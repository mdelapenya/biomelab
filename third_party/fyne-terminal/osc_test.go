package terminal

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOSC_Title(t *testing.T) {
	term := New()
	assert.Equal(t, "", term.config.Title)

	term.handleOSC("0;Test")
	assert.Equal(t, "Test", term.config.Title)

	term.handleOSC("0;Testing;123")
	assert.Equal(t, "Testing;123", term.config.Title)
}

func TestParseDirectoryURI(t *testing.T) {
	for _, tc := range []struct {
		uri     string
		windows bool
		want    string
		ok      bool
	}{
		{"file://me/tmp/some%20dir", false, "/tmp/some dir", true},
		{"file:///tmp/x", false, "/tmp/x", true},
		{"file://localhost/tmp/x", false, "/tmp/x", true},
		{"file://ME/tmp/x", false, "/tmp/x", true},
		// Unencoded prompt output: '?', '#' and '%' are path characters.
		{"file://me/work/issue#42", false, "/work/issue#42", true},
		{"file://me/work/what?", false, "/work/what?", true},
		{"file://me/tmp/100%", false, "/tmp/100%", true},
		// The drive-letter form is only rewritten on Windows.
		{"file://me/C:/Users/me/repo", true, `C:\Users\me\repo`, true},
		{"file://me/a:/x", false, "/a:/x", true},
		// Another host's path is not a local directory.
		{"file://remote/home/u", false, "", false},
		{"file://server/share/dir", true, "", false},
		// Malformed and short input.
		{"f", false, "", false},
		{"file:", false, "", false},
		{"file://", false, "", false},
		{"file://me", false, "", false},
		{"http://me/x", false, "", false},
	} {
		got, ok := parseDirectoryURI(tc.uri, "me", tc.windows)
		assert.Equal(t, tc.ok, ok, tc.uri)
		assert.Equal(t, tc.want, got, tc.uri)
	}
}

func TestOSC_DirectoryDoesNotChangeProcessDirectory(t *testing.T) {
	before, err := os.Getwd()
	assert.NoError(t, err)
	term := New()

	term.handleOSC("7;file:///" + "biomelab-osc7-test")
	assert.NotEmpty(t, term.config.PWD)
	reported := term.config.PWD

	// Malformed reports are ignored and keep the last good value.
	for _, bad := range []string{"7;f", "7;file:", "7;file://", "7;%zz"} {
		term.handleOSC(bad)
	}
	assert.Equal(t, reported, term.config.PWD)

	after, err := os.Getwd()
	assert.NoError(t, err)
	assert.Equal(t, before, after, "OSC 7 must not change the host process directory")
}
