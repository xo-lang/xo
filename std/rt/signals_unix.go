//go:build !windows

package rt

import (
	"os"
	"syscall"
)

// sigSystem is each Signal tag's system signal (decision 0407).
var sigSystem = [7]os.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGWINCH}
