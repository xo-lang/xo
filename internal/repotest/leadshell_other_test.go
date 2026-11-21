//go:build !unix

package repotest

//pushq:owner TestLeadShell

import "os/exec"

func leadShellGroup(cmd *exec.Cmd) {}

func leadShellReap(cmd *exec.Cmd) {}
