//go:build !darwin && !linux && !windows

package embeddedterminal

import (
	"context"
	"errors"
	"io"
)

// Unsupported platforms retain the existing external-terminal path until a
// native transport has been validated there.
type Process struct{}

func Start(context.Context, string, []string, uint16, uint16) (*Process, error) {
	return nil, errors.New("integrated terminals are unavailable on this platform; use Open in external terminal")
}
func (*Process) Read([]byte) (int, error)    { return 0, io.EOF }
func (*Process) Write([]byte) (int, error)   { return 0, io.ErrClosedPipe }
func (*Process) Resize(uint16, uint16) error { return io.ErrClosedPipe }
func (*Process) Stop()                       {}
func (*Process) Close() error                { return nil }
func (*Process) Wait() error                 { return nil }

func (*Process) WaitStopped() {}
