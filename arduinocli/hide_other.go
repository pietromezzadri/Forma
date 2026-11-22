//go:build !windows

package arduinocli

import "os/exec"

func hideWindow(*exec.Cmd) {}
