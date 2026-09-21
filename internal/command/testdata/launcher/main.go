// The Windows integration test builds this with -H=windowsgui so its child
// process starts from the same subsystem as packaged Biomelab.
package main

import (
	"os"

	"github.com/mdelapenya/biomelab/internal/command"
)

func main() {
	cmd := command.Background(os.Args[1], os.Args[2:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}
