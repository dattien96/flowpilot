//go:build !windows

package app

import "os/exec"

func applyClipboardSysProcAttr(_ *exec.Cmd) {}
