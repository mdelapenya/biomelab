package command

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configurePlatform(cmd *exec.Cmd) {
	// These constructors own a fresh Cmd. Do not combine CREATE_NO_WINDOW with
	// CREATE_NEW_CONSOLE or DETACHED_PROCESS: Windows would ignore it.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}
