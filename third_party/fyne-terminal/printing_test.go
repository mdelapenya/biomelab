package terminal

import (
	"bytes"
	_ "embed"
	"strings"
	"testing"
	"unicode/utf16"

	"fyne.io/fyne/v2"
	"github.com/stretchr/testify/assert"
)

func TestHandleOutput_PrintMode(t *testing.T) {
	tests := map[string]struct {
		inputSeq string
	}{
		"pdf printing": {
			inputSeq: esc("_set printer:_editor:tmp/DBRRPT-20231026-022112-27.3446959766687.pdf") + "\000" + esc("\\"),
		},
	}

	// Iterate through the test cases
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			terminal := New()
			terminal.handleOutput([]byte(test.inputSeq))
		})
	}
}

func TestHandleOutput_Printing(t *testing.T) {
	tests := map[string]struct {
		inputSeq              []byte
		expectedPrintingState bool
		expectedPrintData     []byte
		expectedSpooledData   []byte
		expectedScreenData    string
	}{
		"start printing": {
			inputSeq:              []byte(esc("[5ithisshouldbeprinted")),
			expectedPrintData:     []byte("thisshouldbeprinted"),
			expectedPrintingState: true,
		},
		"complete printing": {
			inputSeq:              []byte(esc("[5i") + "thisshouldbeprinted" + esc("[4i")),
			expectedSpooledData:   []byte("thisshouldbeprinted"),
			expectedPrintingState: false,
		},
		"printing with embedded esc": {
			inputSeq:              []byte(esc("[5i") + esc("����B�") + esc("[4i")),
			expectedSpooledData:   []byte(esc("����B�")),
			expectedPrintingState: false,
		},
		"UTF-8 Content": {
			inputSeq:              []byte(esc("[5i") + "Hello, 世界!" + esc("[4i")),
			expectedSpooledData:   []byte("Hello, 世界!"),
			expectedPrintingState: false,
		},
		"UTF-16 Content": {
			inputSeq: []byte(esc("[5i") + string(utf16.Decode([]uint16{
				0x0048, 0x0065, 0x006c, 0x006c, 0x006f, 0x002c, 0x0020, 0x4e16, 0x754c, 0x0021,
			})) + esc("[4i")),
			expectedSpooledData:   []byte("Hello, 世界!"),
			expectedPrintingState: false,
		},
		"ISO-8859-1 Content": {
			inputSeq:              []byte(esc("[5iH") + "\xe9llo, W\xf6rld!" + esc("[4i")),
			expectedSpooledData:   []byte{0x48, 0xe9, 0x6c, 0x6c, 0x6f, 0x2c, 0x20, 0x57, 0xf6, 0x72, 0x6c, 0x64, 0x21},
			expectedPrintingState: false,
		},
		"when print is mid buffer": {
			inputSeq:            []byte("abc" + esc("[5i") + "def" + esc("[4i") + "ghi"),
			expectedScreenData:  "abcghi",
			expectedSpooledData: []byte("def"),
		},
	}

	// Iterate through the test cases
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			terminal := New()
			terminal.Resize(fyne.NewSize(50, 50))
			var spooledData []byte
			terminal.printer = PrinterFunc(func(d []byte) {
				spooledData = d
			})
			terminal.handleOutput(test.inputSeq)

			assert.Equal(t, test.expectedScreenData, terminal.content.Text())
			assert.Equal(t, test.expectedPrintingState, terminal.state.printing)
			assert.Equal(t, test.expectedSpooledData, spooledData)
			assert.Equal(t, test.expectedPrintData, terminal.printData)
		})
	}
}

//go:embed test_data/chn.pdf
var examplePDFData []byte

func TestHandleOutput_Printing_PDF(t *testing.T) {
	terminal := New()
	var spooledData []byte
	terminal.printer = PrinterFunc(func(d []byte) {
		spooledData = d
	})

	data := []byte{asciiEscape, '[', '5', 'i'}
	data = append(data, examplePDFData...)
	data = append(data, []byte{asciiEscape, '[', '4', 'i'}...)

	for i := 0; i < len(data); i += bufLen {
		end := i + bufLen
		if end > len(data) {
			end = len(data)
		}
		t.Logf("sending chunk")
		terminal.handleOutput(data[i:end])
	}

	assert.Equal(t, spooledData, examplePDFData)
}

// An unterminated or oversized print job must not grow without bound.
func TestPrintModeIsBounded(t *testing.T) {
	big := bytes.Repeat([]byte("x"), 2*maxPrintData)

	// Without a printer nothing is kept beyond what detects the terminator.
	term := New()
	term.Resize(fyne.NewSize(500, 150))
	term.handleOutput(append([]byte(esc("[5i")), big...))
	assert.LessOrEqual(t, len(term.printData), 3)
	assert.True(t, term.state.printing)
	term.handleOutput([]byte(esc("[4i") + "after"))
	assert.False(t, term.state.printing)
	assert.Equal(t, "after", term.content.Text())

	// With a printer, an oversized job is dropped and a later one prints.
	term = New()
	term.Resize(fyne.NewSize(500, 150))
	var spooled [][]byte
	term.printer = PrinterFunc(func(d []byte) { spooled = append(spooled, d) })
	term.handleOutput(append(append([]byte(esc("[5i")), big...), []byte(esc("[4i"))...))
	assert.Empty(t, spooled)
	term.handleOutput([]byte(esc("[5i") + "small" + esc("[4i")))
	assert.Equal(t, [][]byte{[]byte("small")}, spooled)
}

// A printer attached part-way through a job must not receive the truncated
// remainder as if it were the whole job.
func TestPrinterAttachedMidJobGetsNoTruncatedJob(t *testing.T) {
	term := New()
	term.Resize(fyne.NewSize(500, 150))
	term.handleOutput([]byte(esc("[5i") + strings.Repeat("x", 1024)))
	var spooled [][]byte
	term.printer = PrinterFunc(func(d []byte) { spooled = append(spooled, d) })
	term.handleOutput([]byte("0123456789" + esc("[4i")))
	assert.Empty(t, spooled)
	term.handleOutput([]byte(esc("[5i") + "whole" + esc("[4i")))
	assert.Equal(t, [][]byte{[]byte("whole")}, spooled)
}
