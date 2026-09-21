//go:build !windows

package command

import "os/exec"

func configurePlatform(_ *exec.Cmd) {}
