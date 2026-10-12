//go:build !unix

package selfhost

import "os"

// maxRSS is 0: the system does not report a peak resident size.
func maxRSS(*os.ProcessState) int64 { return 0 }

// totalMemory is 0: unknown.
func totalMemory() int64 { return 0 }
