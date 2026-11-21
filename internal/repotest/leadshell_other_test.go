//go:build !unix

package repotest

import "os/exec"

func leadShellGroup(cmd *exec.Cmd) {}

func leadShellReap(cmd *exec.Cmd) {}
