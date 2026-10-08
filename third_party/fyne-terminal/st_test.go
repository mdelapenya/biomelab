package terminal

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/stretchr/testify/assert"
)

// st is the String Terminator: ESC followed by a backslash (0x5c).
const st = "\x1b\x5c"

// String sequences (OSC, DCS, APC) may end with ST. A sequence that never
// ends swallows all later output, and a terminator that leaves escape state
// behind eats the next printable character.
func TestStringSequencesEndWithST(t *testing.T) {
	var apc string
	RegisterAPCHandler("st-test:", func(_ *Terminal, s string) { apc = s })

	for name, tc := range map[string]struct {
		input, want string
	}{
		"APC":                     {"\x1b_st-test:Hi" + st + "after", "after"},
		"APC NUL still supported": {"\x1b_st-test:Hi\x00after", "after"},
		"DCS":                     {"\x1bPpayload" + st + "after", "after"},
		"DCS with backslash":      {"\x1bPa\x5cb" + st + "after", "after"},
		"OSC":                     {"\x1b]0;title" + st + "after", "after"},
		"OSC BEL still supported": {"\x1b]0;title\x07after", "after"},
	} {
		t.Run(name, func(t *testing.T) {
			apc = ""
			term := New()
			term.Resize(fyne.NewSize(500, 150))
			term.handleOutput([]byte(tc.input))
			assert.Equal(t, tc.want, term.content.Text())
			assert.False(t, term.state.apc || term.state.dcs || term.state.osc || term.state.escNext,
				"string state must be cleared after its terminator")
			if strings.HasPrefix(name, "APC") {
				assert.Equal(t, "Hi", apc)
			}
		})
	}
}
