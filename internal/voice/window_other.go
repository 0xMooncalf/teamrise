//go:build !windows

package voice

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
