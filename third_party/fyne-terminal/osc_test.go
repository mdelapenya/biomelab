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

func TestOSC_DirectoryDoesNotChangeProcessDirectory(t *testing.T) {
	before, err := os.Getwd()
	assert.NoError(t, err)
	term := New()

	term.handleOSC("7;file://host/tmp/some%20dir")
	assert.Equal(t, "/tmp/some dir", term.config.PWD)
	term.handleOSC("7;file:///C:/Users/me/repo")
	assert.Equal(t, `C:\Users\me\repo`, term.config.PWD)

	// Malformed and short reports are ignored without panicking or
	// clobbering the last good value.
	for _, bad := range []string{"7;f", "7;file:", "7;file://", "7;http://host/x", "7;%zz"} {
		term.handleOSC(bad)
	}
	assert.Equal(t, `C:\Users\me\repo`, term.config.PWD)

	after, err := os.Getwd()
	assert.NoError(t, err)
	assert.Equal(t, before, after, "OSC 7 must not change the host process directory")
}
