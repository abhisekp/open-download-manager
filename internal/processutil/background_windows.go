package processutil

import (
	"os/exec"
	"syscall"
)

// Background prevents helper consoles from appearing over the desktop window.
func Background(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
