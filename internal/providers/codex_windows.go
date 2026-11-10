package providers

import (
	"os/exec"
	"syscall"
)

func hideCodexWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
