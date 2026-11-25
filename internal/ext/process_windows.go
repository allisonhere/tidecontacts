//go:build windows

package ext

import "os/exec"

func setProcessGroup(*exec.Cmd) {}

func killProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
