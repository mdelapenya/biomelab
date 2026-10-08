package terminal

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"github.com/stretchr/testify/assert"
)

// Process output is untrusted: no escape sequence may panic the application.
func TestEscapeSequencesDoNotPanic(t *testing.T) {
	for name, input := range map[string]string{
		"multibyte CSI final":         "\x1b[世m",
		"multibyte CSI with params":   "\x1b[1;2世",
		"delete chars past row end":   "ab\x1b[20G\x1b[P",
		"insert chars past row end":   "ab\x1b[20G\x1b[@",
		"delete chars on empty row":   "\x1b[5;20H\x1b[3P",
		"insert chars on empty row":   "\x1b[5;20H\x1b[3@",
		"multibyte then plain output": "\x1b[é\x1b[2Cx",
	} {
		t.Run(name, func(t *testing.T) {
			term := New()
			term.Resize(fyne.NewSize(500, 150))
			assert.NotPanics(t, func() { term.handleOutput([]byte(input)) })
		})
	}
}

func TestDeleteAndInsertCharsWithinRow(t *testing.T) {
	term := New()
	term.Resize(fyne.NewSize(500, 150))
	term.handleOutput([]byte("abcdef\x1b[3G\x1b[2P"))
	assert.Equal(t, "abef", term.content.Text())

	term = New()
	term.Resize(fyne.NewSize(500, 150))
	term.handleOutput([]byte("abcd\x1b[3G\x1b[2@"))
	assert.Equal(t, "ab  cd", term.content.Text())
}

func TestModeAndAttributeSequencesDoNotPanic(t *testing.T) {
	for name, input := range map[string]string{
		"bare reset mode":      "\x1b[l",
		"bare set mode":        "\x1b[0h",
		"non-private set mode": "\x1b[4h",
		"secondary DA":         "\x1b[>c",
		"tertiary DA":          "\x1b[=c",
	} {
		t.Run(name, func(t *testing.T) {
			term := New()
			term.debug = true
			term.AttachWriter(NopCloser(&discard{}))
			term.Resize(fyne.NewSize(500, 150))
			assert.NotPanics(t, func() { term.handleOutput([]byte(input)) })
		})
	}
}

// Numeric parameters come from untrusted output; huge values must not make
// the UI thread allocate or loop without bound.
func TestHugeEscapeParametersAreBounded(t *testing.T) {
	term := New()
	term.Resize(fyne.NewSize(500, 150))
	done := make(chan struct{})
	go func() {
		defer close(done)
		term.handleOutput([]byte("x\x1b[999999999b"))           // REP
		term.handleOutput([]byte("\x1b[1;100000000r\x1b[L"))    // DECSTBM + IL
		term.handleOutput([]byte("\x1b[r\x1b[999999999S"))      // SU
		term.handleOutput([]byte("\x1b[999999999;999999999H?")) // CUP
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("huge parameters hung the parser")
	}
	assert.Less(t, len(term.content.Rows), 20000)
	assert.Less(t, term.scrollBottom, int(term.config.Rows))
}

// Inserting at the end of a row's content still appends styled blanks.
func TestInsertCharsAtEndOfContent(t *testing.T) {
	term := New()
	term.Resize(fyne.NewSize(500, 150))
	term.handleOutput([]byte("ab\x1b[44m\x1b[3@"))
	assert.Len(t, term.content.Rows[0].Cells, 5)
}

type discard struct{}

func (*discard) Write(b []byte) (int, error) { return len(b), nil }
