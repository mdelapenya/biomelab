// The Windows integration test builds this with -H=windowsgui so its child
// process starts from the same subsystem as packaged Biomelab.
package main

import (
	"fmt"
	"os"

	"github.com/mdelapenya/biomelab/internal/command"
	"github.com/mdelapenya/biomelab/internal/command/internal/consolediag"
)

func main() {
	cmd := command.Background(os.Args[1], os.Args[2:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := consolediag.Run("gui-launcher", cmd); err != nil {
		fmt.Fprintln(os.Stderr, "GUI launcher failed; inspect process trace stages and exit codes")
		os.Exit(1)
	}
}
