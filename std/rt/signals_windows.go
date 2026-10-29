//go:build windows

package rt

import (
	"os"
	"syscall"
)

// sigSystem on Windows: Go delivers Ctrl-C and Ctrl-Break as SIGINT and
// the console's close, a logoff and a shutdown as SIGTERM; the others
// never arrive (decision 0407).
var sigSystem = [7]os.Signal{nil, syscall.SIGINT, nil, syscall.SIGTERM, nil, nil, nil}
