//go:build !windows

package processutil

import "os/exec"

func Background(command *exec.Cmd) {}
