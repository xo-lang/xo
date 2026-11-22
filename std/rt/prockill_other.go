//go:build windows || plan9 || js || wasip1

package rt

import (
	"errors"
	"os"
	"os/exec"
)

// sendSignal: only KILL can be sent where there are no Unix signals
// (os.Process.Kill).
func sendSignal(pid int, sig string) error {
	if sig != "KILL" {
		return errors.New("signal " + sig + " is not supported on this system")
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return killProcess(p)
}

// groupsSupported: no process groups here; Child.kill_group is an error
// (decision 0739).
const groupsSupported = false

// setGroup does nothing where there are no process groups.
func setGroup(*exec.Cmd) {}
