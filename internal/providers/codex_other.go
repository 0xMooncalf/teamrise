//go:build !windows

package providers

import "os/exec"

func hideCodexWindow(cmd *exec.Cmd) {}
